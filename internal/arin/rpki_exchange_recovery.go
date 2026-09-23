package arin

import (
	"encoding/json"
)

func readOnlyRPKIOperation(operation string) bool {
	return operation == "publication-list" || operation == "updown-list"
}

// RecoverRead abandons exactly one pending inventory request without inventing
// an authenticated response or resetting signing-time watermarks. Mutations
// require separate reconciliation and cannot be cleared through this method.
func (l *rpkiExchangeLease) RecoverRead(digest string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.poisoned || !exchangeDigest.MatchString(digest) || l.state.Pending == nil || l.state.Pending.RequestSHA256 != digest || !readOnlyRPKIOperation(l.state.Pending.Operation) {
		return errRPKIExchangeState
	}
	recovered := *l.state.Pending
	l.state.RecoveredRead = &recovered
	l.state.Pending = nil
	if err := l.save(); err != nil {
		l.poisoned = true
		return errRPKIExchangeState
	}
	return nil
}

// InspectRPKIJournal returns public operation metadata from an existing journal.
// It takes the ordinary exclusive lease and never removes a crash or live lock.
func InspectRPKIJournal(directory, peerID string) ([]byte, error) {
	return accessRPKIJournal(directory, peerID, "")
}

// RecoverRPKIRead explicitly abandons a read-only request identified by its exact
// signed request digest. A repeated recovery fails rather than consuming any
// later operation. This function performs no HTTP request.
func RecoverRPKIRead(directory, peerID, digest string) ([]byte, error) {
	if !exchangeDigest.MatchString(digest) {
		return nil, errRPKIExchangeState
	}
	return accessRPKIJournal(directory, peerID, digest)
}
func accessRPKIJournal(directory, peerID, digest string) (result []byte, err error) {
	if !exchangeDigest.MatchString(peerID) {
		return nil, errRPKIExchangeState
	}
	lease, e := openRPKIExchangeMode(directory, peerID, false)
	if e != nil {
		return nil, e
	}
	defer func() {
		if e := lease.Close(); e != nil {
			result = nil
			err = e
		}
	}()
	if digest != "" {
		if e := lease.RecoverRead(digest); e != nil {
			return nil, e
		}
	}
	state, e := lease.State()
	if e != nil {
		return nil, e
	}
	return json.MarshalIndent(state, "", "  ")
}
