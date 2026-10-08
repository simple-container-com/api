// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

//go:build unix

package scoped

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

// Concurrent read-modify-write cycles under LockStore lose nothing. Without the
// lock, the sleep between read and write makes them lose updates every run.
func TestLockStoreSerializesReadModifyWrite(t *testing.T) {
	scDir := t.TempDir()
	counter := filepath.Join(scDir, "counter")
	if err := os.WriteFile(counter, []byte("0"), 0o600); err != nil {
		t.Fatal(err)
	}
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock, err := LockStore(scDir)
			if err != nil {
				errs <- err
				return
			}
			defer unlock()
			data, _ := os.ReadFile(counter)
			v, _ := strconv.Atoi(string(data))
			time.Sleep(2 * time.Millisecond)
			errs <- writeFileAtomic(counter, []byte(strconv.Itoa(v+1)), 0o600)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if data, _ := os.ReadFile(counter); string(data) != strconv.Itoa(n) {
		t.Fatalf("counter = %s after %d locked increments", data, n)
	}
}

// Two repositories do not share a lock.
func TestLockStoreIsPerRepository(t *testing.T) {
	unlock, err := LockStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	done := make(chan struct{})
	go func() {
		u, err := LockStore(t.TempDir())
		if err == nil {
			u()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("another repository's store waited for this one's lock")
	}
}
