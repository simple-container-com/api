package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Cache provides TTL-based, HMAC-authenticated caching for security
// operation results (SBOM, image scan, signature checks).
//
// Integrity model: each entry is signed with HMAC-SHA256 using a 32-byte
// key persisted at <baseDir>/.hmac.key (mode 0600, generated from
// crypto/rand on first use). Tampering with the on-disk JSON, or
// substituting a file written under a different key, causes the entry
// to fail verification and be silently discarded. This replaces the
// previous mtime-based tamper-detection heuristic, which was trivially
// forgeable with `touch -t`.
type Cache struct {
	baseDir string
	key     []byte // HMAC-SHA256 key, 32 bytes
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

// signedEntry is the on-disk shape: the entry plus a hex-encoded
// HMAC-SHA256 of the entry's canonical JSON.
type signedEntry struct {
	Entry CacheEntry `json:"entry"`
	MAC   string     `json:"mac"`
}

// TTL durations for different operations
const (
	TTL_SBOM       = 24 * time.Hour // SBOM: 24h
	TTL_SCAN_GRYPE = 6 * time.Hour  // Grype scan: 6h
	TTL_SCAN_TRIVY = 6 * time.Hour  // Trivy scan: 6h
)

const (
	// hmacKeyFilename is the name of the per-cache HMAC key file. The
	// leading dot lets cleanup walkers skip it cheaply by name; mode is
	// 0o600 so even other processes running as the same user can't
	// silently rotate the key from under us.
	hmacKeyFilename = ".hmac.key"
	hmacKeyLen      = 32 // 256 bits, matches HMAC-SHA256 block-size guidance
)

// NewCache creates a new cache instance, generating or loading the
// per-cache HMAC key. Returns an error if the existing key file is
// the wrong size (likely corrupted or truncated).
func NewCache(baseDir string) (*Cache, error) {
	if baseDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home directory: %w", err)
		}
		baseDir = filepath.Join(homeDir, ".simple-container", "cache", "security")
	}

	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	key, err := loadOrCreateHMACKey(filepath.Join(baseDir, hmacKeyFilename))
	if err != nil {
		return nil, fmt.Errorf("initializing cache HMAC key: %w", err)
	}

	return &Cache{
		baseDir: baseDir,
		key:     key,
	}, nil
}

// loadOrCreateHMACKey reads the HMAC key from path; if absent, generates
// a new 32-byte random key and persists it with mode 0o600.
func loadOrCreateHMACKey(path string) ([]byte, error) {
	existing, err := os.ReadFile(path)
	if err == nil {
		if len(existing) != hmacKeyLen {
			return nil, fmt.Errorf("hmac key at %s is %d bytes, expected %d (corrupted?)",
				path, len(existing), hmacKeyLen)
		}
		return existing, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("reading hmac key: %w", err)
	}

	key := make([]byte, hmacKeyLen)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generating hmac key: %w", err)
	}
	// O_EXCL ensures we never silently overwrite a concurrently-created key.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		// Race: another process created the key between our Stat and
		// OpenFile. Fall back to reading what they wrote.
		if errors.Is(err, os.ErrExist) {
			return loadOrCreateHMACKey(path)
		}
		return nil, fmt.Errorf("creating hmac key file: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(key); err != nil {
		return nil, fmt.Errorf("writing hmac key: %w", err)
	}
	return key, nil
}

// computeMAC returns the HMAC-SHA256 of entryJSON using the cache's key.
func (c *Cache) computeMAC(entryJSON []byte) []byte {
	h := hmac.New(sha256.New, c.key)
	h.Write(entryJSON)
	return h.Sum(nil)
}

