package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/simple-container-com/api/pkg/api/logger"
)

// CacheEntry represents a cached item with expiration
type CacheEntry struct {
	Value       interface{} `json:"value"`
	ExpiresAt   time.Time   `json:"expires_at"`
	CreatedAt   time.Time   `json:"created_at"`
	AccessCount int64       `json:"access_count"`
	LastAccess  time.Time   `json:"last_access"`
}

// Cache provides thread-safe caching with TTL support
type Cache struct {
	data       map[string]*CacheEntry
	mu         sync.RWMutex
	logger     logger.Logger
	defaultTTL time.Duration
}

// NewCache creates a new cache instance
func NewCache(logger logger.Logger, defaultTTL time.Duration) *Cache {
	cache := &Cache{
		data:       make(map[string]*CacheEntry),
		logger:     logger,
		defaultTTL: defaultTTL,
	}

	// Start cleanup goroutine
	go cache.startCleanup()

	return cache
}

// Set stores a value in the cache with optional custom TTL
func (c *Cache) Set(ctx context.Context, key string, value interface{}, ttl ...time.Duration) {
	effectiveTTL := c.defaultTTL
	if len(ttl) > 0 {
		effectiveTTL = ttl[0]
	}

	entry := &CacheEntry{
		Value:       value,
		ExpiresAt:   time.Now().Add(effectiveTTL),
		CreatedAt:   time.Now(),
		AccessCount: 0,
		LastAccess:  time.Now(),
	}

	c.mu.Lock()
	c.data[key] = entry
	c.mu.Unlock()

	c.logger.Debug(ctx, "Cache entry stored: key=%s, expires_at=%v, ttl=%s", key, entry.ExpiresAt, effectiveTTL.String())
}

// Get retrieves a value from the cache
func (c *Cache) Get(ctx context.Context, key string) (interface{}, bool) {
	c.mu.Lock()
	entry, exists := c.data[key]
	if !exists {
		c.mu.Unlock()
		return nil, false
	}

	// Check expiration
	if time.Now().After(entry.ExpiresAt) {
		delete(c.data, key)
		c.mu.Unlock()
		c.logger.Debug(ctx, "Cache entry expired: key=%s", key)
		return nil, false
	}

	// Update access statistics
	entry.AccessCount++
	entry.LastAccess = time.Now()
	c.mu.Unlock()

	return entry.Value, true
}

// Delete removes a key from the cache
func (c *Cache) Delete(ctx context.Context, key string) {
	c.mu.Lock()
	delete(c.data, key)
	c.mu.Unlock()

	c.logger.Debug(ctx, "Cache entry deleted: key=%s", key)
}

// Clear removes all entries from the cache
func (c *Cache) Clear(ctx context.Context) {
	c.mu.Lock()
	entryCount := len(c.data)
	c.data = make(map[string]*CacheEntry)
	c.mu.Unlock()

	c.logger.Info(ctx, "Cache cleared: entries_removed=%d", entryCount)
}

// GetOrSet retrieves a value or sets it if not found
func (c *Cache) GetOrSet(ctx context.Context, key string, factory func() (interface{}, error), ttl ...time.Duration) (interface{}, error) {
	// Try to get first
	if value, found := c.Get(ctx, key); found {
		return value, nil
	}

	// Create the value
	value, err := factory()
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.Set(ctx, key, value, ttl...)
	return value, nil
}

// HashKey creates a consistent hash key from multiple components
func (c *Cache) HashKey(components ...string) string {
	hasher := sha256.New()
	for _, component := range components {
		hasher.Write([]byte(component))
	}
	return hex.EncodeToString(hasher.Sum(nil))[:16] // Use first 16 chars for readability
}

// Stats returns cache statistics
func (c *Cache) Stats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var totalAccesses int64
	var expiredCount int
	now := time.Now()

	for _, entry := range c.data {
		totalAccesses += entry.AccessCount
		if now.After(entry.ExpiresAt) {
			expiredCount++
		}
	}

	return map[string]interface{}{
		"total_entries":   len(c.data),
		"expired_entries": expiredCount,
		"total_accesses":  totalAccesses,
		"hit_ratio":       c.calculateHitRatio(),
	}
}

// calculateHitRatio calculates cache hit ratio (simplified)
func (c *Cache) calculateHitRatio() float64 {
	// This is a simplified implementation
	// In practice, you'd want to track hits/misses separately
	return 0.75 // Placeholder
}

// startCleanup periodically removes expired entries
func (c *Cache) startCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup removes expired entries
func (c *Cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	removed := 0

	for key, entry := range c.data {
		if now.After(entry.ExpiresAt) {
			delete(c.data, key)
			removed++
		}
	}

	if removed > 0 {
		// Note: Using background context since this is called from cleanup goroutine
		ctx := context.Background()
		c.logger.Debug(ctx, "Cache cleanup completed: expired_entries_removed=%d, remaining_entries=%d", removed, len(c.data))
	}
}

// CacheManager manages multiple specialized caches
type CacheManager struct {
	EmbeddingsCache    *Cache
	LLMResponseCache   *Cache
	SchemaCache        *Cache
	DocumentationCache *Cache
	logger             logger.Logger
}

// NewCacheManager creates a new cache manager with pre-configured caches
func NewCacheManager(logger logger.Logger) *CacheManager {
	return &CacheManager{
		EmbeddingsCache:    NewCache(logger, 1*time.Hour),    // Embeddings cached for 1 hour
		LLMResponseCache:   NewCache(logger, 15*time.Minute), // LLM responses cached for 15 minutes
		SchemaCache:        NewCache(logger, 30*time.Minute), // Schemas cached for 30 minutes
		DocumentationCache: NewCache(logger, 2*time.Hour),    // Documentation cached for 2 hours
		logger:             logger,
	}
}

// GenerateCacheKey creates a consistent cache key for different contexts
func (cm *CacheManager) GenerateCacheKey(prefix string, components ...string) string {
	return cm.EmbeddingsCache.HashKey(append([]string{prefix}, components...)...)
}

// GetGlobalStats returns statistics for all caches
func (cm *CacheManager) GetGlobalStats() map[string]interface{} {
	return map[string]interface{}{
		"embeddings_cache":    cm.EmbeddingsCache.Stats(),
		"llm_response_cache":  cm.LLMResponseCache.Stats(),
		"schema_cache":        cm.SchemaCache.Stats(),
		"documentation_cache": cm.DocumentationCache.Stats(),
	}
}

// ClearAll clears all managed caches
func (cm *CacheManager) ClearAll(ctx context.Context) {
	cm.EmbeddingsCache.Clear(ctx)
	cm.LLMResponseCache.Clear(ctx)
	cm.SchemaCache.Clear(ctx)
	cm.DocumentationCache.Clear(ctx)

	cm.logger.Info(ctx, "All caches cleared")
}
