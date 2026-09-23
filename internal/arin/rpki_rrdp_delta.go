package arin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"math/big"
	"strings"
	"time"
)

// Objects belong exclusively to this notification URI and session. Never merge
// maps from different repositories before applying a delta.
type rrdpRepository struct {
	NotificationURI, Session, Serial string
	Objects                          map[string][]byte
}

// applyRRDPDelta returns a new repository only after every operation succeeds.
// Neither the base map nor its byte buffers are changed or shared with output.
func applyRRDPDelta(body []byte, notificationURI, session string, ref rrdpDeltaReference, base rrdpRepository) (rrdpRepository, error) {
	var empty rrdpRepository
	if !validRRDPURL(notificationURI) || base.NotificationURI != notificationURI || base.Session != session || !rrdpSerial.MatchString(base.Serial) || !rrdpSerial.MatchString(ref.Serial) {
		return empty, errRPKIRRDP
	}
	next, _ := new(big.Int).SetString(base.Serial, 10)
	next.Add(next, big.NewInt(1))
	if next.String() != ref.Serial {
		return empty, errRPKIRRDP
	}
	changes, err := parseRRDPObjectFile(body, session, ref.Serial, ref.File.Hash, true)
	if err != nil {
		return empty, err
	}
	if len(base.Objects) > 10000 {
		return empty, errRPKIRRDP
	}
	objects := make(map[string][]byte, len(base.Objects))
	total := 0
	for uri, data := range base.Objects {
		if !publicationURI(uri) || strings.HasSuffix(uri, "/") || len(data) == 0 || len(data) > 4<<20 {
			return empty, errRPKIRRDP
		}
		total += len(data)
		if total > 64<<20 {
			return empty, errRPKIRRDP
		}
		objects[uri] = bytes.Clone(data)
	}
	for _, c := range changes {
		old, exists := objects[c.URI]
		if c.OldHash == nil {
			if exists || c.Withdraw {
				return empty, errRPKIRRDP
			}
		} else if !exists || sha256.Sum256(old) != *c.OldHash {
			return empty, errRPKIRRDP
		}
		total -= len(old)
		if c.Withdraw {
			delete(objects, c.URI)
		} else {
			objects[c.URI] = c.Data
			total += len(c.Data)
		}
		if len(objects) > 10000 || total > 64<<20 {
			return empty, errRPKIRRDP
		}
	}
	return rrdpRepository{NotificationURI: notificationURI, Session: session, Serial: ref.Serial, Objects: objects}, nil
}

func (c rrdpHTTPClient) FetchDelta(ctx context.Context, notificationURI, session string, ref rrdpDeltaReference, base rrdpRepository) (rrdpRepository, error) {
	if base.NotificationURI != notificationURI || base.Session != session {
		return rrdpRepository{}, errRPKIRRDP
	}
	body, _, _, err := c.get(ctx, ref.File.URI, 128<<20, time.Time{})
	if err != nil {
		return rrdpRepository{}, err
	}
	return applyRRDPDelta(body, notificationURI, session, ref, base)
}
