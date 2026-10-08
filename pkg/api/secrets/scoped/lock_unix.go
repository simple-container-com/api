// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

//go:build unix

package scoped

import (
	"errors"
	"os"
	"syscall"
)

// tryLock takes an exclusive flock without blocking; false means another process
// holds it.
func tryLock(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

func unlock(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }

// unsupportedLock reports the errors a filesystem gives when it cannot flock a
// directory (Linux NFS emulates flock with fcntl, which needs a writable file).
func unsupportedLock(err error) bool {
	for _, e := range []error{syscall.ENOLCK, syscall.EOPNOTSUPP, syscall.EBADF, syscall.EISDIR, syscall.EINVAL} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
