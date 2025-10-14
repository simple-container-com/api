package analysis

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"golang.org/x/sync/errgroup"
)

// runTechStackDetectorsParallel runs tech stack detectors in parallel using errgroup
func (pa *ProjectAnalyzer) runTechStackDetectorsParallel(projectPath string) []TechStackInfo {
	var mu sync.Mutex
	var detectedStacks []TechStackInfo
	completedCount := 0

	// Use errgroup for better parallel execution
	g, _ := errgroup.WithContext(context.Background())

	// Limit concurrent detectors to avoid resource exhaustion
	g.SetLimit(4)

	// Start all detectors in parallel
	for _, detector := range pa.detectors {
		detector := detector // capture loop variable
		g.Go(func() error {
			if stack, err := detector.Detect(projectPath); err == nil && stack != nil {
				mu.Lock()
				detectedStacks = append(detectedStacks, *stack)
				completedCount++
				// Report progress for each completed detector
				if pa.progressTracker != nil {
					pa.progressTracker.CompleteTask("tech_stack",
						fmt.Sprintf("Detected %s (%d/%d detectors)", stack.Framework, completedCount, len(pa.detectors)))
				}
				mu.Unlock()
			} else {
				mu.Lock()
				completedCount++
				if pa.progressTracker != nil {
					pa.progressTracker.CompleteTask("tech_stack",
						fmt.Sprintf("Running detectors (%d/%d completed)", completedCount, len(pa.detectors)))
				}
				mu.Unlock()
			}
			return nil // Never fail the group, just skip failed detectors
		})
	}

	// Wait for all detectors to complete
	_ = g.Wait() // Ignore errors as individual operations handle their own failures

	// Sort by confidence (highest first)
	sort.Slice(detectedStacks, func(i, j int) bool {
		return detectedStacks[i].Confidence > detectedStacks[j].Confidence
	})

	return detectedStacks
}
