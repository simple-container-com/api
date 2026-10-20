// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"net/url"
	"os"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/fs"
)

// stateDirMode is deliberately 0700 and not 0755. The checkpoint holds every
// `${secret:...}` of the stack, encrypted with the passphrase provider's key — a
// directory other local users can list is a needless disclosure of which stacks
// exist and how often they change.
const stateDirMode = 0o700

// initFileSystemStateStore creates the state directory before Pulumi opens it.
//
// Every other state storage SC supports provisions its own backing store (the AWS
// one creates the bucket, the GCP one the bucket, Yandex the object-storage
// bucket), and `fs` is the only one that had no init hook at all — so a `file://`
// path that did not already exist failed at `unable to open bucket …: no such file
// or directory`, after the stack had been created, with nothing in the message
// saying SC would not create it.
//
// That is the whole of what this does: a `mkdir -p`. It makes the documented
// byo-host example runnable from a clean checkout, which it was not before
// 2026-10-18 — the `fs` path's only other exercise is an `//go:build e2e` test
// pointed at a home directory that already exists.
func initFileSystemStateStore(ctx context.Context, stateStoreCfg api.StateStorageConfig, log logger.Logger) error {
	u, err := url.Parse(stateStoreCfg.StorageUrl())
	if err != nil {
		return errors.Wrapf(err, "failed to parse file system state storage url")
	}
	// Only `file://` is ours to create. `fs` should never produce anything else, but
	// an operator who wrote a cloud URL under `type: fs` gets Pulumi's own error
	// rather than a mkdir of something meaningless.
	if u.Scheme != "" && u.Scheme != "file" {
		log.Warn(ctx, "state storage %q is configured as %q but its url has scheme %q, skipping directory creation",
			fs.StateStorageTypeFileSystem, fs.StateStorageTypeFileSystem, u.Scheme)
		return nil
	}
	dir := u.Path
	if dir == "" {
		dir = u.Opaque
	}
	if dir == "" {
		return errors.Errorf("file system state storage has an empty path, set `path` in the provisioner config")
	}
	if err := os.MkdirAll(dir, stateDirMode); err != nil {
		return errors.Wrapf(err, "failed to create state storage directory %q", dir)
	}
	log.Debug(ctx, "state storage directory %q is ready", dir)
	return nil
}
