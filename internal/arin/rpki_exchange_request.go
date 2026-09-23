package arin

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// BeginSigned durably saves the exact request before recording dispatch intent.
// The fixed per-peer sidecar retains at most one 4 MiB request. It is replaced
// only when no request is pending; no signing key is included in CMS evidence.
func (l *rpkiExchangeLease) BeginSigned(digest, operation string, signingTime time.Time, signed []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.canBeginLocked(digest, operation, signingTime) || len(signed) == 0 || len(signed) > 4<<20 || fmt.Sprintf("%x", sha256.Sum256(signed)) != digest {
		return errRPKIExchangeState
	}
	if err := saveRPKISignedRequest(l.path+".request.der", signed); err != nil {
		l.poisoned = true
		return errRPKIExchangeState
	}
	return l.beginLocked(digest, operation, signingTime)
}

// PendingRequest returns digest-bound evidence, never a reconstructed request.
// Legacy journals without evidence fail closed. Reading evidence does not clear
// the pending operation or establish whether the remote mutation succeeded.
func (l *rpkiExchangeLease) PendingRequest() ([]byte, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.poisoned || l.state.Pending == nil {
		return nil, errRPKIExchangeState
	}
	path := l.path + ".request.der"
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() <= 0 || info.Size() > 4<<20 {
		return nil, errRPKIExchangeState
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errRPKIExchangeState
	}
	actual, statErr := f.Stat()
	raw, readErr := io.ReadAll(io.LimitReader(f, (4<<20)+1))
	closeErr := f.Close()
	if statErr != nil || !os.SameFile(info, actual) || readErr != nil || closeErr != nil || len(raw) > 4<<20 || fmt.Sprintf("%x", sha256.Sum256(raw)) != l.state.Pending.RequestSHA256 {
		return nil, errRPKIExchangeState
	}
	return raw, nil
}
func saveRPKISignedRequest(path string, signed []byte) error {
	info, err := os.Lstat(path)
	if !os.IsNotExist(err) && (err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
		return errRPKIExchangeState
	}
	directory := filepath.Dir(path)
	f, err := os.CreateTemp(directory, ".rpki-request-*")
	if err != nil {
		return errRPKIExchangeState
	}
	name := f.Name()
	defer os.Remove(name)
	_, err = f.Write(signed)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return errRPKIExchangeState
	}
	if os.Rename(name, path) != nil || syncRPKIExchangeDirectory(directory) != nil {
		return errRPKIExchangeState
	}
	return nil
}
