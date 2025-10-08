package analysis

import (
	"os"
	"path/filepath"
	"strings"
)

// analyzeFiles provides file-level analysis
func (pa *ProjectAnalyzer) analyzeFiles(projectPath string) ([]FileInfo, error) {
	var files []FileInfo

	err := filepath.WalkDir(projectPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Continue on errors
		}

		// Skip if this path should be filtered
		if pa.shouldSkipPath(path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil // Skip directories, only analyze files
		}

		relPath, _ := filepath.Rel(projectPath, path)

		file := FileInfo{
			Path:     relPath,
			Type:     pa.getFileType(d.Name()),
			Language: "",
		}

		// Determine programming language from extension
		ext := strings.ToLower(filepath.Ext(d.Name()))
		switch ext {
		case ".js", ".jsx", ".mjs":
			file.Language = "javascript"
		case ".ts", ".tsx":
			file.Language = "typescript"
		case ".py":
			file.Language = "python"
		case ".go":
			file.Language = "go"
		case ".java":
			file.Language = "java"
		case ".rb":
			file.Language = "ruby"
		case ".php":
			file.Language = "php"
		case ".rs":
			file.Language = "rust"
		case ".cpp", ".cc", ".cxx":
			file.Language = "cpp"
		case ".c":
			file.Language = "c"
		case ".cs":
			file.Language = "csharp"
		}

		// Analyze complexity for source files (conditionally skip for performance)
		if file.Type == "source" && file.Language != "" && !pa.skipComplexity {
			complexityAnalyzer := NewComplexityAnalyzer()
			if complexity, err := complexityAnalyzer.AnalyzeFile(path, file.Language); err == nil {
				file.Complexity = complexity
			}
		}

		files = append(files, file)
		return nil
	})

	return files, err
}

func (pa *ProjectAnalyzer) getFileType(filename string) string {
	configFiles := map[string]bool{
		"package.json":        true,
		"requirements.txt":    true,
		"go.mod":              true,
		"Cargo.toml":          true,
		"pom.xml":             true,
		"build.gradle":        true,
		"composer.json":       true,
		"Gemfile":             true,
		"setup.py":            true,
		"pyproject.toml":      true,
		"yarn.lock":           true,
		"package-lock.json":   true,
		"go.sum":              true,
		"Cargo.lock":          true,
		"Pipfile":             true,
		"poetry.lock":         true,
		"docker-compose.yml":  true,
		"docker-compose.yaml": true,
		"Dockerfile":          true,
		"dockerfile":          true,
		"Makefile":            true,
		"makefile":            true,
		".gitignore":          true,
		".env":                true,
		".env.example":        true,
		".env.local":          true,
		"tsconfig.json":       true,
		"webpack.config.js":   true,
		"vite.config.js":      true,
		"next.config.js":      true,
		"nuxt.config.js":      true,
		"babel.config.js":     true,
		"eslint.config.js":    true,
		".eslintrc":           true,
		".eslintrc.js":        true,
		".eslintrc.json":      true,
		"prettier.config.js":  true,
		".prettierrc":         true,
		"jest.config.js":      true,
		"vitest.config.js":    true,
		".github":             true,
		"LICENSE":             true,
		"license":             true,
	}

	// Documentation files should be categorized as 'docs'
	docsFiles := map[string]bool{
		"README.md":       true,
		"readme.md":       true,
		"README":          true,
		"readme":          true,
		"CHANGELOG.md":    true,
		"changelog.md":    true,
		"CONTRIBUTING.md": true,
		"contributing.md": true,
		"docs.md":         true,
		"DOCS.md":         true,
	}

	if docsFiles[filename] {
		return "docs"
	}

	if configFiles[filename] {
		return "config"
	}

	ext := strings.ToLower(filepath.Ext(filename))
	sourceExtensions := map[string]bool{
		".js": true, ".jsx": true, ".mjs": true,
		".ts": true, ".tsx": true,
		".py": true, ".pyx": true,
		".go":   true,
		".java": true, ".kt": true, ".scala": true,
		".rb": true, ".rake": true,
		".php": true,
		".rs":  true,
		".cpp": true, ".cc": true, ".cxx": true, ".c": true,
		".cs": true, ".fs": true, ".vb": true,
		".swift": true,
		".m":     true, ".mm": true,
		".dart": true,
		".r":    true, ".R": true,
		".pl": true, ".pm": true,
		".sh": true, ".bash": true, ".zsh": true,
		".sql":  true,
		".html": true, ".htm": true,
		".css": true, ".scss": true, ".sass": true, ".less": true,
		".vue": true, ".svelte": true,
	}

	if sourceExtensions[ext] {
		return "source"
	}

	testExtensions := map[string]bool{
		".test.js": true, ".spec.js": true,
		".test.ts": true, ".spec.ts": true,
		".test.py": true, "_test.py": true,
		"_test.go":   true,
		".test.java": true, "Test.java": true,
		".test.rb": true, "_spec.rb": true,
	}

	if testExtensions[ext] || strings.Contains(strings.ToLower(filename), "test") {
		return "test"
	}

	return "other"
}
