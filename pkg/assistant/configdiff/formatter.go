package configdiff

import (
	"fmt"
	"strings"
)

// Formatter handles different output formats for configuration diffs
type Formatter struct {
	options DiffOptions
}

// NewFormatter creates a new Formatter instance
func NewFormatter(options DiffOptions) *Formatter {
	return &Formatter{
		options: options,
	}
}

// FormatDiff formats a ConfigDiff according to the specified format
func (f *Formatter) FormatDiff(diff *ConfigDiff) string {
	if len(diff.Changes) == 0 {
		return f.formatNoChanges(diff)
	}

	switch f.options.Format {
	case FormatUnified:
		return f.formatUnified(diff)
	case FormatSplit:
		return f.formatSplit(diff)
	case FormatInline:
		return f.formatInline(diff)
	case FormatCompact:
		return f.formatCompact(diff)
	default:
		return f.formatSplit(diff) // Default to split format
	}
}

// formatNoChanges formats the output when there are no changes
func (f *Formatter) formatNoChanges(diff *ConfigDiff) string {
	var output strings.Builder

	output.WriteString(fmt.Sprintf("📊 Configuration Diff: %s/%s.yaml\n", diff.StackName, diff.ConfigType))
	output.WriteString(fmt.Sprintf("Comparing: %s → %s\n\n", diff.CompareFrom, diff.CompareTo))
	output.WriteString("✅ No changes detected\n\n")
	output.WriteString(fmt.Sprintf("Configuration is identical to %s after resolving inheritance.", diff.CompareFrom))

	return output.String()
}

// formatUnified formats the diff in git diff style with +/-
func (f *Formatter) formatUnified(diff *ConfigDiff) string {
	var output strings.Builder

	// Header
	output.WriteString(fmt.Sprintf("📊 Configuration Diff: %s/%s.yaml\n", diff.StackName, diff.ConfigType))
	output.WriteString(fmt.Sprintf("Comparing: %s → %s\n", diff.CompareFrom, diff.CompareTo))
	output.WriteString("Resolved with inheritance applied\n\n")

	// File headers
	output.WriteString(fmt.Sprintf("--- .sc/stacks/%s/%s.yaml (%s) [resolved]\n", diff.StackName, diff.ConfigType, diff.CompareFrom))
	output.WriteString(fmt.Sprintf("+++ .sc/stacks/%s/%s.yaml (%s) [resolved]\n", diff.StackName, diff.ConfigType, diff.CompareTo))

	// Group changes by context for unified diff
	contextGroups := f.groupChangesByContext(diff.Changes)

	for i, group := range contextGroups {
		if i > 0 {
			output.WriteString("\n")
		}

		// Context header (simplified)
		output.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n",
			group.StartLine, group.OldLines, group.StartLine, group.NewLines))

		// Output changes in this group
		for _, change := range group.Changes {
			switch change.Type {
			case DiffLineAdded:
				output.WriteString(fmt.Sprintf("+%s\n", f.formatUnifiedLine(change)))
			case DiffLineRemoved:
				output.WriteString(fmt.Sprintf("-%s\n", f.formatUnifiedLine(change)))
			case DiffLineModified:
				output.WriteString(fmt.Sprintf("-%s\n", f.formatValueForUnified(change.Path, change.OldValue)))
				output.WriteString(fmt.Sprintf("+%s\n", f.formatValueForUnified(change.Path, change.NewValue)))
			case DiffLineUnchanged:
				output.WriteString(fmt.Sprintf(" %s\n", f.formatUnifiedLine(change)))
			}
		}
	}

	// Summary
	output.WriteString(f.formatSummary(diff))

	return output.String()
}

