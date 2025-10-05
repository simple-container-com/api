package analysis

import (
	"fmt
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectAnalyzer(t *testing.T) {
	analyzer := NewProjectAnalyzer()

	t.Run("test analyzer initialization", func(t *testing.T) {
		assert.True(t, len(analyzer.detectors) >= 4)

		// Verify detectors are sorted by priority
		for i := 1; i < len(analyzer.detectors); i++ {
			assert.True(t, analyzer.detectors[i-1].Priority() >= analyzer.detectors[i].Priority())
		}
	})

	t.Run("test add custom detector", func(t *testing.T) {
		customDetector := &TestDetector{name: "test", priority: 100}
		analyzer.AddDetector(customDetector)

		// Should be first due to highest priority
		assert.Equal(t, "test", analyzer.detectors[0].Name())
	})
}

func TestNodeJSDetector(t *testing.T) {
	detector := &NodeJSDetector{}

	t.Run("test nodejs detection success", func(t *testing.T) {
		// Create temporary project directory
		tmpDir := createTempProject(t, map[string]string{
			"package.json": `{
				"name": "test-app",
				"version": "1.0.0",
				"dependencies": {
					"express": "^4.18.0",
					"mongodb": "^4.0.0"
				},
				"devDependencies": {
					"nodemon": "^2.0.0"
				},
				"scripts": {
					"start": "node server.js",
					"dev": "nodemon server.js"
				},
				"engines": {
					"node": ">=18.0.0"
				}
			}`,
			"server.js": "console.log('Hello World');",
		})
		defer os.RemoveAll(tmpDir)

		stack, err := detector.Detect(tmpDir)
		require.NoError(t, err)

		assert.Equal(t, "javascript", stack.Language)
		assert.Equal(t, "nodejs", stack.Runtime)
		assert.Equal(t, "express", stack.Framework)
		assert.Equal(t, ">=18.0.0", stack.Version)
		assert.True(t, stack.Confidence >= 0.9)
		assert.Contains(t, stack.Evidence, "package.json found")
		assert.Contains(t, stack.Evidence, "express dependency found")

		// Check dependencies
		assert.True(t, len(stack.Dependencies) >= 2)
		assert.True(t, len(stack.DevDeps) >= 1)

		// Check scripts
		assert.Equal(t, "node server.js", stack.Scripts["start"])
	})

	t.Run("test nodejs detection failure", func(t *testing.T) {
		tmpDir := createTempProject(t, map[string]string{
			"main.py": "print('Hello World')",
		})
		defer os.RemoveAll(tmpDir)

		_, err := detector.Detect(tmpDir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "package.json not found")
	})

	t.Run("test framework detection", func(t *testing.T) {
		testCases := []struct {
			deps     map[string]string
			expected string
		}{
			{map[string]string{"express": "^4.0.0"}, "express"},
			{map[string]string{"koa": "^2.0.0"}, "koa"},
			{map[string]string{"fastify": "^3.0.0"}, "fastify"},
			{map[string]string{"react": "^18.0.0"}, "react"},
			{map[string]string{"vue": "^3.0.0"}, "vue"},
			{map[string]string{"@nestjs/core": "^8.0.0"}, "nestjs"},
			{map[string]string{"@angular/core": "^14.0.0"}, "angular"},
			{map[string]string{"unknown-package": "^1.0.0"}, ""},
		}

		for _, tc := range testCases {
			result := detector.detectFramework(tc.deps)
			assert.Equal(t, tc.expected, result, "Failed for deps: %v", tc.deps)
		}
	})
}

func TestPythonDetector(t *testing.T) {
	detector := &PythonDetector{}

	t.Run("test python detection with requirements.txt", func(t *testing.T) {
		tmpDir := createTempProject(t, map[string]string{
			"requirements.txt": `Django>=4.0.0
psycopg2-binary>=2.8.0
redis>=4.0.0
# Comment line
celery>=5.0.0`,
			"app.py": "print('Hello Django')",
		})
		defer os.RemoveAll(tmpDir)

		stack, err := detector.Detect(tmpDir)
		require.NoError(t, err)

		assert.Equal(t, "python", stack.Language)
		assert.Equal(t, "python", stack.Runtime)
		assert.Equal(t, "django", stack.Framework)
		assert.True(t, stack.Confidence >= 0.8)
		assert.Contains(t, stack.Evidence, "requirements.txt")
		assert.Contains(t, stack.Evidence, "parsed requirements.txt")

		// Check dependencies parsing
		assert.True(t, len(stack.Dependencies) >= 3)

		djangoFound := false
		for _, dep := range stack.Dependencies {
			if dep.Name == "Django" {
				djangoFound = true
				assert.Equal(t, ">=4.0.0", dep.Version)
				assert.Equal(t, "runtime", dep.Type)
			}
		}
		assert.True(t, djangoFound, "Django dependency should be found")
	})

	t.Run("test python detection with setup.py", func(t *testing.T) {
		tmpDir := createTempProject(t, map[string]string{
			"setup.py": `from setuptools import setup
setup(
    name="test-package",
    version="1.0.0"
)`,
			"main.py": "import flask",
		})
		defer os.RemoveAll(tmpDir)

		stack, err := detector.Detect(tmpDir)
		require.NoError(t, err)

		assert.Equal(t, "python", stack.Language)
		assert.Contains(t, stack.Evidence, "setup.py found")
		assert.Equal(t, "setuptools", stack.Metadata["build_system"])
	})

	t.Run("test python detection with pyproject.toml", func(t *testing.T) {
		tmpDir := createTempProject(t, map[string]string{
			"pyproject.toml": `[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"`,
			"src/__init__.py": "",
		})
		defer os.RemoveAll(tmpDir)

		stack, err := detector.Detect(tmpDir)
		require.NoError(t, err)

		assert.Equal(t, "python", stack.Language)
		assert.Contains(t, stack.Evidence, "pyproject.toml found")
		assert.Equal(t, "modern", stack.Metadata["build_system"])
	})

	t.Run("test python detection with only .py files", func(t *testing.T) {
		tmpDir := createTempProject(t, map[string]string{
			"app.py":    "print('Hello World')",
			"models.py": "class User: pass",
		})
		defer os.RemoveAll(tmpDir)

		stack, err := detector.Detect(tmpDir)
		require.NoError(t, err)

		assert.Equal(t, "python", stack.Language)
	})

	t.Run("test framework detection", func(t *testing.T) {
		testCases := []struct {
			deps     []Dependency
			expected string
		}{
			{[]Dependency{{Name: "Django"}}, "django"},
			{[]Dependency{{Name: "flask"}}, "flask"},
			{[]Dependency{{Name: "FastAPI"}}, "fastapi"},
			{[]Dependency{{Name: "tornado"}}, "tornado"},
			{[]Dependency{{Name: "unknown"}}, ""},
		}

		for _, tc := range testCases {
			result := detector.detectFramework(tc.deps)
			assert.Equal(t, tc.expected, result)
		}
	})
}

func TestGoDetector(t *testing.T) {
	detector := &GoDetector{}

	t.Run("test go detection with go.mod", func(t *testing.T) {
		tmpDir := createTempProject(t, map[string]string{
			"go.mod": `module github.com/test/myapp

go 1.21

require (
    github.com/gin-gonic/gin v1.9.0
    github.com/spf13/cobra v1.7.0
)`,
			"main.go": `package main

import "fmt"

func main() {
    fmt.Println("Hello World")
}`,
		})
		defer os.RemoveAll(tmpDir)

		stack, err := detector.Detect(tmpDir)
		require.NoError(t, err)

		assert.Equal(t, "go", stack.Language)
		assert.Equal(t, "go", stack.Runtime)
		assert.Equal(t, "gin", stack.Framework)
		assert.Equal(t, "1.21", stack.Version)
		assert.True(t, stack.Confidence >= 0.9)
		assert.Contains(t, stack.Evidence, "go.mod found")
		assert.Contains(t, stack.Evidence, "gin framework detected")
		assert.Equal(t, "github.com/test/myapp", stack.Metadata["module"])
		assert.Equal(t, "modules", stack.Metadata["mode"])
	})

	t.Run("test go detection legacy GOPATH", func(t *testing.T) {
		tmpDir := createTempProject(t, map[string]string{
			"main.go": "package main\n\nfunc main() {}",
			"util.go": "package main\n\nfunc util() {}",
		})
		defer os.RemoveAll(tmpDir)

		stack, err := detector.Detect(tmpDir)
		require.NoError(t, err)

		assert.Equal(t, "go", stack.Language)
		assert.True(t, stack.Confidence < 0.8) // Lower confidence without go.mod
		assert.Contains(t, stack.Evidence[0], "legacy GOPATH mode")
		assert.Equal(t, "gopath", stack.Metadata["mode"])
	})

	t.Run("test framework detection", func(t *testing.T) {
		frameworks := map[string]string{
			"gin-gonic/gin": "gin",
			"gorilla/mux":   "gorilla-mux",
			"labstack/echo": "echo",
			"gofiber/fiber": "fiber",
			"spf13/cobra":   "cobra",
		}

		for importPath, expected := range frameworks {
			goModContent := fmt.Sprintf("module test\ngo 1.21\nrequire %s v1.0.0", importPath)
			tmpDir := createTempProject(t, map[string]string{
				"go.mod":  goModContent,
				"main.go": "package main",
			})

			result := detector.detectFramework(tmpDir)
			assert.Equal(t, expected, result, "Failed for import path: %s", importPath)

			os.RemoveAll(tmpDir)
		}
	})
}

func TestDockerDetector(t *testing.T) {
	detector := &DockerDetector{}

	t.Run("test docker detection success", func(t *testing.T) {
		tmpDir := createTempProject(t, map[string]string{
			"Dockerfile": `FROM node:18-alpine
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
EXPOSE 3000
CMD ["npm", "start"]`,
			"docker-compose.yml": `version: '3.8'
services:
  web:
    build: .
    ports:
      - "3000:3000"`,
			".dockerignore": `node_modules
.git`,
		})
		defer os.RemoveAll(tmpDir)

		stack, err := detector.Detect(tmpDir)
		require.NoError(t, err)

		assert.Equal(t, "docker", stack.Language)
		assert.Equal(t, "docker", stack.Runtime)
		assert.Equal(t, "nodejs", stack.Framework) // Detected from base image
		assert.True(t, stack.Confidence >= 0.7)
		assert.Contains(t, stack.Evidence, "Dockerfile")
		assert.Contains(t, stack.Evidence, "docker-compose.yml")
		assert.Contains(t, stack.Evidence, ".dockerignore")
		assert.Equal(t, "node:18-alpine", stack.Metadata["base_image"])
	})

	t.Run("test base image detection", func(t *testing.T) {
		testCases := []struct {
			dockerfile string
			expected   string
		}{
			{"FROM python:3.11-slim\nWORKDIR /app", "python"},
			{"FROM golang:1.21-alpine\nWORKDIR /app", "go"},
			{"FROM node:18\nWORKDIR /app", "nodejs"},
			{"FROM ubuntu:22.04\nWORKDIR /app", ""},
		}

		for _, tc := range testCases {
			tmpDir := createTempProject(t, map[string]string{
				"Dockerfile": tc.dockerfile,
			})

			stack, _ := detector.Detect(tmpDir)
			assert.Equal(t, tc.expected, stack.Framework)

			os.RemoveAll(tmpDir)
		}
	})
}

func TestProjectAnalyzerIntegration(t *testing.T) {
	analyzer := NewProjectAnalyzer()

	t.Run("test nodejs project analysis", func(t *testing.T) {
		tmpDir := createTempProject(t, map[string]string{
			"package.json": `{
				"name": "test-api",
				"version": "1.0.0",
				"dependencies": {
					"express": "^4.18.0",
					"mongoose": "^6.0.0",
					"redis": "^4.0.0"
				},
				"scripts": {
					"start": "node server.js"
				}
			}`,
			"server.js": `const express = require('express');
const app = express();
app.listen(3000);`,
			"Dockerfile": "FROM node:18-alpine",
		})
		defer os.RemoveAll(tmpDir)

		analysis, err := analyzer.AnalyzeProject(tmpDir)
		require.NoError(t, err)

		// Verify basic analysis
		assert.Equal(t, tmpDir, analysis.Path)
		assert.True(t, len(analysis.TechStacks) >= 2) // Node.js + Docker
		assert.NotNil(t, analysis.PrimaryStack)
		assert.True(t, analysis.Confidence > 0)

		// Verify primary stack
		assert.Equal(t, "javascript", analysis.PrimaryStack.Language)
		assert.Equal(t, "express", analysis.PrimaryStack.Framework)

		// Verify recommendations
		assert.True(t, len(analysis.Recommendations) > 0)

		// Check for database recommendations (MongoDB, Redis)
		mongodbRec := false
		redisRec := false
		for _, rec := range analysis.Recommendations {
			if rec.Resource == "mongodb-atlas" {
				mongodbRec = true
			}
			if rec.Resource == "redis-cache" {
				redisRec = true
			}
		}
		assert.True(t, mongodbRec, "Should recommend MongoDB resource")
		assert.True(t, redisRec, "Should recommend Redis resource")

		// Verify architecture detection
		assert.NotEmpty(t, analysis.Architecture)
	})

	t.Run("test microservice architecture detection", func(t *testing.T) {
		tmpDir := createTempProject(t, map[string]string{
			"docker-compose.yml": `version: '3.8'
services:
  api:
    build: ./api
  auth:
    build: ./auth
  database:
    image: postgres:13`,
			"api/package.json":  `{"name": "api"}`,
			"auth/package.json": `{"name": "auth"}`,
		})
		defer os.RemoveAll(tmpDir)

		analysis, err := analyzer.AnalyzeProject(tmpDir)
		require.NoError(t, err)

		assert.Equal(t, "microservice", analysis.Architecture)

		// Should recommend Kubernetes template
		k8sRec := false
		for _, rec := range analysis.Recommendations {
			if rec.Template == "kubernetes-native" {
				k8sRec = true
			}
		}
		assert.True(t, k8sRec, "Should recommend Kubernetes template for microservices")
	})

	t.Run("test static site detection", func(t *testing.T) {
		tmpDir := createTempProject(t, map[string]string{
			"index.html": `<!DOCTYPE html>
<html><head><title>Test</title></head>
<body><h1>Hello World</h1></body></html>`,
			"package.json": `{
				"name": "static-site",
				"dependencies": {
					"gatsby": "^4.0.0"
				}
			}`,
		})
		defer os.RemoveAll(tmpDir)

		analysis, err := analyzer.AnalyzeProject(tmpDir)
		require.NoError(t, err)

		assert.Equal(t, "static-site", analysis.Architecture)

		// Should recommend static site template
		staticRec := false
		for _, rec := range analysis.Recommendations {
			if rec.Template == "static-site" {
				staticRec = true
			}
		}
		assert.True(t, staticRec, "Should recommend static site template")
	})

	t.Run("test file analysis", func(t *testing.T) {
		tmpDir := createTempProject(t, map[string]string{
			"package.json": `{"name": "test"}`,
			"src/index.js": "console.log('hello');",
			"src/utils.ts": "export const util = () => {};",
			"README.md":    "# Test Project",
			"Dockerfile":   "FROM node:18",
		})
		defer os.RemoveAll(tmpDir)

		analysis, err := analyzer.AnalyzeProject(tmpDir)
		require.NoError(t, err)

		assert.True(t, len(analysis.Files) >= 4)

		// Verify file types and languages
		fileMap := make(map[string]FileInfo)
		for _, file := range analysis.Files {
			fileMap[file.Path] = file
		}

		assert.Equal(t, "config", fileMap["package.json"].Type)
		assert.Equal(t, "javascript", fileMap["src/index.js"].Language)
		assert.Equal(t, "typescript", fileMap["src/utils.ts"].Language)
		assert.Equal(t, "docs", fileMap["README.md"].Type)
		assert.Equal(t, "config", fileMap["Dockerfile"].Type)
	})
}

// Helper functions and test utilities

type TestDetector struct {
	name     string
	priority int
}

func (d *TestDetector) Name() string  { return d.name }
func (d *TestDetector) Priority() int { return d.priority }
func (d *TestDetector) Detect(projectPath string) (*TechStackInfo, error) {
	return &TechStackInfo{
		Language:   d.name,
		Confidence: 1.0,
	}, nil
}

func createTempProject(t *testing.T, files map[string]string) string {
	tmpDir, err := os.MkdirTemp("", "test-project-*")
	require.NoError(t, err)

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)

		// Create directory if needed
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("Failed to write file %s: %v", path, err)
		}
	}

	return tmpDir
}

