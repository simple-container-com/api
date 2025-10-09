package analysis

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	QuickMode     AnalysisMode = iota // Fast analysis for chat startup
	FullMode                          // Comprehensive analysis for reports
	CachedMode                        // Load from cache if available
	ForceFullMode                     // Force full analysis even if cache exists
	SetupMode                         // For project setup, includes resource confirmation
)

// ProjectAnalyzer orchestrates tech stack detection and generates recommendations
type ProjectAnalyzer struct {
	// Core detector registries (full sets available)
	allTechStackDetectors []TechStackDetector
	allResourceDetectors  []ResourceDetector

	// Active detectors for current mode
	detectors         []TechStackDetector
	resourceDetectors []ResourceDetector

	llmProvider      LLMProvider
	embeddingsDB     *embeddings.Database
	progressReporter ProgressReporter
	progressTracker  *ProgressTracker
	analysisMode     AnalysisMode

	// Mode-specific configurations
	maxTokens          int  // Maximum tokens to send to LLM (0 = no limit)
	skipLLMIfExpensive bool // Skip LLM if estimated tokens exceed maxTokens
	enableComplexity   bool // Enable code complexity analysis
	enableGitAnalysis  bool // Enable git operations
	enableFileAnalysis bool // Enable detailed file analysis
}

// NewProjectAnalyzer creates a new analyzer with default detectors
func NewProjectAnalyzer() *ProjectAnalyzer {
	// Initialize embeddings database for context enrichment
	embeddingsDB, _ := embeddings.LoadEmbeddedDatabase(context.Background())

	// Define all available detectors
	allTechDetectors := []TechStackDetector{
		&SimpleContainerDetector{},
		&NodeJSDetector{},
		&PythonDetector{},
		&GoDetector{},
		&TerraformDetector{},
		&PulumiDetector{},
		&DockerDetector{},
	}

	allResourceDetectors := []ResourceDetector{
		&EnvironmentVariableDetector{},
		&SecretDetector{},
		&DatabaseDetector{},
		&QueueDetector{},
		&StorageDetector{},
		&ExternalAPIDetector{},
	}

	pa := &ProjectAnalyzer{
		allTechStackDetectors: allTechDetectors,
		allResourceDetectors:  allResourceDetectors,
		llmProvider:           nil, // Can be set later with SetLLMProvider
		embeddingsDB:          embeddingsDB,
		progressReporter:      &NoOpProgressReporter{},
		analysisMode:          QuickMode, // Default to quick analysis
	}

	// Configure detectors for the default mode
	pa.configureDetectorsForMode(QuickMode)

	return pa
}

// SetLLMProvider sets the LLM provider for enhanced analysis
func (pa *ProjectAnalyzer) SetLLMProvider(provider LLMProvider) {
	pa.llmProvider = provider
}

