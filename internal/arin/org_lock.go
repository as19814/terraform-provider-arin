package arin

import (
	"context"
	"sync"
)

type organizationLock struct {
	token chan struct{}
	users int
}

var organizationLocks = struct {
	sync.Mutex
	entries map[string]*organizationLock
}{entries: map[string]*organizationLock{}}

// lockOrganization serializes mutations across provider aliases in this process.
// OT&E association endpoints can lose concurrent updates on the same org.
func lockOrganization(ctx context.Context, origin, org string) (func(), error) {
	key := origin + "/" + org
	organizationLocks.Lock()
	entry := organizationLocks.entries[key]
	if entry == nil {
		entry = &organizationLock{token: make(chan struct{}, 1)}
		organizationLocks.entries[key] = entry
	}
	entry.users++
	organizationLocks.Unlock()
	release := func() {
		organizationLocks.Lock()
		defer organizationLocks.Unlock()
		entry.users--
		if entry.users == 0 {
			delete(organizationLocks.entries, key)
		}
	}
	select {
	case entry.token <- struct{}{}:
		return func() { <-entry.token; release() }, nil
	case <-ctx.Done():
		release()
		return nil, ctx.Err()
	}
}
