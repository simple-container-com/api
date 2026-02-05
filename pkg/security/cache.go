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

// Cache provides caching for security operation results
type Cache struct {
	baseDir string
	ttl     time.Duration
}

// CacheKey identifies a cached item
type CacheKey struct {
	Operation   string // "sbom", "scan-grype", "scan-trivy", "signature"
	ImageDigest string // sha256:abc123...
	ConfigHash  string // Hash of relevant config
}

// CacheEntry represents a cached item with metadata
type CacheEntry struct {
	Key       CacheKey  `json:"key"`
	Data      []byte    `json:"data"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewCache creates a new cache instance
func NewCache(baseDir string, ttl time.Duration) (*Cache, error) {
	if baseDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		baseDir = filepath.Join(homeDir, ".simple-container", "cache", "security")
	}

	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &Cache{
		baseDir: baseDir,
		ttl:     ttl,
	}, nil
}

// Get retrieves cached result
func (c *Cache) Get(key CacheKey) ([]byte, bool, error) {
	filePath := c.getCacheFilePath(key)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, false, nil
	}

	// Read cache entry
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, false, fmt.Errorf("failed to read cache file: %w", err)
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		// Cache corrupted, delete it
		_ = os.Remove(filePath)
		return nil, false, nil
	}

	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		_ = os.Remove(filePath)
		return nil, false, nil
	}

	return entry.Data, true, nil
}

// Set stores result in cache
func (c *Cache) Set(key CacheKey, data []byte) error {
	entry := CacheEntry{
		Key:       key,
		Data:      data,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(c.ttl),
	}

	entryData, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	filePath := c.getCacheFilePath(key)

	// Create directory if needed
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filePath, entryData, 0600); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// Invalidate removes cached result
func (c *Cache) Invalidate(key CacheKey) error {
	filePath := c.getCacheFilePath(key)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove cache file: %w", err)
	}
	return nil
}

// Clean removes expired entries
func (c *Cache) Clean() error {
	return filepath.Walk(c.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Read and check expiration
		data, err := os.ReadFile(path)
		if err != nil {
			// Can't read, skip
			return nil
		}

		var entry CacheEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			// Corrupted, delete it
			_ = os.Remove(path)
			return nil
		}

		// Remove if expired
		if time.Now().After(entry.ExpiresAt) {
			_ = os.Remove(path)
		}

		return nil
	})
}

// getCacheFilePath generates file path for cache key
func (c *Cache) getCacheFilePath(key CacheKey) string {
	// Create a hash of the key for the filename
	keyStr := fmt.Sprintf("%s:%s:%s", key.Operation, key.ImageDigest, key.ConfigHash)
	hash := sha256.Sum256([]byte(keyStr))
	filename := hex.EncodeToString(hash[:])

	// Organize by operation type
	return filepath.Join(c.baseDir, key.Operation, filename+".json")
}

// HashConfig creates a hash of configuration for cache key
func HashConfig(config interface{}) (string, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:16]), nil // Use first 16 bytes (32 hex chars)
}

// GetDefaultTTL returns default TTL for operation type
func GetDefaultTTL(operation string) time.Duration {
	switch operation {
	case "sbom":
		return 24 * time.Hour // SBOMs valid for 24 hours
	case "scan-grype", "scan-trivy":
		return 6 * time.Hour // Scan results valid for 6 hours
	case "signature":
		return 0 // Don't cache signatures
	case "provenance":
		return 0 // Don't cache provenance (unique per build)
	default:
		return 12 * time.Hour
	}
}