// NewProjectAnalyzerWithEmbeddings creates an analyzer with existing embeddings DB (for reuse)
func NewProjectAnalyzerWithEmbeddings(embeddingsDB *embeddings.Database) *ProjectAnalyzer {
	// Define all available detectors
	allTechDetectors := []TechStackDetector{
		&SimpleContainerDetector{},
		&NodeJSDetector{},
		&PythonDetector{},
		&GoDetector{},
		&TerraformDetector{},
		&PulumiDetector{},
		&DockerDetector{},
	}

	allResourceDetectors := []ResourceDetector{
		&EnvironmentVariableDetector{},
		&SecretDetector{},
		&DatabaseDetector{},
		&QueueDetector{},
		&StorageDetector{},
		&ExternalAPIDetector{},
	}

	pa := &ProjectAnalyzer{
		allTechStackDetectors: allTechDetectors,
		allResourceDetectors:  allResourceDetectors,
		llmProvider:           nil,
		embeddingsDB:          embeddingsDB,
		progressReporter:      &NoOpProgressReporter{},
		analysisMode:          QuickMode, // Default to quick analysis
	}

	// Configure detectors for the default mode
	pa.configureDetectorsForMode(QuickMode)

	return pa
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

// SetAnalysisMode configures the analysis depth and active detectors
func (pa *ProjectAnalyzer) SetAnalysisMode(mode AnalysisMode) {
	pa.analysisMode = mode
	pa.configureDetectorsForMode(mode)
}

// configureDetectorsForMode configures active detectors based on analysis mode
func (pa *ProjectAnalyzer) configureDetectorsForMode(mode AnalysisMode) {
	// Core tech stack detectors (always active for basic project detection)
	coreTechDetectors := []TechStackDetector{
		&NodeJSDetector{},
		&PythonDetector{},
		&GoDetector{},
	}

	// Extended tech stack detectors
	extendedTechDetectors := []TechStackDetector{
		&SimpleContainerDetector{},
		&TerraformDetector{},
		&PulumiDetector{},
		&DockerDetector{},
	}

	// Lightweight resource detectors (minimal overhead)
	lightweightResourceDetectors := []ResourceDetector{
		&EnvironmentVariableDetector{},
	}

	// Full resource detectors (can be expensive)
	fullResourceDetectors := []ResourceDetector{
		&SecretDetector{},
		&DatabaseDetector{},
		&QueueDetector{},
		&StorageDetector{},
		&ExternalAPIDetector{},
	}

	switch mode {
	case QuickMode:
		// Minimal detectors for fast startup
		pa.detectors = coreTechDetectors
		pa.resourceDetectors = lightweightResourceDetectors
		pa.maxTokens = 1000
		pa.skipLLMIfExpensive = true
		pa.enableComplexity = false
		pa.enableGitAnalysis = false
		pa.enableFileAnalysis = false

	case FullMode, ForceFullMode:
		// All detectors for comprehensive analysis
		pa.detectors = append(coreTechDetectors, extendedTechDetectors...)
		pa.resourceDetectors = append(lightweightResourceDetectors, fullResourceDetectors...)
		pa.maxTokens = 0 // No limit
		pa.skipLLMIfExpensive = false
		pa.enableComplexity = true
		pa.enableGitAnalysis = true
		pa.enableFileAnalysis = true

	case CachedMode:
		// Core detectors, but will prefer cache over resource detection
		pa.detectors = append(coreTechDetectors, extendedTechDetectors...)
		pa.resourceDetectors = []ResourceDetector{} // Empty - will load from cache
		pa.maxTokens = 1000
		pa.skipLLMIfExpensive = true
		pa.enableComplexity = false
		pa.enableGitAnalysis = false
		pa.enableFileAnalysis = false

	case SetupMode:
		// Core + extended tech detectors, selective resource detectors with user confirmation
		pa.detectors = append(coreTechDetectors, extendedTechDetectors...)
		pa.resourceDetectors = append(lightweightResourceDetectors, fullResourceDetectors...)
		pa.maxTokens = 2000
		pa.skipLLMIfExpensive = false
		pa.enableComplexity = false // Skip complexity for setup but enable resources
		pa.enableGitAnalysis = true
		pa.enableFileAnalysis = false
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

// AnalyzeProject performs project analysis with caching support
func (pa *ProjectAnalyzer) AnalyzeProject(projectPath string) (*ProjectAnalysis, error) {
	// Try to load from cache first (unless ForceFullMode)
	if pa.analysisMode != ForceFullMode {
		if cache, err := LoadAnalysisCache(projectPath); err == nil {
			pa.progressReporter.ReportProgress("completion", "Loaded analysis from cache", 100)
			return ConvertCacheToAnalysis(cache), nil
		}
	}

	return pa.analyzeProjectFresh(projectPath)
}

// analyzeProjectFresh performs fresh analysis without cache
func (pa *ProjectAnalyzer) analyzeProjectFresh(projectPath string) (*ProjectAnalysis, error) {
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

	// File analysis (conditional)
	g.Go(func() error {
		if pa.enableFileAnalysis {
			if fileResult, err := pa.analyzeFiles(projectPath); err == nil {
				files = fileResult
			}
			pa.progressTracker.CompleteTask("parallel_analysis", "File analysis completed")
		} else {
			pa.progressTracker.CompleteTask("parallel_analysis", "File analysis skipped")
		}
		return nil
	})

	// Resource detection (conditional based on mode and detectors)
	g.Go(func() error {
		if len(pa.resourceDetectors) == 0 {
			// Try to load resources from cache (CachedMode)
			if cachedResources, err := GetResourcesFromCache(projectPath); err == nil {
				resources = cachedResources
				pa.progressTracker.CompleteTask("parallel_analysis", "Resource detection loaded from cache")
			} else {
				pa.progressTracker.CompleteTask("parallel_analysis", "Resource detection skipped (no cache)")
			}
		} else if pa.analysisMode == SetupMode {
			// Ask user for confirmation before resource analysis
			if pa.confirmResourceAnalysis() {
				if resourceResult, err := pa.analyzeResourcesParallel(projectPath); err == nil {
					resources = resourceResult
				}
				pa.progressTracker.CompleteTask("parallel_analysis", "Resource detection completed")
			} else {
				pa.progressTracker.CompleteTask("parallel_analysis", "Resource detection skipped by user")
			}
		} else {
			// Standard resource detection with configured detectors
			if resourceResult, err := pa.analyzeResourcesParallel(projectPath); err == nil {
				resources = resourceResult
			}
			pa.progressTracker.CompleteTask("parallel_analysis", "Resource detection completed")
		}
		return nil
	})

	// Git analysis (conditional)
	g.Go(func() error {
		if pa.enableGitAnalysis {
			gitAnalyzer := NewGitAnalyzer(projectPath)
			var err error
			gitAnalysis, err = gitAnalyzer.AnalyzeGitRepository()
			if err != nil {
				gitAnalysis = nil // Ignore git errors
			}
			pa.progressTracker.CompleteTask("parallel_analysis", "Git analysis completed")
		} else {
			pa.progressTracker.CompleteTask("parallel_analysis", "Git analysis skipped")
		}
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

	// Save cache for future quick access
	if err := SaveAnalysisCache(analysis, projectPath); err != nil {
		// Log warning but don't fail
		fmt.Printf("⚠️ Warning: Could not save analysis cache: %v\n", err)
	}

	// Save detailed analysis report for future LLM reference
	if err := pa.saveAnalysisReport(analysis, projectPath); err != nil {
		// Log error but don't fail the analysis
		pa.progressReporter.ReportProgress("completion", fmt.Sprintf("Analysis complete! (Warning: Could not save report: %v)", err), 100)
	} else {
		pa.progressReporter.ReportProgress("completion", "Analysis complete! Reports saved to .sc/analysis-*.md/json", 100)
	}

	return analysis, nil
}

// confirmResourceAnalysis asks user for confirmation before running resource analysis
func (pa *ProjectAnalyzer) confirmResourceAnalysis() bool {
	fmt.Printf("\n🔍 Resource detection can take some time on large projects (like those with node_modules).\n")
	fmt.Printf("   This analyzes your code to detect databases, APIs, storage systems, etc.\n")
	fmt.Printf("   \n")
	fmt.Printf("   💡 This is mainly useful for initial project setup.\n")
	fmt.Printf("   \n")
	fmt.Printf("   Run resource detection? [y/N]: ")

	var response string
	if _, err := fmt.Scanln(&response); err != nil {
		// If there's an error reading input, default to "no"
		return false
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}
