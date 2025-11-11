package configdiff

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Differ handles YAML configuration comparison
type Differ struct {
	options DiffOptions
}

// NewDiffer creates a new Differ instance
func NewDiffer(options DiffOptions) *Differ {
	return &Differ{
		options: options,
	}
}

// CompareConfigs compares two resolved configurations and returns the differences
func (d *Differ) CompareConfigs(before, after *ResolvedConfig) (*ConfigDiff, error) {
	if before.StackName != after.StackName {
		return nil, fmt.Errorf("cannot compare configurations for different stacks: %s vs %s", before.StackName, after.StackName)
	}

	if before.ConfigType != after.ConfigType {
		return nil, fmt.Errorf("cannot compare different config types: %s vs %s", before.ConfigType, after.ConfigType)
	}

	// Parse both configurations
	beforeData := before.ParsedConfig
	afterData := after.ParsedConfig

	// Find differences
	changes := d.findDifferences("", beforeData, afterData)

	// Generate warnings
	warnings := d.generateWarnings(changes)

	// Calculate summary
	summary := d.calculateSummary(changes)

	diff := &ConfigDiff{
		StackName:   before.StackName,
		ConfigType:  before.ConfigType,
		CompareFrom: before.GitRef,
		CompareTo:   after.GitRef,
		Changes:     changes,
		Summary:     summary,
		Warnings:    warnings,
		GeneratedAt: time.Now(),
	}

	return diff, nil
}

// findDifferences recursively finds differences between two YAML structures
func (d *Differ) findDifferences(path string, before, after interface{}) []DiffLine {
	var changes []DiffLine

	// Handle nil cases
	if before == nil && after == nil {
		return changes
	}

	if before == nil {
		// Added
		changes = append(changes, DiffLine{
			Type:        DiffLineAdded,
			Path:        path,
			OldValue:    "",
			NewValue:    d.formatValue(after),
			Description: fmt.Sprintf("Added %s", path),
		})
		return changes
	}

	if after == nil {
		// Removed
		changes = append(changes, DiffLine{
			Type:        DiffLineRemoved,
			Path:        path,
			OldValue:    d.formatValue(before),
			NewValue:    "",
			Description: fmt.Sprintf("Removed %s", path),
		})
		return changes
	}

	// Compare based on type
	beforeType := reflect.TypeOf(before)
	afterType := reflect.TypeOf(after)

	if beforeType != afterType {
		// Type changed
		changes = append(changes, DiffLine{
			Type:        DiffLineModified,
			Path:        path,
			OldValue:    d.formatValue(before),
			NewValue:    d.formatValue(after),
			Description: fmt.Sprintf("Type changed at %s", path),
		})
		return changes
	}

	switch beforeVal := before.(type) {
	case map[string]interface{}:
		afterVal := after.(map[string]interface{})
		changes = append(changes, d.compareObjects(path, beforeVal, afterVal)...)

	case []interface{}:
		afterVal := after.([]interface{})
		changes = append(changes, d.compareArrays(path, beforeVal, afterVal)...)

	default:
		// Primitive values
		if !reflect.DeepEqual(before, after) {
			changes = append(changes, DiffLine{
				Type:        DiffLineModified,
				Path:        path,
				OldValue:    d.formatValue(before),
				NewValue:    d.formatValue(after),
				Description: d.generateChangeDescription(path, before, after),
			})
		}
	}

	return changes
}

// compareObjects compares two YAML objects (maps)
func (d *Differ) compareObjects(path string, before, after map[string]interface{}) []DiffLine {
	var changes []DiffLine

	// Get all keys from both objects
	allKeys := make(map[string]bool)
	for key := range before {
		allKeys[key] = true
	}
	for key := range after {
		allKeys[key] = true
	}

	// Sort keys for consistent output
	keys := make([]string, 0, len(allKeys))
	for key := range allKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		newPath := d.buildPath(path, key)
		beforeVal, beforeExists := before[key]
		afterVal, afterExists := after[key]

		if !beforeExists {
			// Key added
			changes = append(changes, d.findDifferences(newPath, nil, afterVal)...)
		} else if !afterExists {
			// Key removed
			changes = append(changes, d.findDifferences(newPath, beforeVal, nil)...)
		} else {
			// Key exists in both, compare values
			changes = append(changes, d.findDifferences(newPath, beforeVal, afterVal)...)
		}
	}

	return changes
}

