// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package security

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

// ComputeConfigHash returns an error when the value cannot be JSON-marshaled.
func TestComputeConfigHashMarshalError(t *testing.T) {
	RegisterTestingT(t)

	t.Run("marshalable struct succeeds", func(t *testing.T) {
		RegisterTestingT(t)
		h, err := ComputeConfigHash(struct{ A string }{A: "x"})
		Expect(err).ToNot(HaveOccurred())
		Expect(h).ToNot(BeEmpty())
	})

	t.Run("channel cannot be marshaled", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ComputeConfigHash(make(chan int))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("marshaling config"))
	})

	t.Run("func cannot be marshaled", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ComputeConfigHash(func() {})
		Expect(err).To(HaveOccurred())
	})
}

// SetWithTTL fails when the per-operation subdirectory cannot be created
// because a regular file already occupies that name.
func TestSetWithTTLMkdirError(t *testing.T) {
	RegisterTestingT(t)

	base := t.TempDir()
	cache, err := NewCache(base)
	Expect(err).ToNot(HaveOccurred())

	// getPath derives the subdir from Operation ("scan-grype"). Plant a file
	// there so MkdirAll(base/scan-grype) fails with ENOTDIR.
	blocker := filepath.Join(base, "scan-grype")
	Expect(os.WriteFile(blocker, []byte("x"), 0o600)).To(Succeed())

	key := CacheKey{Operation: "scan-grype", ImageDigest: "sha256:1", ConfigHash: "h"}
	err = cache.Set(key, []byte("data"))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("creating cache directory"))
}

// Get surfaces a non-NotExist read error: when the cache path is unexpectedly
// a directory, os.ReadFile returns EISDIR rather than ErrNotExist.
func TestGetReadErrorWhenPathIsDirectory(t *testing.T) {
	RegisterTestingT(t)

	base := t.TempDir()
	cache, err := NewCache(base)
	Expect(err).ToNot(HaveOccurred())

	key := CacheKey{Operation: "sbom", ImageDigest: "sha256:1", ConfigHash: "h"}
	// Create a directory exactly where the cache file would live.
	path := cache.getPath(key)
	Expect(os.MkdirAll(path, 0o700)).To(Succeed())

	_, found, err := cache.Get(key)
	Expect(err).To(HaveOccurred())
	Expect(found).To(BeFalse())
	Expect(err.Error()).To(ContainSubstring("reading cache file"))
}

// Invalidate surfaces a non-NotExist remove error. Removing a non-empty
// directory through os.Remove yields ENOTEMPTY (not ErrNotExist).
func TestInvalidateRemoveError(t *testing.T) {
	RegisterTestingT(t)

	base := t.TempDir()
	cache, err := NewCache(base)
	Expect(err).ToNot(HaveOccurred())

	key := CacheKey{Operation: "sbom", ImageDigest: "sha256:2", ConfigHash: "h"}
	path := cache.getPath(key)
	// Make the would-be cache file a non-empty directory so os.Remove fails.
	Expect(os.MkdirAll(filepath.Join(path, "child"), 0o700)).To(Succeed())

	err = cache.Invalidate(key)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("removing cache file"))
}

// Invalidate is a no-op (nil error) for a key that was never stored.
func TestInvalidateMissingKeyIsNoOp(t *testing.T) {
	RegisterTestingT(t)

	cache, err := NewCache(t.TempDir())
	Expect(err).ToNot(HaveOccurred())
	Expect(cache.Invalidate(CacheKey{Operation: "sbom", ImageDigest: "x", ConfigHash: "y"})).To(Succeed())
}

// NewCache fails when the base directory cannot be created because a path
// component is a regular file.
func TestNewCacheMkdirError(t *testing.T) {
	RegisterTestingT(t)

	parent := t.TempDir()
	blocker := filepath.Join(parent, "afile")
	Expect(os.WriteFile(blocker, []byte("x"), 0o600)).To(Succeed())

	// baseDir lives "under" a regular file => MkdirAll fails with ENOTDIR.
	_, err := NewCache(filepath.Join(blocker, "cache"))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("creating cache directory"))
}

// SetWithTTL with a non-positive explicit TTL falls back to the per-operation
// default TTL, and the entry is retrievable.
func TestSetWithTTLNonPositiveFallsBackToDefault(t *testing.T) {
	RegisterTestingT(t)

	cache, err := NewCache(t.TempDir())
	Expect(err).ToNot(HaveOccurred())

	key := CacheKey{Operation: "sbom", ImageDigest: "sha256:9", ConfigHash: "h"}
	Expect(cache.SetWithTTL(key, []byte("payload"), 0)).To(Succeed())

	got, found, err := cache.Get(key)
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(string(got)).To(Equal("payload"))
}
