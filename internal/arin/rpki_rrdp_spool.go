package arin

import (
	"context"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
)

var rrdpDiskLimits = rrdpStreamLimits{Encoded: 2 << 30, Decoded: 1 << 30, Objects: 1000000}

type rrdpSpoolEntry struct {
	Offset int64
	Size   int
	Hash   [sha256.Size]byte
}

// A spool owns a temporary packed object file. It becomes visible to its caller
// only after complete XML, identity, uniqueness and digest validation. Contents
// still require RPKI manifest and path validation. Close removes the file.
// This is an ingestion stage, not a persistent repository cache.
type rrdpSnapshotSpool struct {
	file    *os.File
	entries map[string]rrdpSpoolEntry
	size    int64
}

func spoolRRDPSnapshot(ctx context.Context, directory string, input io.Reader, n *rrdpNotification) (*rrdpSnapshotSpool, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(directory) || input == nil || n == nil || !rrdpSession.MatchString(n.Session) || !rrdpSerial.MatchString(n.Serial) {
		return nil, errRPKIRRDP
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errRPKIRRDPCache
	}
	f, err := os.CreateTemp(directory, ".rpki-snapshot-*")
	if err != nil {
		return nil, errRPKIRRDPCache
	}
	spool := &rrdpSnapshotSpool{file: f, entries: make(map[string]rrdpSpoolEntry)}
	success := false
	defer func() {
		if !success {
			spool.Close()
		}
	}()
	uriBytes := 0
	err = streamRRDPObjectFile(&rrdpContextReader{ctx: ctx, input: input}, n.Session, n.Serial, n.Snapshot.Hash, false, rrdpDiskLimits, func(c rrdpChange) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, exists := spool.entries[c.URI]; exists {
			return errRPKIRRDP
		}
		uriBytes += len(c.URI)
		if uriBytes > 128<<20 {
			return errRPKIRRDP
		}
		if _, err := f.Write(c.Data); err != nil {
			return errRPKIRRDPCache
		}
		spool.entries[c.URI] = rrdpSpoolEntry{Offset: spool.size, Size: len(c.Data), Hash: sha256.Sum256(c.Data)}
		spool.size += int64(len(c.Data))
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if f.Sync() != nil {
		return nil, errRPKIRRDPCache
	}
	success = true
	return spool, nil
}

func (s *rrdpSnapshotSpool) ReadObject(uri string) ([]byte, error) {
	if s == nil || s.file == nil {
		return nil, errRPKIRRDPCache
	}
	entry, ok := s.entries[uri]
	if !ok {
		return nil, os.ErrNotExist
	}
	if entry.Offset < 0 || entry.Size <= 0 || entry.Size > 4<<20 || entry.Offset > s.size-int64(entry.Size) {
		return nil, errRPKIRRDPCache
	}
	data := make([]byte, entry.Size)
	if _, err := s.file.ReadAt(data, entry.Offset); err != nil || sha256.Sum256(data) != entry.Hash {
		return nil, errRPKIRRDPCache
	}
	return data, nil
}

func (s *rrdpSnapshotSpool) Close() error {
	if s == nil || s.file == nil {
		return nil
	}
	name := s.file.Name()
	err := s.file.Close()
	s.file = nil
	s.entries = nil
	if e := os.Remove(name); e != nil {
		err = e
	}
	return err
}

type rrdpContextReader struct {
	ctx   context.Context
	input io.Reader
}

func (r *rrdpContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.input.Read(p)
}
