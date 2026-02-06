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
}

// NewScanner creates a new scanner for the specified tool
func NewScanner(tool ScanTool) (Scanner, error) {
	switch tool {
	case ScanToolGrype:
		return NewGrypeScanner(), nil
	case ScanToolTrivy:
		return NewTrivyScanner(), nil
	default:
		return NewGrypeScanner(), nil
	}
}
