package core

import (
	"context"
	"fmt"
	"time"

	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/assistant/cache"
	"github.com/simple-container-com/api/pkg/assistant/embeddings"
	"github.com/simple-container-com/api/pkg/assistant/performance"
	"github.com/simple-container-com/api/pkg/assistant/plugins"
	"github.com/simple-container-com/api/pkg/assistant/security"
	"github.com/simple-container-com/api/pkg/assistant/testing"
)

// ManagerConfig holds configuration for the AI Assistant manager
type ManagerConfig struct {
	EnablePerformanceMonitoring bool                    `json:"enable_performance_monitoring"`
	EnableCaching               bool                    `json:"enable_caching"`
	EnableSecurity              bool                    `json:"enable_security"`
	SecurityConfig              security.SecurityConfig `json:"security_config"`
	CacheTTL                    time.Duration           `json:"cache_ttl"`
	PerformanceMonitorInterval  time.Duration           `json:"performance_monitor_interval"`
	EnablePlugins               bool                    `json:"enable_plugins"`
	EnableTesting               bool                    `json:"enable_testing"`
}

// DefaultManagerConfig returns a default configuration
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		EnablePerformanceMonitoring: true,
		EnableCaching:               true,
		EnableSecurity:              true,
		SecurityConfig:              security.GetSecurityConfig("development"),
		CacheTTL:                    30 * time.Minute,
		PerformanceMonitorInterval:  5 * time.Minute,
		EnablePlugins:               true,
		EnableTesting:               false, // Only enable when needed
	}
}

// Manager coordinates all AI Assistant components
type Manager struct {
	config          ManagerConfig
	logger          logger.Logger
	profiler        *performance.Profiler
	cacheManager    *cache.CacheManager
	securityManager *security.SecurityManager
	pluginRegistry  *plugins.Registry
	testFramework   *testing.TestFramework
	embeddingsDB    *embeddings.Database
}

// NewManager creates a new AI Assistant manager
func NewManager(config ManagerConfig, logger logger.Logger) *Manager {
	ctx := context.Background() // Use background context for initialization

	manager := &Manager{
		config: config,
		logger: logger,
	}

	// Initialize performance monitoring
	if config.EnablePerformanceMonitoring {
		manager.profiler = performance.NewProfiler(logger)
		logger.Info(ctx, "Performance monitoring enabled")
	}

	// Initialize caching
	if config.EnableCaching {
		manager.cacheManager = cache.NewCacheManager(logger)
		logger.Info(ctx, "Caching system enabled: default_ttl=%s", config.CacheTTL.String())
	}

	// Initialize security
	if config.EnableSecurity {
		manager.securityManager = security.NewSecurityManager(config.SecurityConfig, logger)
		logger.Info(ctx, "Security manager enabled: security_level=%s", string(config.SecurityConfig.Level))
	}

	// Initialize plugin registry
	if config.EnablePlugins {
		manager.pluginRegistry = plugins.NewRegistry(logger)
		logger.Info(ctx, "Plugin registry enabled")
	}

	// Initialize testing framework
	if config.EnableTesting {
		manager.testFramework = testing.NewTestFramework(logger)
		logger.Info(ctx, "Testing framework enabled")
	}

	return manager
}

