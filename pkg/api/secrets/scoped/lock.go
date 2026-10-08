// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
)

const (
	// lockTimeoutEnv overrides how long a command waits for another sc process
	// to finish changing the store, as a Go duration.
	lockTimeoutEnv = "SC_SCOPE_LOCK_TIMEOUT"
	// lockFileName is the fallback lock file in .sc, used where the filesystem
	// cannot lock a directory (NFS and some network or FUSE mounts).
	lockFileName = ".scope-store.lock"
)

var (
	lockTimeout = 2 * time.Minute
	// lockWaitNotice is how long a command waits before saying it is waiting.
	lockWaitNotice = 5 * time.Second
	// tryLockFn and lockFallbackNeeded are seams for tests.
	tryLockFn          = tryLock
	lockFallbackNeeded = unsupportedLock
)

// LockStore serializes the commands that read, change and write scope files and
// scopes.yaml in one repository. The writes are atomic, but two concurrent
// read-modify-write runs (parallel `scope set` calls in a CI job) would each save
// the file they read and drop the other's change.
//
// The lock is an advisory flock on the .sc directory itself, so no file is
// created and every process reaching the repository shares it. Where the
// filesystem cannot lock a directory, a lock file in .sc is locked instead.
// waiting, when not nil, is called once if the lock is not free after a few
// seconds. Call the returned function to release the lock.
func LockStore(scDir string, waiting func()) (func(), error) {
	if err := os.MkdirAll(scDir, 0o755); err != nil {
		return nil, errors.Wrapf(err, "failed to create %s", scDir)
	}
	timeout := lockTimeout
	if v := os.Getenv(lockTimeoutEnv); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return nil, errors.Errorf("%s=%q is not a positive duration", lockTimeoutEnv, v)
		}
		timeout = d
	}
	f, err := os.Open(scDir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open %s to lock it", scDir)
	}
	start := time.Now()
	noticed := false
	for {
		locked, err := tryLockFn(f)
		if err != nil && lockFallbackNeeded(err) && f.Name() == scDir {
			_ = f.Close()
			if f, err = os.OpenFile(filepath.Join(scDir, lockFileName), os.O_CREATE|os.O_RDWR, 0o600); err != nil {
				return nil, errors.Wrapf(err, "failed to open the lock file in %s", scDir)
			}
			continue
		}
		if err != nil {
			_ = f.Close()
			return nil, errors.Wrapf(err, "failed to lock %s", scDir)
		}
		if locked {
			break
		}
		waited := time.Since(start)
		if waited >= timeout {
			_ = f.Close()
			return nil, errors.Errorf("another sc process has held the lock on %s for %s; retry when it finishes (or raise %s)", scDir, timeout, lockTimeoutEnv)
		}
		if !noticed && waited >= lockWaitNotice && waiting != nil {
			waiting()
			noticed = true
		}
		time.Sleep(time.Duration(50+rand.Intn(100)) * time.Millisecond) //nolint:gosec // jitter, not security
	}
	return func() {
		_ = unlock(f)
		_ = f.Close()
	}, nil
}
