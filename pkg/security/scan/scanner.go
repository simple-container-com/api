package scan

import (
	"context"
	"fmt"
	"os"
)

// Scanner is the interface for vulnerability scanners
type Scanner interface {
	// Scan performs vulnerability scanning on an image
	Scan(ctx context.Context, image string) (*ScanResult, error)

	// Tool returns the scanner tool name
	Tool() ScanTool

	// Version returns the scanner version
	Version(ctx context.Context) (string, error)

	// CheckInstalled checks if the scanner is installed
	CheckInstalled(ctx context.Context) error

	// CheckVersion checks if the scanner meets minimum version requirements
	CheckVersion(ctx context.Context) error

	// Install installs the scanner if not already present
	Install(ctx context.Context) error
}

// NewScanner creates a new scanner for the specified tool.
// Versions can be overridden via SC_GRYPE_VERSION or SC_TRIVY_VERSION env vars.
func NewScanner(tool ScanTool) (Scanner, error) {
	return NewScannerWithVersion(tool, "")
}

// NewScannerWithVersion creates a scanner pinned to a specific version.
// Priority: explicit version arg > SC_GRYPE_VERSION / SC_TRIVY_VERSION env var > built-in default.
// The resolved version is used for both install target and minimum required check.
func NewScannerWithVersion(tool ScanTool, version string) (Scanner, error) {
	switch tool {
	case ScanToolGrype:
		s := NewGrypeScanner()
		if v := resolveVersion(version, "SC_GRYPE_VERSION"); v != "" {
			s.installVersion = v
			s.minVersion = v
		}
		return s, nil
	case ScanToolTrivy:
		s := NewTrivyScanner()
		if v := resolveVersion(version, "SC_TRIVY_VERSION"); v != "" {
			s.installVersion = v
			s.minVersion = v
		}
		return s, nil
	default:
		return nil, fmt.Errorf("unsupported scan tool %q: supported tools are %q and %q", tool, ScanToolGrype, ScanToolTrivy)
	}
}

// resolveVersion returns the first non-empty value from: explicit arg, env var.
func resolveVersion(explicit, envVar string) string {
	if explicit != "" {
		return explicit
	}
	return os.Getenv(envVar)
}
