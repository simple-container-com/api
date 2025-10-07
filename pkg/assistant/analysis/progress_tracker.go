package analysis

import (
	"fmt"
	"sync"
)

// ProgressPhase represents a phase of analysis with its weight and sub-tasks
type ProgressPhase struct {
	Name           string
	Weight         float32 // How much this phase contributes to total progress (0.0-1.0)
	TotalTasks     int     // Total number of tasks in this phase
	CompletedTasks int     // Number of completed tasks
	Description    string  // Current description for this phase
}

// ProgressTracker manages dynamic progress reporting based on actual analyzer completion
type ProgressTracker struct {
	phases   map[string]*ProgressPhase
	reporter ProgressReporter
	mu       sync.Mutex
}

// NewProgressTracker creates a new progress tracker with predefined phases
func NewProgressTracker(reporter ProgressReporter, totalDetectors, totalResourceDetectors int) *ProgressTracker {
	tracker := &ProgressTracker{
		phases:   make(map[string]*ProgressPhase),
		reporter: reporter,
	}

	// Define phases with their weights based on typical execution time
	tracker.phases["initialization"] = &ProgressPhase{
		Name:        "initialization",
		Weight:      0.05, // 5% - very fast
		TotalTasks:  1,
		Description: "Starting analysis...",
	}

	tracker.phases["tech_stack"] = &ProgressPhase{
		Name:        "tech_stack",
		Weight:      0.15, // 15% - tech stack detection is relatively fast
		TotalTasks:  totalDetectors,
		Description: "Detecting technology stacks...",
	}

	tracker.phases["architecture"] = &ProgressPhase{
		Name:        "architecture",
		Weight:      0.05, // 5% - architecture detection is fast
		TotalTasks:  1,
		Description: "Detecting architecture patterns...",
	}

	tracker.phases["recommendations"] = &ProgressPhase{
		Name:        "recommendations",
		Weight:      0.10, // 10% - generating recommendations
		TotalTasks:  1,
		Description: "Generating initial recommendations...",
	}

	tracker.phases["parallel_analysis"] = &ProgressPhase{
		Name:        "parallel_analysis",
		Weight:      0.50, // 50% - the heaviest phase (file + resource + git analysis)
		TotalTasks:  3,    // file analysis, resource analysis, git analysis
		Description: "Running parallel analysis...",
	}

	tracker.phases["resource_analysis"] = &ProgressPhase{
		Name:        "resource_analysis",
		Weight:      0.0, // Part of parallel_analysis
		TotalTasks:  totalResourceDetectors,
		Description: "Detecting resources...",
	}

	tracker.phases["enhanced_recommendations"] = &ProgressPhase{
		Name:        "enhanced_recommendations",
		Weight:      0.10, // 10% - enhanced recommendations
		TotalTasks:  1,
		Description: "Generating contextual recommendations...",
	}

	tracker.phases["llm_enhancement"] = &ProgressPhase{
		Name:        "llm_enhancement",
		Weight:      0.05, // 5% - LLM enhancement (if enabled)
		TotalTasks:  1,
		Description: "Enhancing with AI insights...",
	}

	return tracker
}

// StartPhase marks the beginning of a phase
func (pt *ProgressTracker) StartPhase(phaseName string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if phase, exists := pt.phases[phaseName]; exists {
		phase.CompletedTasks = 0 // Reset completed tasks
		pt.reportCurrentProgress(phaseName, phase.Description)
	}
}

// CompleteTask marks one task as completed in the given phase
func (pt *ProgressTracker) CompleteTask(phaseName, taskDescription string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if phase, exists := pt.phases[phaseName]; exists {
		phase.CompletedTasks++
		if phase.CompletedTasks > phase.TotalTasks {
			phase.CompletedTasks = phase.TotalTasks // Cap at total
		}

		// Update description with task completion info
		description := taskDescription
		if phase.TotalTasks > 1 {
			description = fmt.Sprintf("%s (%d/%d completed)",
				taskDescription, phase.CompletedTasks, phase.TotalTasks)
		}

		pt.reportCurrentProgress(phaseName, description)
	}
}

// CompletePhase marks an entire phase as completed
func (pt *ProgressTracker) CompletePhase(phaseName, description string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if phase, exists := pt.phases[phaseName]; exists {
		phase.CompletedTasks = phase.TotalTasks
		pt.reportCurrentProgress(phaseName, description)
	}
}

// SetPhaseDescription updates the description for a phase
func (pt *ProgressTracker) SetPhaseDescription(phaseName, description string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if phase, exists := pt.phases[phaseName]; exists {
		phase.Description = description
		pt.reportCurrentProgress(phaseName, description)
	}
}

// reportCurrentProgress calculates and reports the current overall progress
func (pt *ProgressTracker) reportCurrentProgress(currentPhase, description string) {
	totalProgress := float32(0.0)

	for _, phase := range pt.phases {
		if phase.Weight > 0 { // Only count phases with weight > 0
			phaseProgress := float32(phase.CompletedTasks) / float32(phase.TotalTasks)
			totalProgress += phase.Weight * phaseProgress
		}
	}

	// Convert to percentage (0-100)
	percentage := int(totalProgress * 100)
	if percentage > 100 {
		percentage = 100
	}

	pt.reporter.ReportProgress(currentPhase, description, percentage)
}

// GetOverallProgress returns the current overall progress percentage
func (pt *ProgressTracker) GetOverallProgress() int {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	totalProgress := float32(0.0)

	for _, phase := range pt.phases {
		if phase.Weight > 0 {
			phaseProgress := float32(phase.CompletedTasks) / float32(phase.TotalTasks)
			totalProgress += phase.Weight * phaseProgress
		}
	}

	percentage := int(totalProgress * 100)
	if percentage > 100 {
		percentage = 100
	}

	return percentage
}

// GetPhaseProgress returns the progress of a specific phase
func (pt *ProgressTracker) GetPhaseProgress(phaseName string) (int, bool) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if phase, exists := pt.phases[phaseName]; exists {
		progress := int((float32(phase.CompletedTasks) / float32(phase.TotalTasks)) * 100)
		return progress, true
	}

	return 0, false
}
