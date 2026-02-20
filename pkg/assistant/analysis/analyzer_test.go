package analysis

import (
	. "github.com/onsi/gomega"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

)

func TestProjectAnalyzer(t *testing.T) {
	analyzer := NewProjectAnalyzer()

	t.Run("test analyzer initialization", func(t *testing.T) {
		Expect(len(analyzer.detectors).To(BeTrue()) >= 3)

		// Verify detectors are sorted by priority
		for i := 1; i < len(analyzer.detectors); i++ {
			Expect(analyzer.detectors[i-1].Priority().To(BeTrue()) >= analyzer.detectors[i].Priority())
		}
	})

	t.Run("test add custom detector", func(t *testing.T) {
		customDetector := &TestDetector{name: "test", priority: 100}
		analyzer.AddDetector(customDetector)

		// Should be first due to highest priority
		Expect(analyzer.detectors[0].Name().To(Equal("test")))
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
		Expect(err).ToNot(HaveOccurred())

		Expect(stack.Language).To(Equal("javascript"))
		Expect(stack.Runtime).To(Equal("nodejs"))
		Expect(stack.Framework).To(Equal("express"))
		Expect(stack.Version).To(Equal(">=18.0.0"))
		Expect(stack.Confidence >= 0.9).To(BeTrue())
		Expect(stack.Evidence).To(ContainSubstring("package.json found"))
		Expect(stack.Evidence).To(ContainSubstring("express dependency found"))

		// Check dependencies
		Expect(len(stack.Dependencies).To(BeTrue()) >= 2)
		Expect(len(stack.DevDeps).To(BeTrue()) >= 1)

		// Check scripts
		Expect(stack.Scripts["start"]).To(Equal("node server.js"))
	})

	t.Run("test nodejs detection failure", func(t *testing.T) {
		tmpDir := createTempProject(t, map[string]string{
			"main.py": "print('Hello World')",
		})
		defer os.RemoveAll(tmpDir)

		_, err := detector.Detect(tmpDir)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("package.json not found"))
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
			Expect(result, "Failed for deps: %v", tc.deps).To(Equal(tc.expected))
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
		Expect(err).ToNot(HaveOccurred())

		Expect(stack.Language).To(Equal("python"))
		Expect(stack.Runtime).To(Equal("python"))
		Expect(stack.Framework).To(Equal("django"))
		Expect(stack.Confidence >= 0.8).To(BeTrue())
		Expect(stack.Evidence).To(ContainSubstring("requirements.txt"))
		Expect(stack.Evidence).To(ContainSubstring("parsed requirements.txt"))

		// Check dependencies parsing
		Expect(len(stack.Dependencies).To(BeTrue()) >= 3)

		djangoFound := false
		for _, dep := range stack.Dependencies {
			if dep.Name == "Django" {
				djangoFound = true
				Expect(dep.Version).To(Equal(">=4.0.0"))
				Expect(dep.Type).To(Equal("runtime"))
			}
		}
		Expect(djangoFound, "Django dependency should be found").To(BeTrue())
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
		Expect(err).ToNot(HaveOccurred())

		Expect(stack.Language).To(Equal("python"))
		Expect(stack.Evidence).To(ContainSubstring("setup.py found"))
		Expect(stack.Metadata["build_system"]).To(Equal("setuptools"))
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
		Expect(err).ToNot(HaveOccurred())

		Expect(stack.Language).To(Equal("python"))
		Expect(stack.Evidence).To(ContainSubstring("pyproject.toml found"))
		Expect(stack.Metadata["build_system"]).To(Equal("modern"))
	})

	t.Run("test python detection with only .py files", func(t *testing.T) {
		tmpDir := createTempProject(t, map[string]string{
			"app.py":    "print('Hello World')",
			"models.py": "class User: pass",
		})
		defer os.RemoveAll(tmpDir)

		stack, err := detector.Detect(tmpDir)
		Expect(err).ToNot(HaveOccurred())

		Expect(stack.Language).To(Equal("python"))
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
			Expect(result).To(Equal(tc.expected))
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

import "fmt""

func main() {
    fmt.Println("Hello World")
}`,
		})
		defer os.RemoveAll(tmpDir)

		stack, err := detector.Detect(tmpDir)
		Expect(err).ToNot(HaveOccurred())

		Expect(stack.Language).To(Equal("go"))
		Expect(stack.Runtime).To(Equal("go"))
		Expect(stack.Framework).To(Equal("gin"))
		Expect(stack.Version).To(Equal("1.21"))
		Expect(stack.Confidence >= 0.9).To(BeTrue())
		Expect(stack.Evidence).To(ContainSubstring("go.mod found"))
		Expect(stack.Evidence).To(ContainSubstring("gin framework detected"))
		Expect(stack.Metadata["module"]).To(Equal("github.com/test/myapp"))
		Expect(stack.Metadata["mode"]).To(Equal("modules"))
	})

	t.Run("test go detection legacy GOPATH", func(t *testing.T) {
		tmpDir := createTempProject(t, map[string]string{
			"main.go": "package main\n\nfunc main() {}",
			"util.go": "package main\n\nfunc util() {}",
		})
		defer os.RemoveAll(tmpDir)

		stack, err := detector.Detect(tmpDir)
		Expect(err).ToNot(HaveOccurred())

		Expect(stack.Language).To(Equal("go"))
		Expect(stack.Confidence < 0.8).To(BeTrue()) // Lower confidence without go.mod
		Expect(stack.Evidence[0]).To(ContainSubstring("legacy GOPATH mode"))
		Expect(stack.Metadata["mode"]).To(Equal("gopath"))
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
			Expect(result, "Failed for import path: %s", importPath).To(Equal(expected))

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
		Expect(err).ToNot(HaveOccurred())

		Expect(stack.Language).To(Equal("docker"))
		Expect(stack.Runtime).To(Equal("docker"))
		Expect(stack.Framework).To(Equal("nodejs")) // Detected from base image
		Expect(stack.Confidence >= 0.7).To(BeTrue())
		Expect(stack.Evidence).To(ContainSubstring("Dockerfile"))
		Expect(stack.Evidence).To(ContainSubstring("docker-compose.yml"))
		Expect(stack.Evidence).To(ContainSubstring(".dockerignore"))
		Expect(stack.Metadata["base_image"]).To(Equal("node:18-alpine"))
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
			Expect(stack.Framework).To(Equal(tc.expected))

			os.RemoveAll(tmpDir)
		}
	})
}

func TestProjectAnalyzerIntegration(t *testing.T) {
	analyzer := NewProjectAnalyzer()
	analyzer.EnableFullAnalysis() // Enable full analysis for integration tests

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
		Expect(err).ToNot(HaveOccurred())

		// Verify basic analysis
		Expect(analysis.Path).To(Equal(tmpDir))
		Expect(len(analysis.TechStacks).To(BeTrue()) >= 2) // Node.js + Docker
		Expect(analysis.PrimaryStack).ToNot(BeNil())
		Expect(analysis.Confidence > 0).To(BeTrue())

		// Verify primary stack
		Expect(analysis.PrimaryStack.Language).To(Equal("javascript"))
		Expect(analysis.PrimaryStack.Framework).To(Equal("express"))

		// Verify recommendations
		Expect(len(analysis.Recommendations).To(BeTrue()) > 0)

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
		Expect(mongodbRec, "Should recommend MongoDB resource").To(BeTrue())
		Expect(redisRec, "Should recommend Redis resource").To(BeTrue())

		// Verify architecture detection
		Expect(analysis.Architecture).ToNot(BeEmpty())
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
		Expect(err).ToNot(HaveOccurred())

		Expect(analysis.Architecture).To(Equal("microservice"))

		// Should recommend Kubernetes template
		k8sRec := false
		for _, rec := range analysis.Recommendations {
			if rec.Template == "kubernetes-native" {
				k8sRec = true
			}
		}
		Expect(k8sRec, "Should recommend Kubernetes template for microservices").To(BeTrue())
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
		Expect(err).ToNot(HaveOccurred())

		Expect(analysis.Architecture).To(Equal("static-site"))

		// Should recommend static site template
		staticRec := false
		for _, rec := range analysis.Recommendations {
			if rec.Template == "static-site" {
				staticRec = true
			}
		}
		Expect(staticRec, "Should recommend static site template").To(BeTrue())
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
		Expect(err).ToNot(HaveOccurred())

		Expect(len(analysis.Files).To(BeTrue()) >= 4)

		// Verify file types and languages
		fileMap := make(map[string]FileInfo)
		for _, file := range analysis.Files {
			fileMap[file.Path] = file
		}

		Expect(fileMap["package.json"].Type).To(Equal("config"))
		Expect(fileMap["src/index.js"].Language).To(Equal("javascript"))
		Expect(fileMap["src/utils.ts"].Language).To(Equal("typescript"))
		Expect(fileMap["README.md"].Type).To(Equal("docs"))
		Expect(fileMap["Dockerfile"].Type).To(Equal("config"))
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
	Expect(err).ToNot(HaveOccurred())

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

// Test LLM Enhancement functionality

// MockLLMProvider implements LLMProvider for testing
type MockLLMProvider struct {
	response string
	err      error
}

func (m *MockLLMProvider) GenerateResponse(ctx context.Context, prompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestLLMEnhancement(t *testing.T) {
	t.Run("test analyzer without LLM provider", func(t *testing.T) {
		tmpDir := createTempProject(t, map[string]string{
			"package.json": `{"name": "test", "dependencies": {"express": "^4.0.0"}}`,
		})
		defer os.RemoveAll(tmpDir)

		analyzer := NewProjectAnalyzer()
		// Don't set LLM provider

		analysis, err := analyzer.AnalyzeProject(tmpDir)
		Expect(err).ToNot(HaveOccurred())

		// Should work fine without LLM enhancement
		Expect(analysis.TechStacks).ToNot(BeEmpty())
		Expect(analysis.Recommendations).ToNot(BeEmpty())

		// Should not have LLM enhancement metadata
		_, hasLLMEnhanced := analysis.Metadata["llm_enhanced"]
		Expect(hasLLMEnhanced, "Should not have LLM enhancement without provider").To(BeFalse())
	})

	t.Run("test LLM provider interface", func(t *testing.T) {
		tmpDir := createTempProject(t, map[string]string{
			"go.mod":  "module test\ngo 1.21\nrequire github.com/gin-gonic/gin v1.9.0",
			"main.go": "package main",
		})
		defer os.RemoveAll(tmpDir)

		analyzer := NewProjectAnalyzer()

		// Test with mock LLM that returns simple response
		mockLLM := &MockLLMProvider{response: "LLM analysis complete"}
		analyzer.SetLLMProvider(mockLLM)

		analysis, err := analyzer.AnalyzeProject(tmpDir)
		Expect(err).ToNot(HaveOccurred())

		// Should have basic analysis
		Expect(analysis.TechStacks).ToNot(BeEmpty())
		Expect(analysis.PrimaryStack.Language).To(Equal("go"))

		// LLM response should be stored in metadata since it's not valid JSON
		Expect(analysis.Metadata["llm_insights"]).To(Equal("LLM analysis complete"))
	})
}
