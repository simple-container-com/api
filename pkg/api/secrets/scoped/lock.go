// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"os"
	"time"

	"github.com/pkg/errors"
)

// lockTimeout bounds how long a command waits for another sc process to finish
// changing the store. A package variable so tests can shorten it.
var lockTimeout = 2 * time.Minute

// LockStore serializes the commands that read, change and write scope files and
// scopes.yaml in one repository. The writes are atomic, but two concurrent
// read-modify-write runs (parallel `scope set` calls in a CI job) would each save
// the file they read and drop the other's change.
//
// The lock is an advisory lock on the .sc directory itself: no lock file is
// created, every process that reaches the repository through any path, user or
// container shares it, and it disappears with the process. Call the returned
// function to release it.
func LockStore(scDir string) (func(), error) {
	if err := os.MkdirAll(scDir, 0o755); err != nil {
		return nil, errors.Wrapf(err, "failed to create %s", scDir)
	}
	d, err := os.Open(scDir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open %s to lock it", scDir)
	}
	deadline := time.Now().Add(lockTimeout)
	for {
		locked, err := tryLock(d)
		if err != nil {
			_ = d.Close()
			return nil, errors.Wrapf(err, "failed to lock %s", scDir)
		}
		if locked {
			break
		}
		if time.Now().After(deadline) {
			_ = d.Close()
			return nil, errors.Errorf("another sc process has held the lock on %s for %s; retry when it finishes", scDir, lockTimeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return func() {
		_ = unlock(d)
		_ = d.Close()
	}, nil
}
