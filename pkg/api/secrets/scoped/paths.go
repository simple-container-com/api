// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// ScopesPath returns the governance file path: <scDir>/scopes.yaml.
func ScopesPath(scDir string) string {
	return filepath.Join(scDir, ScopesFileName)
}

// writeFileAtomic writes data to a UNIQUELY-named sibling temp file then renames it
// over path, so a crash mid-write can never leave a truncated/corrupt scope or scopes
// file (which would then hard-fail deploys). Rename is atomic on the same filesystem.
// A random temp suffix (os.CreateTemp) means two concurrent writers to the same file
// never collide on the temp path.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errors.Wrapf(err, "failed to create directory for %s", path)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return errors.Wrapf(err, "failed to create temp file for %s", path)
	}
	tmpName := tmp.Name()
	// Clean up the temp file on any early return; the successful path renames it away
	// first, so the Remove then harmlessly no-ops.
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return errors.Wrapf(err, "failed to write %s", tmpName)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return errors.Wrapf(err, "failed to chmod %s", tmpName)
	}
	if err := tmp.Close(); err != nil {
		return errors.Wrapf(err, "failed to close %s", tmpName)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return errors.Wrapf(err, "failed to rename %s -> %s", tmpName, path)
	}
	return nil
}

// StackDir returns <stacksDir>/<stack>. stacksDir is the resolved stacks root
// the provisioner reads from (see provisioner.ResolveStacksDir), not the .sc dir.
func StackDir(stacksDir, stack string) string {
	return filepath.Join(stacksDir, stack)
}

// ScopeFilePath returns <stacksDir>/<stack>/secrets.<scope>.yaml.
func ScopeFilePath(stacksDir, stack, scope string) string {
	return filepath.Join(StackDir(stacksDir, stack), ScopeFileName(scope))
}

// IsScopeFile reports whether path is a file the scoped store wrote: its name is
// secrets.<scope>.yaml AND its top-level YAML carries a stack, scope or recipients
// key. Save always writes all three, while the legacy format (and so a plaintext
// secrets.example.yaml or a secrets.backup.yaml copy) has none of them; both formats
// carry schemaVersion, so it is no marker. Unparseable YAML is not ours either. A
// file that passes this check and then fails LoadScopeFile is corrupt or tampered,
// and stays a hard error there.
func IsScopeFile(path string) (bool, error) {
	if ScopeNameFromFile(path) == "" {
		return false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, errors.Wrapf(err, "failed to read %s", path)
	}
	var top map[string]yaml.Node
	if yaml.Unmarshal(data, &top) != nil {
		return false, nil
	}
	for _, k := range []string{"stack", "scope", "recipients"} {
		if _, ok := top[k]; ok {
			return true, nil
		}
	}
	return false, nil
}

// ScopeFilesIn splits the secrets.<x>.yaml files directly in stackDir into real
// scope files and look-alikes (see IsScopeFile), both sorted. A missing stackDir
// yields nothing.
func ScopeFilesIn(stackDir string) (scopeFiles, lookalikes []string, err error) {
	entries, err := os.ReadDir(stackDir)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to list %s", stackDir)
	}
	for _, e := range entries {
		if e.IsDir() || ScopeNameFromFile(e.Name()) == "" {
			continue
		}
		p := filepath.Join(stackDir, e.Name())
		ok, err := IsScopeFile(p)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			scopeFiles = append(scopeFiles, p)
		} else {
			lookalikes = append(lookalikes, p)
		}
	}
	sort.Strings(scopeFiles)
	sort.Strings(lookalikes)
	return scopeFiles, lookalikes, nil
}

// ListScopeFiles returns every committed scope file under <stacksRoot>/*/,
// sorted. It deliberately does NOT match the legacy plaintext secrets.yaml
// (that has no scope segment) nor look-alikes. Used by allow/disallow resealing.
func ListScopeFiles(stacksRoot string) ([]string, error) {
	files, _, err := ListScopeFilesAndLookalikes(stacksRoot)
	return files, err
}

// ListScopeFilesAndLookalikes is ListScopeFiles plus the secrets.<x>.yaml files
// that are not scope files, so lint can name them.
func ListScopeFilesAndLookalikes(stacksRoot string) (scopeFiles, lookalikes []string, err error) {
	entries, err := os.ReadDir(stacksRoot)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to list %s", stacksRoot)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sf, la, err := ScopeFilesIn(filepath.Join(stacksRoot, e.Name()))
		if err != nil {
			return nil, nil, err
		}
		scopeFiles = append(scopeFiles, sf...)
		lookalikes = append(lookalikes, la...)
	}
	sort.Strings(scopeFiles)
	sort.Strings(lookalikes)
	return scopeFiles, lookalikes, nil
}
