// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package security

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestNewCache(t *testing.T) {
	RegisterTestingT(t)

	// Test with custom directory
	customDir := filepath.Join(t.TempDir(), "custom-cache")
	cache, err := NewCache(customDir)
	Expect(err).ToNot(HaveOccurred())
	Expect(cache.baseDir).To(Equal(customDir))

	// Verify directory was created
	_, err = os.Stat(customDir)
	Expect(os.IsNotExist(err)).To(BeFalse(), "Cache directory was not created")

	// Test with empty directory (should use default).
	// Override HOME to avoid writing to real home directory.
	t.Setenv("HOME", t.TempDir())
	cache2, err := NewCache("")
	Expect(err).ToNot(HaveOccurred())
	Expect(cache2.baseDir).ToNot(BeEmpty())
}

func TestCacheSetAndGet(t *testing.T) {
	RegisterTestingT(t)

	cache, err := NewCache(t.TempDir())
	Expect(err).ToNot(HaveOccurred())

	key := CacheKey{
		Operation:   "sbom",
		ImageDigest: "sha256:abc123",
		ConfigHash:  "hash456",
	}

	data := []byte("test data")

	// Test Set
	err = cache.Set(key, data)
	Expect(err).ToNot(HaveOccurred())

	// Test Get
	retrieved, found, err := cache.Get(key)
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeTrue(), "Expected cache hit, got miss")
	Expect(string(retrieved)).To(Equal(string(data)))

	// Test Get with non-existent key
	nonExistentKey := CacheKey{
		Operation:   "nonexistent",
		ImageDigest: "sha256:xyz789",
		ConfigHash:  "hash999",
	}
	_, found, err = cache.Get(nonExistentKey)
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeFalse(), "Expected cache miss, got hit")
}

func TestCacheTTLExpiration(t *testing.T) {
	RegisterTestingT(t)

	cache, err := NewCache(t.TempDir())
	Expect(err).ToNot(HaveOccurred())

	key := CacheKey{
		Operation:   "sbom",
		ImageDigest: "sha256:abc123",
		ConfigHash:  "hash456",
	}

	data := []byte("test data")

	// Set data
	err = cache.Set(key, data)
	Expect(err).ToNot(HaveOccurred())

	// Verify it's there
	_, found, err := cache.Get(key)
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeTrue(), "Expected cache hit immediately after set")

	// Manually modify the cache entry to be expired
	path := cache.getPath(key)
	entry := CacheEntry{
		Key:       key,
		Data:      data,
		CreatedAt: time.Now().Add(-25 * time.Hour), // 25 hours ago
		ExpiresAt: time.Now().Add(-1 * time.Hour),  // Expired 1 hour ago
	}

	// Write expired entry
	entryData, _ := marshalJSON(entry)
	err = os.WriteFile(path, entryData, 0o600)
	Expect(err).ToNot(HaveOccurred())

	// Try to get expired entry
	_, found, err = cache.Get(key)
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeFalse(), "Expected cache miss for expired entry, got hit")

	// Verify file was deleted
	_, err = os.Stat(path)
	Expect(os.IsNotExist(err)).To(BeTrue(), "Expected expired cache file to be deleted")
}

func marshalJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func TestCacheInvalidate(t *testing.T) {
	RegisterTestingT(t)

	cache, err := NewCache(t.TempDir())
	Expect(err).ToNot(HaveOccurred())

	key := CacheKey{
		Operation:   "sbom",
		ImageDigest: "sha256:abc123",
		ConfigHash:  "hash456",
	}

	data := []byte("test data")

	// Set data
	err = cache.Set(key, data)
	Expect(err).ToNot(HaveOccurred())

	// Invalidate
	err = cache.Invalidate(key)
	Expect(err).ToNot(HaveOccurred())

	// Verify it's gone
	_, found, err := cache.Get(key)
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeFalse(), "Expected cache miss after invalidation, got hit")

	// Invalidate non-existent key should not error
	err = cache.Invalidate(key)
	Expect(err).ToNot(HaveOccurred())
}

