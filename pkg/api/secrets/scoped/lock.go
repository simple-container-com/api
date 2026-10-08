// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// LockStore serializes the commands that read, change and write scope files and
// scopes.yaml for one repository. The writes are atomic, but two concurrent
// read-modify-write runs (parallel `scope set` calls in a CI job) would each save
// the file they read and drop the other's change. The lock file lives in the
// temporary directory, keyed by the .sc directory, so nothing appears in the
// repository. Call the returned function to release it.
func LockStore(scDir string) (func(), error) {
	abs, err := filepath.Abs(scDir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to resolve %s", scDir)
	}
	sum := sha256.Sum256([]byte(abs))
	path := filepath.Join(os.TempDir(), "sc-scopes-"+hex.EncodeToString(sum[:8])+".lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open the scope store lock %s", path)
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		return nil, errors.Wrapf(err, "failed to lock the scope store (%s)", path)
	}
	return func() {
		_ = unlockFile(f)
		_ = f.Close()
	}, nil
}
