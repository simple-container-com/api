package analysis

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// saveAnalysisReport saves a comprehensive analysis report for future LLM reference
func (pa *ProjectAnalyzer) saveAnalysisReport(analysis *ProjectAnalysis, projectPath string) error {
	// Create .sc directory if it doesn't exist (standard Simple Container directory)
	reportDir := filepath.Join(projectPath, ".sc")
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return fmt.Errorf("failed to create report directory: %w", err)
	}

	// Generate detailed report content
	reportContent := pa.generateDetailedReport(analysis)

	// Write report to file
	reportPath := filepath.Join(reportDir, "analysis-report.md")
	if err := os.WriteFile(reportPath, []byte(reportContent), 0o644); err != nil {
		return fmt.Errorf("failed to write analysis report: %w", err)
	}

	return nil
}

// generateDetailedReport creates a comprehensive markdown report of the analysis
func (pa *ProjectAnalyzer) generateDetailedReport(analysis *ProjectAnalysis) string {
	var report strings.Builder

	// Header
	report.WriteString("# Simple Container Project Analysis Report\n\n")
	report.WriteString(fmt.Sprintf("**Generated:** %s\n", time.Now().Format("2006-01-02 15:04:05 -07")))
	report.WriteString("**Analyzer Version:** 1.0\n")
	report.WriteString(fmt.Sprintf("**Overall Confidence:** %.1f%%\n\n", analysis.Confidence*100))

	// Project Overview
	report.WriteString("## Project Overview\n\n")
	report.WriteString(fmt.Sprintf("- **Name:** %s\n", analysis.Name))
	report.WriteString(fmt.Sprintf("- **Path:** %s\n", analysis.Path))
	report.WriteString(fmt.Sprintf("- **Architecture:** %s\n", analysis.Architecture))
	if analysis.PrimaryStack != nil {
		report.WriteString(fmt.Sprintf("- **Primary Technology:** %s %s (%.1f%% confidence)\n\n",
			analysis.PrimaryStack.Language, analysis.PrimaryStack.Framework, analysis.PrimaryStack.Confidence*100))
	}

	// Technology Stacks
	report.WriteString("## Technology Stacks\n\n")
	for i, stack := range analysis.TechStacks {
		report.WriteString(fmt.Sprintf("### %d. %s %s\n\n", i+1, stack.Language, stack.Framework))
		report.WriteString(fmt.Sprintf("- **Confidence:** %.1f%%\n", stack.Confidence*100))
		report.WriteString(fmt.Sprintf("- **Runtime:** %s\n", stack.Runtime))
		report.WriteString(fmt.Sprintf("- **Version:** %s\n", stack.Version))
		report.WriteString("- **Evidence:**\n")
		for _, evidence := range stack.Evidence {
			report.WriteString(fmt.Sprintf("  - %s\n", evidence))
		}
		if stack.Metadata != nil {
			report.WriteString("- **Additional Information:**\n")
			for key, value := range stack.Metadata {
				report.WriteString(fmt.Sprintf("  - %s: %v\n", key, value))
			}
		}
		report.WriteString("\n")
	}

	// Git Analysis
	if analysis.Git != nil && analysis.Git.IsGitRepo {
		report.WriteString("## Git Repository Analysis\n\n")
		report.WriteString(fmt.Sprintf("- **Branch:** %s\n", analysis.Git.Branch))
		report.WriteString(fmt.Sprintf("- **Remote URL:** %s\n", analysis.Git.RemoteURL))
		if analysis.Git.CommitActivity != nil {
			report.WriteString(fmt.Sprintf("- **Total Commits:** %d\n", analysis.Git.CommitActivity.TotalCommits))
			report.WriteString(fmt.Sprintf("- **Recent Commits (30d):** %d\n", analysis.Git.CommitActivity.RecentCommits))
		}
		report.WriteString(fmt.Sprintf("- **Contributors:** %d\n", len(analysis.Git.Contributors)))
		if len(analysis.Git.Contributors) > 0 {
			report.WriteString("- **Top Contributors:**\n")
			for i, contributor := range analysis.Git.Contributors {
				if i >= 5 { // Limit to top 5
					break
				}
				report.WriteString(fmt.Sprintf("  - %s (%d commits)\n", contributor.Name, contributor.Commits))
			}
		}
		report.WriteString(fmt.Sprintf("- **Has CI/CD:** %t\n", analysis.Git.HasCI))
		if len(analysis.Git.Tags) > 0 {
			report.WriteString("- **Recent Tags:** ")
			for i, tag := range analysis.Git.Tags {
				if i >= 10 { // Limit to 10 most recent
					break
				}
				if i > 0 {
					report.WriteString(", ")
				}
				report.WriteString(tag)
			}
			report.WriteString("\n")
		}
		report.WriteString("\n")
	}

	// Resources
	if analysis.Resources != nil {
		report.WriteString("## Detected Resources\n\n")

		// Databases
		if len(analysis.Resources.Databases) > 0 {
			report.WriteString("### Databases\n\n")
			for _, db := range analysis.Resources.Databases {
				report.WriteString(fmt.Sprintf("- **%s** (%.1f%% confidence)\n", db.Type, db.Confidence*100))
				if len(db.Sources) > 0 {
					report.WriteString("  - Sources: " + strings.Join(db.Sources, ", ") + "\n")
				}
				if db.Connection != "" {
					report.WriteString(fmt.Sprintf("  - Connection: %s\n", db.Connection))
				}
				if db.Recommended != "" {
					report.WriteString(fmt.Sprintf("  - Recommended Resource: %s\n", db.Recommended))
				}
			}
			report.WriteString("\n")
		}

		// External APIs
		if len(analysis.Resources.ExternalAPIs) > 0 {
			report.WriteString("### External APIs\n\n")
			for _, api := range analysis.Resources.ExternalAPIs {
				report.WriteString(fmt.Sprintf("- **%s** (%.1f%% confidence)\n", api.Name, api.Confidence*100))
				if len(api.Sources) > 0 {
					report.WriteString("  - Sources: " + strings.Join(api.Sources, ", ") + "\n")
				}
				if api.Purpose != "" {
					report.WriteString(fmt.Sprintf("  - Purpose: %s\n", api.Purpose))
				}
			}
			report.WriteString("\n")
		}

		// Storage
		if len(analysis.Resources.Storage) > 0 {
			report.WriteString("### Storage\n\n")
			for _, storage := range analysis.Resources.Storage {
				report.WriteString(fmt.Sprintf("- **%s** (%.1f%% confidence)\n", storage.Type, storage.Confidence*100))
				if len(storage.Sources) > 0 {
					report.WriteString("  - Sources: " + strings.Join(storage.Sources, ", ") + "\n")
				}
				if storage.Purpose != "" {
					report.WriteString(fmt.Sprintf("  - Purpose: %s\n", storage.Purpose))
				}
			}
			report.WriteString("\n")
		}

		// Queues
		if len(analysis.Resources.Queues) > 0 {
			report.WriteString("### Message Queues\n\n")
			for _, queue := range analysis.Resources.Queues {
				report.WriteString(fmt.Sprintf("- **%s** (%.1f%% confidence)\n", queue.Type, queue.Confidence*100))
				if len(queue.Sources) > 0 {
					report.WriteString("  - Sources: " + strings.Join(queue.Sources, ", ") + "\n")
				}
				if len(queue.Topics) > 0 {
					report.WriteString("  - Topics: " + strings.Join(queue.Topics, ", ") + "\n")
				}
			}
			report.WriteString("\n")
		}

		// Environment Variables (summary only to avoid exposing sensitive data)
		if len(analysis.Resources.EnvironmentVars) > 0 {
			report.WriteString("### Environment Variables\n\n")
			report.WriteString(fmt.Sprintf("- **Total:** %d environment variables detected\n", len(analysis.Resources.EnvironmentVars)))

			// Group by sources
			sources := make(map[string]int)
			for _, env := range analysis.Resources.EnvironmentVars {
				for _, source := range env.Sources {
					sources[source]++
				}
			}

			report.WriteString("- **Sources:**\n")
			for source, count := range sources {
				report.WriteString(fmt.Sprintf("  - %s: %d variables\n", source, count))
			}
			report.WriteString("\n")
		}

		// Secrets (summary only for security)
		if len(analysis.Resources.Secrets) > 0 {
			report.WriteString("### Detected Secrets\n\n")
			report.WriteString(fmt.Sprintf("- **Total:** %d potential secrets detected\n", len(analysis.Resources.Secrets)))

			// Group by type
			types := make(map[string]int)
			for _, secret := range analysis.Resources.Secrets {
				types[secret.Type]++
			}

			report.WriteString("- **Types:**\n")
			for secretType, count := range types {
				report.WriteString(fmt.Sprintf("  - %s: %d instances\n", secretType, count))
			}
			report.WriteString("\n")
		}
	}

	// Recommendations
	if len(analysis.Recommendations) > 0 {
		report.WriteString("## Recommendations\n\n")

		// Group by priority
		priorityGroups := map[string][]Recommendation{
			"critical": {},
			"high":     {},
			"medium":   {},
			"low":      {},
		}

		for _, rec := range analysis.Recommendations {
			priorityGroups[rec.Priority] = append(priorityGroups[rec.Priority], rec)
		}

		for _, priority := range []string{"critical", "high", "medium", "low"} {
			recs := priorityGroups[priority]
			if len(recs) == 0 {
				continue
			}

			report.WriteString(fmt.Sprintf("### %s Priority\n\n", cases.Title(language.English, cases.NoLower).String(priority)))
			for _, rec := range recs {
				report.WriteString(fmt.Sprintf("**%s**\n", rec.Title))
				report.WriteString(fmt.Sprintf("- %s\n", rec.Description))
				if rec.Action != "" {
					report.WriteString(fmt.Sprintf("- Action: %s\n", rec.Action))
				}
				if rec.Resource != "" {
					report.WriteString(fmt.Sprintf("- Resource: %s\n", rec.Resource))
				}
				report.WriteString("\n")
			}
		}
	}

	// Simple Container Setup Guide
	report.WriteString("## Simple Container Setup Guide\n\n")
	report.WriteString("Based on this analysis, here's how to get started with Simple Container:\n\n")
	report.WriteString("1. **Initialize Simple Container**\n")
	report.WriteString("   ```bash\n")
	report.WriteString("   sc init\n")
	report.WriteString("   ```\n\n")

	if analysis.PrimaryStack != nil {
		report.WriteString(fmt.Sprintf("2. **Configure for %s %s**\n", analysis.PrimaryStack.Language, analysis.PrimaryStack.Framework))
		report.WriteString("   - Simple Container will automatically detect your technology stack\n")
		report.WriteString("   - Review the generated configuration files\n\n")
	}

	report.WriteString("3. **Deploy**\n")
	report.WriteString("   ```bash\n")
	report.WriteString("   sc deploy\n")
	report.WriteString("   ```\n\n")

	report.WriteString("For more information, visit: https://simple-container.com/docs\n")

	return report.String()
}