func TestCacheClean(t *testing.T) {
	RegisterTestingT(t)

	cache, err := NewCache(t.TempDir())
	Expect(err).ToNot(HaveOccurred())

	// Create multiple cache entries
	for i := 0; i < 5; i++ {
		key := CacheKey{
			Operation:   "sbom",
			ImageDigest: "sha256:" + string(rune('a'+i)),
			ConfigHash:  "hash",
		}
		data := []byte("test data")
		err = cache.Set(key, data)
		Expect(err).ToNot(HaveOccurred())
	}

	// Clean should not remove valid entries
	err = cache.Clean()
	Expect(err).ToNot(HaveOccurred())

	// Verify entries still exist
	key := CacheKey{
		Operation:   "sbom",
		ImageDigest: "sha256:a",
		ConfigHash:  "hash",
	}
	_, found, err := cache.Get(key)
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeTrue(), "Valid entry should not be cleaned")
}

func TestCacheSize(t *testing.T) {
	RegisterTestingT(t)

	cache, err := NewCache(t.TempDir())
	Expect(err).ToNot(HaveOccurred())

	initialSize, err := cache.Size()
	Expect(err).ToNot(HaveOccurred())

	// Add some data
	key := CacheKey{
		Operation:   "sbom",
		ImageDigest: "sha256:abc123",
		ConfigHash:  "hash456",
	}
	data := []byte("test data with some content")
	err = cache.Set(key, data)
	Expect(err).ToNot(HaveOccurred())

	newSize, err := cache.Size()
	Expect(err).ToNot(HaveOccurred())
	Expect(newSize).To(BeNumerically(">", initialSize))
}

func TestCacheClear(t *testing.T) {
	RegisterTestingT(t)

	cache, err := NewCache(t.TempDir())
	Expect(err).ToNot(HaveOccurred())

	// Add some data
	key := CacheKey{
		Operation:   "sbom",
		ImageDigest: "sha256:abc123",
		ConfigHash:  "hash456",
	}
	data := []byte("test data")
	err = cache.Set(key, data)
	Expect(err).ToNot(HaveOccurred())

	// Clear
	err = cache.Clear()
	Expect(err).ToNot(HaveOccurred())

	// Verify it's gone
	_, found, err := cache.Get(key)
	if err == nil {
		Expect(found).To(BeFalse(), "Expected cache miss after clear, got hit")
	}

	// Verify directory is gone
	_, err = os.Stat(cache.baseDir)
	Expect(os.IsNotExist(err)).To(BeTrue(), "Expected cache directory to be deleted after clear")
}

func TestComputeConfigHash(t *testing.T) {
	RegisterTestingT(t)

	config1 := map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
	}

	hash1, err := ComputeConfigHash(config1)
	Expect(err).ToNot(HaveOccurred())
	Expect(hash1).ToNot(BeEmpty())

	// Same config should produce same hash
	hash2, err := ComputeConfigHash(config1)
	Expect(err).ToNot(HaveOccurred())
	Expect(hash1).To(Equal(hash2))

	// Different config should produce different hash
	config2 := map[string]interface{}{
		"key1": "different",
		"key2": "value2",
	}
	hash3, err := ComputeConfigHash(config2)
	Expect(err).ToNot(HaveOccurred())
	Expect(hash1).ToNot(Equal(hash3))
}

func TestCacheGetTTL(t *testing.T) {
	RegisterTestingT(t)

	cache, err := NewCache(t.TempDir())
	Expect(err).ToNot(HaveOccurred())

	tests := []struct {
		operation string
		expected  time.Duration
	}{
		{"sbom", TTL_SBOM},
		{"scan-grype", TTL_SCAN_GRYPE},
		{"scan-trivy", TTL_SCAN_TRIVY},
		{"unknown", 6 * time.Hour}, // Default TTL
	}

	for _, tt := range tests {
		t.Run(tt.operation, func(t *testing.T) {
			RegisterTestingT(t)
			ttl := cache.getTTL(tt.operation)
			Expect(ttl).To(Equal(tt.expected))
		})
	}
}

