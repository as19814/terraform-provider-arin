package arin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type rrdpDiskRecord struct {
	Version                          int
	NotificationURI, Session, Serial string
	Snapshot                         rrdpFileReference
	LastModified, LastAttempt        time.Time
	Pack                             string
	Size                             int64
	Entries                          map[string]rrdpSpoolEntry
}

type rrdpDiskCache struct {
	Record  rrdpDiskRecord
	Objects *rrdpSnapshotSpool
}

func (c *rrdpDiskCache) Close() error {
	if c == nil {
		return nil
	}
	return c.Objects.Close()
}

func rrdpDiskPrefix(uri string) string { return "rpki-rrdp-" + rpkiManifestDigest([]byte(uri)) }

func validateRRDPDiskRecord(r rrdpDiskRecord, uri string) error {
	if r.Version != 2 || r.NotificationURI != uri || !validRRDPURL(uri) || r.LastAttempt.IsZero() || (r.LastAttempt.Year() < 1970 || r.LastAttempt.Year() > 9999) || !validExchangeTime(r.LastModified) {
		return errRPKIRRDPCache
	}
	if r.Session == "" {
		if r.Serial != "" || r.Snapshot != (rrdpFileReference{}) || !r.LastModified.IsZero() || r.Pack != "" || r.Size != 0 || len(r.Entries) != 0 {
			return errRPKIRRDPCache
		}
		return nil
	}
	prefix := rrdpDiskPrefix(uri) + "-pack-"
	suffix, ok := strings.CutPrefix(r.Pack, prefix)
	if !ok || suffix == "" || strings.Trim(suffix, "0123456789") != "" || !rrdpSession.MatchString(r.Session) || !rrdpSerial.MatchString(r.Serial) || !validRRDPURL(r.Snapshot.URI) || r.Size < 0 || r.Size > rrdpDiskLimits.Decoded || len(r.Entries) > rrdpDiskLimits.Objects {
		return errRPKIRRDPCache
	}
	entries := make([]rrdpSpoolEntry, 0, len(r.Entries))
	uriBytes := 0
	for uri, e := range r.Entries {
		uriBytes += len(uri)
		if uriBytes > 128<<20 || !publicationURI(uri) || strings.HasSuffix(uri, "/") || e.Size <= 0 || e.Size > 4<<20 || e.Offset < 0 || e.Offset > r.Size-int64(e.Size) {
			return errRPKIRRDPCache
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Offset < entries[j].Offset })
	var end int64
	for _, e := range entries {
		if e.Offset != end {
			return errRPKIRRDPCache
		}
		end += int64(e.Size)
	}
	if end != r.Size {
		return errRPKIRRDPCache
	}
	return nil
}

