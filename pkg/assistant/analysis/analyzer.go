package analysis

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/simple-container-com/api/pkg/assistant/embeddings"
)

// LLMProvider interface for project analysis enhancement
type LLMProvider interface {
	GenerateResponse(ctx context.Context, prompt string) (string, error)
}

// ProgressReporter interface for reporting analysis progress
type ProgressReporter interface {
	ReportProgress(phase string, message string, percentage int)
}

// NoOpProgressReporter is a no-op implementation for backward compatibility
type NoOpProgressReporter struct{}

func (n *NoOpProgressReporter) ReportProgress(phase string, message string, percentage int) {
	// Do nothing
}

// AnalysisMode defines the depth of analysis
type AnalysisMode int

const (
	QuickMode AnalysisMode = iota // Fast analysis for chat startup
	FullMode                      // Comprehensive analysis for reports
)

// ProjectAnalyzer orchestrates tech stack detection and generates recommendations
type ProjectAnalyzer struct {
	detectors          []TechStackDetector
	resourceDetectors  []ResourceDetector
	llmProvider        LLMProvider
	embeddingsDB       *embeddings.Database
	progressReporter   ProgressReporter
	progressTracker    *ProgressTracker
	maxTokens          int          // Maximum tokens to send to LLM (0 = no limit)
	skipLLMIfExpensive bool         // Skip LLM if estimated tokens exceed maxTokens
	analysisMode       AnalysisMode // Quick vs Full analysis mode
	skipGitAnalysis    bool         // Skip heavy git operations
	skipComplexity     bool         // Skip code complexity analysis
	skipTestFiles      bool         // Skip test files and testdata directories
	skipExamples       bool         // Skip example files and documentation
}

// NewProjectAnalyzer creates a new analyzer with default detectors
func NewProjectAnalyzer() *ProjectAnalyzer {
	// Initialize embeddings database for context enrichment
	embeddingsDB, _ := embeddings.LoadEmbeddedDatabase(context.Background())

	return &ProjectAnalyzer{
		detectors: []TechStackDetector{
			&SimpleContainerDetector{},
			&NodeJSDetector{},
			&PythonDetector{},
			&GoDetector{},
			&TerraformDetector{},
			&PulumiDetector{},
			&DockerDetector{},
		},
		resourceDetectors: []ResourceDetector{
			&EnvironmentVariableDetector{},
			&SecretDetector{},
			&DatabaseDetector{},
			&QueueDetector{},
			&StorageDetector{},
			&ExternalAPIDetector{},
		},
		llmProvider:        nil, // Can be set later with SetLLMProvider
		embeddingsDB:       embeddingsDB,
		progressReporter:   &NoOpProgressReporter{},
		maxTokens:          2000,      // Default reasonable limit
		skipLLMIfExpensive: true,      // Skip by default to save costs
		analysisMode:       QuickMode, // Default to quick analysis for chat
		skipGitAnalysis:    true,      // Skip heavy git operations by default
		skipComplexity:     true,      // Skip complexity analysis by default
		skipTestFiles:      true,      // Skip test files for faster analysis
		skipExamples:       true,      // Skip example/doc files for faster analysis
	}
}

// SetLLMProvider sets the LLM provider for enhanced analysis
func (pa *ProjectAnalyzer) SetLLMProvider(provider LLMProvider) {
	pa.llmProvider = provider
}

// NewProjectAnalyzerWithEmbeddings creates an analyzer with existing embeddings DB (for reuse)
func NewProjectAnalyzerWithEmbeddings(embeddingsDB *embeddings.Database) *ProjectAnalyzer {
	return &ProjectAnalyzer{
		detectors: []TechStackDetector{
			&SimpleContainerDetector{},
			&NodeJSDetector{},
			&PythonDetector{},
			&GoDetector{},
			&TerraformDetector{},
			&PulumiDetector{},
			&DockerDetector{},
		},
		resourceDetectors: []ResourceDetector{
			&EnvironmentVariableDetector{},
			&SecretDetector{},
			&DatabaseDetector{},
			&QueueDetector{},
			&StorageDetector{},
			&ExternalAPIDetector{},
		},
		llmProvider:        nil,
		embeddingsDB:       embeddingsDB,
		progressReporter:   &NoOpProgressReporter{},
		maxTokens:          2000,      // Default reasonable limit
		skipLLMIfExpensive: true,      // Skip by default to save costs
		analysisMode:       QuickMode, // Default to quick analysis for chat
		skipGitAnalysis:    true,      // Skip heavy git operations by default
		skipComplexity:     true,      // Skip complexity analysis by default
		skipTestFiles:      true,      // Skip test files for faster analysis
		skipExamples:       true,      // Skip example/doc files for faster analysis
	}
}

