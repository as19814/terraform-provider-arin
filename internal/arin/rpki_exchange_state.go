package arin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

var exchangeDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)
var exchangeOperation = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var errRPKIExchangeState = errors.New("RPKI exchange journal unavailable or requires reconciliation")

type rpkiPendingExchange struct {
	RequestSHA256 string    `json:"request_sha256"`
	Operation     string    `json:"operation"`
	SigningTime   time.Time `json:"signing_time"`
}
type rpkiExchangeState struct {
	Version       int                  `json:"version"`
	PeerID        string               `json:"peer_id"`
	LastSent      time.Time            `json:"last_sent"`
	LastReceived  time.Time            `json:"last_received"`
	Pending       *rpkiPendingExchange `json:"pending,omitempty"`
	RecoveredRead *rpkiPendingExchange `json:"recovered_read,omitempty"`
}

// rpkiExchangeLease holds an exclusive filesystem lease for one protocol peer.
// A process crash leaves the lock directory in place for explicit reconciliation.
// Deleting journals or using separate directories defeats this local guarantee.
type rpkiExchangeLease struct {
	mu               sync.Mutex
	path, lock       string
	state            rpkiExchangeState
	closed, poisoned bool
}

func openRPKIExchange(directory, peerID string) (*rpkiExchangeLease, error) {
	return openRPKIExchangeMode(directory, peerID, true)
}
func openRPKIExchangeMode(directory, peerID string, create bool) (*rpkiExchangeLease, error) {
	if !filepath.IsAbs(directory) || !exchangeDigest.MatchString(peerID) {
		return nil, errRPKIExchangeState
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errRPKIExchangeState
	}
	path := filepath.Join(directory, "rpki-exchange-"+peerID+".json")
	lease := &rpkiExchangeLease{path: path, lock: path + ".lock", state: rpkiExchangeState{Version: 1, PeerID: peerID}}
	if err := os.Mkdir(lease.lock, 0700); err != nil {
		return nil, errRPKIExchangeState
	}
	success := false
	defer func() {
		if !success {
			_ = os.Remove(lease.lock)
		}
	}()
	if err := syncRPKIExchangeDirectory(directory); err != nil {
		return nil, errRPKIExchangeState
	}
	info, err = os.Lstat(path)
	if os.IsNotExist(err) {
		if !create {
			return nil, errRPKIExchangeState
		}
		if err = lease.save(); err != nil {
			return nil, errRPKIExchangeState
		}
	} else {
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return nil, errRPKIExchangeState
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, errRPKIExchangeState
		}
		actual, statErr := f.Stat()
		raw, readErr := io.ReadAll(io.LimitReader(f, 65537))
		closeErr := f.Close()
		if statErr != nil || !os.SameFile(info, actual) || readErr != nil || closeErr != nil || len(raw) > 65536 {
			return nil, errRPKIExchangeState
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		var loaded rpkiExchangeState
		if err := decoder.Decode(&loaded); err != nil {
			return nil, errRPKIExchangeState
		}
		canonical, marshalErr := json.Marshal(loaded)
		if marshalErr != nil || !bytes.Equal(bytes.TrimSpace(raw), canonical) {
			return nil, errRPKIExchangeState
		}
		lease.state = loaded
		var extra any
		if decoder.Decode(&extra) != io.EOF || lease.state.Version != 1 || lease.state.PeerID != peerID || !validExchangeTime(lease.state.LastSent) || !validExchangeTime(lease.state.LastReceived) {
			return nil, errRPKIExchangeState
		}
		if lease.state.Pending == nil && lease.state.RecoveredRead == nil && lease.state.LastSent.IsZero() != lease.state.LastReceived.IsZero() {
			return nil, errRPKIExchangeState
		}
		if p := lease.state.RecoveredRead; p != nil {
			if !readOnlyRPKIOperation(p.Operation) || !exchangeDigest.MatchString(p.RequestSHA256) || p.SigningTime.IsZero() || !validExchangeTime(p.SigningTime) || p.SigningTime.After(lease.state.LastSent) || (lease.state.Pending != nil && !lease.state.Pending.SigningTime.After(p.SigningTime)) {
				return nil, errRPKIExchangeState
			}
		}
		if p := lease.state.Pending; p != nil {
			if !exchangeDigest.MatchString(p.RequestSHA256) || !exchangeOperation.MatchString(p.Operation) || p.SigningTime.IsZero() || !validExchangeTime(p.SigningTime) || !p.SigningTime.Equal(lease.state.LastSent) {
				return nil, errRPKIExchangeState
			}
		}
	}
	success = true
	return lease, nil
}
func validExchangeTime(t time.Time) bool {
	return t.IsZero() || (t.Nanosecond() == 0 && t.Year() >= 1970 && t.Year() <= 9999)
}
func (l *rpkiExchangeLease) State() (rpkiExchangeState, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.poisoned {
		return rpkiExchangeState{}, errRPKIExchangeState
	}
	state := l.state
	if state.Pending != nil {
		pending := *state.Pending
		state.Pending = &pending
	}
	if state.RecoveredRead != nil {
		recovered := *state.RecoveredRead
		state.RecoveredRead = &recovered
	}
	return state, nil
}

// Begin must succeed before the caller dispatches HTTP. Any existing pending
// record blocks another request, including after a process restart.
func (l *rpkiExchangeLease) Begin(digest, operation string, signingTime time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.poisoned || l.state.Pending != nil || !exchangeDigest.MatchString(digest) || !exchangeOperation.MatchString(operation) || signingTime.IsZero() || !validExchangeTime(signingTime) || signingTime.Before(l.state.LastSent) || (l.state.RecoveredRead != nil && !signingTime.After(l.state.RecoveredRead.SigningTime)) {
		return errRPKIExchangeState
	}
	l.state.LastSent = signingTime.UTC()
	l.state.Pending = &rpkiPendingExchange{RequestSHA256: digest, Operation: operation, SigningTime: signingTime.UTC()}
	if err := l.save(); err != nil {
		l.poisoned = true
		return errRPKIExchangeState
	}
	return nil
}

// Complete is only for a fully authenticated and correlated response. It does
// not itself authenticate CMS or decide whether an uncertain mutation succeeded.
func (l *rpkiExchangeLease) Complete(digest string, receivedTime time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.poisoned || l.state.Pending == nil || l.state.Pending.RequestSHA256 != digest || receivedTime.IsZero() || !validExchangeTime(receivedTime) || receivedTime.Before(l.state.LastReceived) {
		return errRPKIExchangeState
	}
	l.state.LastReceived = receivedTime.UTC()
	l.state.Pending = nil
	if err := l.save(); err != nil {
		l.poisoned = true
		return errRPKIExchangeState
	}
	return nil
}
func (l *rpkiExchangeLease) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	if err := os.Remove(l.lock); err != nil {
		return errRPKIExchangeState
	}
	if err := syncRPKIExchangeDirectory(filepath.Dir(l.path)); err != nil {
		return errRPKIExchangeState
	}
	return nil
}
func (l *rpkiExchangeLease) save() error {
	dir := filepath.Dir(l.path)
	f, err := os.CreateTemp(dir, ".rpki-exchange-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = json.NewEncoder(f).Encode(l.state); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(name, l.path); err != nil {
		return err
	}
	return syncRPKIExchangeDirectory(dir)
}
func syncRPKIExchangeDirectory(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	syncErr := d.Sync()
	closeErr := d.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
