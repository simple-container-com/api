package analysis

import (
	"context"
	"fmt"
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

	// Track progress for resource detection
	var completedResourceDetectors int
	totalResourceDetectors := len(pa.resourceDetectors)

	// Start resource detection phase
	if pa.progressTracker != nil {
		pa.progressTracker.StartPhase("resource_analysis")
	}

	// Use errgroup for better parallel execution
	g, _ := errgroup.WithContext(context.Background())

	// Limit concurrent resource detectors to avoid overwhelming the file system
	g.SetLimit(3)

	// In QuickMode, use faster, more targeted detectors but include all important resources for Simple Container
	if pa.analysisMode == QuickMode {
		// Use fast environment variable detector only
		fastEnvDetector := &FastEnvironmentVariableDetector{analyzer: pa}
		g.Go(func() error {
			if analysis, err := fastEnvDetector.Detect(projectPath); err == nil && analysis != nil {
				mu.Lock()
				combined.EnvironmentVars = append(combined.EnvironmentVars, analysis.EnvironmentVars...)
				completedResourceDetectors++
				if pa.progressTracker != nil {
					pa.progressTracker.CompleteTask("resource_analysis",
						fmt.Sprintf("Detected environment variables (%d/%d resource detectors)", completedResourceDetectors, totalResourceDetectors))
				}
				mu.Unlock()
			} else {
				mu.Lock()
				completedResourceDetectors++
				if pa.progressTracker != nil {
					pa.progressTracker.CompleteTask("resource_analysis",
						fmt.Sprintf("Running resource detectors (%d/%d completed)", completedResourceDetectors, totalResourceDetectors))
				}
				mu.Unlock()
			}
			return nil
		})

		// Run most resource detectors in QuickMode since they're essential for Simple Container configs
		// Only skip complex detectors that have heavy performance impact
		simpleContainerDetectors := []ResourceDetector{}
		for _, detector := range pa.resourceDetectors {
			switch detector.Name() {
			case "database", "secret", "queue", "storage", "external_api":
				// Include all important resources for Simple Container configuration
				simpleContainerDetectors = append(simpleContainerDetectors, detector)
			case "environment_variable":
				// Skip regular env detector since we use FastEnvironmentVariableDetector
				continue
			default:
				// Include other detectors by default unless they prove to be performance bottlenecks
				simpleContainerDetectors = append(simpleContainerDetectors, detector)
			}
		}

		for _, detector := range simpleContainerDetectors {
			detector := detector
			g.Go(func() error {
				if analysis, err := detector.Detect(projectPath); err == nil && analysis != nil {
					mu.Lock()
					// Merge all resource types
					combined.Secrets = append(combined.Secrets, analysis.Secrets...)
					combined.Databases = append(combined.Databases, analysis.Databases...)
					combined.Queues = append(combined.Queues, analysis.Queues...)
					combined.Storage = append(combined.Storage, analysis.Storage...)
					combined.ExternalAPIs = append(combined.ExternalAPIs, analysis.ExternalAPIs...)
					completedResourceDetectors++
					if pa.progressTracker != nil {
						resourceCount := len(analysis.Secrets) + len(analysis.Databases) + len(analysis.Queues) + len(analysis.Storage) + len(analysis.ExternalAPIs)
						if resourceCount > 0 {
							pa.progressTracker.CompleteTask("resource_analysis",
								fmt.Sprintf("Detected %s resources (%d/%d resource detectors)", detector.Name(), completedResourceDetectors, totalResourceDetectors))
						} else {
							pa.progressTracker.CompleteTask("resource_analysis",
								fmt.Sprintf("Running resource detectors (%d/%d completed)", completedResourceDetectors, totalResourceDetectors))
						}
					}
					mu.Unlock()
				} else {
					mu.Lock()
					completedResourceDetectors++
					if pa.progressTracker != nil {
						pa.progressTracker.CompleteTask("resource_analysis",
							fmt.Sprintf("Running resource detectors (%d/%d completed)", completedResourceDetectors, totalResourceDetectors))
					}
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
					completedResourceDetectors++
					if pa.progressTracker != nil {
						resourceCount := len(analysis.EnvironmentVars) + len(analysis.Secrets) + len(analysis.Databases) + len(analysis.Queues) + len(analysis.Storage) + len(analysis.ExternalAPIs)
						if resourceCount > 0 {
							pa.progressTracker.CompleteTask("resource_analysis",
								fmt.Sprintf("Detected %s resources (%d/%d resource detectors)", detector.Name(), completedResourceDetectors, totalResourceDetectors))
						} else {
							pa.progressTracker.CompleteTask("resource_analysis",
								fmt.Sprintf("Running resource detectors (%d/%d completed)", completedResourceDetectors, totalResourceDetectors))
						}
					}
					mu.Unlock()
				} else {
					mu.Lock()
					completedResourceDetectors++
					if pa.progressTracker != nil {
						pa.progressTracker.CompleteTask("resource_analysis",
							fmt.Sprintf("Running resource detectors (%d/%d completed)", completedResourceDetectors, totalResourceDetectors))
					}
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

	// Skip test files and directories in QuickMode and CachedMode
	if pa.analysisMode == QuickMode || pa.analysisMode == CachedMode {
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

	// Skip example/documentation files in QuickMode and CachedMode
	if pa.analysisMode == QuickMode || pa.analysisMode == CachedMode {
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