// Initialize starts all manager components
func (m *Manager) Initialize(ctx context.Context) error {
	m.logger.Info(ctx, "Initializing AI Assistant Manager: performance_monitoring=%v, caching=%v, security=%v, plugins=%v",
		m.config.EnablePerformanceMonitoring, m.config.EnableCaching, m.config.EnableSecurity, m.config.EnablePlugins)

	// Load embeddings database
	if m.profiler != nil {
		err := m.profiler.TimeOperation(ctx, "embedding_load", func() error {
			db, err := embeddings.LoadEmbeddedDatabase(ctx)
			if err != nil {
				return err
			}
			m.embeddingsDB = db
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to load embeddings database: %w", err)
		}
	} else {
		db, err := embeddings.LoadEmbeddedDatabase(ctx)
		if err != nil {
			return fmt.Errorf("failed to load embeddings database: %w", err)
		}
		m.embeddingsDB = db
	}

	// Start performance monitoring if enabled
	if m.profiler != nil && m.config.PerformanceMonitorInterval > 0 {
		m.profiler.StartMemoryMonitoring(ctx, m.config.PerformanceMonitorInterval)
	}

	// Initialize plugins if enabled
	if m.pluginRegistry != nil {
		if err := m.pluginRegistry.InitializeAll(ctx); err != nil {
			m.logger.Warn(ctx, "Some plugins failed to initialize: error=%s", err.Error())
		}
	}

	// Record initial memory usage
	if m.profiler != nil {
		m.profiler.RecordMemoryUsage(ctx)
	}

	m.logger.Info(ctx, "AI Assistant Manager initialized successfully")
	return nil
}

// ValidateRequest validates a request using security manager
func (m *Manager) ValidateRequest(ctx context.Context, req *security.SecurityRequest) error {
	if m.securityManager == nil {
		return nil // Security disabled
	}

	return m.securityManager.ValidateRequest(ctx, req)
}

// GetCachedResult retrieves a cached result
func (m *Manager) GetCachedResult(ctx context.Context, cacheType string, key string) (interface{}, bool) {
	if m.cacheManager == nil {
		return nil, false
	}

	var targetCache *cache.Cache
	switch cacheType {
	case "embeddings":
		targetCache = m.cacheManager.EmbeddingsCache
	case "llm_response":
		targetCache = m.cacheManager.LLMResponseCache
	case "schema":
		targetCache = m.cacheManager.SchemaCache
	case "documentation":
		targetCache = m.cacheManager.DocumentationCache
	default:
		return nil, false
	}

	return targetCache.Get(ctx, key)
}

// SetCachedResult stores a result in cache
func (m *Manager) SetCachedResult(ctx context.Context, cacheType string, key string, value interface{}, ttl ...time.Duration) {
	if m.cacheManager == nil {
		return
	}

	var targetCache *cache.Cache
	switch cacheType {
	case "embeddings":
		targetCache = m.cacheManager.EmbeddingsCache
	case "llm_response":
		targetCache = m.cacheManager.LLMResponseCache
	case "schema":
		targetCache = m.cacheManager.SchemaCache
	case "documentation":
		targetCache = m.cacheManager.DocumentationCache
	default:
		return
	}

	targetCache.Set(ctx, key, value, ttl...)
}

// GetOrSetCachedResult gets a cached result or creates it
func (m *Manager) GetOrSetCachedResult(ctx context.Context, cacheType string, key string, factory func() (interface{}, error), ttl ...time.Duration) (interface{}, error) {
	if m.cacheManager == nil {
		return factory()
	}

	var targetCache *cache.Cache
	switch cacheType {
	case "embeddings":
		targetCache = m.cacheManager.EmbeddingsCache
	case "llm_response":
		targetCache = m.cacheManager.LLMResponseCache
	case "schema":
		targetCache = m.cacheManager.SchemaCache
	case "documentation":
		targetCache = m.cacheManager.DocumentationCache
	default:
		return factory()
	}

	return targetCache.GetOrSet(ctx, key, factory, ttl...)
}

// SearchDocumentation performs cached semantic search
func (m *Manager) SearchDocumentation(ctx context.Context, query string, limit int) ([]embeddings.SearchResult, error) {
	if m.embeddingsDB == nil {
		return nil, fmt.Errorf("embeddings database not initialized")
	}

	// Use caching for search results
	cacheKey := fmt.Sprintf("search:%s:%d", query, limit)

	result, err := m.GetOrSetCachedResult(ctx, "documentation", cacheKey, func() (interface{}, error) {
		if m.profiler != nil {
			var results []embeddings.SearchResult
			err := m.profiler.TimeOperation(ctx, "semantic_search", func() error {
				var searchErr error
				results, searchErr = embeddings.SearchDocumentation(m.embeddingsDB, query, limit)
				return searchErr
			})
			return results, err
		} else {
			return embeddings.SearchDocumentation(m.embeddingsDB, query, limit)
		}
	}, 10*time.Minute) // Cache search results for 10 minutes

	if err != nil {
		return nil, err
	}

	results, ok := result.([]embeddings.SearchResult)
	if !ok {
		return nil, fmt.Errorf("invalid cached search result type")
	}

	return results, nil
}

// GetPlugin retrieves a plugin by ID
func (m *Manager) GetPlugin(pluginID string) (plugins.Plugin, error) {
	if m.pluginRegistry == nil {
		return nil, fmt.Errorf("plugin registry not enabled")
	}

	return m.pluginRegistry.GetPlugin(pluginID)
}

// GetPluginsByType retrieves plugins by type
func (m *Manager) GetPluginsByType(pluginType plugins.PluginType) []plugins.Plugin {
	if m.pluginRegistry == nil {
		return nil
	}

	return m.pluginRegistry.GetPluginsByType(pluginType)
}

// RunTests executes comprehensive tests
func (m *Manager) RunTests(ctx context.Context, projectPath string) (map[string]*testing.TestSuite, error) {
	if m.testFramework == nil {
		return nil, fmt.Errorf("testing framework not enabled")
	}

	return m.testFramework.RunAllTests(ctx, projectPath), nil
}

// GetPerformanceMetrics returns current performance metrics
func (m *Manager) GetPerformanceMetrics() map[string]interface{} {
	if m.profiler == nil {
		return map[string]interface{}{
			"performance_monitoring": "disabled",
		}
	}

	return m.profiler.GeneratePerformanceReport()
}

// GetCacheStats returns cache statistics
func (m *Manager) GetCacheStats() map[string]interface{} {
	if m.cacheManager == nil {
		return map[string]interface{}{
			"caching": "disabled",
		}
	}

	return m.cacheManager.GetGlobalStats()
}

// GetSecurityStats returns security statistics
func (m *Manager) GetSecurityStats() map[string]interface{} {
	if m.securityManager == nil {
		return map[string]interface{}{
			"security": "disabled",
		}
	}

	return map[string]interface{}{
		"security_enabled": true,
		"security_level":   string(m.config.SecurityConfig.Level),
		"rate_limiting":    m.config.SecurityConfig.EnableRateLimiting,
		"input_validation": m.config.SecurityConfig.EnableInputValidation,
		"audit_logging":    m.config.SecurityConfig.EnableAuditLogging,
	}
}

// GetSystemHealth returns overall system health
func (m *Manager) GetSystemHealth(ctx context.Context) map[string]interface{} {
	health := map[string]interface{}{
		"status":     "healthy",
		"timestamp":  time.Now(),
		"components": map[string]interface{}{},
	}

	// Check embeddings database
	if m.embeddingsDB != nil {
		health["components"].(map[string]interface{})["embeddings"] = "healthy"
	} else {
		health["components"].(map[string]interface{})["embeddings"] = "not_initialized"
		health["status"] = "degraded"
	}

	// Check plugins
	if m.pluginRegistry != nil {
		pluginHealth := m.pluginRegistry.HealthCheck(ctx)
		hasUnhealthyPlugins := false
		for _, err := range pluginHealth {
			if err != nil {
				hasUnhealthyPlugins = true
				break
			}
		}

		if hasUnhealthyPlugins {
			health["components"].(map[string]interface{})["plugins"] = "degraded"
			health["status"] = "degraded"
		} else {
			health["components"].(map[string]interface{})["plugins"] = "healthy"
		}
	}

	// Add performance metrics
	if m.profiler != nil {
		memoryMB := m.profiler.MemoryUsageMB()
		health["memory_usage_mb"] = memoryMB

		if memoryMB > 1000 { // 1GB threshold
			health["status"] = "degraded"
		}
	}

	return health
}

// Shutdown gracefully shuts down all manager components
func (m *Manager) Shutdown(ctx context.Context) error {
	m.logger.Info(ctx, "Shutting down AI Assistant Manager")

	// Shutdown plugins
	if m.pluginRegistry != nil {
		m.pluginRegistry.ShutdownAll(ctx)
	}

	// Clear caches
	if m.cacheManager != nil {
		m.cacheManager.ClearAll(ctx)
	}

	// Force final GC
	if m.profiler != nil {
		m.profiler.ForceGC(ctx)
	}

	m.logger.Info(ctx, "AI Assistant Manager shutdown complete")
	return nil
}
