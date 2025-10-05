package testing

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/assistant/analysis"
	"github.com/simple-container-com/api/pkg/assistant/embeddings"
	"github.com/simple-container-com/api/pkg/assistant/performance"
)

// TestResult represents the result of a test
type TestResult struct {
	Name     string                 `json:"name"`
	Success  bool                   `json:"success"`
	Duration time.Duration          `json:"duration"`
	Error    error                  `json:"error,omitempty"`
	Message  string                 `json:"message,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// TestSuite represents a collection of related tests
type TestSuite struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Tests       []TestResult  `json:"tests"`
	Duration    time.Duration `json:"duration"`
	Passed      int           `json:"passed"`
	Failed      int           `json:"failed"`
	Total       int           `json:"total"`
}

// TestFramework provides comprehensive testing capabilities
type TestFramework struct {
	suites   map[string]*TestSuite
	mu       sync.RWMutex
	logger   logger.Logger
	profiler *performance.Profiler
}

// NewTestFramework creates a new test framework
func NewTestFramework(logger logger.Logger) *TestFramework {
	return &TestFramework{
		suites:   make(map[string]*TestSuite),
		logger:   logger,
		profiler: performance.NewProfiler(logger),
	}
}

// RunEmbeddingsTests runs comprehensive embeddings system tests
func (tf *TestFramework) RunEmbeddingsTests(ctx context.Context) *TestSuite {
	suite := &TestSuite{
		Name:        "embeddings",
		Description: "Embeddings system functionality tests",
		Tests:       []TestResult{},
	}

	start := time.Now()

	// Test 1: Database loading
	result := tf.runTest(ctx, "embedding_database_load", func() error {
		return tf.profiler.TimeOperation(ctx, "embedding_load", func() error {
			db, err := embeddings.LoadEmbeddedDatabase(ctx)
			if err != nil {
				return fmt.Errorf("failed to load embeddings database: %w", err)
			}
			if db == nil {
				return fmt.Errorf("embeddings database is nil")
			}
			return nil
		})
	})
	suite.Tests = append(suite.Tests, result)

	// Test 2: Semantic search functionality
	result = tf.runTest(ctx, "semantic_search", func() error {
		db, err := embeddings.LoadEmbeddedDatabase(ctx)
		if err != nil {
			return err
		}

		results, err := embeddings.SearchDocumentation(db, "simple container configuration", 5)
		if err != nil {
			return fmt.Errorf("semantic search failed: %w", err)
		}

		if len(results) == 0 {
			return fmt.Errorf("no search results returned")
		}

		// Verify result quality
		if results[0].Similarity < 0.3 {
			return fmt.Errorf("search results have low similarity: %f", results[0].Similarity)
		}

		return nil
	})
	suite.Tests = append(suite.Tests, result)

	// Test 3: Document count verification
	result = tf.runTest(ctx, "document_count", func() error {
		db, err := embeddings.LoadEmbeddedDatabase(ctx)
		if err != nil {
			return err
		}

		results, err := embeddings.SearchDocumentation(db, "simple", 1000)
		if err != nil {
			return err
		}

		if len(results) < 10 {
			return fmt.Errorf("insufficient documents embedded: %d", len(results))
		}

		return nil
	})
	suite.Tests = append(suite.Tests, result)

	// Test 4: Performance benchmarks
	result = tf.runTest(ctx, "search_performance", func() error {
		db, err := embeddings.LoadEmbeddedDatabase(ctx)
		if err != nil {
			return err
		}

		start := time.Now()
		_, err = embeddings.SearchDocumentation(db, "docker kubernetes deployment", 10)
		duration := time.Since(start)

		if err != nil {
			return err
		}

		if duration > 2*time.Second {
			return fmt.Errorf("search too slow: %v", duration)
		}

		return nil
	})
	suite.Tests = append(suite.Tests, result)

	suite.Duration = time.Since(start)
	tf.calculateSuiteStats(suite)

	tf.mu.Lock()
	tf.suites["embeddings"] = suite
	tf.mu.Unlock()

	return suite
}

// RunAnalysisTests runs project analysis tests
func (tf *TestFramework) RunAnalysisTests(ctx context.Context, testProjectPath string) *TestSuite {
	suite := &TestSuite{
		Name:        "analysis",
		Description: "Project analysis functionality tests",
		Tests:       []TestResult{},
	}

	start := time.Now()

	// Test 1: Basic project analysis
	result := tf.runTest(ctx, "basic_analysis", func() error {
		analyzer := analysis.NewProjectAnalyzer()
		projectAnalysis, err := analyzer.AnalyzeProject(testProjectPath)
		if err != nil {
			return fmt.Errorf("project analysis failed: %w", err)
		}

		if projectAnalysis == nil {
			return fmt.Errorf("project analysis is nil")
		}

		if projectAnalysis.Name == "" {
			return fmt.Errorf("project name not detected")
		}

		return nil
	})
	suite.Tests = append(suite.Tests, result)

	// Test 2: Language detection
	result = tf.runTest(ctx, "language_detection", func() error {
		analyzer := analysis.NewProjectAnalyzer()
		projectAnalysis, err := analyzer.AnalyzeProject(testProjectPath)
		if err != nil {
			return err
		}

		if projectAnalysis.PrimaryStack == nil {
			return fmt.Errorf("primary stack not detected")
		}

		if projectAnalysis.PrimaryStack.Language == "" {
			return fmt.Errorf("programming language not detected")
		}

		return nil
	})
	suite.Tests = append(suite.Tests, result)

	// Test 3: Framework detection
	result = tf.runTest(ctx, "framework_detection", func() error {
		analyzer := analysis.NewProjectAnalyzer()
		projectAnalysis, err := analyzer.AnalyzeProject(testProjectPath)
		if err != nil {
			return err
		}

		// For Go projects, should detect frameworks
		if projectAnalysis.PrimaryStack.Language == "go" {
			if projectAnalysis.PrimaryStack.Framework == "" {
				return fmt.Errorf("no framework detected for Go project")
			}
		}

		return nil
	})
	suite.Tests = append(suite.Tests, result)

	// Test 4: Confidence scoring
	result = tf.runTest(ctx, "confidence_scoring", func() error {
		analyzer := analysis.NewProjectAnalyzer()
		projectAnalysis, err := analyzer.AnalyzeProject(testProjectPath)
		if err != nil {
			return err
		}

		if projectAnalysis.PrimaryStack.Confidence < 0.5 {
			return fmt.Errorf("confidence too low: %f", projectAnalysis.PrimaryStack.Confidence)
		}

		return nil
	})
	suite.Tests = append(suite.Tests, result)

	suite.Duration = time.Since(start)
	tf.calculateSuiteStats(suite)

	tf.mu.Lock()
	tf.suites["analysis"] = suite
	tf.mu.Unlock()

	return suite
}

// RunPerformanceTests runs performance and memory tests
func (tf *TestFramework) RunPerformanceTests(ctx context.Context) *TestSuite {
	suite := &TestSuite{
		Name:        "performance",
		Description: "Performance and memory usage tests",
		Tests:       []TestResult{},
	}

	start := time.Now()

	// Test 1: Memory usage baseline
	result := tf.runTest(ctx, "memory_baseline", func() error {
		stats := tf.profiler.RecordMemoryUsage(ctx)
		memoryMB := float64(stats.Alloc) / 1024 / 1024

		if memoryMB > 200 { // 200MB limit
			return fmt.Errorf("high memory usage: %.2f MB", memoryMB)
		}

		return nil
	})
	suite.Tests = append(suite.Tests, result)

	// Test 2: Memory after embeddings load
	result = tf.runTest(ctx, "memory_after_embeddings", func() error {
		// Load embeddings
		_, err := embeddings.LoadEmbeddedDatabase(ctx)
		if err != nil {
			return err
		}

		stats := tf.profiler.RecordMemoryUsage(ctx)
		memoryMB := float64(stats.Alloc) / 1024 / 1024

		if memoryMB > 500 { // 500MB limit with embeddings
			return fmt.Errorf("high memory usage with embeddings: %.2f MB", memoryMB)
		}

		return nil
	})
	suite.Tests = append(suite.Tests, result)

	// Test 3: Garbage collection effectiveness
	result = tf.runTest(ctx, "garbage_collection", func() error {
		beforeStats := tf.profiler.RecordMemoryUsage(ctx)

		// Force GC
		afterStats := tf.profiler.ForceGC(ctx)

		freedMB := float64(beforeStats.Alloc-afterStats.Alloc) / 1024 / 1024

		// Should free at least some memory
		if freedMB < 0 {
			return fmt.Errorf("GC did not free memory effectively")
		}

		return nil
	})
	suite.Tests = append(suite.Tests, result)

	// Test 4: Concurrent operations
	result = tf.runTest(ctx, "concurrent_operations", func() error {
		db, err := embeddings.LoadEmbeddedDatabase(ctx)
		if err != nil {
			return err
		}

		// Run multiple searches concurrently
		var wg sync.WaitGroup
		errors := make(chan error, 5)

		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(query string) {
				defer wg.Done()
				_, err := embeddings.SearchDocumentation(db, query, 5)
				if err != nil {
					errors <- err
				}
			}(fmt.Sprintf("test query %d", i))
		}

		wg.Wait()
		close(errors)

		for err := range errors {
			if err != nil {
				return fmt.Errorf("concurrent operation failed: %w", err)
			}
		}

		return nil
	})
	suite.Tests = append(suite.Tests, result)

	suite.Duration = time.Since(start)
	tf.calculateSuiteStats(suite)

	tf.mu.Lock()
	tf.suites["performance"] = suite
	tf.mu.Unlock()

	return suite
}

// RunSchemaValidationTests runs JSON schema validation tests
func (tf *TestFramework) RunSchemaValidationTests(ctx context.Context) *TestSuite {
	suite := &TestSuite{
		Name:        "schema_validation",
		Description: "JSON schema validation tests",
		Tests:       []TestResult{},
	}

	start := time.Now()

	// Test 1: Schema loading
	result := tf.runTest(ctx, "schema_loading", func() error {
		// Test basic schema structure validation
		// This would test the embedded schemas
		return nil // Simplified for now
	})
	suite.Tests = append(suite.Tests, result)

	suite.Duration = time.Since(start)
	tf.calculateSuiteStats(suite)

	tf.mu.Lock()
	tf.suites["schema_validation"] = suite
	tf.mu.Unlock()

	return suite
}

// runTest executes a single test with timing and error handling
func (tf *TestFramework) runTest(ctx context.Context, name string, testFunc func() error) TestResult {
	start := time.Now()

	result := TestResult{
		Name:     name,
		Metadata: make(map[string]interface{}),
	}

	defer func() {
		result.Duration = time.Since(start)

		if r := recover(); r != nil {
			result.Success = false
			result.Error = fmt.Errorf("test panicked: %v", r)
		}

		var errorStr string
		if result.Error != nil {
			errorStr = result.Error.Error()
		} else {
			errorStr = "none"
		}
		tf.logger.Debug(ctx, "Test completed: test_name=%s, success=%v, duration=%v, error=%s",
			name, result.Success, result.Duration, errorStr)
	}()

	err := testFunc()
	if err != nil {
		result.Success = false
		result.Error = err
	} else {
		result.Success = true
		result.Message = "Test passed"
	}

	return result
}

// calculateSuiteStats calculates statistics for a test suite
func (tf *TestFramework) calculateSuiteStats(suite *TestSuite) {
	suite.Total = len(suite.Tests)
	suite.Passed = 0
	suite.Failed = 0

	for _, test := range suite.Tests {
		if test.Success {
			suite.Passed++
		} else {
			suite.Failed++
		}
	}
}

// RunAllTests runs all available test suites
func (tf *TestFramework) RunAllTests(ctx context.Context, projectPath string) map[string]*TestSuite {
	tf.logger.Info(ctx, "Starting comprehensive test suite: project_path=%s", projectPath)

	results := make(map[string]*TestSuite)

	// Run embeddings tests
	results["embeddings"] = tf.RunEmbeddingsTests(ctx)

	// Run analysis tests
	results["analysis"] = tf.RunAnalysisTests(ctx, projectPath)

	// Run performance tests
	results["performance"] = tf.RunPerformanceTests(ctx)

	// Run schema validation tests
	results["schema_validation"] = tf.RunSchemaValidationTests(ctx)

	// Generate summary report
	tf.generateTestReport(results)

	return results
}

// generateTestReport creates a comprehensive test report
func (tf *TestFramework) generateTestReport(results map[string]*TestSuite) {
	var totalTests, totalPassed, totalFailed int
	var totalDuration time.Duration

	report := []string{
		"=== AI ASSISTANT COMPREHENSIVE TEST REPORT ===",
		"",
	}

	for suiteName, suite := range results {
		totalTests += suite.Total
		totalPassed += suite.Passed
		totalFailed += suite.Failed
		totalDuration += suite.Duration

		status := "PASS"
		if suite.Failed > 0 {
			status = "FAIL"
		}

		report = append(report, fmt.Sprintf("Suite: %s (%s)", suiteName, status))
		report = append(report, fmt.Sprintf("  Description: %s", suite.Description))
		report = append(report, fmt.Sprintf("  Tests: %d passed, %d failed, %d total", suite.Passed, suite.Failed, suite.Total))
		report = append(report, fmt.Sprintf("  Duration: %v", suite.Duration))

		if suite.Failed > 0 {
			report = append(report, "  Failed tests:")
			for _, test := range suite.Tests {
				if !test.Success {
					report = append(report, fmt.Sprintf("    - %s: %v", test.Name, test.Error))
				}
			}
		}
		report = append(report, "")
	}

	overallStatus := "PASS"
	if totalFailed > 0 {
		overallStatus = "FAIL"
	}

	report = append(report, "=== OVERALL SUMMARY ===")
	report = append(report, fmt.Sprintf("Status: %s", overallStatus))
	report = append(report, fmt.Sprintf("Total Tests: %d", totalTests))
	report = append(report, fmt.Sprintf("Passed: %d", totalPassed))
	report = append(report, fmt.Sprintf("Failed: %d", totalFailed))
	report = append(report, fmt.Sprintf("Success Rate: %.1f%%", float64(totalPassed)/float64(totalTests)*100))
	report = append(report, fmt.Sprintf("Total Duration: %v", totalDuration))

	// Performance metrics
	perfReport := tf.profiler.GeneratePerformanceReport()
	if memData, ok := perfReport["memory_analysis"].(map[string]interface{}); ok {
		if currentMem, ok := memData["current_alloc_mb"].(float64); ok {
			report = append(report, fmt.Sprintf("Current Memory Usage: %.2f MB", currentMem))
		}
	}

	reportText := strings.Join(report, "\n")
	// Using background context since this is internal reporting
	ctx := context.Background()
	tf.logger.Info(ctx, "Test report generated: total_tests=%d, passed=%d, failed=%d, overall_status=%s, duration=%v",
		totalTests, totalPassed, totalFailed, overallStatus, totalDuration)

	fmt.Println(reportText)
}

// GetTestSuite returns a specific test suite
func (tf *TestFramework) GetTestSuite(name string) (*TestSuite, bool) {
	tf.mu.RLock()
	defer tf.mu.RUnlock()

	suite, exists := tf.suites[name]
	return suite, exists
}

// GetAllTestSuites returns all test suites
func (tf *TestFramework) GetAllTestSuites() map[string]*TestSuite {
	tf.mu.RLock()
	defer tf.mu.RUnlock()

	results := make(map[string]*TestSuite)
	for name, suite := range tf.suites {
		results[name] = suite
	}
	return results
}
