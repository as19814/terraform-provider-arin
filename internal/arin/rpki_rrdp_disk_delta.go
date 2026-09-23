package arin

import (
	"context"
	"crypto/sha256"
	"math/big"
	"net/http"
	"os"
	"time"
)

func copyRRDPSpool(ctx context.Context, dir string, source *rrdpSnapshotSpool) (*rrdpSnapshotSpool, error) {
	f, err := os.CreateTemp(dir, ".rpki-snapshot-*")
	if err != nil {
		return nil, errRPKIRRDPCache
	}
	out := &rrdpSnapshotSpool{file: f, entries: make(map[string]rrdpSpoolEntry, len(source.entries))}
	success := false
	defer func() {
		if !success {
			out.Close()
		}
	}()
	for uri := range source.entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := source.ReadObject(uri)
		if err != nil {
			return nil, err
		}
		if out.size+int64(len(data)) > rrdpDiskLimits.Decoded {
			return nil, errRPKIRRDP
		}
		if _, err = f.Write(data); err != nil {
			return nil, errRPKIRRDPCache
		}
		out.entries[uri] = rrdpSpoolEntry{Offset: out.size, Size: len(data), Hash: sha256.Sum256(data)}
		out.size += int64(len(data))
	}
	if f.Sync() != nil {
		return nil, errRPKIRRDPCache
	}
	success = true
	return out, nil
}

// A nil candidate requests full snapshot fallback. The candidate is private
// throughout the complete delta chain; partial updates never reach the cache.
func (c rrdpHTTPClient) diskDeltaCandidate(ctx context.Context, dir string, n *rrdpNotification, base *rrdpDiskCache) (*rrdpSnapshotSpool, error) {
	if base.Record.Session != n.Session || base.Objects == nil {
		return nil, nil
	}
	next, _ := new(big.Int).SetString(base.Record.Serial, 10)
	next.Add(next, big.NewInt(1))
	start := -1
	for i, d := range n.Deltas {
		if d.Serial == next.String() {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, nil
	}
	candidate, err := copyRRDPSpool(ctx, dir, base.Objects)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, nil
	}
	defer candidate.Close()
	for _, ref := range n.Deltas[start:] {
		if ref.Serial != next.String() {
			return nil, nil
		}
		if err = c.applyDiskDelta(ctx, candidate, n.Session, ref); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, nil
		}
		next.Add(next, big.NewInt(1))
	}
	next.Sub(next, big.NewInt(1))
	if next.String() != n.Serial {
		return nil, nil
	}
	// Repack to remove superseded/withdrawn objects and restore contiguous
	// offsets before publishing the index. Old files stay untouched.
	out, err := copyRRDPSpool(ctx, dir, candidate)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, nil
	}
	return out, nil
}

func (c rrdpHTTPClient) applyDiskDelta(ctx context.Context, candidate *rrdpSnapshotSpool, session string, ref rrdpDeltaReference) error {
	response, err := c.openGET(ctx, ref.File.URI, time.Time{})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > rrdpDiskLimits.Encoded {
		return errRPKIRRDPHTTP
	}
	var decoded int64
	uriBytes := 0
	for uri, e := range candidate.entries {
		decoded += int64(e.Size)
		uriBytes += len(uri)
	}
	return streamRRDPObjectFile(&rrdpContextReader{ctx: ctx, input: response.Body}, session, ref.Serial, ref.File.Hash, true, rrdpDiskLimits, func(change rrdpChange) error {
		old, exists := candidate.entries[change.URI]
		if change.OldHash == nil {
			if exists || change.Withdraw {
				return errRPKIRRDP
			}
		} else if !exists || old.Hash != *change.OldHash {
			return errRPKIRRDP
		}
		if exists {
			decoded -= int64(old.Size)
			uriBytes -= len(change.URI)
		}
		if change.Withdraw {
			delete(candidate.entries, change.URI)
			return nil
		}
		decoded += int64(len(change.Data))
		uriBytes += len(change.URI)
		if decoded > rrdpDiskLimits.Decoded || uriBytes > 128<<20 || (!exists && len(candidate.entries) >= rrdpDiskLimits.Objects) || candidate.size+int64(len(change.Data)) > 2<<30 {
			return errRPKIRRDP
		}
		if _, err := candidate.file.Write(change.Data); err != nil {
			return errRPKIRRDPCache
		}
		candidate.entries[change.URI] = rrdpSpoolEntry{Offset: candidate.size, Size: len(change.Data), Hash: sha256.Sum256(change.Data)}
		candidate.size += int64(len(change.Data))
		return nil
	})
}
