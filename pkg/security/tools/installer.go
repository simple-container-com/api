package tools

import (
	"context"
	"fmt"
	"os/exec"
)

// ToolInstaller checks tool availability and provides installation guidance
type ToolInstaller struct {
	registry *ToolRegistry
}

// NewToolInstaller creates a new tool installer
func NewToolInstaller() *ToolInstaller {
	return &ToolInstaller{
		registry: NewToolRegistry(),
	}
}

// CheckInstalled checks if a tool is available in PATH
func (i *ToolInstaller) CheckInstalled(ctx context.Context, toolName string) error {
	tool, err := i.registry.GetTool(toolName)
	if err != nil {
		return err
	}

	// Check if command exists in PATH
	_, err = exec.LookPath(tool.Command)
	if err != nil {
		return fmt.Errorf("tool '%s' not found in PATH. Install from: %s", toolName, tool.InstallURL)
	}

	return nil
}

// CheckInstalledWithVersion checks if a tool is installed and meets minimum version requirements
func (i *ToolInstaller) CheckInstalledWithVersion(ctx context.Context, toolName string) error {
	// First check if tool is installed
	if err := i.CheckInstalled(ctx, toolName); err != nil {
		return err
	}

	// Get tool metadata
	tool, err := i.registry.GetTool(toolName)
	if err != nil {
		return err
	}

	// Check version if minimum version is specified
	if tool.MinVersion != "" {
		checker := NewVersionChecker()
		version, err := checker.GetInstalledVersion(ctx, toolName)
		if err != nil {
			return fmt.Errorf("failed to get %s version: %w. Required: %s+", toolName, err, tool.MinVersion)
		}

		if err := checker.ValidateVersion(toolName, version); err != nil {
			return fmt.Errorf("version check failed: %w. Install %s+ from: %s", err, tool.MinVersion, tool.InstallURL)
		}
	}

	return nil
}

// CheckAllTools checks all required tools for a given security configuration
func (i *ToolInstaller) CheckAllTools(ctx context.Context, config interface{}) error {
	requiredTools := i.getRequiredTools(config)

	var errors []error
	for _, toolName := range requiredTools {
		if err := i.CheckInstalledWithVersion(ctx, toolName); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("tool check failed: %v", errors)
	}

	return nil
}

// GetInstallURL returns the installation URL for a tool
func (i *ToolInstaller) GetInstallURL(toolName string) (string, error) {
	tool, err := i.registry.GetTool(toolName)
	if err != nil {
		return "", err
	}
	return tool.InstallURL, nil
}

// ListAvailableTools returns all available tools in the registry
func (i *ToolInstaller) ListAvailableTools() []ToolMetadata {
	return i.registry.ListTools()
}

// getRequiredTools extracts required tools from security configuration
func (i *ToolInstaller) getRequiredTools(config interface{}) []string {
	// This is a simplified version - in a full implementation, this would
	// introspect the config structure to determine required tools

	// For now, return common security tools
	tools := []string{}

	// Use type assertion to check config types
	// This would be expanded based on actual config structure
	// For now, we'll check for common tools

	// Always include cosign for signing operations
	tools = append(tools, "cosign")

	// Check for SBOM generation
	tools = append(tools, "syft")

	// Check for vulnerability scanning
	tools = append(tools, "grype", "trivy")

	return tools
}

// IsToolAvailable checks if a tool is available without returning an error
func (i *ToolInstaller) IsToolAvailable(ctx context.Context, toolName string) bool {
	return i.CheckInstalled(ctx, toolName) == nil
}

// GetToolCommand returns the command name for a tool
func (i *ToolInstaller) GetToolCommand(toolName string) (string, error) {
	tool, err := i.registry.GetTool(toolName)
	if err != nil {
		return "", err
	}
	return tool.Command, nil
}
