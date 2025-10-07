package analysis

import (
	"context"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

// analyzeResourcesParallel runs resource detectors in parallel using errgroup
func (pa *ProjectAnalyzer) analyzeResourcesParallel(projectPath string) (*ResourceAnalysis, error) {
	var mu sync.Mutex
	combined := &ResourceAnalysis{
		EnvironmentVars: []EnvironmentVariable{},
		Secrets:         []Secret{},
		Databases:       []Database{},
		Queues:          []Queue{},
		Storage:         []Storage{},
		ExternalAPIs:    []ExternalAPI{},
	}

	// Use errgroup for better parallel execution
	g, _ := errgroup.WithContext(context.Background())

	// Limit concurrent resource detectors to avoid overwhelming the file system
	g.SetLimit(3)

	// In QuickMode, use faster, more targeted detectors
	if pa.analysisMode == QuickMode {
		// Use fast environment variable detector only
		fastEnvDetector := &FastEnvironmentVariableDetector{analyzer: pa}
		g.Go(func() error {
			if analysis, err := fastEnvDetector.Detect(projectPath); err == nil && analysis != nil {
				mu.Lock()
				combined.EnvironmentVars = append(combined.EnvironmentVars, analysis.EnvironmentVars...)
				mu.Unlock()
			}
			return nil
		})

		// Only run essential detectors in QuickMode
		essentialDetectors := []ResourceDetector{}
		for _, detector := range pa.resourceDetectors {
			switch detector.Name() {
			case "database", "secret": // Only run database and secret detection
				essentialDetectors = append(essentialDetectors, detector)
			}
		}

		for _, detector := range essentialDetectors {
			detector := detector
			g.Go(func() error {
				if analysis, err := detector.Detect(projectPath); err == nil && analysis != nil {
					mu.Lock()
					combined.Secrets = append(combined.Secrets, analysis.Secrets...)
					combined.Databases = append(combined.Databases, analysis.Databases...)
					mu.Unlock()
				}
				return nil
			})
		}
	} else {
		// In FullMode, run all resource detectors
		for _, detector := range pa.resourceDetectors {
			detector := detector // capture loop variable
			g.Go(func() error {
				if analysis, err := detector.Detect(projectPath); err == nil && analysis != nil {
					mu.Lock()
					// Merge results safely
					combined.EnvironmentVars = append(combined.EnvironmentVars, analysis.EnvironmentVars...)
					combined.Secrets = append(combined.Secrets, analysis.Secrets...)
					combined.Databases = append(combined.Databases, analysis.Databases...)
					combined.Queues = append(combined.Queues, analysis.Queues...)
					combined.Storage = append(combined.Storage, analysis.Storage...)
					combined.ExternalAPIs = append(combined.ExternalAPIs, analysis.ExternalAPIs...)
					mu.Unlock()
				}
				return nil // Never fail the group, just skip failed detectors
			})
		}
	}

	// Wait for all detectors to complete
	_ = g.Wait() // Ignore errors as individual operations handle their own failures

	return combined, nil
}

// shouldSkipPath determines if a file/directory path should be skipped for performance
func (pa *ProjectAnalyzer) shouldSkipPath(path string) bool {
	lowercasePath := strings.ToLower(path)

	// Always skip these heavy directories
	heavyDirs := []string{
		"node_modules", "vendor", ".git", ".svn", ".hg",
		"target", "build", "dist", ".next", ".nuxt",
		"__pycache__", ".pytest_cache", ".coverage",
		"coverage", "htmlcov", ".nyc_output",
	}

	for _, dir := range heavyDirs {
		if strings.Contains(lowercasePath, dir) {
			return true
		}
	}

	// Skip test files and directories in QuickMode
	if pa.skipTestFiles {
		testPatterns := []string{
			"test", "tests", "_test", ".test", "spec", "_spec", ".spec",
			"testdata", "test_", "spec_", "__tests__", ".jest",
			"__mocks__", "mock", "fixture", "fixtures",
		}

		for _, pattern := range testPatterns {
			if strings.Contains(lowercasePath, pattern) {
				return true
			}
		}
	}

	// Skip example/documentation files in QuickMode
	if pa.skipExamples {
		examplePatterns := []string{
			"example", "examples", "docs", "doc", "documentation",
			"demo", "demos", "sample", "samples", "tutorial",
			"tutorials", "guide", "guides", ".github", "readme",
			"changelog", "contributing", "license", "authors",
		}

		for _, pattern := range examplePatterns {
			if strings.Contains(lowercasePath, pattern) {
				return true
			}
		}
	}

	return false
}