// SetProgressReporter sets the progress reporter for analysis feedback
func (pa *ProjectAnalyzer) SetProgressReporter(reporter ProgressReporter) {
	pa.progressReporter = reporter

	// Initialize progress tracker with dynamic phase weights based on actual detector counts
	pa.progressTracker = NewProgressTracker(
		reporter,
		len(pa.detectors),         // Total tech stack detectors
		len(pa.resourceDetectors), // Total resource detectors
	)
}

// SetTokenLimit configures maximum tokens to send to LLM
func (pa *ProjectAnalyzer) SetTokenLimit(maxTokens int, skipIfExpensive bool) {
	pa.maxTokens = maxTokens
	pa.skipLLMIfExpensive = skipIfExpensive
}

// EnableLLMEnhancement enables LLM with unlimited token usage (expensive)
func (pa *ProjectAnalyzer) EnableLLMEnhancement() {
	pa.maxTokens = 0 // No limit
	pa.skipLLMIfExpensive = false
}

// SetAnalysisMode configures the analysis depth
func (pa *ProjectAnalyzer) SetAnalysisMode(mode AnalysisMode) {
	pa.analysisMode = mode

	switch mode {
	case QuickMode:
		pa.skipGitAnalysis = true
		pa.skipComplexity = true
		pa.skipTestFiles = true
		pa.skipExamples = true
		pa.maxTokens = 1000
		pa.skipLLMIfExpensive = true
	case FullMode:
		pa.skipGitAnalysis = false
		pa.skipComplexity = false
		pa.skipTestFiles = false
		pa.skipExamples = false
		pa.maxTokens = 0 // No limit
		pa.skipLLMIfExpensive = false
	}
}

// EnableFullAnalysis enables comprehensive analysis (git, complexity, LLM)
func (pa *ProjectAnalyzer) EnableFullAnalysis() {
	pa.SetAnalysisMode(FullMode)
}

// AddDetector adds a custom detector
func (pa *ProjectAnalyzer) AddDetector(detector TechStackDetector) {
	pa.detectors = append(pa.detectors, detector)

	// Re-sort by priority
	sort.Slice(pa.detectors, func(i, j int) bool {
		return pa.detectors[i].Priority() > pa.detectors[j].Priority()
	})
}

// AddResourceDetector adds a custom resource detector
func (pa *ProjectAnalyzer) AddResourceDetector(detector ResourceDetector) {
	pa.resourceDetectors = append(pa.resourceDetectors, detector)

	// Re-sort by priority
	sort.Slice(pa.resourceDetectors, func(i, j int) bool {
		return pa.resourceDetectors[i].Priority() > pa.resourceDetectors[j].Priority()
	})
}

