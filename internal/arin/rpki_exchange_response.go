package arin

import (
	"io"
	"os"
	"path/filepath"
	"time"
)

// This receipt records what the exchange authenticated and accepted. Retained
// bytes must be authenticated and correlated again before historical recovery.
type rpkiCompletedResponse struct {
	Request     rpkiPendingExchange `json:"request"`
	SHA256      string              `json:"sha256"`
	SigningTime time.Time           `json:"signing_time"`
}

func validRPKICompletedResponse(r rpkiCompletedResponse) bool {
	return exchangeDigest.MatchString(r.SHA256) && exchangeDigest.MatchString(r.Request.RequestSHA256) && exchangeOperation.MatchString(r.Request.Operation) && !r.Request.SigningTime.IsZero() && validExchangeTime(r.Request.SigningTime) && !r.SigningTime.IsZero() && validExchangeTime(r.SigningTime)
}

// CompleteSigned is called only after CMS authentication and request/response
// correlation. Evidence is synced before completion clears the pending request.
// Digest-named sidecars preserve the last accepted response through failed
// completion writes. Only a successful durable replacement removes the old one.
func (l *rpkiExchangeLease) CompleteSigned(digest string, receivedTime time.Time, body []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.canCompleteLocked(digest, receivedTime) || len(body) == 0 || len(body) > 4<<20 {
		return errRPKIExchangeState
	}
	receipt := rpkiCompletedResponse{Request: *l.state.Pending, SHA256: rpkiManifestDigest(body), SigningTime: receivedTime.UTC()}
	if !validRPKICompletedResponse(receipt) {
		return errRPKIExchangeState
	}
	path := l.path + ".response-" + receipt.SHA256 + ".der"
	if err := saveRPKISignedRequest(path, body); err != nil {
		l.poisoned = true
		return errRPKIExchangeState
	}
	old := l.state.LastResponse
	l.state.LastResponse = &receipt
	if err := l.completeLocked(digest, receivedTime); err != nil {
		return err
	}
	if old != nil && old.SHA256 != receipt.SHA256 {
		// An unlink failure leaves obsolete evidence, never a missing current file.
		_ = os.Remove(l.path + ".response-" + old.SHA256 + ".der")
		_ = syncRPKIExchangeDirectory(filepath.Dir(l.path))
	}
	return nil
}

// CompletedResponse reads the last completed exchange even while a later request
// is pending. It neither reconstructs responses nor changes pending state.
func (l *rpkiExchangeLease) CompletedResponse() (rpkiCompletedResponse, []byte, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.poisoned || l.state.LastResponse == nil || !validRPKICompletedResponse(*l.state.LastResponse) {
		return rpkiCompletedResponse{}, nil, errRPKIExchangeState
	}
	receipt := *l.state.LastResponse
	path := l.path + ".response-" + receipt.SHA256 + ".der"
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() <= 0 || info.Size() > 4<<20 {
		return rpkiCompletedResponse{}, nil, errRPKIExchangeState
	}
	f, err := os.Open(path)
	if err != nil {
		return rpkiCompletedResponse{}, nil, errRPKIExchangeState
	}
	actual, se := f.Stat()
	raw, re := io.ReadAll(io.LimitReader(f, (4<<20)+1))
	ce := f.Close()
	if se != nil || !os.SameFile(info, actual) || re != nil || ce != nil || len(raw) > 4<<20 || rpkiManifestDigest(raw) != receipt.SHA256 {
		return rpkiCompletedResponse{}, nil, errRPKIExchangeState
	}
	return receipt, raw, nil
}
