// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package actions

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/simple-container-com/api/pkg/api"
)

// removeStaleParentSecrets deletes the revealed secrets file of every parent
// stack from the workspace before the parent's stacks are copied in. The copy
// merges into what is there, so a store revealed by an earlier step of the same
// job would otherwise outlive a run that could not reveal it, and be resolved
// as that run's own. What remains afterwards is exactly what this run revealed.
func removeStaleParentSecrets(parentStacksDir, workspaceStacksDir string) ([]string, error) {
	entries, err := os.ReadDir(parentStacksDir)
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		stale := filepath.Join(workspaceStacksDir, entry.Name(), api.SecretsDescriptorFileName)
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