func BenchmarkProjectAnalysis(b *testing.B) {
	// Create a realistic project for benchmarking
	tmpDir := createBenchmarkProject(b)
	defer os.RemoveAll(tmpDir)

	analyzer := NewProjectAnalyzer()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := analyzer.AnalyzeProject(tmpDir)
		if err != nil {
			b.Fatalf("Analysis failed: %v", err)
		}
	}
}

func createBenchmarkProject(b *testing.B) string {
	tmpDir, err := os.MkdirTemp("", "benchmark-project-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}

	files := map[string]string{
		"package.json": `{
			"name": "benchmark-app",
			"version": "1.0.0",
			"dependencies": {
				"express": "^4.18.0",
				"mongoose": "^6.0.0",
				"redis": "^4.0.0",
				"lodash": "^4.17.0",
				"moment": "^2.29.0"
			},
			"devDependencies": {
				"nodemon": "^2.0.0",
				"jest": "^28.0.0",
				"eslint": "^8.0.0"
			}
		}`,
		"server.js":          "const express = require('express'); const app = express(); app.listen(3000);",
		"Dockerfile":         "FROM node:18-alpine\nWORKDIR /app\nCOPY . .\nRUN npm install\nCMD ['npm', 'start']",
		"docker-compose.yml": "version: '3.8'\nservices:\n  web:\n    build: .\n    ports:\n      - '3000:3000'",
	}

	// Create source files
	for i := 0; i < 20; i++ {
		files[fmt.Sprintf("src/module%d.js", i)] = fmt.Sprintf("// Module %d\nmodule.exports = {};", i)
	}

	// Create files
	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			b.Fatalf("Failed to create directory: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			b.Fatalf("Failed to write file: %v", err)
		}
	}

	return tmpDir
}
