package arin

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

var errRPKIManifestState = errors.New("RPKI manifest history unavailable or rollback detected")

type rpkiManifestWatermark struct {
	Number     string    `json:"number"`
	ThisUpdate time.Time `json:"this_update"`
	SHA256     string    `json:"sha256"`
}
type rpkiManifestHistory struct {
	Version int                              `json:"version"`
	Anchor  string                           `json:"anchor"`
	Entries map[string]rpkiManifestWatermark `json:"entries"`
}

func rpkiManifestDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Verify and atomically remember all manifest versions under a per-anchor
// filesystem lock. A crash leaves the lock for explicit recovery. The private
// directory must persist across runs; deleting it discards rollback protection.
func verifyAndRecordRPKIManifestPath(directory string, path []*x509.Certificate, anchor *x509.Certificate, publications []rpkiPathPublication, now time.Time) (resources *rpkiCertificateResources, err error) {
	if !filepath.IsAbs(directory) || anchor == nil || len(anchor.Raw) == 0 || len(anchor.Raw) > 512000 {
		return nil, errRPKIManifestState
	}
	info, e := os.Lstat(directory)
	if e != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errRPKIManifestState
	}
	anchorID := rpkiManifestDigest(anchor.Raw)
	statePath := filepath.Join(directory, "rpki-manifests-"+anchorID+".json")
	lock := statePath + ".lock"
	if os.Mkdir(lock, 0700) != nil {
		return nil, errRPKIManifestState
	}
	defer func() {
		if os.Remove(lock) != nil || syncRPKIExchangeDirectory(directory) != nil {
			resources = nil
			err = errRPKIManifestState
		}
	}()
	if syncRPKIExchangeDirectory(directory) != nil {
		return nil, errRPKIManifestState
	}
	history, e := readRPKIManifestHistory(statePath, anchorID)
	if e != nil {
		return nil, e
	}
	resources, e = verifyRPKIManifestPath(path, anchor, publications, now)
	if e != nil {
		return nil, e
	}
	for i, p := range publications {
		issuer, e := x509.ParseCertificate(path[i+1].Raw)
		if e != nil {
			return nil, errRPKIManifestState
		}
		scope, _ := json.Marshal([]string{rpkiManifestDigest(issuer.RawSubjectPublicKeyInfo), p.ManifestURI})
		id := rpkiManifestDigest(scope)
		m, e := decodeRPKIManifest(p.ManifestDER)
		if e != nil {
			return nil, e
		}
		next := rpkiManifestWatermark{Number: m.Content.Number.String(), ThisUpdate: m.Content.ThisUpdate.UTC(), SHA256: rpkiManifestDigest(p.ManifestDER)}
		if old, exists := history.Entries[id]; exists {
			oldNumber, _ := new(big.Int).SetString(old.Number, 10)
			comparison := m.Content.Number.Cmp(oldNumber)
			if comparison < 0 || (comparison == 0 && (next.SHA256 != old.SHA256 || !next.ThisUpdate.Equal(old.ThisUpdate))) || (comparison > 0 && !next.ThisUpdate.After(old.ThisUpdate)) {
				return nil, errRPKIManifestState
			}
		}
		history.Entries[id] = next
	}
	if len(history.Entries) > 4096 {
		return nil, errRPKIManifestState
	}
	if e := saveRPKIManifestHistory(statePath, history); e != nil {
		return nil, e
	}
	return resources, nil
}

func readRPKIManifestHistory(path, anchor string) (rpkiManifestHistory, error) {
	out := rpkiManifestHistory{Version: 1, Anchor: anchor, Entries: map[string]rpkiManifestWatermark{}}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return out, errRPKIManifestState
	}
	f, err := os.Open(path)
	if err != nil {
		return out, errRPKIManifestState
	}
	actual, statErr := f.Stat()
	raw, readErr := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	closeErr := f.Close()
	if statErr != nil || !os.SameFile(info, actual) || readErr != nil || closeErr != nil || len(raw) > 1<<20 {
		return out, errRPKIManifestState
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil {
		return out, errRPKIManifestState
	}
	canonical, err := json.Marshal(out)
	if err != nil || !bytes.Equal(bytes.TrimSpace(raw), canonical) || out.Version != 1 || out.Anchor != anchor || out.Entries == nil || len(out.Entries) > 4096 {
		return out, errRPKIManifestState
	}
	for id, w := range out.Entries {
		n, ok := new(big.Int).SetString(w.Number, 10)
		if !exchangeDigest.MatchString(id) || !exchangeDigest.MatchString(w.SHA256) || !ok || n.Sign() < 0 || n.BitLen() > 159 || n.String() != w.Number || w.ThisUpdate.IsZero() || w.ThisUpdate.Nanosecond() != 0 || w.ThisUpdate.Year() < 1 || w.ThisUpdate.Year() > 9999 {
			return out, errRPKIManifestState
		}
	}
	return out, nil
}

func saveRPKIManifestHistory(path string, history rpkiManifestHistory) error {
	raw, err := json.Marshal(history)
	if err != nil || len(raw) > 1<<20 {
		return errRPKIManifestState
	}
	directory := filepath.Dir(path)
	f, err := os.CreateTemp(directory, ".rpki-manifests-*")
	if err != nil {
		return errRPKIManifestState
	}
	name := f.Name()
	defer os.Remove(name)
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return errRPKIManifestState
	}
	if os.Rename(name, path) != nil || syncRPKIExchangeDirectory(directory) != nil {
		return errRPKIManifestState
	}
	return nil
}