// formatSplit formats the diff in GitHub style, one line per change
func (f *Formatter) formatSplit(diff *ConfigDiff) string {
	var output strings.Builder

	// Header
	output.WriteString(fmt.Sprintf("📊 Configuration Diff: %s/%s.yaml\n", diff.StackName, diff.ConfigType))
	output.WriteString(fmt.Sprintf("Comparing: %s → %s\n", diff.CompareFrom, diff.CompareTo))
	output.WriteString("Resolved with inheritance applied\n\n")

	// Group changes by environment
	envGroups := f.groupChangesByEnvironment(diff.Changes)

	for i, env := range diff.Summary.EnvironmentsAffected {
		if i > 0 {
			output.WriteString("\n")
		}

		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
		output.WriteString(fmt.Sprintf("🔹 Environment: %s\n\n", env))

		changes := envGroups[env]
		for _, change := range changes {
			output.WriteString(f.formatSplitChange(change))
			output.WriteString("\n")
		}
	}

	output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// Summary
	output.WriteString(f.formatSummary(diff))

	// Warnings
	if len(diff.Warnings) > 0 {
		output.WriteString("\n⚠️  Warnings:\n")
		for _, warning := range diff.Warnings {
			output.WriteString(fmt.Sprintf("  • %s\n", warning))
		}
	}

	return output.String()
}

// formatInline formats the diff in compact path: old → new format
func (f *Formatter) formatInline(diff *ConfigDiff) string {
	var output strings.Builder

	// Header
	output.WriteString(fmt.Sprintf("📊 Configuration Diff: %s/%s.yaml\n", diff.StackName, diff.ConfigType))
	output.WriteString(fmt.Sprintf("Comparing: %s → %s\n\n", diff.CompareFrom, diff.CompareTo))

	// Group changes by environment
	envGroups := f.groupChangesByEnvironment(diff.Changes)

	for _, env := range diff.Summary.EnvironmentsAffected {
		output.WriteString(fmt.Sprintf("🔹 %s:\n", env))

		changes := envGroups[env]
		for _, change := range changes {
			output.WriteString(f.formatInlineChange(change))
			output.WriteString("\n")
		}
		output.WriteString("\n")
	}

	// Summary
	output.WriteString(fmt.Sprintf("📈 %d changes | %d additions | %d deletions",
		diff.Summary.TotalChanges, diff.Summary.Additions, diff.Summary.Deletions))

	return output.String()
}

// formatCompact formats the diff in the most compact format
func (f *Formatter) formatCompact(diff *ConfigDiff) string {
	var output strings.Builder

	// Header
	output.WriteString(fmt.Sprintf("📊 %s/%s.yaml (%s → %s) - %d changes\n\n",
		diff.StackName, diff.ConfigType, diff.CompareFrom, diff.CompareTo, diff.Summary.TotalChanges))

	// Changes (without stacks prefix)
	for _, change := range diff.Changes {
		output.WriteString(f.formatCompactChange(change))
		output.WriteString("\n")
	}

	// Warnings count
	if len(diff.Warnings) > 0 {
		output.WriteString(fmt.Sprintf("\n⚠️  %d warnings", len(diff.Warnings)))
	}

	return output.String()
}

// Helper methods for formatting

func (f *Formatter) formatSplitChange(change DiffLine) string {
	var output strings.Builder

	switch change.Type {
	case DiffLineAdded:
		output.WriteString(fmt.Sprintf("  %s (NEW)\n", change.Path))
		output.WriteString(fmt.Sprintf("  │ + %s", change.NewValue))
	case DiffLineRemoved:
		output.WriteString(fmt.Sprintf("  %s (REMOVED)\n", change.Path))
		output.WriteString(fmt.Sprintf("  │ - %s", change.OldValue))
	case DiffLineModified:
		output.WriteString(fmt.Sprintf("  %s\n", change.Path))
		output.WriteString(fmt.Sprintf("  │ %s → %s", change.OldValue, change.NewValue))
		if change.Description != "" {
			output.WriteString(fmt.Sprintf("  (%s)", change.Description))
		}
	}

	if change.Warning != "" {
		output.WriteString(fmt.Sprintf("  ⚠️  %s", change.Warning))
	}

	return output.String()
}

