// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package actions

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/secrets/scoped"
)

// removeStaleParentSecrets deletes the revealed secrets file of every parent stack
// that has secret scopes from the workspace, before the parent's stacks are copied
// in. The copy merges into what is there, so a store revealed by an earlier step of
// the same job would otherwise outlive a run that could not reveal it and, being
// the whole-file store, win over the scopes this run opens. A stack without scope
// files is left alone: there the revealed file is the only store, and an earlier
// step may have revealed it on purpose.
func removeStaleParentSecrets(parentStacksDir, workspaceStacksDir string) ([]string, error) {
	entries, err := os.ReadDir(parentStacksDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil // the copy reports a parent without stacks
	}
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		hasScopes, err := hasScopeFiles(filepath.Join(parentStacksDir, entry.Name()))
		if err != nil {
			return removed, err
		}
		if !hasScopes {
			continue
		}
		stackDir := filepath.Join(workspaceStacksDir, entry.Name())
		if st, err := os.Lstat(stackDir); err != nil || !st.IsDir() {
			continue // absent, or a symlink whose target is not ours to touch
		}
		stale := filepath.Join(stackDir, api.SecretsDescriptorFileName)
		if err := os.Remove(stale); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return removed, err
		}
		removed = append(removed, stale)
	}
	return removed, nil
}

func hasScopeFiles(dir string) (bool, error) {
	if _, err := os.ReadDir(dir); err != nil {
		return false, err
	}
	files, _, err := scoped.ScopeFilesIn(dir)
	return len(files) > 0, err
}