func TestCacheKeyPath(t *testing.T) {
	RegisterTestingT(t)

	cache, err := NewCache(t.TempDir())
	Expect(err).ToNot(HaveOccurred())

	key := CacheKey{
		Operation:   "sbom",
		ImageDigest: "sha256:abc123",
		ConfigHash:  "hash456",
	}

	path := cache.getPath(key)

	// Verify path structure
	Expect(filepath.IsAbs(path)).To(BeTrue(), "Expected absolute path, got %s", path)

	// Verify operation directory is in path
	Expect(containsSubstring(path, key.Operation)).To(BeTrue(), "Expected operation %s in path %s", key.Operation, path)

	// Two keys with same data should produce same path
	path2 := cache.getPath(key)
	Expect(path).To(Equal(path2))

	// Different keys should produce different paths
	key2 := CacheKey{
		Operation:   "scan-grype",
		ImageDigest: "sha256:xyz789",
		ConfigHash:  "hash999",
	}
	path3 := cache.getPath(key2)
	Expect(path).ToNot(Equal(path3))
}

func containsSubstring(s, substr string) bool {
	return filepath.Base(filepath.Dir(s)) == substr || filepath.Base(s) == substr
}

// ---------------------------------------------------------------------
// HMAC integrity tests (Phase 5 — replaces the prior mtime tamper check).
// ---------------------------------------------------------------------

func TestCacheHMACKeyPersisted(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()

	cache1, err := NewCache(dir)
	Expect(err).ToNot(HaveOccurred())
	Expect(cache1.key).To(HaveLen(hmacKeyLen))

	keyPath := filepath.Join(dir, hmacKeyFilename)
	stat, err := os.Stat(keyPath)
	Expect(err).ToNot(HaveOccurred())
	Expect(stat.Size()).To(Equal(int64(hmacKeyLen)))
	Expect(stat.Mode().Perm()).To(Equal(os.FileMode(0o600)))

	// Re-open the same dir: must reuse the existing key, not regenerate.
	cache2, err := NewCache(dir)
	Expect(err).ToNot(HaveOccurred())
	Expect(cache2.key).To(Equal(cache1.key))
}

func TestCacheHMACKeyWrongSizeRejected(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	// Pre-seed a truncated key file.
	Expect(os.WriteFile(filepath.Join(dir, hmacKeyFilename), []byte("short"), 0o600)).To(Succeed())

	_, err := NewCache(dir)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("hmac key"))
}

func TestCacheTamperDetection(t *testing.T) {
	RegisterTestingT(t)

	cache, err := NewCache(t.TempDir())
	Expect(err).ToNot(HaveOccurred())

	key := CacheKey{Operation: "sbom", ImageDigest: "sha256:tamper", ConfigHash: "h"}
	Expect(cache.Set(key, []byte("original"))).To(Succeed())

	path := cache.getPath(key)
	raw, err := os.ReadFile(path)
	Expect(err).ToNot(HaveOccurred())

	var sig signedEntry
	Expect(json.Unmarshal(raw, &sig)).To(Succeed())

	// Tamper the entry data; leave the MAC alone — HMAC must catch it.
	sig.Entry.Data = []byte("malicious")
	tampered, err := json.Marshal(sig)
	Expect(err).ToNot(HaveOccurred())
	Expect(os.WriteFile(path, tampered, 0o600)).To(Succeed())

	got, found, err := cache.Get(key)
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeFalse(), "tampered entry must be treated as cache miss")
	Expect(got).To(BeNil())

	// The corrupt file must also be removed.
	_, err = os.Stat(path)
	Expect(os.IsNotExist(err)).To(BeTrue())
}

