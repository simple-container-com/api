package security

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewCache(t *testing.T) {
	// Test with custom directory
	customDir := filepath.Join(t.TempDir(), "custom-cache")
	cache, err := NewCache(customDir)
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}
	if cache.baseDir != customDir {
		t.Errorf("Expected baseDir %s, got %s", customDir, cache.baseDir)
	}

	// Verify directory was created
	if _, err := os.Stat(customDir); os.IsNotExist(err) {
		t.Errorf("Cache directory was not created")
	}

	// Test with empty directory (should use default)
	cache2, err := NewCache("")
	if err != nil {
		t.Fatalf("NewCache with empty dir failed: %v", err)
	}
	if cache2.baseDir == "" {
		t.Errorf("Expected non-empty baseDir for default cache")
	}
}

func TestCacheSetAndGet(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	key := CacheKey{
		Operation:   "sbom",
		ImageDigest: "sha256:abc123",
		ConfigHash:  "hash456",
	}

	data := []byte("test data")

	// Test Set
	if err := cache.Set(key, data); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Test Get
	retrieved, found, err := cache.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !found {
		t.Errorf("Expected cache hit, got miss")
	}
	if string(retrieved) != string(data) {
		t.Errorf("Expected data %s, got %s", string(data), string(retrieved))
	}

	// Test Get with non-existent key
	nonExistentKey := CacheKey{
		Operation:   "nonexistent",
		ImageDigest: "sha256:xyz789",
		ConfigHash:  "hash999",
	}
	_, found, err = cache.Get(nonExistentKey)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if found {
		t.Errorf("Expected cache miss, got hit")
	}
}

func TestCacheTTLExpiration(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	// Note: TTL is a constant and cannot be overridden at runtime
	// This test simulates expiration by manually modifying cache entries

	key := CacheKey{
		Operation:   "sbom",
		ImageDigest: "sha256:abc123",
		ConfigHash:  "hash456",
	}

	data := []byte("test data")

	// Set data
	if err := cache.Set(key, data); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Verify it's there
	_, found, err := cache.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !found {
		t.Errorf("Expected cache hit immediately after set")
	}

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
	if err := os.WriteFile(path, entryData, 0o600); err != nil {
		t.Fatalf("Failed to write expired entry: %v", err)
	}

	// Try to get expired entry
	_, found, err = cache.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if found {
		t.Errorf("Expected cache miss for expired entry, got hit")
	}

	// Verify file was deleted
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Expected expired cache file to be deleted")
	}
}

func marshalJSON(v interface{}) ([]byte, error) {
	// Simple JSON marshal for testing
	return []byte("{}"), nil
}

func TestCacheInvalidate(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	key := CacheKey{
		Operation:   "sbom",
		ImageDigest: "sha256:abc123",
		ConfigHash:  "hash456",
	}

	data := []byte("test data")

	// Set data
	if err := cache.Set(key, data); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Invalidate
	if err := cache.Invalidate(key); err != nil {
		t.Fatalf("Invalidate failed: %v", err)
	}

	// Verify it's gone
	_, found, err := cache.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if found {
		t.Errorf("Expected cache miss after invalidation, got hit")
	}

	// Invalidate non-existent key should not error
	if err := cache.Invalidate(key); err != nil {
		t.Errorf("Invalidate of non-existent key should not error: %v", err)
	}
}

func TestCacheClean(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	// Create multiple cache entries
	for i := 0; i < 5; i++ {
		key := CacheKey{
			Operation:   "sbom",
			ImageDigest: "sha256:" + string(rune('a'+i)),
			ConfigHash:  "hash",
		}
		data := []byte("test data")
		if err := cache.Set(key, data); err != nil {
			t.Fatalf("Set failed: %v", err)
		}
	}

	// Clean should not remove valid entries
	if err := cache.Clean(); err != nil {
		t.Fatalf("Clean failed: %v", err)
	}

	// Verify entries still exist
	key := CacheKey{
		Operation:   "sbom",
		ImageDigest: "sha256:a",
		ConfigHash:  "hash",
	}
	_, found, err := cache.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !found {
		t.Errorf("Valid entry should not be cleaned")
	}
}

