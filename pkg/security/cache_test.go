package security

import (
	"encoding/json"
	"os"
	"path/filepath"
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
