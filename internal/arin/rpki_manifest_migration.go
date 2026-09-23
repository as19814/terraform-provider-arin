package arin

import (
	"crypto/x509"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// MigrateRPKIManifestHistory carries existing rollback watermarks into the scope
// of an explicitly trusted replacement anchor. It neither selects nor establishes
// trust in that anchor. Existing destination watermarks are merged monotonically;
// conflicting evidence fails without changing either history. The source remains
// intact. Stop other users of the history directory before an anchor transition.
func MigrateRPKIManifestHistory(directory, previousAnchorPEM, replacementAnchorPEM string) error {
	return migrateRPKIManifestHistory(directory, previousAnchorPEM, replacementAnchorPEM, time.Now())
}

func migrateRPKIManifestHistory(directory, previousAnchorPEM, replacementAnchorPEM string, now time.Time) (err error) {
	old, err := rpkiPEMCertificates(previousAnchorPEM, 1)
	if err != nil || len(old) != 1 {
		return errRPKIManifestState
	}
	next, err := rpkiPEMCertificates(replacementAnchorPEM, 1)
	if err != nil || len(next) != 1 {
		return errRPKIManifestState
	}
	// The old anchor may have expired during renewal, but both inputs must be
	// resource CA certificates. The newly selected anchor must validate now.
	if validateRPKICAProfile(old[0]) != nil {
		return errRPKIManifestState
	}
	if _, err := verifyRPKICertificatePath([]*x509.Certificate{next[0]}, next[0], nil, now); err != nil {
		return errRPKIManifestState
	}
	oldID, newID := rpkiManifestDigest(old[0].Raw), rpkiManifestDigest(next[0].Raw)
	if oldID == newID || !filepath.IsAbs(directory) {
		return errRPKIManifestState
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errRPKIManifestState
	}
	oldPath := filepath.Join(directory, "rpki-manifests-"+oldID+".json")
	newPath := filepath.Join(directory, "rpki-manifests-"+newID+".json")
	locks := []string{oldPath + ".lock", newPath + ".lock"}
	sort.Strings(locks)
	var acquired []string
	defer func() {
		for i := len(acquired) - 1; i >= 0; i-- {
			if os.Remove(acquired[i]) != nil {
				err = errRPKIManifestState
			}
		}
		if len(acquired) > 0 && syncRPKIExchangeDirectory(directory) != nil {
			err = errRPKIManifestState
		}
	}()
	for _, lock := range locks {
		if os.Mkdir(lock, 0700) != nil {
			return errRPKIManifestState
		}
		acquired = append(acquired, lock)
	}
	if syncRPKIExchangeDirectory(directory) != nil {
		return errRPKIManifestState
	}
	// Missing source history must not silently create an empty destination.
	if _, err := os.Lstat(oldPath); err != nil {
		return errRPKIManifestState
	}
	source, err := readRPKIManifestHistory(oldPath, oldID)
	if err != nil {
		return err
	}
	destination, err := readRPKIManifestHistory(newPath, newID)
	if err != nil {
		return err
	}
	for id, incoming := range source.Entries {
		if existing, ok := destination.Entries[id]; ok {
			a, _ := new(big.Int).SetString(incoming.Number, 10)
			b, _ := new(big.Int).SetString(existing.Number, 10)
			switch a.Cmp(b) {
			case 0:
				if incoming.SHA256 != existing.SHA256 || !incoming.ThisUpdate.Equal(existing.ThisUpdate) {
					return errRPKIManifestState
				}
				continue
			case -1:
				if !incoming.ThisUpdate.Before(existing.ThisUpdate) {
					return errRPKIManifestState
				}
				continue
			case 1:
				if !incoming.ThisUpdate.After(existing.ThisUpdate) {
					return errRPKIManifestState
				}
			}
		}
		destination.Entries[id] = incoming
	}
	if len(destination.Entries) > 4096 {
		return errRPKIManifestState
	}
	return saveRPKIManifestHistory(newPath, destination)
}