func TestCacheMtimeForgeryNoLongerHelpful(t *testing.T) {
	RegisterTestingT(t)

	cache, err := NewCache(t.TempDir())
	Expect(err).ToNot(HaveOccurred())

	key := CacheKey{Operation: "sbom", ImageDigest: "sha256:mtime", ConfigHash: "h"}
	Expect(cache.Set(key, []byte("payload"))).To(Succeed())

	// Advance mtime far into the future — the old code would have
	// invalidated the entry here. The HMAC-based check ignores mtime,
	// so the entry must remain valid.
	path := cache.getPath(key)
	future := time.Now().Add(48 * time.Hour)
	Expect(os.Chtimes(path, future, future)).To(Succeed())

	got, found, err := cache.Get(key)
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeTrue(), "mtime is no longer load-bearing for integrity")
	Expect(string(got)).To(Equal("payload"))
}

func TestCacheKeyMismatchRejectsEntry(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()

	// Create the cache, write an entry, then swap the HMAC key as if a
	// different process (or the user's clearing it) had rotated the key.
	c1, err := NewCache(dir)
	Expect(err).ToNot(HaveOccurred())
	key := CacheKey{Operation: "sbom", ImageDigest: "sha256:k", ConfigHash: "h"}
	Expect(c1.Set(key, []byte("data"))).To(Succeed())

	// Replace the key file with random bytes of the right length.
	newKey := make([]byte, hmacKeyLen)
	newKey[0] = 0xff
	Expect(os.WriteFile(filepath.Join(dir, hmacKeyFilename), newKey, 0o600)).To(Succeed())

	c2, err := NewCache(dir)
	Expect(err).ToNot(HaveOccurred())

	got, found, err := c2.Get(key)
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeFalse(), "entry signed under the previous key must fail verification")
	Expect(got).To(BeNil())
}

func TestCacheLegacyUnsignedEntryDiscarded(t *testing.T) {
	RegisterTestingT(t)

	cache, err := NewCache(t.TempDir())
	Expect(err).ToNot(HaveOccurred())

	key := CacheKey{Operation: "sbom", ImageDigest: "sha256:legacy", ConfigHash: "h"}
	path := cache.getPath(key)
	Expect(os.MkdirAll(filepath.Dir(path), 0o700)).To(Succeed())

	// Simulate a pre-HMAC entry: just the unwrapped CacheEntry shape.
	legacy := CacheEntry{
		Key:       key,
		Data:      []byte("pre-hmac"),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	legacyBytes, err := json.Marshal(legacy)
	Expect(err).ToNot(HaveOccurred())
	Expect(os.WriteFile(path, legacyBytes, 0o600)).To(Succeed())

	got, found, err := cache.Get(key)
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeFalse(), "legacy unsigned entries must be discarded on upgrade")
	Expect(got).To(BeNil())

	_, err = os.Stat(path)
	Expect(os.IsNotExist(err)).To(BeTrue())
}

// Cross-key copy: a signed file from key A copied to key B's path
// would have a valid MAC but should NOT satisfy `Get(keyB)`. The
// embedded CacheKey must match the requested key.
func TestCacheCrossKeyCopyRejected(t *testing.T) {
	RegisterTestingT(t)

	cache, err := NewCache(t.TempDir())
	Expect(err).ToNot(HaveOccurred())

	keyA := CacheKey{Operation: "sbom", ImageDigest: "sha256:A", ConfigHash: "h"}
	keyB := CacheKey{Operation: "sbom", ImageDigest: "sha256:B", ConfigHash: "h"}

	Expect(cache.Set(keyA, []byte("payload-A"))).To(Succeed())

	// Copy the validly-signed file from A's path to B's path.
	src, dst := cache.getPath(keyA), cache.getPath(keyB)
	Expect(os.MkdirAll(filepath.Dir(dst), 0o700)).To(Succeed())
	bytes, err := os.ReadFile(src)
	Expect(err).ToNot(HaveOccurred())
	Expect(os.WriteFile(dst, bytes, 0o600)).To(Succeed())

	got, found, err := cache.Get(keyB)
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeFalse(), "MAC alone must not bind a signed file to an arbitrary cache path")
	Expect(got).To(BeNil())

	// The misplaced copy must be removed.
	_, err = os.Stat(dst)
	Expect(os.IsNotExist(err)).To(BeTrue())

	// keyA's own entry must still be intact.
	got, found, err = cache.Get(keyA)
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(string(got)).To(Equal("payload-A"))
}

