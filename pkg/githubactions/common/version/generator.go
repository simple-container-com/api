package version

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/simple-container-com/api/pkg/githubactions/config"
	"github.com/simple-container-com/api/pkg/githubactions/utils/logging"
)

// Generator handles version generation for deployments
type Generator struct {
	cfg     *config.Config
	logger  logging.Logger
	version string // cached generated version
}

// NewGenerator creates a new version generator
func NewGenerator(cfg *config.Config, logger logging.Logger) *Generator {
	return &Generator{
		cfg:    cfg,
		logger: logger,
	}
}

// GenerateCalVer generates a Calendar Versioning (CalVer) version string
func (v *Generator) GenerateCalVer(ctx context.Context) (string, error) {
	v.logger.Info("Generating CalVer version")

	// If app-image-version is provided, use that instead
	if v.cfg.AppImageVersion != "" {
		v.logger.Info("Using provided app-image-version", "version", v.cfg.AppImageVersion)
		v.version = v.cfg.AppImageVersion
		return v.version, nil
	}

	// Generate CalVer format: YYYY.M.D.BUILD_NUMBER
	now := time.Now().UTC()
	year := now.Year()
	month := int(now.Month()) // Remove leading zero
	day := now.Day()          // Remove leading zero

	// Use GitHub run number as build number, fallback to timestamp
	buildNumber := v.getBuildNumber()

	// Base version
	version := fmt.Sprintf("%d.%d.%d.%d", year, month, day, buildNumber)

	// Add suffix if provided
	if v.cfg.VersionSuffix != "" {
		version = version + v.cfg.VersionSuffix
	}

	// Validate version doesn't conflict (for production deployments)
	if !v.cfg.PRPreview {
		version = v.validateVersion(ctx, version)
	}

	v.logger.Info("Generated CalVer version", "version", version)
	v.version = version
	return version, nil
}

// getBuildNumber determines the build number for versioning
func (v *Generator) getBuildNumber() int {
	// Try to use GitHub run number first
	if v.cfg.GitHubRunNumber != "" {
		if runNumber, err := strconv.Atoi(v.cfg.GitHubRunNumber); err == nil {
			return runNumber
		}
	}

	// Fallback to timestamp-based build number (HHMMSS format)
	now := time.Now().UTC()
	timeNumber := now.Hour()*10000 + now.Minute()*100 + now.Second()
	return timeNumber
}

// validateVersion ensures the version doesn't conflict with existing releases
func (v *Generator) validateVersion(ctx context.Context, version string) string {
	// For now, we'll just return the version as-is
	// In a more advanced implementation, we could check GitHub releases API
	// to ensure the version doesn't already exist

	// If we detect a potential conflict, we could append a timestamp
	// But for GitHub Actions, run numbers should be unique enough

	return version
}

// GetCurrentVersion returns the currently generated version
func (v *Generator) GetCurrentVersion() string {
	return v.version
}

// FormatVersionForTag formats the version for use as a Git tag
func (v *Generator) FormatVersionForTag() string {
	if v.version == "" {
		return ""
	}

	// Git tags should start with 'v'
	if !strings.HasPrefix(v.version, "v") {
		return "v" + v.version
	}

	return v.version
}

// GenerateImageTag generates a container image tag
func (v *Generator) GenerateImageTag() string {
	if v.version == "" {
		return "latest"
	}

	// Container tags should not have 'v' prefix
	tag := strings.TrimPrefix(v.version, "v")

	// Replace any invalid characters for container tags
	tag = strings.ReplaceAll(tag, "+", "-")

	return tag
}

// IsPreviewVersion returns true if this is a preview/development version
func (v *Generator) IsPreviewVersion() bool {
	return v.cfg.PRPreview || strings.Contains(v.version, "preview") || strings.Contains(v.version, "dev")
}
