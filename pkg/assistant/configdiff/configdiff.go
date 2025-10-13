package configdiff

import (
	"fmt"

	"github.com/simple-container-com/api/pkg/api"
)

// ConfigDiffService provides the main interface for configuration diffing
type ConfigDiffService struct {
	resolver        *ConfigResolver
	versionProvider ConfigVersionProvider
}

// NewConfigDiffService creates a new ConfigDiffService instance
func NewConfigDiffService(stacksMap api.StacksMap) *ConfigDiffService {
	versionProvider := NewDefaultConfigVersionProvider()
	resolver := NewConfigResolver(stacksMap, versionProvider)

	return &ConfigDiffService{
		resolver:        resolver,
		versionProvider: versionProvider,
	}
}

// NewConfigDiffServiceWithProvider creates a new ConfigDiffService instance with a custom provider
func NewConfigDiffServiceWithProvider(stacksMap api.StacksMap, versionProvider ConfigVersionProvider) *ConfigDiffService {
	resolver := NewConfigResolver(stacksMap, versionProvider)

	return &ConfigDiffService{
		resolver:        resolver,
		versionProvider: versionProvider,
	}
}

// GenerateConfigDiff generates a configuration diff based on the provided parameters
func (s *ConfigDiffService) GenerateConfigDiff(params ConfigDiffParams) (*ConfigDiffResult, error) {
	// Validate parameters
	if params.StackName == "" {
		return &ConfigDiffResult{
			Success: false,
			Error:   "stack_name is required",
		}, fmt.Errorf("stack_name is required")
	}

	if params.ConfigType == "" {
		params.ConfigType = "client" // Default to client
	}

	if params.CompareWith == "" {
		params.CompareWith = "HEAD~1" // Default to previous commit
	}

	// Set default options if not provided
	if params.Options.Format == "" {
		params.Options = DefaultDiffOptions()
	}

	// Get current configuration
	currentConfig, err := s.versionProvider.GetCurrent(params.StackName, params.ConfigType)
	if err != nil {
		return &ConfigDiffResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to get current configuration: %v", err),
		}, err
	}

	// Get comparison configuration
	var compareConfig *ResolvedConfig
	if params.CompareWith == "current" || params.CompareWith == "working" {
		compareConfig = currentConfig
	} else {
		// Try to get from git
		compareConfig, err = s.versionProvider.GetFromGit(params.StackName, params.ConfigType, params.CompareWith)
		if err != nil {
			return &ConfigDiffResult{
				Success: false,
				Error:   fmt.Sprintf("Failed to get configuration from %s: %v", params.CompareWith, err),
			}, err
		}
	}

	// Create differ and compare configurations
	differ := NewDiffer(params.Options)
	diff, err := differ.CompareConfigs(compareConfig, currentConfig)
	if err != nil {
		return &ConfigDiffResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to compare configurations: %v", err),
		}, err
	}

	// Format the diff
	formatter := NewFormatter(params.Options)
	message := formatter.FormatDiff(diff)

	return &ConfigDiffResult{
		Diff:    diff,
		Message: message,
		Success: true,
	}, nil
}

// GetConfigSnapshot gets a configuration snapshot for a specific reference
func (s *ConfigDiffService) GetConfigSnapshot(stackName, configType, ref string) (*ResolvedConfig, error) {
	if ref == "" || ref == "current" || ref == "working" {
		return s.versionProvider.GetCurrent(stackName, configType)
	}

	return s.versionProvider.GetFromGit(stackName, configType, ref)
}

// ValidateStackExists checks if a stack exists in the current configuration
func (s *ConfigDiffService) ValidateStackExists(stackName string) error {
	// Try to get current configuration to validate stack exists
	_, err := s.versionProvider.GetCurrent(stackName, "client")
	if err != nil {
		// Try server configuration as fallback
		_, serverErr := s.versionProvider.GetCurrent(stackName, "server")
		if serverErr != nil {
			return fmt.Errorf("stack '%s' not found in client.yaml or server.yaml", stackName)
		}
	}
	return nil
}

// ListAvailableStacks returns a list of available stacks
func (s *ConfigDiffService) ListAvailableStacks() ([]string, error) {
	// This would need to be implemented by scanning the .sc/stacks directory
	// For now, return an empty list
	return []string{}, nil
}

// GetSupportedFormats returns the list of supported diff formats
func (s *ConfigDiffService) GetSupportedFormats() []DiffFormat {
	return []DiffFormat{
		FormatUnified,
		FormatSplit,
		FormatInline,
		FormatCompact,
	}
}

// GetFormatDescription returns a description of a specific format
func (s *ConfigDiffService) GetFormatDescription(format DiffFormat) string {
	switch format {
	case FormatUnified:
		return "Git diff style with +/- (familiar to developers)"
	case FormatSplit:
		return "GitHub style, one line per change (recommended - readable with explanations)"
	case FormatInline:
		return "Compact path: old → new (quick overview)"
	case FormatCompact:
		return "Shortest format without stacks prefix (minimal text)"
	default:
		return "Unknown format"
	}
}
