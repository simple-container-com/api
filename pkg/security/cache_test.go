package security

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewCache(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Create cache
	cache, err := NewCache(tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	if cache.baseDir != tmpDir {
		t.Errorf("Expected baseDir '%s', got '%s'", tmpDir, cache.baseDir)
	}

	if cache.ttl != time.Hour {
		t.Errorf("Expected ttl 1h, got %v", cache.ttl)
	}

	// Check directory was created
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Error("Cache directory was not created")
	}
}

func TestCache_SetAndGet(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	cache, err := NewCache(tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	key := CacheKey{
		Operation:   "sbom",
		ImageDigest: "sha256:abc123",
		ConfigHash:  "config123",
	}
	data := []byte("test data")

	// Set
	err = cache.Set(key, data)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get
	retrieved, found, err := cache.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !found {
		t.Error("Expected to find cached data")
	}

	if string(retrieved) != string(data) {
		t.Errorf("Expected data '%s', got '%s'", string(data), string(retrieved))
	}
}

func TestCache_GetNotFound(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	cache, err := NewCache(tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	key := CacheKey{
		Operation:   "sbom",
		ImageDigest: "sha256:notfound",
		ConfigHash:  "config123",
	}

	// Get
	_, found, err := cache.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if found {
		t.Error("Expected not to find cached data")
	}
}

func TestCache_Expiration(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	cache, err := NewCache(tmpDir, 100*time.Millisecond) // Short TTL for testing
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	key := CacheKey{
		Operation:   "sbom",
		ImageDigest: "sha256:expires",
		ConfigHash:  "config123",
	}
	data := []byte("test data")

	// Set
	err = cache.Set(key, data)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Wait for expiration
	time.Sleep(200 * time.Millisecond)

	// Get - should not find expired data
	_, found, err := cache.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if found {
		t.Error("Expected not to find expired cached data")
	}
}

func TestCache_Invalidate(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	cache, err := NewCache(tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	key := CacheKey{
		Operation:   "sbom",
		ImageDigest: "sha256:invalidate",
		ConfigHash:  "config123",
	}
	data := []byte("test data")

	// Set
	err = cache.Set(key, data)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Invalidate
	err = cache.Invalidate(key)
	if err != nil {
		t.Fatalf("Invalidate failed: %v", err)
	}

	// Get - should not find invalidated data
	_, found, err := cache.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if found {
		t.Error("Expected not to find invalidated cached data")
	}
}

func TestCache_Clean(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	cache, err := NewCache(tmpDir, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	// Add multiple entries
	for i := 0; i < 5; i++ {
		key := CacheKey{
			Operation:   "sbom",
			ImageDigest: string(rune('a' + i)),
			ConfigHash:  "config123",
		}
		err = cache.Set(key, []byte("data"))
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}
	}

	// Wait for expiration
	time.Sleep(200 * time.Millisecond)

	// Clean
	err = cache.Clean()
	if err != nil {
		t.Fatalf("Clean failed: %v", err)
	}

	// Check that files were removed
	files, err := filepath.Glob(filepath.Join(tmpDir, "sbom", "*.json"))
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}

	if len(files) > 0 {
		t.Errorf("Expected all expired files to be removed, but found %d files", len(files))
	}
}

func TestHashConfig(t *testing.T) {
	config1 := map[string]string{"key": "value"}
	config2 := map[string]string{"key": "value"}
	config3 := map[string]string{"key": "different"}

	hash1, err := HashConfig(config1)
	if err != nil {
		t.Fatalf("HashConfig failed: %v", err)
	}

	hash2, err := HashConfig(config2)
	if err != nil {
		t.Fatalf("HashConfig failed: %v", err)
	}

	hash3, err := HashConfig(config3)
	if err != nil {
		t.Fatalf("HashConfig failed: %v", err)
	}

	// Same config should produce same hash
	if hash1 != hash2 {
		t.Error("Expected same hash for identical configs")
	}

	// Different config should produce different hash
	if hash1 == hash3 {
		t.Error("Expected different hash for different configs")
	}
}

func TestGetDefaultTTL(t *testing.T) {
	tests := []struct {
		operation string
		expected  time.Duration
	}{
		{"sbom", 24 * time.Hour},
		{"scan-grype", 6 * time.Hour},
		{"scan-trivy", 6 * time.Hour},
		{"signature", 0},
		{"provenance", 0},
		{"unknown", 12 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.operation, func(t *testing.T) {
			result := GetDefaultTTL(tt.operation)
			if result != tt.expected {
				t.Errorf("Expected TTL %v, got %v", tt.expected, result)
			}
		})
	}
}
