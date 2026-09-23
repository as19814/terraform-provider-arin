package arin

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestOrganizationLockScopeAndCancellation(t *testing.T) {
	ctx := context.Background()
	release, err := lockOrganization(ctx, "https://example.test", "ONE")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for _, key := range [][2]string{{"https://example.test", "TWO"}, {"https://other.test", "ONE"}} {
		unlock, err := lockOrganization(ctx, key[0], key[1])
		if err != nil {
			t.Fatal(err)
		}
		unlock()
	}
	timeout, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if _, err := lockOrganization(timeout, "https://example.test", "ONE"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected cancellation: %v", err)
	}
}
func TestOrganizationLockSerializesAndReleases(t *testing.T) {
	var wg sync.WaitGroup
	active, maxActive := 0, 0
	var mu sync.Mutex
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := lockOrganization(context.Background(), "https://example.test", "SHARED")
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			active--
			mu.Unlock()
			release()
		}()
	}
	wg.Wait()
	if maxActive != 1 {
		t.Fatal("organization operations overlapped")
	}
	organizationLocks.Lock()
	defer organizationLocks.Unlock()
	if _, ok := organizationLocks.entries["https://example.test/SHARED"]; ok {
		t.Fatal("unused lock retained")
	}
}
