package configdiff

import (
	"testing"
	"time"

	"github.com/simple-container-com/api/pkg/api"
)

func TestConfigDiffService_GetSupportedFormats(t *testing.T) {
	// Create a minimal StacksMap for testing
	stacksMap := api.StacksMap{
		"test-stack": api.Stack{
			Name: "test-stack",
		},
	}

	service := NewConfigDiffService(stacksMap)
	formats := service.GetSupportedFormats()

	expectedFormats := []DiffFormat{
		FormatUnified,
		FormatSplit,
		FormatInline,
		FormatCompact,
	}

	if len(formats) != len(expectedFormats) {
		t.Errorf("Expected %d formats, got %d", len(expectedFormats), len(formats))
	}

	for i, expected := range expectedFormats {
		if i >= len(formats) || formats[i] != expected {
			t.Errorf("Expected format %s at index %d, got %s", expected, i, formats[i])
		}
	}
}

func TestConfigDiffService_GetFormatDescription(t *testing.T) {
	stacksMap := api.StacksMap{}
	service := NewConfigDiffService(stacksMap)

	testCases := []struct {
		format      DiffFormat
		expectEmpty bool
	}{
		{FormatUnified, false},
		{FormatSplit, false},
		{FormatInline, false},
		{FormatCompact, false},
		{DiffFormat("invalid"), false}, // Should return "Unknown format"
	}

	for _, tc := range testCases {
		desc := service.GetFormatDescription(tc.format)
		if tc.expectEmpty && desc != "" {
			t.Errorf("Expected empty description for format %s, got: %s", tc.format, desc)
		}
		if !tc.expectEmpty && desc == "" {
			t.Errorf("Expected non-empty description for format %s", tc.format)
		}
	}
}

func TestDefaultDiffOptions(t *testing.T) {
	options := DefaultDiffOptions()

	if options.Format != FormatSplit {
		t.Errorf("Expected default format to be %s, got %s", FormatSplit, options.Format)
	}

	if !options.ObfuscateSecrets {
		t.Error("Expected ObfuscateSecrets to be true by default")
	}

	if options.ContextLines != 3 {
		t.Errorf("Expected ContextLines to be 3, got %d", options.ContextLines)
	}

	if options.MaxChanges != 100 {
		t.Errorf("Expected MaxChanges to be 100, got %d", options.MaxChanges)
	}
}

func TestDiffer_FormatValue(t *testing.T) {
	options := DefaultDiffOptions()
	differ := NewDiffer(options)

	testCases := []struct {
		input    interface{}
		expected string
	}{
		{nil, "<nil>"},
		{"hello", `"hello"`},
		{true, "true"},
		{false, "false"},
		{42, "42"},
		{3.14, "3.14"},
	}

	for _, tc := range testCases {
		result := differ.formatValue(tc.input)
		if result != tc.expected {
			t.Errorf("formatValue(%v) = %s, expected %s", tc.input, result, tc.expected)
		}
	}
}

func TestDiffer_IsSecretValue(t *testing.T) {
	options := DefaultDiffOptions()
	differ := NewDiffer(options)

	testCases := []struct {
		input    string
		expected bool
	}{
		{"password123", true},
		{"my-secret-key", true},
		{"AKIAIOSFODNN7EXAMPLE", true}, // AWS access key format
		{"sk_test_123456789", true},    // Stripe key format
		{"normal-value", false},
		{"short", false},
		{"", false},
	}

	for _, tc := range testCases {
		result := differ.isSecretValue(tc.input)
		if result != tc.expected {
			t.Errorf("isSecretValue(%s) = %t, expected %t", tc.input, result, tc.expected)
		}
	}
}

func TestFormatter_FormatNoChanges(t *testing.T) {
	options := DefaultDiffOptions()
	formatter := NewFormatter(options)

	diff := &ConfigDiff{
		StackName:   "test-stack",
		ConfigType:  "client",
		CompareFrom: "HEAD~1",
		CompareTo:   "current",
		Changes:     []DiffLine{}, // No changes
		GeneratedAt: time.Now(),
	}

	result := formatter.FormatDiff(diff)

	if result == "" {
		t.Error("Expected non-empty result for no changes")
	}

	// Should contain "No changes detected"
	if !contains(result, "No changes detected") {
		t.Error("Expected 'No changes detected' in output")
	}
}

func TestResolvedConfig_Structure(t *testing.T) {
	config := &ResolvedConfig{
		StackName:    "test-stack",
		ConfigType:   "client",
		Content:      "test: value",
		ParsedConfig: map[string]interface{}{"test": "value"},
		ResolvedAt:   time.Now(),
		FilePath:     ".sc/stacks/test-stack/client.yaml",
		Metadata:     map[string]interface{}{"test": true},
	}

	if config.StackName != "test-stack" {
		t.Errorf("Expected StackName to be 'test-stack', got %s", config.StackName)
	}

	if config.ConfigType != "client" {
		t.Errorf("Expected ConfigType to be 'client', got %s", config.ConfigType)
	}

	if config.ParsedConfig["test"] != "value" {
		t.Errorf("Expected ParsedConfig['test'] to be 'value', got %v", config.ParsedConfig["test"])
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsInMiddle(s, substr))))
}

func containsInMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
