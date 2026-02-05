package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
)

// Installer checks for required tool installation and versions
type Installer struct {
	logger   logger.Logger
	executor *CommandExecutor
}

// NewInstaller creates a new tool installer
func NewInstaller(log logger.Logger) *Installer {
	return &Installer{
		logger:   log,
		executor: NewCommandExecutor(0), // Use default timeout
	}
}

// CheckInstalled verifies tool is installed and meets version requirements
func (i *Installer) CheckInstalled(ctx context.Context, tool ToolMetadata) (bool, string, error) {
	// Check if command exists
	if !i.executor.CheckCommandExists(tool.Command) {
		err := fmt.Errorf("tool not found: %s (minimum version %s required). Install from: %s",
			tool.Name, tool.MinVersion, tool.InstallURL)
		return false, "", err
	}

	// Get version
	version, err := i.getVersion(ctx, tool)
	if err != nil {
		return false, "", fmt.Errorf("failed to get %s version: %w", tool.Name, err)
	}

	// Compare version if minimum specified
	if tool.MinVersion != "" {
		if !i.meetsVersion(version, tool.MinVersion) {
			err := fmt.Errorf("tool version mismatch: %s version %s does not meet minimum %s",
				tool.Name, version, tool.MinVersion)
			return false, version, err
		}
	}

	return true, version, nil
}

// getVersion extracts version from tool
func (i *Installer) getVersion(ctx context.Context, tool ToolMetadata) (string, error) {
	var args []string

	switch tool.Name {
	case "Cosign":
		args = []string{"cosign", "version"}
	case "Syft":
		args = []string{"syft", "version"}
	case "Grype":
		args = []string{"grype", "version"}
	case "Trivy":
		args = []string{"trivy", "--version"}
	default:
		args = []string{tool.Command, "--version"}
	}

	stdout, stderr, err := i.executor.ExecuteWithStderr(ctx, args, nil)
	if err != nil {
		// Some tools output version to stderr
		if len(stderr) > 0 {
			output := string(stderr)
			version := ExtractVersionFromOutput(output, strings.ToLower(tool.Name))
			if version != "" {
				return version, nil
			}
		}
		return "", fmt.Errorf("failed to get version: %w", err)
	}

	output := string(stdout)
	version := ExtractVersionFromOutput(output, strings.ToLower(tool.Name))
	if version == "" {
		return "", fmt.Errorf("failed to parse version from output: %s", output)
	}

	return version, nil
}

// meetsVersion checks if current version meets minimum requirement
func (i *Installer) meetsVersion(current, minimum string) bool {
	currentVer, err := ParseVersion(current)
	if err != nil {
		i.logger.Warn(context.Background(), "Failed to parse current version %s: %v", current, err)
		return false
	}

	minVer, err := ParseVersion(minimum)
	if err != nil {
		i.logger.Warn(context.Background(), "Failed to parse minimum version %s: %v", minimum, err)
		return false
	}

	return currentVer.MeetsMinimum(minVer)
}

// CheckAllTools validates all required tools for configuration
func (i *Installer) CheckAllTools(ctx context.Context, config *api.SecurityDescriptor) error {
	var missing []string
	var warnings []string

	// Check signing tools
	if config.Signing != nil && config.Signing.Enabled {
		ok, version, err := i.CheckInstalled(ctx, ToolRegistry["cosign"])
		if !ok {
			missing = append(missing, fmt.Sprintf("cosign: %v", err))
		} else {
			i.logger.Info(ctx, "Found cosign version %s", version)
		}
	}

	// Check SBOM tools
	if config.SBOM != nil && config.SBOM.Enabled {
		ok, version, err := i.CheckInstalled(ctx, ToolRegistry["syft"])
		if !ok {
			missing = append(missing, fmt.Sprintf("syft: %v", err))
		} else {
			i.logger.Info(ctx, "Found syft version %s", version)
		}
	}

	// Check scanning tools
	if config.Scan != nil && config.Scan.Enabled {
		for _, toolConfig := range config.Scan.Tools {
			toolMeta, exists := ToolRegistry[toolConfig.Name]
			if !exists {
				warnings = append(warnings, fmt.Sprintf("Unknown scanner: %s", toolConfig.Name))
				continue
			}

			ok, version, err := i.CheckInstalled(ctx, toolMeta)
			if !ok {
				if toolConfig.Required {
					missing = append(missing, fmt.Sprintf("%s: %v", toolConfig.Name, err))
				} else {
					warnings = append(warnings, fmt.Sprintf("%s: %v (optional)", toolConfig.Name, err))
				}
			} else {
				i.logger.Info(ctx, "Found %s version %s", toolConfig.Name, version)
			}
		}
	}

	// Log warnings
	for _, warning := range warnings {
		i.logger.Warn(ctx, "%s", warning)
	}

	// Return error if any required tools are missing
	if len(missing) > 0 {
		return fmt.Errorf("missing required tools:\n%s\n\nSee installation guide: https://docs.simple-container.com/security/tools",
			strings.Join(missing, "\n"))
	}

	return nil
}

// GetToolVersion is a convenience function to get a tool's version
func (i *Installer) GetToolVersion(ctx context.Context, toolName string) (string, error) {
	tool, exists := ToolRegistry[toolName]
	if !exists {
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}

	_, version, err := i.CheckInstalled(ctx, tool)
	return version, err
}
