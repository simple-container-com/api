package analysis

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// TechStackDetector interface for implementing different detection strategies
type TechStackDetector interface {
	Detect(projectPath string) (*TechStackInfo, error)
	Priority() int // Higher priority detectors run first
	Name() string
}

// TechStackInfo represents detected technology information
type TechStackInfo struct {
	Language     string            `json:"language"`
	Framework    string            `json:"framework,omitempty"`
	Runtime      string            `json:"runtime,omitempty"`
	Version      string            `json:"version,omitempty"`
	Dependencies []Dependency      `json:"dependencies,omitempty"`
	DevDeps      []Dependency      `json:"dev_dependencies,omitempty"`
	Scripts      map[string]string `json:"scripts,omitempty"`
	Confidence   float32           `json:"confidence"`
	Evidence     []string          `json:"evidence,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Dependency represents a project dependency
type Dependency struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Type    string `json:"type,omitempty"` // "runtime", "dev", "peer", etc.
}

// ProjectAnalysis contains complete analysis results
type ProjectAnalysis struct {
	Path            string                 `json:"path"`
	Name            string                 `json:"name"`
	TechStacks      []TechStackInfo        `json:"tech_stacks"`
	PrimaryStack    *TechStackInfo         `json:"primary_stack,omitempty"`
	Architecture    string                 `json:"architecture,omitempty"`
	Recommendations []Recommendation       `json:"recommendations"`
	Files           []FileInfo             `json:"files,omitempty"`
	Confidence      float32                `json:"overall_confidence"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// Recommendation for Simple Container configuration
type Recommendation struct {
	Type        string `json:"type"`     // "resource", "template", "configuration", "optimization"
	Category    string `json:"category"` // "database", "storage", "compute", "security", etc.
	Priority    string `json:"priority"` // "high", "medium", "low"
	Title       string `json:"title"`
	Description string `json:"description"`
	Action      string `json:"action,omitempty"`   // Specific action to take
	Resource    string `json:"resource,omitempty"` // Simple Container resource type
	Template    string `json:"template,omitempty"` // Simple Container template
	Code        string `json:"code,omitempty"`     // Code snippet
}

// FileInfo represents analyzed file information
type FileInfo struct {
	Path     string            `json:"path"`
	Type     string            `json:"type"` // "config", "source", "build", "docs"
	Language string            `json:"language,omitempty"`
	Purpose  string            `json:"purpose,omitempty"`
	Size     int64             `json:"size"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// NodeJSDetector detects Node.js projects
type NodeJSDetector struct{}

func (d *NodeJSDetector) Name() string  { return "nodejs" }
func (d *NodeJSDetector) Priority() int { return 90 }

func (d *NodeJSDetector) Detect(projectPath string) (*TechStackInfo, error) {
	packageJSONPath := filepath.Join(projectPath, "package.json")

	if _, err := os.Stat(packageJSONPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("package.json not found")
	}

	content, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return nil, err
	}

	var packageJSON struct {
		Name            string            `json:"name"`
		Version         string            `json:"version"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
		Scripts         map[string]string `json:"scripts"`
		Engines         struct {
			Node string `json:"node"`
			NPM  string `json:"npm"`
		} `json:"engines"`
	}

	if err := json.Unmarshal(content, &packageJSON); err != nil {
		return nil, fmt.Errorf("invalid package.json: %w", err)
	}

	stack := &TechStackInfo{
		Language:   "javascript",
		Runtime:    "nodejs",
		Version:    packageJSON.Engines.Node,
		Scripts:    packageJSON.Scripts,
		Confidence: 0.95,
		Evidence:   []string{"package.json found"},
		Metadata: map[string]string{
			"package_name":    packageJSON.Name,
			"package_version": packageJSON.Version,
		},
	}

	// Parse dependencies
	for name, version := range packageJSON.Dependencies {
		stack.Dependencies = append(stack.Dependencies, Dependency{
			Name:    name,
			Version: version,
			Type:    "runtime",
		})
	}

	for name, version := range packageJSON.DevDependencies {
		stack.DevDeps = append(stack.DevDeps, Dependency{
			Name:    name,
			Version: version,
			Type:    "dev",
		})
	}

	// Detect framework
	framework := d.detectFramework(packageJSON.Dependencies)
	if framework != "" {
		stack.Framework = framework
		stack.Evidence = append(stack.Evidence, fmt.Sprintf("%s dependency found", framework))
	}

	return stack, nil
}