// Concurrent NewCache on the same fresh directory must converge on a
// single HMAC key. With the previous Rename-based publish, two racing
// processes could each install their own distinct key, causing each
// to invalidate the other's cache entries (codex round-2 P2). The Link
// based publish forces a winner and re-reads the on-disk truth.
func TestCacheHMACKeyConcurrentInitConverges(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	const n = 8

	keys := make(chan []byte, n)
	errs := make(chan error, n)
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		go func() {
			<-start
			c, err := NewCache(dir)
			if err != nil {
				errs <- err
				return
			}
			cp := make([]byte, len(c.key))
			copy(cp, c.key)
			keys <- cp
		}()
	}
	close(start) // unleash all goroutines simultaneously

	var first []byte
	for i := 0; i < n; i++ {
		select {
		case err := <-errs:
			t.Fatalf("NewCache failed: %v", err)
		case k := <-keys:
			if first == nil {
				first = k
				continue
			}
			Expect(k).To(Equal(first),
				"goroutine %d got a different HMAC key — Link-based publish is broken", i)
		}
	}
}

// Clean must not delete an in-flight `Set()` temp file — gemini-flagged
// race where Clean reads a partial file, fails to unmarshal, and removes
// it from under a still-running writer.
func TestCacheCleanSkipsInFlightTempFiles(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	cache, err := NewCache(dir)
	Expect(err).ToNot(HaveOccurred())

	// Simulate an in-flight Set() by dropping a `.cache.tmp.*` file in
	// the operation subdir.
	opDir := filepath.Join(dir, "sbom")
	Expect(os.MkdirAll(opDir, 0o700)).To(Succeed())
	cacheTmp := filepath.Join(opDir, ".cache.tmp.12345")
	Expect(os.WriteFile(cacheTmp, []byte("partial bytes — not valid JSON yet"), 0o600)).To(Succeed())

	Expect(cache.Clean()).To(Succeed())
	_, err = os.Stat(cacheTmp)
	Expect(err).ToNot(HaveOccurred(), "Clean must leave .cache.tmp.* files alone")

	// Same guard for .hmac.key.tmp.* at the baseDir level.
	hmacTmp := filepath.Join(dir, ".hmac.key.tmp.67890")
	Expect(os.WriteFile(hmacTmp, []byte("partial key bytes"), 0o600)).To(Succeed())
	Expect(cache.Clean()).To(Succeed())
	_, err = os.Stat(hmacTmp)
	Expect(err).ToNot(HaveOccurred(), "Clean must leave .hmac.key.tmp.* files alone")
}

// getPath must contain malicious Operation values that try to escape
// baseDir (gemini round-3 P3 — defense in depth; no caller is hostile
// today, but a future careless callsite shouldn't be able to corrupt
// the parent filesystem).
func TestCachePathTraversalContained(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	cache, err := NewCache(dir)
	Expect(err).ToNot(HaveOccurred())

	hostile := []string{
		"../../../etc",
		"..",
		"/etc/passwd",
		`..\..\windows`,
		"a/b",
	}
	for _, op := range hostile {
		key := CacheKey{Operation: op, ImageDigest: "sha256:x", ConfigHash: "h"}
		p := cache.getPath(key)
		Expect(strings.HasPrefix(p, dir+string(filepath.Separator))).To(BeTrue(),
			"getPath(%q) = %q must stay under baseDir %q", op, p, dir)
		Expect(strings.Contains(p, "..")).To(BeFalse(),
			"getPath(%q) = %q must not contain `..`", op, p)
	}
}