func readRRDPDisk(path, uri string) (*rrdpDiskCache, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 256<<20 {
		return nil, errRPKIRRDPCache
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errRPKIRRDPCache
	}
	actual, se := f.Stat()
	raw, re := io.ReadAll(io.LimitReader(f, (256<<20)+1))
	ce := f.Close()
	if se != nil || !os.SameFile(info, actual) || re != nil || ce != nil || len(raw) > 256<<20 {
		return nil, errRPKIRRDPCache
	}
	var record rrdpDiskRecord
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil || validateRRDPDiskRecord(record, uri) != nil {
		return nil, errRPKIRRDPCache
	}
	canonical, err := json.Marshal(record)
	if err != nil || !bytes.Equal(raw, canonical) {
		return nil, errRPKIRRDPCache
	}
	cache := &rrdpDiskCache{Record: record}
	if record.Session == "" {
		return cache, nil
	}
	packPath := filepath.Join(filepath.Dir(path), record.Pack)
	info, err = os.Lstat(packPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() != record.Size {
		return nil, errRPKIRRDPCache
	}
	f, err = os.Open(packPath)
	if err != nil {
		return nil, errRPKIRRDPCache
	}
	actual, err = f.Stat()
	if err != nil || !os.SameFile(info, actual) {
		f.Close()
		return nil, errRPKIRRDPCache
	}
	cache.Objects = &rrdpSnapshotSpool{file: f, entries: record.Entries, size: record.Size, keep: true}
	return cache, nil
}

// A true published result includes the rename-before-directory-fsync failure
// case. Never remove a generation that the on-disk pointer may already name.
func saveRRDPDisk(path string, r rrdpDiskRecord) (published bool, err error) {
	if validateRRDPDiskRecord(r, r.NotificationURI) != nil {
		return false, errRPKIRRDPCache
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw) > 256<<20 {
		return false, errRPKIRRDPCache
	}
	dir := filepath.Dir(path)
	f, e := os.CreateTemp(dir, ".rpki-disk-state-*")
	if e != nil {
		return false, errRPKIRRDPCache
	}
	defer os.Remove(f.Name())
	_, e = f.Write(raw)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil || ce != nil {
		return false, errRPKIRRDPCache
	}
	if os.Rename(f.Name(), path) != nil {
		return false, errRPKIRRDPCache
	}
	if syncRPKIExchangeDirectory(dir) != nil {
		return true, errRPKIRRDPCache
	}
	return true, nil
}

func stageRRDPDiskPack(ctx context.Context, dir, uri string, source *rrdpSnapshotSpool) (*rrdpSnapshotSpool, error) {
	f, err := os.CreateTemp(dir, rrdpDiskPrefix(uri)+"-pack-*")
	if err != nil {
		return nil, errRPKIRRDPCache
	}
	result := &rrdpSnapshotSpool{file: f, entries: source.entries, size: source.size}
	success := false
	defer func() {
		if !success {
			result.Close()
		}
	}()
	count, err := io.Copy(f, &rrdpContextReader{ctx: ctx, input: io.NewSectionReader(source.file, 0, source.size)})
	if err != nil || count != source.size || f.Sync() != nil || syncRPKIExchangeDirectory(dir) != nil {
		return nil, errRPKIRRDPCache
	}
	success = true
	return result, nil
}

// RefreshDiskPersistent holds the repository lease through durable attempt and
// generation publication. Returned handles must be closed, including on poll or
// retrieval errors that retain the previous generation. Valid version 1 caches
// migrate under this same lease without losing rollback or polling state.
func (c rrdpHTTPClient) RefreshDiskPersistent(ctx context.Context, dir, uri string, now time.Time) (cache *rrdpDiskCache, err error) {
	if !filepath.IsAbs(dir) || !validRRDPURL(uri) || now.IsZero() || (now.Year() < 1970 || now.Year() > 9999) {
		return nil, errRPKIRRDPCache
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	info, e := os.Lstat(dir)
	if e != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errRPKIRRDPCache
	}
	path := filepath.Join(dir, rrdpDiskPrefix(uri)+".json")
	lock := path + ".lock"
	if os.Mkdir(lock, 0700) != nil {
		return nil, errRPKIRRDPCache
	}
	defer func() {
		if os.Remove(lock) != nil || syncRPKIExchangeDirectory(dir) != nil {
			cache.Close()
			cache = nil
			err = errRPKIRRDPCache
		}
	}()
	if syncRPKIExchangeDirectory(dir) != nil {
		return nil, errRPKIRRDPCache
	}
	cache, e = readRRDPDisk(path, uri)
	if e != nil {
		cache, e = migrateRRDPDisk(ctx, path, uri)
		if e != nil {
			return nil, e
		}
	}
	if cache == nil {
		cache = &rrdpDiskCache{Record: rrdpDiskRecord{Version: 2, NotificationURI: uri}}
	} else if now.Before(cache.Record.LastAttempt.Add(time.Minute)) {
		return cache, errRPKIRRDPPoll
	}
	cache.Record.LastAttempt = now.UTC()
	if _, e = saveRRDPDisk(path, cache.Record); e != nil {
		cache.Close()
		return nil, e
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	fetched, e := c.FetchNotification(ctx, uri, cache.Record.LastModified)
	if e != nil {
		return cache, e
	}
	if fetched.NotModified {
		if cache.Record.Session == "" {
			return cache, errRPKIRRDP
		}
		return cache, nil
	}
	n := fetched.Notification
	if n.Session == cache.Record.Session {
		old, _ := new(big.Int).SetString(cache.Record.Serial, 10)
		next, _ := new(big.Int).SetString(n.Serial, 10)
		switch next.Cmp(old) {
		case -1:
			return cache, errRPKIRRDP
		case 0:
			if n.Snapshot != cache.Record.Snapshot {
				return cache, errRPKIRRDP
			}
			record := cache.Record
			record.LastModified = fetched.LastModified
			if _, e = saveRRDPDisk(path, record); e != nil {
				cache.Close()
				return nil, e
			}
			cache.Record = record
			return cache, nil
		}
	}
	spool, e := c.diskDeltaCandidate(ctx, dir, n, cache)
	if e != nil {
		return cache, e
	}
	if spool == nil {
		spool, e = c.FetchSnapshotSpool(ctx, dir, n)
		if e != nil {
			return cache, e
		}
	}
	defer spool.Close()
	pack, e := stageRRDPDiskPack(ctx, dir, uri, spool)
	if e != nil {
		return cache, e
	}
	defer pack.Close()
	record := rrdpDiskRecord{Version: 2, NotificationURI: uri, Session: n.Session, Serial: n.Serial, Snapshot: n.Snapshot, LastModified: fetched.LastModified, LastAttempt: now.UTC(), Pack: filepath.Base(pack.file.Name()), Size: pack.size, Entries: pack.entries}
	if e = ctx.Err(); e != nil {
		return cache, e
	}
	published, e := saveRRDPDisk(path, record)
	pack.keep = published
	if e != nil {
		cache.Close()
		return nil, e
	}
	oldPack := cache.Record.Pack
	cache.Close()
	// Reopen under the lease, before exposing the durable generation to callers.
	cache, e = readRRDPDisk(path, uri)
	if e != nil {
		return nil, e
	}
	if oldPack != "" && oldPack != record.Pack {
		_ = os.Remove(filepath.Join(dir, oldPack))
	}
	return cache, nil
}

// Migration is one-way and uses the existing strict v1 reader. A corrupt or
// unknown cache format is never treated as an empty cache or retried remotely.
func migrateRRDPDisk(ctx context.Context, path, uri string) (*rrdpDiskCache, error) {
	old, err := readRRDPCache(path, uri)
	if err != nil || old == nil {
		return nil, errRPKIRRDPCache
	}
	r := rrdpDiskRecord{Version: 2, NotificationURI: uri, Session: old.Repository.Session, Serial: old.Repository.Serial, Snapshot: old.Snapshot, LastModified: old.LastModified, LastAttempt: old.LastAttempt}
	var pack *rrdpSnapshotSpool
	if r.Session != "" {
		f, err := os.CreateTemp(filepath.Dir(path), rrdpDiskPrefix(uri)+"-pack-*")
		if err != nil {
			return nil, errRPKIRRDPCache
		}
		pack = &rrdpSnapshotSpool{file: f, entries: make(map[string]rrdpSpoolEntry, len(old.Repository.Objects))}
		defer pack.Close()
		for uri, data := range old.Repository.Objects {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if _, err := f.Write(data); err != nil {
				return nil, errRPKIRRDPCache
			}
			pack.entries[uri] = rrdpSpoolEntry{Offset: pack.size, Size: len(data), Hash: sha256.Sum256(data)}
			pack.size += int64(len(data))
		}
		if f.Sync() != nil || syncRPKIExchangeDirectory(filepath.Dir(path)) != nil {
			return nil, errRPKIRRDPCache
		}
		r.Pack = filepath.Base(f.Name())
		r.Size = pack.size
		r.Entries = pack.entries
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	published, err := saveRRDPDisk(path, r)
	if pack != nil {
		pack.keep = published
	}
	if err != nil {
		return nil, err
	}
	return readRRDPDisk(path, uri)
}
