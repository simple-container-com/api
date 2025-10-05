package performance

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/simple-container-com/api/pkg/api/logger"
)

// MemoryStats holds memory usage statistics
type MemoryStats struct {
	Alloc       uint64    `json:"alloc"`
	TotalAlloc  uint64    `json:"total_alloc"`
	Sys         uint64    `json:"sys"`
	Mallocs     uint64    `json:"mallocs"`
	Frees       uint64    `json:"frees"`
	LiveObjects uint64    `json:"live_objects"`
	PauseNs     []uint64  `json:"pause_ns"`
	NumGC       uint32    `json:"num_gc"`
	Timestamp   time.Time `json:"timestamp"`
}

// PerformanceMetrics tracks various performance metrics
type PerformanceMetrics struct {
	EmbeddingLoadTime    time.Duration `json:"embedding_load_time"`
	SchemaLoadTime       time.Duration `json:"schema_load_time"`
	LLMResponseTime      time.Duration `json:"llm_response_time"`
	SemanticSearchTime   time.Duration `json:"semantic_search_time"`
	FileGenerationTime   time.Duration `json:"file_generation_time"`
	MCPStartupTime       time.Duration `json:"mcp_startup_time"`
	ChatSessionStartTime time.Duration `json:"chat_session_start_time"`
}

// Profiler provides performance monitoring capabilities
type Profiler struct {
	metrics     PerformanceMetrics
	memoryStats []MemoryStats
	mu          sync.RWMutex
	logger      logger.Logger
}

// NewProfiler creates a new performance profiler
func NewProfiler(logger logger.Logger) *Profiler {
	return &Profiler{
		memoryStats: make([]MemoryStats, 0, 100), // Pre-allocate for efficiency
		logger:      logger,
	}
}

// RecordMemoryUsage captures current memory statistics
func (p *Profiler) RecordMemoryUsage(ctx context.Context) MemoryStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	stats := MemoryStats{
		Alloc:       m.Alloc,
		TotalAlloc:  m.TotalAlloc,
		Sys:         m.Sys,
		Mallocs:     m.Mallocs,
		Frees:       m.Frees,
		LiveObjects: m.Mallocs - m.Frees,
		PauseNs:     make([]uint64, len(m.PauseNs)),
		NumGC:       m.NumGC,
		Timestamp:   time.Now(),
	}

	copy(stats.PauseNs, m.PauseNs[:])

	p.mu.Lock()
	if len(p.memoryStats) >= 100 {
		// Keep only the last 99 entries to avoid unbounded growth
		p.memoryStats = p.memoryStats[1:]
	}
	p.memoryStats = append(p.memoryStats, stats)
	p.mu.Unlock()

	// Log high memory usage
	allocMB := float64(stats.Alloc) / 1024 / 1024
	if allocMB > 100 { // Log if using more than 100MB
		p.logger.Debug(ctx, "High memory usage detected: alloc_mb=%.2f, live_objects=%d, num_gc=%d", allocMB, stats.LiveObjects, stats.NumGC)
	}

	return stats
}

// TimeOperation measures the duration of an operation
func (p *Profiler) TimeOperation(ctx context.Context, operationType string, operation func() error) error {
	start := time.Now()
	err := operation()
	duration := time.Since(start)

	p.mu.Lock()
	switch operationType {
	case "embedding_load":
		p.metrics.EmbeddingLoadTime = duration
	case "schema_load":
		p.metrics.SchemaLoadTime = duration
	case "llm_response":
		p.metrics.LLMResponseTime = duration
	case "semantic_search":
		p.metrics.SemanticSearchTime = duration
	case "file_generation":
		p.metrics.FileGenerationTime = duration
	case "mcp_startup":
		p.metrics.MCPStartupTime = duration
	case "chat_session_start":
		p.metrics.ChatSessionStartTime = duration
	}
	p.mu.Unlock()

	// Log slow operations
	if duration > time.Second {
		p.logger.Warn(ctx, "Slow operation detected: operation=%s, duration=%s", operationType, duration.String())
	}

	return err
}

// GetMemoryStats returns current memory statistics
func (p *Profiler) GetMemoryStats() []MemoryStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := make([]MemoryStats, len(p.memoryStats))
	copy(stats, p.memoryStats)
	return stats
}

// GetPerformanceMetrics returns current performance metrics
func (p *Profiler) GetPerformanceMetrics() PerformanceMetrics {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.metrics
}

// ForceGC triggers garbage collection and measures the impact
func (p *Profiler) ForceGC(ctx context.Context) MemoryStats {
	beforeStats := p.RecordMemoryUsage(ctx)

	runtime.GC()
	runtime.GC() // Run twice to ensure thorough cleanup

	afterStats := p.RecordMemoryUsage(ctx)

	freedMB := float64(beforeStats.Alloc-afterStats.Alloc) / 1024 / 1024
	if freedMB > 10 { // Log significant memory reclaim
		p.logger.Info(ctx, "Garbage collection reclaimed memory: freed_mb=%.2f, before_alloc=%d, after_alloc=%d, live_objects=%d",
			freedMB, beforeStats.Alloc, afterStats.Alloc, afterStats.LiveObjects)
	}

	return afterStats
}

// StartMemoryMonitoring begins periodic memory monitoring
func (p *Profiler) StartMemoryMonitoring(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.RecordMemoryUsage(ctx)
			}
		}
	}()
}

// MemoryUsageMB returns current memory usage in megabytes
func (p *Profiler) MemoryUsageMB() float64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.Alloc) / 1024 / 1024
}

// GeneratePerformanceReport creates a comprehensive performance report
func (p *Profiler) GeneratePerformanceReport() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var totalAlloc, avgAlloc uint64
	var gcCount uint32

	if len(p.memoryStats) > 0 {
		for _, stats := range p.memoryStats {
			totalAlloc += stats.Alloc
			if stats.NumGC > gcCount {
				gcCount = stats.NumGC
			}
		}
		avgAlloc = totalAlloc / uint64(len(p.memoryStats))
	}

	report := map[string]interface{}{
		"memory_analysis": map[string]interface{}{
			"current_alloc_mb": p.MemoryUsageMB(),
			"average_alloc_mb": float64(avgAlloc) / 1024 / 1024,
			"gc_count":         gcCount,
			"samples_taken":    len(p.memoryStats),
		},
		"performance_metrics":          p.metrics,
		"optimization_recommendations": p.generateOptimizationRecommendations(),
	}

	return report
}

// generateOptimizationRecommendations provides performance optimization suggestions
func (p *Profiler) generateOptimizationRecommendations() []string {
	recommendations := []string{}

	currentMemMB := p.MemoryUsageMB()
	if currentMemMB > 500 {
		recommendations = append(recommendations, "Consider implementing lazy loading for embeddings to reduce memory usage")
	}

	if p.metrics.EmbeddingLoadTime > 5*time.Second {
		recommendations = append(recommendations, "Embedding load time is high - consider using pre-built embeddings")
	}

	if p.metrics.LLMResponseTime > 10*time.Second {
		recommendations = append(recommendations, "LLM response time is slow - consider implementing connection pooling")
	}

	if p.metrics.SemanticSearchTime > 2*time.Second {
		recommendations = append(recommendations, "Semantic search is slow - consider optimizing vector operations")
	}

	return recommendations
}
