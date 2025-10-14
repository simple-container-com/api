package configdiff

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/pkg/api"
)

// NewConfigResolver creates a new ConfigResolver instance
func NewConfigResolver(stacksMap api.StacksMap, versionProvider ConfigVersionProvider) *ConfigResolver {
	return &ConfigResolver{
		stacksMap:       stacksMap,
		versionProvider: versionProvider,
	}
}

// ResolveStack resolves a stack configuration with all inheritance applied
func (r *ConfigResolver) ResolveStack(stackName string, configType string) (*ResolvedConfig, error) {
	// Get the stack from StacksMap
	_, exists := r.stacksMap[stackName]
	if !exists {
		return nil, fmt.Errorf("stack '%s' not found", stackName)
	}

	// Apply inheritance resolution to get final state
	resolvedStacksMap := r.stacksMap.ResolveInheritance()
	resolvedStack, exists := (*resolvedStacksMap)[stackName]
	if !exists {
		return nil, fmt.Errorf("failed to resolve inheritance for stack '%s'", stackName)
	}

	// Create a temporary StacksMap with just this resolved stack for serialization
	tempStacksMap := api.StacksMap{
		stackName: resolvedStack,
	}

	// Serialize to YAML
	yamlContent, err := yaml.Marshal(map[string]interface{}{
		"stacks": tempStacksMap,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to serialize resolved configuration: %w", err)
	}

	// Parse the YAML back to get structured data
	var parsedConfig map[string]interface{}
	if err := yaml.Unmarshal(yamlContent, &parsedConfig); err != nil {
		return nil, fmt.Errorf("failed to parse resolved configuration: %w", err)
	}

	// Determine file path
	filePath := fmt.Sprintf(".sc/stacks/%s/%s.yaml", stackName, configType)

	return &ResolvedConfig{
		StackName:    stackName,
		ConfigType:   configType,
		Content:      string(yamlContent),
		ParsedConfig: parsedConfig,
		ResolvedAt:   time.Now(),
		FilePath:     filePath,
		Metadata: map[string]interface{}{
			"inheritance_resolved": true,
			"resolver_version":     "1.0.0",
		},
	}, nil
}

// DefaultConfigVersionProvider implements ConfigVersionProvider
type DefaultConfigVersionProvider struct{}

// NewDefaultConfigVersionProvider creates a new DefaultConfigVersionProvider
func NewDefaultConfigVersionProvider() *DefaultConfigVersionProvider {
	return &DefaultConfigVersionProvider{}
}

// GetCurrent gets the current configuration from the working directory
func (p *DefaultConfigVersionProvider) GetCurrent(stackName, configType string) (*ResolvedConfig, error) {
	filePath := fmt.Sprintf(".sc/stacks/%s/%s.yaml", stackName, configType)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found: %s", filePath)
	}

	// Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file %s: %w", filePath, err)
	}

	// Parse YAML
	var parsedConfig map[string]interface{}
	if err := yaml.Unmarshal(content, &parsedConfig); err != nil {
		return nil, fmt.Errorf("failed to parse YAML from %s: %w", filePath, err)
	}

	return &ResolvedConfig{
		StackName:    stackName,
		ConfigType:   configType,
		Content:      string(content),
		ParsedConfig: parsedConfig,
		ResolvedAt:   time.Now(),
		FilePath:     filePath,
		GitRef:       "current",
		Metadata: map[string]interface{}{
			"source": "working_directory",
		},
	}, nil
}

// GetFromGit gets a configuration from a specific git reference
func (p *DefaultConfigVersionProvider) GetFromGit(stackName, configType, gitRef string) (*ResolvedConfig, error) {
	filePath := fmt.Sprintf(".sc/stacks/%s/%s.yaml", stackName, configType)

	// Use git show to get the file content from the specified reference
	cmd := fmt.Sprintf("git show %s:%s", gitRef, filePath)

	// Execute git command (simplified - in production would use proper git library)
	// For now, return an error indicating git support needs implementation
	return nil, fmt.Errorf("git support not yet implemented - would execute: %s", cmd)
}

// GetFromLocal gets a configuration from a local file path
func (p *DefaultConfigVersionProvider) GetFromLocal(stackName, configType, filePath string) (*ResolvedConfig, error) {
	// Resolve absolute path
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path for %s: %w", filePath, err)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found: %s", absPath)
	}

	// Read the file
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file %s: %w", absPath, err)
	}

	// Parse YAML
	var parsedConfig map[string]interface{}
	if err := yaml.Unmarshal(content, &parsedConfig); err != nil {
		return nil, fmt.Errorf("failed to parse YAML from %s: %w", absPath, err)
	}

	return &ResolvedConfig{
		StackName:    stackName,
		ConfigType:   configType,
		Content:      string(content),
		ParsedConfig: parsedConfig,
		ResolvedAt:   time.Now(),
		FilePath:     absPath,
		GitRef:       "local",
		Metadata: map[string]interface{}{
			"source": "local_file",
		},
	}, nil
}
