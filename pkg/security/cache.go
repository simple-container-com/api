package security

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Cache provides TTL-based caching for security operation results
type Cache struct {
	baseDir string
}

// CacheKey uniquely identifies a cached result
type CacheKey struct {
	Operation   string // "sbom", "scan-grype", "scan-trivy", "signature"
	ImageDigest string // sha256:abc123...
	ConfigHash  string // Hash of relevant config
}

// CacheEntry represents a cached result with metadata
type CacheEntry struct {
	Key       CacheKey  `json:"key"`
	Data      []byte    `json:"data"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// TTL durations for different operations
const (
	TTL_SBOM       = 24 * time.Hour // SBOM: 24h
	TTL_SCAN_GRYPE = 6 * time.Hour  // Grype scan: 6h
	TTL_SCAN_TRIVY = 6 * time.Hour  // Trivy scan: 6h
)

// NewCache creates a new cache instance
func NewCache(baseDir string) (*Cache, error) {
	if baseDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home directory: %w", err)
		}
		baseDir = filepath.Join(homeDir, ".simple-container", "cache", "security")
	}

	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	return &Cache{
		baseDir: baseDir,
	}, nil
}

// Get retrieves a cached result if it exists and hasn't expired
func (c *Cache) Get(key CacheKey) ([]byte, bool, error) {
	path := c.getPath(key)

	// Check if file exists
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("checking cache file: %w", err)
	}

	// Read cache entry
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("reading cache file: %w", err)
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		// Invalid cache entry, treat as cache miss
		_ = os.Remove(path)
		return nil, false, nil
	}

	// Check expiration
	if time.Now().After(entry.ExpiresAt) {
		// Expired, remove and return cache miss
		_ = os.Remove(path)
		return nil, false, nil
	}

	// Verify file modification time hasn't been tampered with
	if info.ModTime().After(entry.CreatedAt.Add(1 * time.Hour)) {
		// File was modified after creation, invalidate
		_ = os.Remove(path)
		return nil, false, nil
	}

	return entry.Data, true, nil
}

// Set stores a result in the cache with appropriate TTL
func (c *Cache) Set(key CacheKey, data []byte) error {
	ttl := c.getTTL(key.Operation)
	now := time.Now()

	entry := CacheEntry{
		Key:       key,
		Data:      data,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}

	entryData, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling cache entry: %w", err)
	}

	path := c.getPath(key)

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	// Write with secure permissions (0600)
	if err := os.WriteFile(path, entryData, 0o600); err != nil {
		return fmt.Errorf("writing cache file: %w", err)
	}

	return nil
}

// Invalidate removes a cached result
func (c *Cache) Invalidate(key CacheKey) error {
	path := c.getPath(key)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil // Already gone
	}
	if err != nil {
		return fmt.Errorf("removing cache file: %w", err)
	}
	return nil
}

// Clean removes expired cache entries
func (c *Cache) Clean() error {
	return filepath.Walk(c.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		if info.IsDir() {
			return nil // Skip directories
		}

		// Read and check expiration
		data, err := os.ReadFile(path)
		if err != nil {
			return nil // Skip files we can't read
		}

		var entry CacheEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			// Invalid entry, remove it
			_ = os.Remove(path)
			return nil
		}

		// Check expiration
		if time.Now().After(entry.ExpiresAt) {
			_ = os.Remove(path)
		}

		return nil
	})
}

// getPath returns the filesystem path for a cache key
func (c *Cache) getPath(key CacheKey) string {
	// Create a deterministic filename from the key
	hash := sha256.New()
	hash.Write([]byte(key.Operation))
	hash.Write([]byte(key.ImageDigest))
	hash.Write([]byte(key.ConfigHash))
	filename := hex.EncodeToString(hash.Sum(nil))

	// Organize by operation type
	return filepath.Join(c.baseDir, key.Operation, filename+".json")
}

// getTTL returns the TTL for a given operation type
func (c *Cache) getTTL(operation string) time.Duration {
	switch operation {
	case "sbom":
		return TTL_SBOM
	case "scan-grype", "scan-trivy":
		return TTL_SCAN_GRYPE
	default:
		return 6 * time.Hour // Default TTL
	}
}

// Size returns the total size of the cache in bytes
func (c *Cache) Size() (int64, error) {
	var size int64
	err := filepath.Walk(c.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// Clear removes all cached entries
func (c *Cache) Clear() error {
	return os.RemoveAll(c.baseDir)
}

// ComputeConfigHash computes a hash of configuration for cache keying
func ComputeConfigHash(config interface{}) (string, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshaling config: %w", err)
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
