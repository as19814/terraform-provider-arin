package arin

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"strings"
	"time"
)

var errRPKIRRDPPoll = errors.New("RPKI repository was polled less than one minute ago")

type rrdpCache struct {
	Repository                rrdpRepository
	Snapshot                  rrdpFileReference
	LastModified, LastAttempt time.Time
}

// Refresh returns the previous repository on failure, with LastAttempt advanced
// for polling control. After a retrieval attempt, callers must persist its
// LastAttempt even when err != nil. Configuration errors do not start attempts.
// Only a complete delta chain or snapshot replaces the repository contents.
func (c rrdpHTTPClient) Refresh(ctx context.Context, uri string, previous *rrdpCache, now time.Time) (rrdpCache, error) {
	next := rrdpCache{Repository: rrdpRepository{NotificationURI: uri}}
	if !validRRDPURL(uri) || now.IsZero() {
		return next, errRPKIRRDP
	}
	if err := ctx.Err(); err != nil {
		return next, err
	}
	if previous != nil {
		next = *previous
		if next.Repository.NotificationURI != uri {
			return next, errRPKIRRDP
		}
		objects, err := cloneRRDPObjects(next.Repository.Objects)
		if err != nil {
			return next, err
		}
		next.Repository.Objects = objects
		if next.Repository.Session != "" {
			if !rrdpSession.MatchString(next.Repository.Session) || !rrdpSerial.MatchString(next.Repository.Serial) || !validRRDPURL(next.Snapshot.URI) {
				return next, errRPKIRRDP
			}
		} else if next.Repository.Serial != "" || len(objects) != 0 || !next.LastModified.IsZero() {
			return next, errRPKIRRDP
		}
		if !next.LastAttempt.IsZero() && now.Before(next.LastAttempt.Add(time.Minute)) {
			return next, errRPKIRRDPPoll
		}
	}
	next.LastAttempt = now.UTC()
	// Bound the whole refresh, not only each individual HTTP request.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	fetched, err := c.FetchNotification(ctx, uri, next.LastModified)
	if err != nil {
		return next, err
	}
	if fetched.NotModified {
		if next.Repository.Session == "" {
			return next, errRPKIRRDP
		}
		return next, nil
	}
	n := fetched.Notification
	current := next.Repository
	if current.Session == n.Session {
		old, _ := new(big.Int).SetString(current.Serial, 10)
		serial, _ := new(big.Int).SetString(n.Serial, 10)
		comparison := serial.Cmp(old)
		if comparison < 0 {
			return next, errRPKIRRDP
		}
		if comparison == 0 {
			if next.Snapshot != n.Snapshot {
				return next, errRPKIRRDP
			}
			next.LastModified = fetched.LastModified
			return next, nil
		}
		wanted := new(big.Int).Add(old, big.NewInt(1)).String()
		start := -1
		for i, d := range n.Deltas {
			if d.Serial == wanted {
				start = i
				break
			}
		}
		if start >= 0 {
			candidate := current
			for _, ref := range n.Deltas[start:] {
				candidate, err = c.FetchDelta(ctx, uri, n.Session, ref, candidate)
				if err != nil {
					break
				}
			}
			if err == nil {
				next.Repository = candidate
				next.Snapshot = n.Snapshot
				next.LastModified = fetched.LastModified
				return next, nil
			}
			if ctx.Err() != nil {
				return next, ctx.Err()
			}
		}
	}
	objects, err := c.FetchSnapshot(ctx, n)
	if err != nil {
		return next, err
	}
	next.Repository = rrdpRepository{NotificationURI: uri, Session: n.Session, Serial: n.Serial, Objects: objects}
	next.Snapshot = n.Snapshot
	next.LastModified = fetched.LastModified
	return next, nil
}

func cloneRRDPObjects(objects map[string][]byte) (map[string][]byte, error) {
	if len(objects) > 10000 {
		return nil, errRPKIRRDP
	}
	out := make(map[string][]byte, len(objects))
	total := 0
	for uri, data := range objects {
		if !publicationURI(uri) || strings.HasSuffix(uri, "/") || len(data) == 0 || len(data) > 4<<20 {
			return nil, errRPKIRRDP
		}
		total += len(data)
		if total > 64<<20 {
			return nil, errRPKIRRDP
		}
		out[uri] = bytes.Clone(data)
	}
	return out, nil
}