// AnalyzeProject performs comprehensive project analysis
func (pa *ProjectAnalyzer) AnalyzeProject(projectPath string) (*ProjectAnalysis, error) {
	// Get absolute path for better display
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		absPath = projectPath
	}

	// Validate project path
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("project path does not exist: %s", absPath)
	}

	// Initialize progress tracker if not already set
	if pa.progressTracker == nil {
		pa.progressTracker = NewProgressTracker(
			pa.progressReporter,
			len(pa.detectors),
			len(pa.resourceDetectors),
		)
	}

	// Complete initialization phase immediately
	pa.progressTracker.CompletePhase("initialization", fmt.Sprintf("Analyzing project at %s", absPath))

	analysis := &ProjectAnalysis{
		Path: projectPath,
		Name: filepath.Base(projectPath),
		Metadata: map[string]interface{}{
			"analyzed_at":      time.Now(),
			"analyzer_version": "1.0",
		},
	}

	// Start tech stack detection phase
	pa.progressTracker.StartPhase("tech_stack")
	detectedStacks := pa.runTechStackDetectorsParallel(projectPath)

	if len(detectedStacks) == 0 {
		return nil, fmt.Errorf("no technology stacks detected in project")
	}

	analysis.TechStacks = detectedStacks

	// Determine primary stack (highest confidence)
	primaryStack := &detectedStacks[0]
	for i := 1; i < len(detectedStacks); i++ {
		if detectedStacks[i].Confidence > primaryStack.Confidence {
			primaryStack = &detectedStacks[i]
		}
	}
	analysis.PrimaryStack = primaryStack

	// Calculate overall confidence
	totalConfidence := float32(0)
	for _, stack := range detectedStacks {
		totalConfidence += stack.Confidence
	}
	analysis.Confidence = totalConfidence / float32(len(detectedStacks))

	// Detect architecture patterns
	pa.progressTracker.StartPhase("architecture")
	analysis.Architecture = pa.detectArchitecture(detectedStacks, projectPath)
	pa.progressTracker.CompletePhase("architecture", "Detected architecture: "+analysis.Architecture)

	// Generate recommendations
	pa.progressTracker.StartPhase("recommendations")
	analysis.Recommendations = pa.generateRecommendations(analysis)
	pa.progressTracker.CompletePhase("recommendations", fmt.Sprintf("Generated %d initial recommendations", len(analysis.Recommendations)))

	// Run independent analysis phases concurrently using errgroup
	pa.progressTracker.StartPhase("parallel_analysis")

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(3) // Limit concurrent operations

	var files []FileInfo
	var resources *ResourceAnalysis
	var gitAnalysis *GitAnalysis

	// File analysis
	g.Go(func() error {
		if fileResult, err := pa.analyzeFiles(projectPath); err == nil {
			files = fileResult
		}
		pa.progressTracker.CompleteTask("parallel_analysis", "File analysis completed")
		return nil
	})

	// Resource detection
	g.Go(func() error {
		if resourceResult, err := pa.analyzeResourcesParallel(projectPath); err == nil {
			resources = resourceResult
		}
		pa.progressTracker.CompleteTask("parallel_analysis", "Resource detection completed")
		return nil
	})

	// Git analysis
	g.Go(func() error {
		gitAnalyzer := NewGitAnalyzer(projectPath)
		var err error
		if !pa.skipGitAnalysis {
			gitAnalysis, err = gitAnalyzer.AnalyzeGitRepository()
		} else {
			gitAnalysis, err = gitAnalyzer.GetBasicGitInfo()
		}
		if err != nil {
			gitAnalysis = nil // Ignore git errors
		}
		pa.progressTracker.CompleteTask("parallel_analysis", "Git analysis completed")
		return nil
	})

	// Wait for all concurrent operations to complete
	_ = g.Wait() // Ignore errors as individual operations handle their own failures

	// Assign results
	analysis.Files = files
	analysis.Resources = resources
	analysis.Git = gitAnalysis

	// Generate enhanced recommendations based on detected resources
	pa.progressTracker.StartPhase("enhanced_recommendations")
	analysis.Recommendations = pa.generateEnhancedRecommendations(analysis)
	pa.progressTracker.CompletePhase("enhanced_recommendations", fmt.Sprintf("Generated %d contextual recommendations", len(analysis.Recommendations)))

	// Enhance analysis with LLM insights
	if pa.llmProvider != nil {
		pa.progressTracker.StartPhase("llm_enhancement")
		if enhanced, err := pa.enhanceWithLLM(context.Background(), analysis); err == nil {
			analysis = enhanced
			pa.progressTracker.CompletePhase("llm_enhancement", "Analysis enhanced with AI insights")
		} else {
			pa.progressTracker.CompletePhase("llm_enhancement", "LLM enhancement skipped")
		}
	}

	// Save detailed analysis report for future LLM reference
	if err := pa.saveAnalysisReport(analysis, projectPath); err != nil {
		// Log error but don't fail the analysis
		pa.progressReporter.ReportProgress("completion", fmt.Sprintf("Analysis complete! (Warning: Could not save report: %v)", err), 100)
	} else {
		pa.progressReporter.ReportProgress("completion", "Analysis complete! Report saved to .sc/analysis-report.md", 100)
	}

	return analysis, nil
}