// Get retrieves a cached result if it exists, hasn't expired, and its
// HMAC verifies against the cache's key. Tampered or unsigned entries
// (legacy files from the pre-HMAC code path) are silently removed and
// reported as a cache miss.
func (c *Cache) Get(key CacheKey) ([]byte, bool, error) {
	path := c.getPath(key)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("reading cache file: %w", err)
	}

	var signed signedEntry
	if err := json.Unmarshal(data, &signed); err != nil {
		// Unparseable — treat as miss.
		_ = os.Remove(path)
		return nil, false, nil
	}

	// Reject pre-HMAC entries (no MAC field) and any zero-length MAC.
	gotMAC, err := hex.DecodeString(signed.MAC)
	if err != nil || len(gotMAC) == 0 {
		_ = os.Remove(path)
		return nil, false, nil
	}

	entryJSON, err := json.Marshal(signed.Entry)
	if err != nil {
		// Should never happen — we just unmarshaled the same shape.
		_ = os.Remove(path)
		return nil, false, nil
	}

	if !hmac.Equal(gotMAC, c.computeMAC(entryJSON)) {
		// Tamper detected (or written by a different key). Discard.
		_ = os.Remove(path)
		return nil, false, nil
	}

	// MAC covers the entry content but not the location on disk. A
	// valid signed file copied from path A to path B would still verify
	// here, and `Get(keyB)` would return keyA's payload — bypassing the
	// integrity story for SBOM/scan data. Bind the lookup to the embedded
	// CacheKey and discard mismatches.
	if signed.Entry.Key != key {
		_ = os.Remove(path)
		return nil, false, nil
	}

	if time.Now().After(signed.Entry.ExpiresAt) {
		_ = os.Remove(path)
		return nil, false, nil
	}

	return signed.Entry.Data, true, nil
}

// Set stores a result in the cache with the appropriate TTL.
func (c *Cache) Set(key CacheKey, data []byte) error {
	return c.SetWithTTL(key, data, c.getTTL(key.Operation))
}

// SetWithTTL stores a result in the cache with an explicit TTL.
func (c *Cache) SetWithTTL(key CacheKey, data []byte, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = c.getTTL(key.Operation)
	}
	now := time.Now()

	entry := CacheEntry{
		Key:       key,
		Data:      data,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}

	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling cache entry: %w", err)
	}

	signed := signedEntry{
		Entry: entry,
		MAC:   hex.EncodeToString(c.computeMAC(entryJSON)),
	}

	out, err := json.Marshal(signed)
	if err != nil {
		return fmt.Errorf("marshaling signed entry: %w", err)
	}

	path := c.getPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	if err := os.WriteFile(path, out, 0o600); err != nil {
		return fmt.Errorf("writing cache file: %w", err)
	}

	return nil
}

// Invalidate removes a cached result
func (c *Cache) Invalidate(key CacheKey) error {
	path := c.getPath(key)
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("removing cache file: %w", err)
	}
	return nil
}

// Clean removes expired or HMAC-invalid cache entries. The HMAC key
// file itself is skipped by name.
func (c *Cache) Clean() error {
	return filepath.Walk(c.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if info.Name() == hmacKeyFilename {
			return nil // Never touch the key.
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var signed signedEntry
		if err := json.Unmarshal(data, &signed); err != nil {
			_ = os.Remove(path)
			return nil
		}

		gotMAC, err := hex.DecodeString(signed.MAC)
		if err != nil || len(gotMAC) == 0 {
			_ = os.Remove(path)
			return nil
		}

		entryJSON, err := json.Marshal(signed.Entry)
		if err != nil || !hmac.Equal(gotMAC, c.computeMAC(entryJSON)) {
			_ = os.Remove(path)
			return nil
		}

		// Mirror the path-to-key binding from Get: a validly-signed
		// entry parked at the wrong filesystem location is also garbage.
		if c.getPath(signed.Entry.Key) != path {
			_ = os.Remove(path)
			return nil
		}

		if time.Now().After(signed.Entry.ExpiresAt) {
			_ = os.Remove(path)
		}
		return nil
	})
}

// getPath returns the filesystem path for a cache key
func (c *Cache) getPath(key CacheKey) string {
	hash := sha256.New()
	hash.Write([]byte(key.Operation))
	hash.Write([]byte(key.ImageDigest))
	hash.Write([]byte(key.ConfigHash))
	filename := hex.EncodeToString(hash.Sum(nil))

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
		return 6 * time.Hour
	}
}

// Size returns the total size of the cache in bytes (incl. the HMAC key,
// which is negligible at 32 bytes).
func (c *Cache) Size() (int64, error) {
	var size int64
	err := filepath.Walk(c.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// Clear removes all cached entries AND the HMAC key. The next NewCache
// call will generate a fresh key.
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