func (f *Formatter) formatInlineChange(change DiffLine) string {
	switch change.Type {
	case DiffLineAdded:
		warning := ""
		if change.Warning != "" {
			warning = " ⚠️"
		}
		return fmt.Sprintf("  %s: + %s%s", change.Path, change.NewValue, warning)
	case DiffLineRemoved:
		warning := ""
		if change.Warning != "" {
			warning = " ⚠️"
		}
		return fmt.Sprintf("  %s: - %s%s", change.Path, change.OldValue, warning)
	case DiffLineModified:
		warning := ""
		if change.Warning != "" {
			warning = " ⚠️"
		}
		return fmt.Sprintf("  %s: %s → %s%s", change.Path, change.OldValue, change.NewValue, warning)
	}
	return ""
}

func (f *Formatter) formatCompactChange(change DiffLine) string {
	// Remove "stacks." prefix for compact format
	path := change.Path
	if strings.HasPrefix(path, "stacks.") {
		path = strings.TrimPrefix(path, "stacks.")
	}

	switch change.Type {
	case DiffLineAdded:
		warning := ""
		if change.Warning != "" {
			warning = " ⚠️"
		}
		return fmt.Sprintf("  %s: + %s%s", path, change.NewValue, warning)
	case DiffLineRemoved:
		warning := ""
		if change.Warning != "" {
			warning = " ⚠️"
		}
		return fmt.Sprintf("  %s: - %s%s", path, change.OldValue, warning)
	case DiffLineModified:
		warning := ""
		if change.Warning != "" {
			warning = " ⚠️"
		}
		return fmt.Sprintf("  %s: %s → %s%s", path, change.OldValue, change.NewValue, warning)
	}
	return ""
}

func (f *Formatter) formatUnifiedLine(change DiffLine) string {
	// Simplified unified format line
	return fmt.Sprintf("%s: %s", change.Path, change.NewValue)
}

func (f *Formatter) formatValueForUnified(path, value string) string {
	return fmt.Sprintf("%s: %s", path, value)
}

func (f *Formatter) formatSummary(diff *ConfigDiff) string {
	var output strings.Builder

	output.WriteString("📈 Summary:\n")
	output.WriteString(fmt.Sprintf("  • %d lines changed\n", diff.Summary.TotalChanges))
	output.WriteString(fmt.Sprintf("  • %d lines added\n", diff.Summary.Additions))
	output.WriteString(fmt.Sprintf("  • %d lines removed\n", diff.Summary.Deletions))

	if len(diff.Summary.EnvironmentsAffected) > 0 {
		output.WriteString(fmt.Sprintf("  • %d environments affected: %s\n",
			len(diff.Summary.EnvironmentsAffected),
			strings.Join(diff.Summary.EnvironmentsAffected, ", ")))
	}

	return output.String()
}

// Helper types and methods for grouping changes

type ContextGroup struct {
	StartLine int
	OldLines  int
	NewLines  int
	Changes   []DiffLine
}

func (f *Formatter) groupChangesByContext(changes []DiffLine) []ContextGroup {
	// Simplified grouping - in a real implementation, this would be more sophisticated
	if len(changes) == 0 {
		return []ContextGroup{}
	}

	return []ContextGroup{
		{
			StartLine: 1,
			OldLines:  len(changes),
			NewLines:  len(changes),
			Changes:   changes,
		},
	}
}

func (f *Formatter) groupChangesByEnvironment(changes []DiffLine) map[string][]DiffLine {
	groups := make(map[string][]DiffLine)

	for _, change := range changes {
		env := f.extractEnvironmentFromPath(change.Path)
		if env == "" {
			env = "global"
		}
		groups[env] = append(groups[env], change)
	}

	return groups
}

func (f *Formatter) extractEnvironmentFromPath(path string) string {
	// Extract environment from path like "stacks.prod.config.scale.min" -> "prod"
	if strings.HasPrefix(path, "stacks.") {
		parts := strings.Split(path, ".")
		if len(parts) >= 2 {
			return parts[1]
		}
	}
	// For single stack diffs, use "config" as the environment name
	return "config"
}