func (d *NodeJSDetector) detectFramework(deps map[string]string) string {
	frameworks := map[string]string{
		"express": "express",
		"koa":     "koa",
		"fastify": "fastify",
		"nestjs":  "nestjs",
		"next":    "nextjs",
		"react":   "react",
		"vue":     "vue",
		"angular": "angular",
		"svelte":  "svelte",
		"gatsby":  "gatsby",
		"nuxt":    "nuxt",
	}

	for dep := range deps {
		if framework, exists := frameworks[dep]; exists {
			return framework
		}
		// Handle scoped packages like @nestjs/core
		if strings.HasPrefix(dep, "@nestjs/") {
			return "nestjs"
		}
		if strings.HasPrefix(dep, "@angular/") {
			return "angular"
		}
	}

	return ""
}

// PythonDetector detects Python projects
type PythonDetector struct{}

func (d *PythonDetector) Name() string  { return "python" }
func (d *PythonDetector) Priority() int { return 85 }

func (d *PythonDetector) Detect(projectPath string) (*TechStackInfo, error) {
	// Check for various Python files
	pythonFiles := []string{
		"requirements.txt",
		"setup.py",
		"pyproject.toml",
		"Pipfile",
		"poetry.lock",
	}

	var foundFiles []string
	var primaryFile string

	for _, file := range pythonFiles {
		if _, err := os.Stat(filepath.Join(projectPath, file)); err == nil {
			foundFiles = append(foundFiles, file)
			if primaryFile == "" {
				primaryFile = file
			}
		}
	}

	if len(foundFiles) == 0 {
		// Check for .py files
		hasPythonFiles, err := d.hasPythonSourceFiles(projectPath)
		if err != nil {
			return nil, err
		}
		if !hasPythonFiles {
			return nil, fmt.Errorf("no Python project files found")
		}
	}

	stack := &TechStackInfo{
		Language:   "python",
		Runtime:    "python",
		Confidence: 0.9,
		Evidence:   foundFiles,
		Metadata:   map[string]string{},
	}

	// Parse dependencies based on primary file
	switch primaryFile {
	case "requirements.txt":
		if err := d.parseRequirements(projectPath, stack); err == nil {
			stack.Evidence = append(stack.Evidence, "parsed requirements.txt")
		}
	case "setup.py":
		stack.Evidence = append(stack.Evidence, "setup.py found")
		stack.Metadata["build_system"] = "setuptools"
	case "pyproject.toml":
		stack.Evidence = append(stack.Evidence, "pyproject.toml found")
		stack.Metadata["build_system"] = "modern"
	case "Pipfile":
		stack.Evidence = append(stack.Evidence, "Pipfile found")
		stack.Metadata["dependency_manager"] = "pipenv"
	}

	// Detect framework
	framework := d.detectFramework(stack.Dependencies)
	if framework != "" {
		stack.Framework = framework
	}

	return stack, nil
}

func (d *PythonDetector) hasPythonSourceFiles(projectPath string) (bool, error) {
	found := false
	err := filepath.WalkDir(projectPath, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if strings.HasSuffix(entry.Name(), ".py") {
			found = true
			return fs.SkipAll // Stop walking
		}
		return nil
	})
	return found, err
}

func (d *PythonDetector) parseRequirements(projectPath string, stack *TechStackInfo) error {
	content, err := os.ReadFile(filepath.Join(projectPath, "requirements.txt"))
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse dependency (handle ==, >=, etc.)
		re := regexp.MustCompile(`^([a-zA-Z0-9\-_]+)([><=!]+.*)?$`)
		matches := re.FindStringSubmatch(line)
		if len(matches) >= 2 {
			dep := Dependency{
				Name: matches[1],
				Type: "runtime",
			}
			if len(matches) > 2 && matches[2] != "" {
				dep.Version = matches[2]
			}
			stack.Dependencies = append(stack.Dependencies, dep)
		}
	}

	return nil
}

func (d *PythonDetector) detectFramework(deps []Dependency) string {
	frameworks := map[string]string{
		"django":    "django",
		"flask":     "flask",
		"fastapi":   "fastapi",
		"tornado":   "tornado",
		"bottle":    "bottle",
		"pyramid":   "pyramid",
		"starlette": "starlette",
	}

	for _, dep := range deps {
		if framework, exists := frameworks[strings.ToLower(dep.Name)]; exists {
			return framework
		}
	}

	return ""
}

// GoDetector detects Go projects
type GoDetector struct{}

func (d *GoDetector) Name() string  { return "go" }
func (d *GoDetector) Priority() int { return 80 }

