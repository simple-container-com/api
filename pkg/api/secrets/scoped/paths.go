// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
)

// stacksDirName is the conventional per-stack subdirectory under the .sc config
// root. Matches the whole-file store's layout (.sc/stacks/<stack>/secrets.yaml).
const stacksDirName = "stacks"

// ScopesPath returns the governance file path: <scDir>/scopes.yaml.
func ScopesPath(scDir string) string {
	return filepath.Join(scDir, ScopesFileName)
}

// StackDir returns <scDir>/stacks/<stack>.
func StackDir(scDir, stack string) string {
	return filepath.Join(scDir, stacksDirName, stack)
}

// ScopeFilePath returns <scDir>/stacks/<stack>/secrets.<scope>.yaml.
func ScopeFilePath(scDir, stack, scope string) string {
	return filepath.Join(StackDir(scDir, stack), ScopeFileName(scope))
}

// ListScopeFiles returns every committed scope file under <scDir>/stacks/*/,
// sorted. It deliberately does NOT match the legacy plaintext secrets.yaml
// (that has no scope segment). Used by `lint` and by allow/disallow resealing.
func ListScopeFiles(scDir string) ([]string, error) {
	stacksRoot := filepath.Join(scDir, stacksDirName)
	entries, err := os.ReadDir(stacksRoot)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list %s", stacksRoot)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		stackDir := filepath.Join(stacksRoot, e.Name())
		files, err := os.ReadDir(stackDir)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list %s", stackDir)
		}
		for _, f := range files {
			name := f.Name()
			// secrets.<scope>.yaml but NOT the legacy secrets.yaml
			if !strings.HasPrefix(name, scopeFilePrefix) || !strings.HasSuffix(name, scopeFileSuffix) {
				continue
			}
			if ScopeNameFromFile(name) == "" {
				continue
			}
			out = append(out, filepath.Join(stackDir, name))
		}
	}
	sort.Strings(out)
	return out, nil
}
