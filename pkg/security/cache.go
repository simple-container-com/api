// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

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
	"strings"
	"syscall"
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
	// tempFileGracePeriod bounds how long Clean() will leave an
	// in-flight `.cache.tmp.*` / `.hmac.key.tmp.*` file alone before
	// reclaiming it. A legitimate writer publishes in sub-second;
	// anything older than 1h is a crashed Set() that won't return.
	tempFileGracePeriod = 1 * time.Hour
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
// a new 32-byte random key and persists it via os.CreateTemp + Rename.
//
// Concurrency: a concurrent reader observes either the fully-written
// 32-byte file or no file at all — never a 0-byte mid-creation view.
// The prior O_CREATE|O_EXCL + write sequence had a narrow window where
// another reader could ReadFile a 0-byte file and fail with "corrupted"
// (codex round-1 P2). Two processes racing here each generate a key;
// only one Rename wins, and the loser's reread picks up the winner's
// bytes (cryptographically equivalent — both random).
func loadOrCreateHMACKey(path string) ([]byte, error) {
	if existing, err := readKeyFile(path); err == nil {
		return existing, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	key := make([]byte, hmacKeyLen)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generating hmac key: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".hmac.key.tmp.*")
	if err != nil {
		return nil, fmt.Errorf("creating hmac key temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = tmp.Close()
		return nil, fmt.Errorf("chmod hmac key temp: %w", err)
	}
	if _, err := tmp.Write(key); err != nil {
		_ = tmp.Close()
		return nil, fmt.Errorf("writing hmac key: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return nil, fmt.Errorf("syncing hmac key: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("closing hmac key temp: %w", err)
	}

	// Publish strategy:
	//   1. os.Link (atomic NO-REPLACE): if two NewCache calls race,
	//      the loser's Link fails with ErrExist and adopts the winner's
	//      key. This is the race-free path (codex round-2 P2).
	//   2. Fallback to os.Rename when the filesystem doesn't support
	//      hardlinks at all — Docker Desktop volume mounts, VirtualBox
	//      vboxsf, some SMB shares (gemini round-3 P2). Rename is
	//      atomic-with-replace, so a concurrent NewCache on the same
	//      dir on such a filesystem can briefly diverge — accepted
	//      trade-off for dev-machine usability. Real production
	//      surface (Linux ext4/overlayFS, macOS APFS, container
	//      layered FS, NFSv3+) supports Link and takes the race-free
	//      path.
	// The same-directory `dir` keeps either op a same-FS operation.
	if err := os.Link(tmpPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			// Race lost — adopt the winner's key.
			return readKeyFile(path)
		}
		if isLinkUnsupported(err) {
			// FS doesn't do hardlinks; fall through to Rename.
			if rerr := os.Rename(tmpPath, path); rerr != nil {
				return nil, fmt.Errorf("placing hmac key (link unsupported, rename also failed): %w", rerr)
			}
			return readKeyFile(path)
		}
		return nil, fmt.Errorf("placing hmac key: %w", err)
	}
	// tmpPath is removed by the deferred cleanup; the linked path is
	// the canonical key from now on.
	return readKeyFile(path)
}

// isLinkUnsupported tells whether the error from os.Link indicates a
// filesystem that rejects hardlinks (Docker Desktop bind mounts,
// vboxsf, some SMB). syscall.EPERM and syscall.ENOTSUP are the usual
// posix-y codes; syscall.EXDEV would mean cross-device (shouldn't
// happen given same-directory temp, but defensive).
func isLinkUnsupported(err error) bool {
	return errors.Is(err, syscall.EPERM) ||
		errors.Is(err, syscall.ENOTSUP) ||
		errors.Is(err, syscall.EXDEV) ||
		errors.Is(err, syscall.EOPNOTSUPP)
}

// readKeyFile reads and validates the HMAC key file. Returns
// os.ErrNotExist when absent. A wrong-size read is permanent
// corruption — there is no race window to retry through, because
// loadOrCreateHMACKey publishes via os.Link of a fully-written,
// fsynced temp file. The canonical path appears atomically with
// exactly hmacKeyLen bytes or not at all.
func readKeyFile(path string) ([]byte, error) {
	existing, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("reading hmac key: %w", err)
	}
	if len(existing) != hmacKeyLen {
		return nil, fmt.Errorf("hmac key at %s is %d bytes, expected %d (corrupted)",
			path, len(existing), hmacKeyLen)
	}
	return existing, nil
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

	payload, ok, shouldRemove := c.verifyAndExtract(data, key, time.Now())
	if shouldRemove {
		_ = os.Remove(path)
	}
	return payload, ok, nil
}