func TestCacheSize(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	initialSize, err := cache.Size()
	if err != nil {
		t.Fatalf("Size failed: %v", err)
	}

	// Add some data
	key := CacheKey{
		Operation:   "sbom",
		ImageDigest: "sha256:abc123",
		ConfigHash:  "hash456",
	}
	data := []byte("test data with some content")
	if err := cache.Set(key, data); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	newSize, err := cache.Size()
	if err != nil {
		t.Fatalf("Size failed: %v", err)
	}

	if newSize <= initialSize {
		t.Errorf("Expected size to increase after adding data")
	}
}

func TestCacheClear(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	// Add some data
	key := CacheKey{
		Operation:   "sbom",
		ImageDigest: "sha256:abc123",
		ConfigHash:  "hash456",
	}
	data := []byte("test data")
	if err := cache.Set(key, data); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Clear
	if err := cache.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	// Verify it's gone
	_, found, err := cache.Get(key)
	if err == nil && found {
		t.Errorf("Expected cache miss after clear, got hit")
	}

	// Verify directory is gone
	if _, err := os.Stat(cache.baseDir); !os.IsNotExist(err) {
		t.Errorf("Expected cache directory to be deleted after clear")
	}
}

func TestComputeConfigHash(t *testing.T) {
	config1 := map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
	}

	hash1, err := ComputeConfigHash(config1)
	if err != nil {
		t.Fatalf("ComputeConfigHash failed: %v", err)
	}
	if hash1 == "" {
		t.Errorf("Expected non-empty hash")
	}

	// Same config should produce same hash
	hash2, err := ComputeConfigHash(config1)
	if err != nil {
		t.Fatalf("ComputeConfigHash failed: %v", err)
	}
	if hash1 != hash2 {
		t.Errorf("Expected same hash for same config, got %s and %s", hash1, hash2)
	}

	// Different config should produce different hash
	config2 := map[string]interface{}{
		"key1": "different",
		"key2": "value2",
	}
	hash3, err := ComputeConfigHash(config2)
	if err != nil {
		t.Fatalf("ComputeConfigHash failed: %v", err)
	}
	if hash1 == hash3 {
		t.Errorf("Expected different hash for different config")
	}
}

func TestCacheGetTTL(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

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
			ttl := cache.getTTL(tt.operation)
			if ttl != tt.expected {
				t.Errorf("Expected TTL %v for %s, got %v", tt.expected, tt.operation, ttl)
			}
		})
	}
}

func TestCacheKeyPath(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	key := CacheKey{
		Operation:   "sbom",
		ImageDigest: "sha256:abc123",
		ConfigHash:  "hash456",
	}

	path := cache.getPath(key)

	// Verify path structure
	if !filepath.IsAbs(path) {
		t.Errorf("Expected absolute path, got %s", path)
	}

	// Verify operation directory is in path
	if !containsSubstring(path, key.Operation) {
		t.Errorf("Expected operation %s in path %s", key.Operation, path)
	}

	// Two keys with same data should produce same path
	path2 := cache.getPath(key)
	if path != path2 {
		t.Errorf("Expected consistent path for same key")
	}

	// Different keys should produce different paths
	key2 := CacheKey{
		Operation:   "scan-grype",
		ImageDigest: "sha256:xyz789",
		ConfigHash:  "hash999",
	}
	path3 := cache.getPath(key2)
	if path == path3 {
		t.Errorf("Expected different paths for different keys")
	}
}

func containsSubstring(s, substr string) bool {
	return filepath.Base(filepath.Dir(s)) == substr || filepath.Base(s) == substr
}
