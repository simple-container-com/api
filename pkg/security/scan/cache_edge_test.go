// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func TestCleanupTrivyCacheDir_EmptyIsNoOp(t *testing.T) {
	RegisterTestingT(t)
	// Empty path returns immediately without touching the filesystem.
	Expect(func() { cleanupTrivyCacheDir("") }).ToNot(Panic())
}

func TestCleanupTrivyCacheDir_RemovesTree(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	sub := filepath.Join(dir, "scan-xyz")
	Expect(os.MkdirAll(sub, 0o755)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(sub, "f"), []byte("x"), 0o644)).To(Succeed())

	cleanupTrivyCacheDir(sub)
	_, err := os.Stat(sub)
	Expect(os.IsNotExist(err)).To(BeTrue())
}

func TestEnsureTrivyCacheDir_FallsBackToTempWhenNoCacheHome(t *testing.T) {
	RegisterTestingT(t)

	// With both XDG_CACHE_HOME and HOME unset, os.UserCacheDir errors on linux and
	// the code falls back to os.TempDir(). The returned dir must still be a
	// scan-* subdir under a "trivy" parent and must be a real directory.
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")

	cacheDir, err := ensureTrivyCacheDir()
	Expect(err).ToNot(HaveOccurred())
	defer cleanupTrivyCacheDir(cacheDir)

	Expect(cacheDir).To(BeADirectory())
	Expect(filepath.Base(filepath.Dir(cacheDir))).To(Equal("trivy"))
	Expect(filepath.Base(cacheDir)).To(HavePrefix("scan-"))
}

func TestEnsureTrivyCacheDir_ParentIsFile(t *testing.T) {
	RegisterTestingT(t)

	// Make <cacheRoot>/trivy a regular FILE so MkdirAll of the parent fails,
	// exercising the "create trivy cache parent directory" error branch.
	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	t.Setenv("HOME", t.TempDir())

	clash := filepath.Join(cacheRoot, "trivy")
	Expect(os.WriteFile(clash, []byte("not a dir"), 0o644)).To(Succeed())

	_, err := ensureTrivyCacheDir()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("trivy cache parent directory"))
}

func TestHasGrypeVulnerabilityDB_NoCacheHome(t *testing.T) {
	RegisterTestingT(t)

	// Unset both so UserCacheDir errors -> function returns false (error branch).
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")
	Expect(hasGrypeVulnerabilityDB()).To(BeFalse())
}

func TestTrivyCVSS_UnmarshalJSON_ArrayWithBadElement(t *testing.T) {
	RegisterTestingT(t)

	// Leading '[' takes the array branch; a non-object element makes the inner
	// json.Unmarshal fail, exercising the array error path.
	var cvss trivyCVSS
	err := json.Unmarshal([]byte(`["not-an-object"]`), &cvss)
	Expect(err).To(HaveOccurred())
}

func TestTrivyCVSS_UnmarshalJSON_ObjectWithBadElement(t *testing.T) {
	RegisterTestingT(t)

	// Leading '{' takes the object branch; a non-object value fails to unmarshal.
	var cvss trivyCVSS
	err := json.Unmarshal([]byte(`{"nvd": "not-an-object"}`), &cvss)
	Expect(err).To(HaveOccurred())
}

func TestTrivyCVSS_UnmarshalJSON_EmptyData(t *testing.T) {
	RegisterTestingT(t)

	// Empty (whitespace-only) payload is treated as null -> no error, zero score.
	var cvss trivyCVSS
	Expect(cvss.UnmarshalJSON([]byte("   "))).To(Succeed())
	Expect(extractTrivyCVSS(cvss)).To(Equal(0.0))
}
