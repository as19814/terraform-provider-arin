package arin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

var errRPKIRRDPCache = errors.New("RPKI repository cache unavailable or requires recovery")

type rrdpCacheRecord struct {
	Version int       `json:"version"`
	Cache   rrdpCache `json:"cache"`
}

// RefreshPersistent serializes refreshes for one notification URI. It saves the
// polling attempt before HTTP and atomically saves the final cache afterwards,
// including retained objects on failure. Crashes leave a lock for recovery.
func (c rrdpHTTPClient) RefreshPersistent(ctx context.Context, directory, uri string, now time.Time) (cache rrdpCache, err error) {
	if !filepath.IsAbs(directory) || !validRRDPURL(uri) || now.IsZero() || now.Year() < 1970 || now.Year() > 9999 {
		return cache, errRPKIRRDPCache
	}
	if err := ctx.Err(); err != nil {
		return cache, err
	}
	info, e := os.Lstat(directory)
	if e != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return cache, errRPKIRRDPCache
	}
	path := filepath.Join(directory, "rpki-rrdp-"+rpkiManifestDigest([]byte(uri))+".json")
	lock := path + ".lock"
	if os.Mkdir(lock, 0700) != nil {
		return cache, errRPKIRRDPCache
	}
	defer func() {
		if os.Remove(lock) != nil || syncRPKIExchangeDirectory(directory) != nil {
			cache = rrdpCache{}
			err = errRPKIRRDPCache
		}
	}()
	if syncRPKIExchangeDirectory(directory) != nil {
		return cache, errRPKIRRDPCache
	}
	previous, e := readRRDPCache(path, uri)
	if e != nil {
		return cache, e
	}
	if previous != nil {
		cache = *previous
		if now.Before(cache.LastAttempt.Add(time.Minute)) {
			return cache, errRPKIRRDPPoll
		}
	} else {
		cache.Repository.NotificationURI = uri
	}
	cache.LastAttempt = now.UTC()
	if e := saveRRDPCache(path, cache); e != nil {
		return rrdpCache{}, e
	}
	cache, err = c.Refresh(ctx, uri, previous, now)
	// A cancellation between pre-save and Refresh must retain the durable attempt.
	cache.LastAttempt = now.UTC()
	if previous != nil && cache.Repository.Session == "" && err != nil {
		cache = *previous
		cache.LastAttempt = now.UTC()
	}
	if e := saveRRDPCache(path, cache); e != nil {
		return rrdpCache{}, e
	}
	return cache, err
}

func validateRRDPCache(cache rrdpCache, uri string) error {
	if cache.Repository.NotificationURI != uri || !validRRDPURL(uri) || cache.LastAttempt.IsZero() || cache.LastAttempt.Year() < 1970 || cache.LastAttempt.Year() > 9999 || !validExchangeTime(cache.LastModified) {
		return errRPKIRRDPCache
	}
	if cache.Repository.Session == "" {
		if cache.Repository.Serial != "" || len(cache.Repository.Objects) != 0 || !cache.LastModified.IsZero() || cache.Snapshot != (rrdpFileReference{}) {
			return errRPKIRRDPCache
		}
	} else if !rrdpSession.MatchString(cache.Repository.Session) || !rrdpSerial.MatchString(cache.Repository.Serial) || !validRRDPURL(cache.Snapshot.URI) {
		return errRPKIRRDPCache
	}
	if _, err := cloneRRDPObjects(cache.Repository.Objects); err != nil {
		return errRPKIRRDPCache
	}
	return nil
}

func readRRDPCache(path, uri string) (*rrdpCache, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 128<<20 {
		return nil, errRPKIRRDPCache
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errRPKIRRDPCache
	}
	actual, statErr := f.Stat()
	raw, readErr := io.ReadAll(io.LimitReader(f, (128<<20)+1))
	closeErr := f.Close()
	if statErr != nil || !os.SameFile(info, actual) || readErr != nil || closeErr != nil || len(raw) > 128<<20 {
		return nil, errRPKIRRDPCache
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var record rrdpCacheRecord
	if decoder.Decode(&record) != nil || record.Version != 1 {
		return nil, errRPKIRRDPCache
	}
	canonical, err := json.Marshal(record)
	if err != nil || !bytes.Equal(bytes.TrimSpace(raw), canonical) || validateRRDPCache(record.Cache, uri) != nil {
		return nil, errRPKIRRDPCache
	}
	return &record.Cache, nil
}

func saveRRDPCache(path string, cache rrdpCache) error {
	if validateRRDPCache(cache, cache.Repository.NotificationURI) != nil {
		return errRPKIRRDPCache
	}
	raw, err := json.Marshal(rrdpCacheRecord{Version: 1, Cache: cache})
	if err != nil || len(raw) > 128<<20 {
		return errRPKIRRDPCache
	}
	directory := filepath.Dir(path)
	f, err := os.CreateTemp(directory, ".rpki-rrdp-*")
	if err != nil {
		return errRPKIRRDPCache
	}
	name := f.Name()
	defer os.Remove(name)
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return errRPKIRRDPCache
	}
	if os.Rename(name, path) != nil || syncRPKIExchangeDirectory(directory) != nil {
		return errRPKIRRDPCache
	}
	return nil
}