// Crashed writers leave behind temp files Clean() would otherwise
// skip forever. After tempFileGracePeriod (1h), Clean reclaims them
// (codex round-3 P3). Test forces the grace by back-dating mtime.
func TestCacheCleanReclaimsStaleTemps(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	cache, err := NewCache(dir)
	Expect(err).ToNot(HaveOccurred())

	opDir := filepath.Join(dir, "sbom")
	Expect(os.MkdirAll(opDir, 0o700)).To(Succeed())

	// Recent temp: must stay. Old temp: must be reclaimed.
	recent := filepath.Join(opDir, ".cache.tmp.recent")
	stale := filepath.Join(opDir, ".cache.tmp.stale")
	Expect(os.WriteFile(recent, []byte("recent"), 0o600)).To(Succeed())
	Expect(os.WriteFile(stale, []byte("stale-from-a-crashed-writer"), 0o600)).To(Succeed())

	// Back-date stale beyond the grace period.
	pastTime := time.Now().Add(-2 * tempFileGracePeriod)
	Expect(os.Chtimes(stale, pastTime, pastTime)).To(Succeed())

	Expect(cache.Clean()).To(Succeed())

	_, err = os.Stat(recent)
	Expect(err).ToNot(HaveOccurred(), "recent temp file must be left alone")
	_, err = os.Stat(stale)
	Expect(os.IsNotExist(err)).To(BeTrue(), "stale temp file must be reclaimed")
}

// Set must publish atomically — the on-disk file is never a partial
// write, and the temp file is cleaned up after a successful Rename.
func TestCacheSetIsAtomic(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	cache, err := NewCache(dir)
	Expect(err).ToNot(HaveOccurred())

	key := CacheKey{Operation: "sbom", ImageDigest: "sha256:atomic", ConfigHash: "h"}
	bigPayload := make([]byte, 256*1024)
	for i := range bigPayload {
		bigPayload[i] = byte(i)
	}

	Expect(cache.Set(key, bigPayload)).To(Succeed())

	got, found, err := cache.Get(key)
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(got).To(Equal(bigPayload))

	// No leftover temp files in the op dir after a successful Set.
	opDir := filepath.Join(dir, "sbom")
	entries, err := os.ReadDir(opDir)
	Expect(err).ToNot(HaveOccurred())
	for _, e := range entries {
		Expect(strings.HasPrefix(e.Name(), ".cache.tmp.")).To(BeFalse(),
			"Set must clean up its temp file after Rename, found %s", e.Name())
	}
}

func TestCacheCleanSkipsHMACKey(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	cache, err := NewCache(dir)
	Expect(err).ToNot(HaveOccurred())

	keyPath := filepath.Join(dir, hmacKeyFilename)
	keyBefore, err := os.ReadFile(keyPath)
	Expect(err).ToNot(HaveOccurred())

	// Drop in a corrupt entry that Clean would normally remove.
	cKey := CacheKey{Operation: "sbom", ImageDigest: "sha256:c", ConfigHash: "h"}
	cPath := cache.getPath(cKey)
	Expect(os.MkdirAll(filepath.Dir(cPath), 0o700)).To(Succeed())
	Expect(os.WriteFile(cPath, []byte("not json"), 0o600)).To(Succeed())

	Expect(cache.Clean()).To(Succeed())

	// The HMAC key must survive.
	keyAfter, err := os.ReadFile(keyPath)
	Expect(err).ToNot(HaveOccurred())
	Expect(keyAfter).To(Equal(keyBefore))

	// The corrupt entry must be gone.
	_, err = os.Stat(cPath)
	Expect(os.IsNotExist(err)).To(BeTrue())
}