// compareArrays compares two YAML arrays
func (d *Differ) compareArrays(path string, before, after []interface{}) []DiffLine {
	var changes []DiffLine

	maxLen := len(before)
	if len(after) > maxLen {
		maxLen = len(after)
	}

	for i := 0; i < maxLen; i++ {
		newPath := fmt.Sprintf("%s[%d]", path, i)

		var beforeVal, afterVal interface{}
		beforeExists := i < len(before)
		afterExists := i < len(after)

		if beforeExists {
			beforeVal = before[i]
		}
		if afterExists {
			afterVal = after[i]
		}

		if !beforeExists {
			// Element added
			changes = append(changes, d.findDifferences(newPath, nil, afterVal)...)
		} else if !afterExists {
			// Element removed
			changes = append(changes, d.findDifferences(newPath, beforeVal, nil)...)
		} else {
			// Element exists in both, compare
			changes = append(changes, d.findDifferences(newPath, beforeVal, afterVal)...)
		}
	}

	return changes
}

// buildPath constructs a YAML path
func (d *Differ) buildPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return fmt.Sprintf("%s.%s", parent, key)
}

// formatValue formats a value for display
func (d *Differ) formatValue(value interface{}) string {
	if value == nil {
		return "<nil>"
	}

	switch v := value.(type) {
	case string:
		// Obfuscate secrets if enabled
		if d.options.ObfuscateSecrets && d.isSecretValue(v) {
			return d.obfuscateSecret(v)
		}
		return fmt.Sprintf(`"%s"`, v)
	case bool:
		return fmt.Sprintf("%t", v)
	case int, int32, int64, float32, float64:
		return fmt.Sprintf("%v", v)
	default:
		// For complex types, use YAML representation
		yamlBytes, err := yaml.Marshal(value)
		if err != nil {
			return fmt.Sprintf("%v", value)
		}
		return strings.TrimSpace(string(yamlBytes))
	}
}

// isSecretValue checks if a value appears to be a secret
func (d *Differ) isSecretValue(value string) bool {
	secretPatterns := []string{
		"password", "secret", "key", "token", "credential",
		"AKIA",       // AWS access key prefix
		"sk_", "pk_", // Stripe keys
	}

	lowerValue := strings.ToLower(value)
	for _, pattern := range secretPatterns {
		if strings.Contains(lowerValue, strings.ToLower(pattern)) {
			return true
		}
	}

	// Check for long alphanumeric strings that might be secrets
	if len(value) > 20 && d.isAlphanumeric(value) {
		return true
	}

	return false
}

// obfuscateSecret obfuscates a secret value
func (d *Differ) obfuscateSecret(value string) string {
	if len(value) <= 4 {
		return "••••"
	}

	// Show first few characters and obfuscate the rest
	visible := 4
	if len(value) < 8 {
		visible = 2
	}

	prefix := value[:visible]
	suffix := strings.Repeat("•", len(value)-visible)
	return prefix + suffix
}