// parseSignedEntry is the pure parse-and-MAC-verify seam shared by Get
// and Clean. It performs only checks that depend on the bytes themselves
// + the cache's HMAC key + clock — NOT on the requested key or the path
// the entry was read from. Callers add those binding checks on top.
//
// Steps:
//
//  1. JSON unmarshal of signedEntry envelope.
//  2. hex-decode of the MAC field; zero-length / non-hex MACs are
//     rejected (this also catches pre-HMAC entries that lack the field).
//  3. canonical re-marshal of the embedded entry + constant-time HMAC
//     compare against the cache's key.
//  4. expiry check against `now`.
//
// Returned `shouldRemove` is true whenever the on-disk file fails any
// check (garbage JSON, tampered MAC, expired). Callers that hold a path
// remove it; callers operating on in-memory bytes can ignore.
//
// Hermetic: no filesystem, no time.Now() side-effect, no Cache state
// mutation. Safe to call concurrently and from fuzz workers.
func (c *Cache) parseSignedEntry(data []byte, now time.Time) (entry CacheEntry, ok bool, shouldRemove bool) {
	var signed signedEntry
	if err := json.Unmarshal(data, &signed); err != nil {
		return CacheEntry{}, false, true
	}

	gotMAC, err := hex.DecodeString(signed.MAC)
	if err != nil || len(gotMAC) == 0 {
		return CacheEntry{}, false, true
	}

	entryJSON, err := json.Marshal(signed.Entry)
	if err != nil {
		return CacheEntry{}, false, true
	}

	if !hmac.Equal(gotMAC, c.computeMAC(entryJSON)) {
		return CacheEntry{}, false, true
	}

	if now.After(signed.Entry.ExpiresAt) {
		return CacheEntry{}, false, true
	}

	return signed.Entry, true, false
}

// verifyAndExtract is the Get-path wrapper around parseSignedEntry. It
// adds the requested-key binding check: a validly-signed entry for keyA
// copied to keyB's path must not be returned from Get(keyB).
func (c *Cache) verifyAndExtract(data []byte, requestedKey CacheKey, now time.Time) (payload []byte, ok bool, shouldRemove bool) {
	entry, parsedOK, parsedShouldRemove := c.parseSignedEntry(data, now)
	if !parsedOK {
		return nil, false, parsedShouldRemove
	}

	// MAC covers the entry content but not the location on disk. A
	// valid signed file copied from path A to path B would still verify
	// in parseSignedEntry, and Get(keyB) would return keyA's payload —
	// bypassing the integrity story for SBOM/scan data. Bind the lookup
	// to the embedded CacheKey and discard mismatches.
	if entry.Key != requestedKey {
		return nil, false, true
	}

	return entry.Data, true, false
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
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	// Atomic publish: CreateTemp in the SAME directory (so Rename is a
	// same-filesystem op and stays atomic) → Chmod → Write → Sync →
	// Close → Rename. Without this, a concurrent Clean() can read a
	// partially-written file, fail to unmarshal, and remove it from
	// under us — corrupting the cache (gemini round-1 P1).
	tmp, err := os.CreateTemp(dir, ".cache.tmp.*")
	if err != nil {
		return fmt.Errorf("creating cache temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod cache temp: %w", err)
	}
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing cache temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing cache temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing cache temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("placing cache file: %w", err)
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
		// In-flight `Set()` / `loadOrCreateHMACKey` temp files: skip if
		// recent (writer's Rename/Link is imminent), reclaim if stale.
		// A legitimate Set takes sub-second; mtime > 1h ago is a
		// crashed writer (codex round-3 P3). Without the grace-period
		// sweep, MB-scale SBOM temps could accumulate indefinitely.
		if strings.HasPrefix(info.Name(), ".cache.tmp.") ||
			strings.HasPrefix(info.Name(), ".hmac.key.tmp.") {
			if time.Since(info.ModTime()) > tempFileGracePeriod {
				_ = os.Remove(path)
			}
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Share the parse/MAC-verify/expiry logic with Get's path via
		// parseSignedEntry — keeps the integrity model in one place so
		// Get and Clean can never drift on (e.g.) an HMAC algorithm
		// rotation. Clean adds its own path-binding check (the entry
		// must sit at the filesystem location its embedded key maps
		// to) which is different from Get's request-key binding.
		entry, ok, shouldRemove := c.parseSignedEntry(data, time.Now())
		if !ok {
			if shouldRemove {
				_ = os.Remove(path)
			}
			return nil
		}

		// Mirror the path-to-key binding from Get: a validly-signed
		// entry parked at the wrong filesystem location is also garbage.
		if c.getPath(entry.Key) != path {
			_ = os.Remove(path)
		}
		return nil
	})
}

// getPath returns the filesystem path for a cache key.
//
// CacheKey.Operation is used directly as a subdirectory name. All
// callers in-tree pass hardcoded values ("sbom", "scan-grype",
// "scan-trivy", "signature"), but defense-in-depth: a `../` in
// Operation would let a future careless caller escape baseDir
// (gemini round-3 P3). `filepath.Base` collapses any path separator
// or traversal segment to a single bare name, so even malicious
// input lands inside baseDir.
func (c *Cache) getPath(key CacheKey) string {
	hash := sha256.New()
	hash.Write([]byte(key.Operation))
	hash.Write([]byte(key.ImageDigest))
	hash.Write([]byte(key.ConfigHash))
	filename := hex.EncodeToString(hash.Sum(nil))

	op := filepath.Base(filepath.Clean(key.Operation))
	// filepath.Base("..") returns ".." — must catch explicitly or the
	// Join walks above baseDir. Same for raw separators on Windows.
	if op == "" || op == "." || op == ".." || strings.ContainsAny(op, `/\`) {
		op = "_unknown"
	}
	return filepath.Join(c.baseDir, op, filename+".json")
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
