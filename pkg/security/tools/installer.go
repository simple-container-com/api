// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// pathMu serializes PATH mutation in InstallIfMissing. os.Setenv is
// process-global, so concurrent installer calls (e.g., parallel tool
// pre-flight) would otherwise race on the read-modify-write of $PATH and
// silently drop newly-installed install dirs.
var pathMu sync.Mutex

// ToolInstaller checks tool availability and auto-installs missing tools.
type ToolInstaller struct {
	registry *ToolRegistry
}

// NewToolInstaller creates a new tool installer.
func NewToolInstaller() *ToolInstaller {
	return &ToolInstaller{
		registry: NewToolRegistry(),
	}
}

// CheckInstalled checks if a tool is available in PATH.
func (i *ToolInstaller) CheckInstalled(ctx context.Context, toolName string) error {
	tool, err := i.registry.GetTool(toolName)
	if err != nil {
		return err
	}

	_, err = exec.LookPath(tool.Command)
	if err != nil {
		return fmt.Errorf("tool '%s' not found in PATH. Install from: %s", toolName, tool.InstallURL)
	}

	return nil
}

// InstallIfMissing checks if a tool is installed at the registered MinVersion
// and auto-installs the pinned version if it is missing OR present-but-stale.
// A bare PATH-only check is unsafe: e.g., a Blacksmith runner shipping cosign
// 2.x would be accepted even though `MinVersion = 3.0.2`, and cosign 3.x
// changed several attestation-related defaults — silent acceptance lets the
// runner-installed binary drive behavior instead of the SC-pinned version.
// Supports: cosign, syft, grype, trivy.
func (i *ToolInstaller) InstallIfMissing(ctx context.Context, toolName string) error {
	if err := i.CheckInstalledWithVersion(ctx, toolName); err == nil {
		return nil
	}

	tool, err := i.registry.GetTool(toolName)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Tool %s not found, attempting auto-install...\n", toolName)

	installDir := resolveInstallDir()
	script, err := installScript(toolName, tool.MinVersion, installDir)
	if err != nil {
		return fmt.Errorf("no auto-install available for %s: install manually from %s", toolName, tool.InstallURL)
	}

	fmt.Fprintf(os.Stderr, "Installing %s %s...\n", toolName, tool.MinVersion)
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	cmd.Stdout = os.Stderr // install output goes to stderr, not stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("auto-install of %s failed: %w — install manually from %s", toolName, err, tool.InstallURL)
	}

	pathMu.Lock()
	os.Setenv("PATH", prependToPath(installDir, os.Getenv("PATH")))
	pathMu.Unlock()

	// Verify installation succeeded AND meets the pinned MinVersion — a
	// bare presence check would silently accept a stale binary that still
	// shadows the install (e.g., kernel/glibc mismatch leaves the new
	// download non-executable and PATH falls back to the old one).
	if err := i.CheckInstalledWithVersion(ctx, toolName); err != nil {
		return fmt.Errorf("%s install verification failed: %w — check %s", toolName, err, installDir)
	}

	fmt.Fprintf(os.Stderr, "Tool %s installed successfully\n", toolName)
	return nil
}

// prependToPath returns currentPath with installDir moved to the front,
// removing any prior occurrence so the freshly-installed binary always wins
// over a stale copy earlier in PATH (e.g., system-package cosign 2.x at
// /usr/bin/cosign vs. our pinned 3.x at ~/.local/bin/cosign). Comparison is
// done on filepath.Clean'd values so a trailing slash on an existing entry
// (e.g., "/usr/local/bin/") doesn't defeat dedup. POSIX-meaningful empty
// entries (which denote "current directory") are preserved, not stripped.
func prependToPath(installDir, currentPath string) string {
	target := filepath.Clean(installDir)
	parts := []string{installDir}
	for _, dir := range filepath.SplitList(currentPath) {
		if dir == "" {
			// Empty entry == CWD in POSIX PATH semantics; preserve.
			parts = append(parts, dir)
			continue
		}
		if filepath.Clean(dir) == target {
			continue
		}
		parts = append(parts, dir)
	}
	return strings.Join(parts, string(os.PathListSeparator))
}

// resolveInstallDir returns a writable bin directory.
// Prefers /usr/local/bin when writable (root or writable dir), falls back to ~/.local/bin.
func resolveInstallDir() string {
	// Check if /usr/local/bin is directly writable (e.g., running as root on Blacksmith)
	testFile := "/usr/local/bin/.sc-write-test"
	if f, err := os.Create(testFile); err == nil {
		f.Close()
		os.Remove(testFile)
		return "/usr/local/bin"
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "bin")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// versionRe validates tool version strings to prevent shell injection.
// Only allows digits, dots, and hyphens (e.g., "2.4.1", "0.98.0-rc1").
var versionRe = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$`)

// installScript returns a shell script that installs the named tool.
// All tools are downloaded as direct binaries or tarballs from GitHub releases
// (no piping remote scripts to sh). Version strings are validated to prevent
// shell injection.
func installScript(toolName, version, installDir string) (string, error) {
	if !versionRe.MatchString(version) {
		return "", fmt.Errorf("invalid version format %q for tool %s", version, toolName)
	}

	switch toolName {
	case "cosign":
		return fmt.Sprintf(`set -e
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT
curl -sSfL "https://github.com/sigstore/cosign/releases/download/v%[1]s/cosign-linux-amd64" \
  -o "$TMP_DIR/cosign"
chmod +x "$TMP_DIR/cosign"
mv "$TMP_DIR/cosign" %[2]s/cosign`, version, installDir), nil

	case "syft":
		// Direct tarball download from GitHub releases (no curl|sh).
		// Detect OS/arch at runtime to support linux/darwin, amd64/arm64.
		return fmt.Sprintf(`set -e
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m); [ "$ARCH" = "x86_64" ] && ARCH="amd64"; [ "$ARCH" = "aarch64" ] && ARCH="arm64"
curl -sSfL "https://github.com/anchore/syft/releases/download/v%[1]s/syft_%[1]s_${OS}_${ARCH}.tar.gz" \
  -o "$TMP_DIR/syft.tar.gz"
tar -xzf "$TMP_DIR/syft.tar.gz" -C "$TMP_DIR" syft
chmod +x "$TMP_DIR/syft"
mv "$TMP_DIR/syft" %[2]s/syft`, version, installDir), nil

	case "grype":
		// Direct tarball download from GitHub releases (no curl|sh).
		return fmt.Sprintf(`set -e
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m); [ "$ARCH" = "x86_64" ] && ARCH="amd64"; [ "$ARCH" = "aarch64" ] && ARCH="arm64"
curl -sSfL "https://github.com/anchore/grype/releases/download/v%[1]s/grype_%[1]s_${OS}_${ARCH}.tar.gz" \
  -o "$TMP_DIR/grype.tar.gz"
tar -xzf "$TMP_DIR/grype.tar.gz" -C "$TMP_DIR" grype
chmod +x "$TMP_DIR/grype"
mv "$TMP_DIR/grype" %[2]s/grype`, version, installDir), nil

	case "trivy":
		return fmt.Sprintf(`set -e
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT
curl -sSfL "https://github.com/aquasecurity/trivy/releases/download/v%[1]s/trivy_%[1]s_Linux-64bit.tar.gz" \
  -o "$TMP_DIR/trivy.tar.gz"
tar -xzf "$TMP_DIR/trivy.tar.gz" -C "$TMP_DIR" trivy
mv "$TMP_DIR/trivy" %[2]s/trivy`, version, installDir), nil

	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
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
