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
	Resources       *ResourceAnalysis      `json:"resources,omitempty"` // Detected resources
	Git             *GitAnalysis           `json:"git,omitempty"`       // Git repository analysis
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
	Path       string            `json:"path"`
	Type       string            `json:"type"` // "config", "source", "build", "docs"
	Language   string            `json:"language,omitempty"`
	Purpose    string            `json:"purpose,omitempty"`
	Size       int64             `json:"size"`
	Complexity *CodeComplexity   `json:"complexity,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// CodeComplexity represents code complexity metrics
type CodeComplexity struct {
	LinesOfCode     int     `json:"lines_of_code"`
	CyclomaticScore int     `json:"cyclomatic_score,omitempty"`
	FunctionCount   int     `json:"function_count,omitempty"`
	ClassCount      int     `json:"class_count,omitempty"`
	ImportCount     int     `json:"import_count,omitempty"`
	CommentRatio    float32 `json:"comment_ratio,omitempty"`
	ComplexityLevel string  `json:"complexity_level"` // "low", "medium", "high", "very_high"
}

// ResourceAnalysis contains detected project resources
type ResourceAnalysis struct {
	EnvironmentVars []EnvironmentVariable `json:"environment_variables,omitempty"`
	Secrets         []Secret              `json:"secrets,omitempty"`
	Databases       []Database            `json:"databases,omitempty"`
	Queues          []Queue               `json:"queues,omitempty"`
	Storage         []Storage             `json:"storage,omitempty"`
	ExternalAPIs    []ExternalAPI         `json:"external_apis,omitempty"`
}

// GitAnalysis contains Git repository analysis
type GitAnalysis struct {
	IsGitRepo      bool              `json:"is_git_repo"`
	RemoteURL      string            `json:"remote_url,omitempty"`
	Branch         string            `json:"current_branch,omitempty"`
	LastCommit     *GitCommit        `json:"last_commit,omitempty"`
	Contributors   []GitContributor  `json:"contributors,omitempty"`
	CommitActivity *CommitActivity   `json:"commit_activity,omitempty"`
	FileChanges    *FileChangeStats  `json:"file_changes,omitempty"`
	HasCI          bool              `json:"has_ci"`
	CIPlatforms    []string          `json:"ci_platforms,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	ProjectAge     int               `json:"project_age_days,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// GitCommit represents a Git commit
type GitCommit struct {
	Hash         string `json:"hash"`
	Author       string `json:"author"`
	Email        string `json:"email"`
	Date         string `json:"date"`
	Message      string `json:"message"`
	FilesChanged int    `json:"files_changed"`
}

// GitContributor represents a contributor
type GitContributor struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Commits int    `json:"commits"`
}

// CommitActivity represents commit patterns
type CommitActivity struct {
	TotalCommits   int     `json:"total_commits"`
	RecentCommits  int     `json:"recent_commits_30d"`
	AveragePerWeek float32 `json:"average_per_week"`
	MostActiveDay  string  `json:"most_active_day,omitempty"`
	MostActiveHour int     `json:"most_active_hour,omitempty"`
}

// FileChangeStats represents file change patterns
type FileChangeStats struct {
	MostChangedFiles []FileChangeInfo `json:"most_changed_files,omitempty"`
	LanguageChanges  map[string]int   `json:"language_changes,omitempty"`
	LargestFiles     []FileInfo       `json:"largest_files,omitempty"`
}

// FileChangeInfo represents file change information
type FileChangeInfo struct {
	Path    string `json:"path"`
	Changes int    `json:"changes"`
	Type    string `json:"type"`
}

