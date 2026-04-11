package scan

import (
	"context"
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

// NewScanner creates a new scanner for the specified tool
func NewScanner(tool ScanTool) (Scanner, error) {
	return NewScannerWithVersion(tool, "")
}

// NewScannerWithVersion creates a scanner pinned to a specific version.
// The version is used both as the install target and as the minimum required version.
// If version is empty, each scanner's built-in minimum version is used.
func NewScannerWithVersion(tool ScanTool, version string) (Scanner, error) {
	switch tool {
	case ScanToolGrype:
		s := NewGrypeScanner()
		if version != "" {
			s.minVersion = version
		}
		return s, nil
	case ScanToolTrivy:
		s := NewTrivyScanner()
		if version != "" {
			s.minVersion = version
		}
		return s, nil
	default:
		s := NewGrypeScanner()
		if version != "" {
			s.minVersion = version
		}
		return s, nil
	}
}
