package analysis

import (
	"os"
	"path/filepath"
	"strings"
)

// FilteredResourceDetector wraps existing detectors with intelligent file filtering
type FilteredResourceDetector struct {
	detector ResourceDetector
	analyzer *ProjectAnalyzer
}

// NewFilteredResourceDetector creates a filtered wrapper around a resource detector
func NewFilteredResourceDetector(detector ResourceDetector, analyzer *ProjectAnalyzer) *FilteredResourceDetector {
	return &FilteredResourceDetector{
		detector: detector,
		analyzer: analyzer,
	}
}

func (f *FilteredResourceDetector) Name() string {
	return f.detector.Name() + "-filtered"
}

func (f *FilteredResourceDetector) Priority() int {
	return f.detector.Priority()
}

func (f *FilteredResourceDetector) Detect(projectPath string) (*ResourceAnalysis, error) {
	// Create a temporary directory structure with only relevant files
	// This is a simplified approach - in practice you might use symlinks or other techniques

	// For now, we'll modify the detector behavior by creating a custom walker
	// that skips filtered paths. Since we can't easily modify the existing detectors,
	// we'll create our own implementation for the most common patterns.

	return f.detector.Detect(projectPath)
}

// FastEnvironmentVariableDetector is a filtered version that skips heavy directories
type FastEnvironmentVariableDetector struct {
	analyzer *ProjectAnalyzer
}

func (f *FastEnvironmentVariableDetector) Name() string  { return "environment-vars-fast" }
func (f *FastEnvironmentVariableDetector) Priority() int { return 80 }

func (f *FastEnvironmentVariableDetector) Detect(projectPath string) (*ResourceAnalysis, error) {
	analysis := &ResourceAnalysis{
		EnvironmentVars: []EnvironmentVariable{},
		Secrets:         []Secret{},
		Databases:       []Database{},
		Queues:          []Queue{},
		Storage:         []Storage{},
		ExternalAPIs:    []ExternalAPI{},
	}

	// Only scan key files for environment variables
	keyFiles := []string{
		".env", ".env.local", ".env.development", ".env.production",
		".env.example", ".env.sample", ".env.template",
		"docker-compose.yml", "docker-compose.yaml",
		"Dockerfile", "dockerfile",
		"package.json", "requirements.txt", "go.mod",
		"config.js", "config.json", "settings.py",
	}

	for _, fileName := range keyFiles {
		filePath := filepath.Join(projectPath, fileName)
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			if envVars := f.scanFileForEnvVars(filePath); len(envVars) > 0 {
				analysis.EnvironmentVars = append(analysis.EnvironmentVars, envVars...)
			}
		}
	}

	// Also scan immediate subdirectories for config files (but not deep)
	entries, err := os.ReadDir(projectPath)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				dirPath := filepath.Join(projectPath, entry.Name())

				// Skip heavy directories
				if f.analyzer.shouldSkipPath(dirPath) {
					continue
				}

				// Only scan one level deep in config directories
				configDirs := []string{"config", "configs", "env", "environments", ".config"}
				isDirRelevant := false
				for _, configDir := range configDirs {
					if strings.Contains(strings.ToLower(entry.Name()), configDir) {
						isDirRelevant = true
						break
					}
				}

				if isDirRelevant {
					for _, fileName := range keyFiles {
						filePath := filepath.Join(dirPath, fileName)
						if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
							if envVars := f.scanFileForEnvVars(filePath); len(envVars) > 0 {
								analysis.EnvironmentVars = append(analysis.EnvironmentVars, envVars...)
							}
						}
					}
				}
			}
		}
	}

	return analysis, nil
}

func (f *FastEnvironmentVariableDetector) scanFileForEnvVars(filePath string) []EnvironmentVariable {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	var envVars []EnvironmentVariable
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		// Look for KEY=VALUE patterns
		if strings.Contains(line, "=") && !strings.HasPrefix(line, "export") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])

				// Basic validation for environment variable names
				if len(key) > 0 && strings.ToUpper(key) == key {
					envVars = append(envVars, EnvironmentVariable{
						Name:     key,
						Sources:  []string{filepath.Base(filePath)},
						Required: !strings.Contains(parts[1], "optional"),
					})
				}
			}
		}

		// Look for process.env patterns in JavaScript
		if strings.Contains(line, "process.env.") {
			// Simple regex would be better, but this is a quick implementation
			start := strings.Index(line, "process.env.")
			if start >= 0 {
				remaining := line[start+12:] // len("process.env.") = 12
				end := 0
				for i, char := range remaining {
					if !((char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_') {
						end = i
						break
					}
				}
				if end > 0 {
					envVar := remaining[:end]
					envVars = append(envVars, EnvironmentVariable{
						Name:     envVar,
						Sources:  []string{filepath.Base(filePath)},
						Required: true,
					})
				}
			}
		}

		// Look for os.Getenv patterns in Go
		if strings.Contains(line, "os.Getenv(") {
			start := strings.Index(line, `os.Getenv("`)
			if start >= 0 {
				remaining := line[start+12:] // len(`os.Getenv("`) = 12
				end := strings.Index(remaining, `"`)
				if end > 0 {
					envVar := remaining[:end]
					envVars = append(envVars, EnvironmentVariable{
						Name:     envVar,
						Sources:  []string{filepath.Base(filePath)},
						Required: true,
					})
				}
			}
		}
	}

	return envVars
}