// EnvironmentVariable represents a detected environment variable
type EnvironmentVariable struct {
	Name        string   `json:"name"`
	Sources     []string `json:"sources"`    // Files where found
	UsageType   string   `json:"usage_type"` // "config", "secret", "url", "feature_flag", etc.
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"required"`
	DefaultVal  string   `json:"default_value,omitempty"`
}

// Secret represents detected sensitive data patterns
type Secret struct {
	Type        string   `json:"type"` // "api_key", "database_url", "jwt_secret", etc.
	Name        string   `json:"name,omitempty"`
	Sources     []string `json:"sources"`               // Files where patterns found
	Pattern     string   `json:"pattern,omitempty"`     // Regex pattern that matched
	Confidence  float32  `json:"confidence"`            // How confident we are this is a secret
	Recommended string   `json:"recommended,omitempty"` // Recommended Simple Container resource
}

// Database represents detected database usage
type Database struct {
	Type        string            `json:"type"` // "postgresql", "mysql", "mongodb", "redis", etc.
	Name        string            `json:"name,omitempty"`
	Sources     []string          `json:"sources"`              // Files where detected
	Connection  string            `json:"connection,omitempty"` // Connection method/library
	Version     string            `json:"version,omitempty"`
	Config      map[string]string `json:"config,omitempty"` // Database-specific config
	Confidence  float32           `json:"confidence"`
	Recommended string            `json:"recommended,omitempty"` // Recommended Simple Container resource
}

// Queue represents detected queue/messaging system
type Queue struct {
	Type        string   `json:"type"` // "rabbitmq", "sqs", "kafka", "redis_pubsub", etc.
	Name        string   `json:"name,omitempty"`
	Sources     []string `json:"sources"`          // Files where detected
	Topics      []string `json:"topics,omitempty"` // Detected topics/queues
	Confidence  float32  `json:"confidence"`
	Recommended string   `json:"recommended,omitempty"` // Recommended Simple Container resource
}

// Storage represents detected storage services
type Storage struct {
	Type        string   `json:"type"` // "s3", "gcs", "azure_blob", "file_upload", etc.
	Name        string   `json:"name,omitempty"`
	Sources     []string `json:"sources"`           // Files where detected
	Buckets     []string `json:"buckets,omitempty"` // Detected bucket names
	Purpose     string   `json:"purpose,omitempty"` // "uploads", "static", "backup", etc.
	Confidence  float32  `json:"confidence"`
	Recommended string   `json:"recommended,omitempty"` // Recommended Simple Container resource
}

// ExternalAPI represents detected external API usage
type ExternalAPI struct {
	Name       string   `json:"name"`                // "stripe", "sendgrid", "openai", etc.
	Sources    []string `json:"sources"`             // Files where detected
	Endpoints  []string `json:"endpoints,omitempty"` // API endpoints found
	Purpose    string   `json:"purpose,omitempty"`   // "payment", "email", "ai", etc.
	Confidence float32  `json:"confidence"`
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

		// Skip dependency directories for performance
		if ShouldSkipPath(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
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

		// Skip dependency directories for performance
		if ShouldSkipPath(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
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

// SimpleContainerDetector detects existing Simple Container usage
type SimpleContainerDetector struct{}

func (d *SimpleContainerDetector) Name() string  { return "simple-container" }
func (d *SimpleContainerDetector) Priority() int { return 95 }

func (d *SimpleContainerDetector) Detect(projectPath string) (*TechStackInfo, error) {
	stack := &TechStackInfo{
		Language:   "yaml",
		Framework:  "simple-container",
		Runtime:    "simple-container",
		Confidence: 0.0,
		Evidence:   []string{},
		Metadata:   make(map[string]string),
	}

	// Check for .sc directory
	scDir := filepath.Join(projectPath, ".sc")
	if _, err := os.Stat(scDir); err == nil {
		stack.Confidence += 0.4
		stack.Evidence = append(stack.Evidence, ".sc directory found")
		stack.Metadata["has_sc_directory"] = "true"
	}

	// Check for client.yaml
	clientYaml := filepath.Join(projectPath, ".sc", "client.yaml")
	if _, err := os.Stat(clientYaml); err == nil {
		stack.Confidence += 0.3
		stack.Evidence = append(stack.Evidence, "client.yaml found")
		stack.Metadata["has_client_config"] = "true"
	}

	// Check for server.yaml
	serverYaml := filepath.Join(projectPath, ".sc", "server.yaml")
	if _, err := os.Stat(serverYaml); err == nil {
		stack.Confidence += 0.3
		stack.Evidence = append(stack.Evidence, "server.yaml found")
		stack.Metadata["has_server_config"] = "true"
	}

	// Check for welder.yaml
	welderYaml := filepath.Join(projectPath, "welder.yaml")
	if _, err := os.Stat(welderYaml); err == nil {
		stack.Confidence += 0.2
		stack.Evidence = append(stack.Evidence, "welder.yaml found")
		stack.Metadata["has_welder_config"] = "true"
	}

	// Check for simple-container references in package files
	packageFiles := []string{"package.json", "requirements.txt", "go.mod", "Cargo.toml", "composer.json"}
	for _, file := range packageFiles {
		filePath := filepath.Join(projectPath, file)
		if content, err := os.ReadFile(filePath); err == nil {
			contentStr := string(content)
			if strings.Contains(contentStr, "simple-container") {
				stack.Confidence += 0.1
				stack.Evidence = append(stack.Evidence, fmt.Sprintf("simple-container reference in %s", file))
			}
		}
	}

	// Check for SC CLI usage in scripts
	scriptFiles := []string{".github/workflows", "scripts", "Makefile", "makefile"}
	for _, dir := range scriptFiles {
		dirPath := filepath.Join(projectPath, dir)
		if err := filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() && (strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml") ||
				strings.HasSuffix(path, ".sh") || strings.HasSuffix(path, "Makefile")) {
				if content, err := os.ReadFile(path); err == nil {
					contentStr := string(content)
					if strings.Contains(contentStr, " sc ") || strings.Contains(contentStr, "simple-container") {
						stack.Confidence += 0.1
						stack.Evidence = append(stack.Evidence, fmt.Sprintf("SC CLI usage in %s", filepath.Base(path)))
					}
				}
			}
			return nil
		}); err == nil && stack.Confidence > 0 {
			break
		}
	}

	if stack.Confidence == 0 {
		return nil, fmt.Errorf("simple container not detected")
	}

	// Determine version/maturity level
	if stack.Confidence >= 0.7 {
		stack.Version = "configured"
		stack.Metadata["maturity"] = "full"
	} else if stack.Confidence >= 0.4 {
		stack.Version = "partial"
		stack.Metadata["maturity"] = "partial"
	} else {
		stack.Version = "minimal"
		stack.Metadata["maturity"] = "minimal"
	}

	return stack, nil
}

// PulumiDetector detects Pulumi infrastructure as code usage
type PulumiDetector struct{}

func (d *PulumiDetector) Name() string  { return "pulumi" }
func (d *PulumiDetector) Priority() int { return 75 }

func (d *PulumiDetector) Detect(projectPath string) (*TechStackInfo, error) {
	stack := &TechStackInfo{
		Language:   "yaml",
		Framework:  "pulumi",
		Runtime:    "pulumi",
		Confidence: 0.0,
		Evidence:   []string{},
		Metadata:   make(map[string]string),
	}

	// Check for Pulumi.yaml
	pulumiYaml := filepath.Join(projectPath, "Pulumi.yaml")
	if _, err := os.Stat(pulumiYaml); err == nil {
		stack.Confidence += 0.6
		stack.Evidence = append(stack.Evidence, "Pulumi.yaml found")

		// Try to read Pulumi.yaml to get more details
		if content, err := os.ReadFile(pulumiYaml); err == nil {
			contentStr := string(content)
			if strings.Contains(contentStr, "runtime:") {
				if strings.Contains(contentStr, "nodejs") {
					stack.Metadata["runtime"] = "nodejs"
				} else if strings.Contains(contentStr, "python") {
					stack.Metadata["runtime"] = "python"
				} else if strings.Contains(contentStr, "go") {
					stack.Metadata["runtime"] = "go"
				}
			}
		}
	}

	// Check for Pulumi stack files (Pulumi.dev.yaml, Pulumi.prod.yaml, etc.)
	entries, err := os.ReadDir(projectPath)
	if err == nil {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "Pulumi.") && strings.HasSuffix(entry.Name(), ".yaml") &&
				entry.Name() != "Pulumi.yaml" {
				stack.Confidence += 0.2
				stack.Evidence = append(stack.Evidence, fmt.Sprintf("stack config %s found", entry.Name()))
			}
		}
	}

	// Check for Pulumi dependencies in package.json
	packageJson := filepath.Join(projectPath, "package.json")
	if content, err := os.ReadFile(packageJson); err == nil {
		contentStr := string(content)
		pulumiPackages := []string{"@pulumi/", "pulumi"}
		for _, pkg := range pulumiPackages {
			if strings.Contains(contentStr, pkg) {
				stack.Confidence += 0.2
				stack.Evidence = append(stack.Evidence, "Pulumi packages in package.json")
				break
			}
		}
	}

	// Check for requirements.txt (Python)
	requirementsTxt := filepath.Join(projectPath, "requirements.txt")
	if content, err := os.ReadFile(requirementsTxt); err == nil {
		contentStr := string(content)
		if strings.Contains(contentStr, "pulumi") {
			stack.Confidence += 0.2
			stack.Evidence = append(stack.Evidence, "pulumi in requirements.txt")
		}
	}

	// Check for go.mod (Go)
	goMod := filepath.Join(projectPath, "go.mod")
	if content, err := os.ReadFile(goMod); err == nil {
		contentStr := string(content)
		if strings.Contains(contentStr, "github.com/pulumi/pulumi") {
			stack.Confidence += 0.2
			stack.Evidence = append(stack.Evidence, "Pulumi SDK in go.mod")
		}
	}

	if stack.Confidence == 0 {
		return nil, fmt.Errorf("pulumi not detected")
	}

	stack.Version = "detected"
	return stack, nil
}

// TerraformDetector detects Terraform infrastructure as code usage
type TerraformDetector struct{}

func (d *TerraformDetector) Name() string  { return "terraform" }
func (d *TerraformDetector) Priority() int { return 75 }

func (d *TerraformDetector) Detect(projectPath string) (*TechStackInfo, error) {
	stack := &TechStackInfo{
		Language:     "hcl",
		Framework:    "terraform",
		Runtime:      "terraform",
		Confidence:   0.0,
		Evidence:     []string{},
		Metadata:     make(map[string]string),
		Dependencies: []Dependency{},
	}

	// Check for .tf files
	tfFiles := 0
	err := filepath.WalkDir(projectPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip dependency directories for performance
		if ShouldSkipPath(path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !d.IsDir() && strings.HasSuffix(d.Name(), ".tf") {
			tfFiles++
			if tfFiles == 1 {
				stack.Evidence = append(stack.Evidence, "Terraform .tf files found")
			}
		}
		return nil
	})

	if err == nil && tfFiles > 0 {
		stack.Confidence += float32(tfFiles) * 0.1
		if stack.Confidence > 0.6 {
			stack.Confidence = 0.6 // Cap at 0.6 for tf files
		}
		stack.Metadata["tf_files_count"] = fmt.Sprintf("%d", tfFiles)
	}

	// Check for terraform.tfvars
	tfvars := filepath.Join(projectPath, "terraform.tfvars")
	if _, err := os.Stat(tfvars); err == nil {
		stack.Confidence += 0.2
		stack.Evidence = append(stack.Evidence, "terraform.tfvars found")
	}

	// Check for .terraform directory (indicates initialized project)
	terraformDir := filepath.Join(projectPath, ".terraform")
	if _, err := os.Stat(terraformDir); err == nil {
		stack.Confidence += 0.3
		stack.Evidence = append(stack.Evidence, ".terraform directory found")
		stack.Metadata["initialized"] = "true"
	}

	// Check for terraform.tfstate or terraform.tfstate.backup
	tfstateFiles := []string{"terraform.tfstate", "terraform.tfstate.backup"}
	for _, file := range tfstateFiles {
		if _, err := os.Stat(filepath.Join(projectPath, file)); err == nil {
			stack.Confidence += 0.1
			stack.Evidence = append(stack.Evidence, fmt.Sprintf("%s found", file))
		}
	}

	// Check for provider configurations in .tf files
	if tfFiles > 0 {
		providers := make(map[string]bool)
		_ = filepath.WalkDir(projectPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			// Skip dependency directories for performance
			if ShouldSkipPath(path) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if !d.IsDir() && strings.HasSuffix(d.Name(), ".tf") {
				if content, err := os.ReadFile(path); err == nil {
					contentStr := string(content)
					commonProviders := []string{"aws", "azure", "google", "kubernetes", "helm", "random", "local"}
					for _, provider := range commonProviders {
						if strings.Contains(contentStr, fmt.Sprintf("provider \"%s\"", provider)) ||
							strings.Contains(contentStr, fmt.Sprintf(`provider "%s"`, provider)) {
							if !providers[provider] {
								providers[provider] = true
								stack.Dependencies = append(stack.Dependencies, Dependency{
									Name: provider,
									Type: "terraform-provider",
								})
							}
						}
					}
				}
			}
			return nil
		})
	}

	if stack.Confidence == 0 {
		return nil, fmt.Errorf("terraform not detected")
	}

	// Determine version/usage level
	if stack.Confidence >= 0.7 {
		stack.Version = "active"
		stack.Metadata["usage"] = "active"
	} else if stack.Confidence >= 0.4 {
		stack.Version = "configured"
		stack.Metadata["usage"] = "configured"
	} else {
		stack.Version = "minimal"
		stack.Metadata["usage"] = "minimal"
	}

	return stack, nil
}