func (d *GoDetector) Detect(projectPath string) (*TechStackInfo, error) {
	goModPath := filepath.Join(projectPath, "go.mod")

	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		// Check for .go files without go.mod (legacy GOPATH mode)
		hasGoFiles, err := d.hasGoSourceFiles(projectPath)
		if err != nil {
			return nil, err
		}
		if !hasGoFiles {
			return nil, fmt.Errorf("no Go project files found")
		}

		return &TechStackInfo{
			Language:   "go",
			Runtime:    "go",
			Confidence: 0.7, // Lower confidence without go.mod
			Evidence:   []string{".go files found (legacy GOPATH mode)"},
			Metadata: map[string]string{
				"mode": "gopath",
			},
		}, nil
	}

	content, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, err
	}

	stack := &TechStackInfo{
		Language:   "go",
		Runtime:    "go",
		Confidence: 0.95,
		Evidence:   []string{"go.mod found"},
		Metadata: map[string]string{
			"mode": "modules",
		},
	}

	// Parse go.mod
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Parse module name
		if strings.HasPrefix(line, "module ") {
			module := strings.TrimPrefix(line, "module ")
			stack.Metadata["module"] = module
		}

		// Parse Go version
		if strings.HasPrefix(line, "go ") {
			version := strings.TrimPrefix(line, "go ")
			stack.Version = version
		}
	}

	// Detect framework by checking imports
	framework := d.detectFramework(projectPath)
	if framework != "" {
		stack.Framework = framework
		stack.Evidence = append(stack.Evidence, fmt.Sprintf("%s framework detected", framework))
	}

	return stack, nil
}

func (d *GoDetector) hasGoSourceFiles(projectPath string) (bool, error) {
	found := false
	err := filepath.WalkDir(projectPath, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found, err
}

func (d *GoDetector) detectFramework(projectPath string) string {
	// Framework detection in priority order: web frameworks first, then CLI frameworks
	frameworks := []struct {
		importPath string
		framework  string
	}{
		{"gin-gonic/gin", "gin"},
		{"gorilla/mux", "gorilla-mux"},
		{"labstack/echo", "echo"},
		{"gofiber/fiber", "fiber"},
		{"go-chi/chi", "chi"},
		{"spf13/cobra", "cobra"},
		{"urfave/cli", "cli"},
	}

	// Read go.mod for dependencies
	goModPath := filepath.Join(projectPath, "go.mod")
	if content, err := os.ReadFile(goModPath); err == nil {
		modContent := string(content)
		for _, fw := range frameworks {
			if strings.Contains(modContent, fw.importPath) {
				return fw.framework
			}
		}
	}

	return ""
}

// DockerDetector detects containerized projects
type DockerDetector struct{}

func (d *DockerDetector) Name() string  { return "docker" }
func (d *DockerDetector) Priority() int { return 70 }

func (d *DockerDetector) Detect(projectPath string) (*TechStackInfo, error) {
	dockerFiles := []string{
		"Dockerfile",
		"docker-compose.yml",
		"docker-compose.yaml",
		".dockerignore",
	}

	var foundFiles []string
	for _, file := range dockerFiles {
		if _, err := os.Stat(filepath.Join(projectPath, file)); err == nil {
			foundFiles = append(foundFiles, file)
		}
	}

	if len(foundFiles) == 0 {
		return nil, fmt.Errorf("no Docker files found")
	}

	stack := &TechStackInfo{
		Language:   "docker",
		Runtime:    "docker",
		Confidence: 0.8,
		Evidence:   foundFiles,
		Metadata:   map[string]string{},
	}

	// Parse Dockerfile for base image
	dockerfilePath := filepath.Join(projectPath, "Dockerfile")
	if content, err := os.ReadFile(dockerfilePath); err == nil {
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(strings.ToUpper(line), "FROM ") {
				baseImage := strings.TrimPrefix(strings.ToUpper(line), "FROM ")
				baseImage = strings.Split(baseImage, " ")[0] // Remove AS alias
				stack.Metadata["base_image"] = strings.ToLower(baseImage)

				// Detect runtime from base image
				baseImageLower := strings.ToLower(baseImage)
				if strings.Contains(baseImageLower, "node") {
					stack.Framework = "nodejs"
				} else if strings.Contains(baseImageLower, "python") {
					stack.Framework = "python"
				} else if strings.Contains(baseImageLower, "golang") {
					stack.Framework = "go"
				}
				break
			}
		}
	}

	return stack, nil
}
