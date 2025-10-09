package analysis

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AnalysisCache represents cached analysis data
type AnalysisCache struct {
	Timestamp       time.Time              `json:"timestamp"`
	ProjectPath     string                 `json:"project_path"`
	AnalyzerVersion string                 `json:"analyzer_version"`
	Resources       *ResourceAnalysis      `json:"resources,omitempty"`
	TechStacks      []TechStackInfo        `json:"tech_stacks"`
	Architecture    string                 `json:"architecture"`
	Confidence      float32                `json:"confidence"`
	PrimaryStack    *TechStackInfo         `json:"primary_stack,omitempty"`
	Files           []FileInfo             `json:"files,omitempty"`
	Git             *GitAnalysis           `json:"git,omitempty"`
	Recommendations []Recommendation       `json:"recommendations"`
	Metadata        map[string]interface{} `json:"metadata"`
}

const (
	// Cache file paths
	AnalysisCacheFile  = ".sc/analysis-cache.json"
	AnalysisReportFile = ".sc/analysis-report.md"
	CacheValidityHours = 24    // Cache expires after 24 hours
	AnalyzerVersion    = "1.1" // Increment when cache format changes
)

// LoadAnalysisCache loads cached analysis from JSON file
func LoadAnalysisCache(projectPath string) (*AnalysisCache, error) {
	cacheFilePath := filepath.Join(projectPath, AnalysisCacheFile)

	// Check if cache file exists
	if _, err := os.Stat(cacheFilePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("cache file not found")
	}

	// Read cache file
	data, err := os.ReadFile(cacheFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	// Parse JSON
	var cache AnalysisCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("failed to parse cache file: %w", err)
	}

	// Check if cache is still valid
	if !isCacheValid(&cache, projectPath) {
		return nil, fmt.Errorf("cache is expired or invalid")
	}

	return &cache, nil
}

// SaveAnalysisCache saves analysis to JSON cache file
func SaveAnalysisCache(analysis *ProjectAnalysis, projectPath string) error {
	// Ensure .sc directory exists
	scDir := filepath.Join(projectPath, ".sc")
	if err := os.MkdirAll(scDir, 0o755); err != nil {
		return fmt.Errorf("failed to create .sc directory: %w", err)
	}

	// Create cache structure
	cache := &AnalysisCache{
		Timestamp:       time.Now(),
		ProjectPath:     projectPath,
		AnalyzerVersion: AnalyzerVersion,
		Resources:       analysis.Resources,
		TechStacks:      analysis.TechStacks,
		Architecture:    analysis.Architecture,
		Confidence:      analysis.Confidence,
		PrimaryStack:    analysis.PrimaryStack,
		Files:           analysis.Files,
		Git:             analysis.Git,
		Recommendations: analysis.Recommendations,
		Metadata:        analysis.Metadata,
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %w", err)
	}

	// Write cache file
	cacheFilePath := filepath.Join(projectPath, AnalysisCacheFile)
	if err := os.WriteFile(cacheFilePath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// isCacheValid checks if cached analysis is still valid
func isCacheValid(cache *AnalysisCache, projectPath string) bool {
	// Check analyzer version
	if cache.AnalyzerVersion != AnalyzerVersion {
		return false
	}

	// Check if cache is too old
	if time.Since(cache.Timestamp) > time.Duration(CacheValidityHours)*time.Hour {
		return false
	}

	// Check if project path matches
	absPath, _ := filepath.Abs(projectPath)
	cacheAbsPath, _ := filepath.Abs(cache.ProjectPath)
	return absPath == cacheAbsPath
}

// CacheExists checks if valid cache exists for a project
func CacheExists(projectPath string) bool {
	cache, err := LoadAnalysisCache(projectPath)
	return err == nil && cache != nil
}

// ConvertCacheToAnalysis converts cached data back to ProjectAnalysis
func ConvertCacheToAnalysis(cache *AnalysisCache) *ProjectAnalysis {
	return &ProjectAnalysis{
		Path:            cache.ProjectPath,
		Name:            filepath.Base(cache.ProjectPath),
		TechStacks:      cache.TechStacks,
		Architecture:    cache.Architecture,
		Confidence:      cache.Confidence,
		PrimaryStack:    cache.PrimaryStack,
		Resources:       cache.Resources,
		Files:           cache.Files,
		Git:             cache.Git,
		Recommendations: cache.Recommendations,
		Metadata:        cache.Metadata,
	}
}

// GetResourcesFromCache loads only resources from cache (fast operation)
func GetResourcesFromCache(projectPath string) (*ResourceAnalysis, error) {
	cache, err := LoadAnalysisCache(projectPath)
	if err != nil {
		return nil, err
	}

	return cache.Resources, nil
}

// HasResourcesInCache checks if cache contains resource analysis
func HasResourcesInCache(projectPath string) bool {
	cache, err := LoadAnalysisCache(projectPath)
	if err != nil {
		return false
	}

	return cache.Resources != nil &&
		(len(cache.Resources.Databases) > 0 ||
			len(cache.Resources.EnvironmentVars) > 0 ||
			len(cache.Resources.Secrets) > 0 ||
			len(cache.Resources.Queues) > 0 ||
			len(cache.Resources.Storage) > 0 ||
			len(cache.Resources.ExternalAPIs) > 0)
}