// isAlphanumeric checks if a string contains only alphanumeric characters
func (d *Differ) isAlphanumeric(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// generateChangeDescription generates a human-readable description for a change
func (d *Differ) generateChangeDescription(path string, oldValue, newValue interface{}) string {
	// Special handling for common configuration paths
	if strings.Contains(path, "scale.min") {
		return d.generateScalingDescription(path, oldValue, newValue, "minimum instances")
	}
	if strings.Contains(path, "scale.max") {
		return d.generateScalingDescription(path, oldValue, newValue, "maximum instances")
	}
	if strings.Contains(path, "parent") {
		return "Inheritance chain modified"
	}
	if strings.Contains(path, "env") && strings.Contains(path, "LOG_LEVEL") {
		return d.generateLogLevelDescription(oldValue, newValue)
	}
	if strings.Contains(path, "DB_POOL_SIZE") {
		return "Database connection pool changed"
	}

	return fmt.Sprintf("Value changed at %s", path)
}

// generateScalingDescription generates description for scaling changes
func (d *Differ) generateScalingDescription(path string, oldValue, newValue interface{}, scaleType string) string {
	oldNum, oldOk := d.toNumber(oldValue)
	newNum, newOk := d.toNumber(newValue)

	if !oldOk || !newOk {
		return fmt.Sprintf("%s changed", scaleType)
	}

	if newNum > oldNum {
		percentage := ((newNum - oldNum) / oldNum) * 100
		return fmt.Sprintf("%s increased by %.0f%%", scaleType, percentage)
	} else if newNum < oldNum {
		percentage := ((oldNum - newNum) / oldNum) * 100
		return fmt.Sprintf("%s decreased by %.0f%%", scaleType, percentage)
	}

	return fmt.Sprintf("%s changed", scaleType)
}

// generateLogLevelDescription generates description for log level changes
func (d *Differ) generateLogLevelDescription(oldValue, newValue interface{}) string {
	oldStr, oldOk := oldValue.(string)
	newStr, newOk := newValue.(string)

	if !oldOk || !newOk {
		return "Log level changed"
	}

	logLevels := map[string]int{
		"debug": 1,
		"info":  2,
		"warn":  3,
		"error": 4,
	}

	oldLevel, oldExists := logLevels[strings.ToLower(oldStr)]
	newLevel, newExists := logLevels[strings.ToLower(newStr)]

	if !oldExists || !newExists {
		return "Log level changed"
	}

	if newLevel > oldLevel {
		return "Log verbosity reduced"
	} else if newLevel < oldLevel {
		return "Log verbosity increased"
	}

	return "Log level changed"
}

// toNumber converts a value to float64 if possible
func (d *Differ) toNumber(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case string:
		// Try to parse string as number
		var result float64
		if num, err := fmt.Sscanf(v, "%f", &result); err == nil && num == 1 {
			return result, true
		}
	}
	return 0, false
}

// generateWarnings generates warnings based on the changes
func (d *Differ) generateWarnings(changes []DiffLine) []string {
	var warnings []string

	for _, change := range changes {
		if strings.Contains(change.Path, "parent") {
			warnings = append(warnings, "Parent stack changed - verify inheritance chain")
		}
		if strings.Contains(change.Path, "LOG_LEVEL") && strings.Contains(change.Description, "reduced") {
			warnings = append(warnings, "Log level reduced - may affect debugging")
		}
		if strings.Contains(change.Path, "scale.min") {
			warnings = append(warnings, "Minimum scaling changed - verify resource requirements")
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	uniqueWarnings := []string{}
	for _, warning := range warnings {
		if !seen[warning] {
			seen[warning] = true
			uniqueWarnings = append(uniqueWarnings, warning)
		}
	}

	return uniqueWarnings
}

// calculateSummary calculates summary statistics for the changes
func (d *Differ) calculateSummary(changes []DiffLine) DiffSummary {
	summary := DiffSummary{
		EnvironmentsAffected: []string{},
	}

	envs := make(map[string]bool)

	for _, change := range changes {
		summary.TotalChanges++

		switch change.Type {
		case DiffLineAdded:
			summary.Additions++
		case DiffLineRemoved:
			summary.Deletions++
		case DiffLineModified:
			summary.Modifications++
		}

		// Extract environment from path (e.g., "stacks.prod.config" -> "prod")
		// For our case, since we're comparing individual stacks, we use the stack name from the diff
		if strings.HasPrefix(change.Path, "stacks.") {
			parts := strings.Split(change.Path, ".")
			if len(parts) >= 2 {
				env := parts[1]
				envs[env] = true
			}
		} else {
			// For single stack diffs, use a default environment name
			envs["config"] = true
		}
	}

	// Convert environments map to slice
	for env := range envs {
		summary.EnvironmentsAffected = append(summary.EnvironmentsAffected, env)
	}
	sort.Strings(summary.EnvironmentsAffected)

	summary.HasWarnings = len(d.generateWarnings(changes)) > 0

	return summary
}
