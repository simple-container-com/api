package analysis

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// generateRecommendations creates Simple Container configuration recommendations
func (pa *ProjectAnalyzer) generateRecommendations(analysis *ProjectAnalysis) []Recommendation {
	var recommendations []Recommendation

	if analysis.PrimaryStack == nil {
		return recommendations
	}

	// Generate language-specific recommendations
	switch analysis.PrimaryStack.Language {
	case "javascript", "typescript":
		recommendations = append(recommendations, pa.getNodeJSRecommendations(analysis.PrimaryStack)...)
	case "python":
		recommendations = append(recommendations, pa.getPythonRecommendations(analysis.PrimaryStack)...)
	case "go":
		recommendations = append(recommendations, pa.getGoRecommendations(analysis.PrimaryStack)...)
	}

	// Architecture-specific recommendations
	switch analysis.Architecture {
	case "microservice":
		recommendations = append(recommendations, pa.getMicroserviceRecommendations()...)
	case "serverless":
		recommendations = append(recommendations, pa.getServerlessRecommendations()...)
	case "static-site":
		recommendations = append(recommendations, pa.getStaticSiteRecommendations()...)
	}

	// Database recommendations
	recommendations = append(recommendations, pa.getDatabaseRecommendations(analysis.PrimaryStack)...)

	// General recommendations
	recommendations = append(recommendations, pa.getGeneralRecommendations(analysis)...)

	return recommendations
}

func (pa *ProjectAnalyzer) getNodeJSRecommendations(stack *TechStackInfo) []Recommendation {
	var recs []Recommendation

	// Dockerfile recommendation
	recs = append(recs, Recommendation{
		Type:        "template",
		Priority:    "high",
		Title:       "Node.js Dockerfile Template",
		Description: "Generate optimized Dockerfile for Node.js application",
		Action:      "generate_dockerfile",
		Template:    "nodejs-dockerfile",
	})

	// Package manager detection
	packageManager := "npm"
	for _, dep := range stack.Dependencies {
		if strings.Contains(dep.Name, "yarn") {
			packageManager = "yarn"
			break
		}
	}

	recs = append(recs, Recommendation{
		Type:        "config",
		Priority:    "medium",
		Title:       fmt.Sprintf("Optimize %s Configuration", cases.Title(language.English, cases.NoLower).String(packageManager)),
		Description: fmt.Sprintf("Configure %s for production builds and caching", packageManager),
		Action:      "optimize_package_manager",
	})

	// Framework-specific recommendations
	if strings.Contains(strings.ToLower(stack.Framework), "express") {
		recs = append(recs, Recommendation{
			Type:        "template",
			Priority:    "high",
			Title:       "Express.js Production Setup",
			Description: "Configure Express.js for production deployment with proper error handling and security",
			Action:      "configure_express",
		})
	}

	if strings.Contains(strings.ToLower(stack.Framework), "react") {
		recs = append(recs, Recommendation{
			Type:        "deployment",
			Priority:    "medium",
			Title:       "React Static Build Optimization",
			Description: "Configure static file serving and build optimization for React application",
			Action:      "optimize_react_build",
		})
	}

	return recs
}

func (pa *ProjectAnalyzer) getPythonRecommendations(stack *TechStackInfo) []Recommendation {
	var recs []Recommendation

	// Dockerfile recommendation
	recs = append(recs, Recommendation{
		Type:        "template",
		Priority:    "high",
		Title:       "Python Dockerfile Template",
		Description: "Generate optimized multi-stage Dockerfile for Python application",
		Action:      "generate_dockerfile",
		Template:    "python-dockerfile",
	})

	// Virtual environment recommendation
	recs = append(recs, Recommendation{
		Type:        "config",
		Priority:    "medium",
		Title:       "Python Virtual Environment Setup",
		Description: "Configure virtual environment and dependency management for consistent deployments",
		Action:      "setup_venv",
	})

	// Framework-specific recommendations
	if strings.Contains(strings.ToLower(stack.Framework), "django") {
		recs = append(recs, Recommendation{
			Type:        "config",
			Priority:    "high",
			Title:       "Django Production Settings",
			Description: "Configure Django settings for production with proper database, static files, and security settings",
			Action:      "configure_django",
		})
	}

	if strings.Contains(strings.ToLower(stack.Framework), "flask") {
		recs = append(recs, Recommendation{
			Type:        "deployment",
			Priority:    "medium",
			Title:       "Flask WSGI Configuration",
			Description: "Configure Flask with Gunicorn for production deployment",
			Action:      "configure_flask_wsgi",
		})
	}

	return recs
}

func (pa *ProjectAnalyzer) getGoRecommendations(stack *TechStackInfo) []Recommendation {
	var recs []Recommendation

	// Dockerfile recommendation
	recs = append(recs, Recommendation{
		Type:        "template",
		Priority:    "high",
		Title:       "Go Multi-stage Dockerfile",
		Description: "Generate optimized multi-stage Dockerfile for Go application with minimal final image",
		Action:      "generate_dockerfile",
		Template:    "go-dockerfile",
	})

	// Build optimization
	recs = append(recs, Recommendation{
		Type:        "optimization",
		Priority:    "medium",
		Title:       "Go Build Optimization",
		Description: "Configure Go build with proper flags for smaller binaries and faster startup",
		Action:      "optimize_go_build",
	})

	// Framework-specific recommendations
	if strings.Contains(strings.ToLower(stack.Framework), "gin") {
		recs = append(recs, Recommendation{
			Type:        "config",
			Priority:    "medium",
			Title:       "Gin Production Configuration",
			Description: "Configure Gin framework for production with proper middleware and error handling",
			Action:      "configure_gin",
		})
	}

	if strings.Contains(strings.ToLower(stack.Framework), "echo") {
		recs = append(recs, Recommendation{
			Type:        "config",
			Priority:    "medium",
			Title:       "Echo Framework Setup",
			Description: "Configure Echo framework with proper middleware and production settings",
			Action:      "configure_echo",
		})
	}

	return recs
}

func (pa *ProjectAnalyzer) getMicroserviceRecommendations() []Recommendation {
	return []Recommendation{
		{
			Type:        "template",
			Priority:    "high",
			Title:       "Kubernetes Native Template",
			Description: "Use kubernetes-native template for microservice deployment with container orchestration",
			Action:      "setup_template",
			Template:    "kubernetes-native",
		},
		{
			Type:        "infrastructure",
			Priority:    "medium",
			Title:       "Service Mesh Setup",
			Description: "Consider implementing service mesh for inter-service communication and observability",
			Action:      "setup_service_mesh",
		},
	}
}

func (pa *ProjectAnalyzer) getServerlessRecommendations() []Recommendation {
	return []Recommendation{
		{
			Type:        "deployment",
			Priority:    "high",
			Title:       "Serverless Deployment Strategy",
			Description: "Configure serverless deployment with proper function boundaries and cold start optimization",
			Action:      "configure_serverless",
		},
	}
}

func (pa *ProjectAnalyzer) getStaticSiteRecommendations() []Recommendation {
	return []Recommendation{
		{
			Type:        "template",
			Priority:    "high",
			Title:       "Static Site Template",
			Description: "Use static-site template for optimal static content deployment",
			Action:      "setup_template",
			Template:    "static-site",
		},
		{
			Type:        "optimization",
			Priority:    "medium",
			Title:       "CDN Setup",
			Description: "Configure Content Delivery Network for faster global content distribution",
			Action:      "setup_cdn",
		},
	}
}

func (pa *ProjectAnalyzer) getDatabaseRecommendations(stack *TechStackInfo) []Recommendation {
	var recs []Recommendation

	// Check for database dependencies
	databases := make(map[string]bool)
	for _, dep := range stack.Dependencies {
		depName := strings.ToLower(dep.Name)
		if strings.Contains(depName, "postgres") || strings.Contains(depName, "pg") {
			databases["postgresql"] = true
		}
		if strings.Contains(depName, "mysql") {
			databases["mysql"] = true
		}
		if strings.Contains(depName, "mongo") {
			databases["mongodb"] = true
		}
		if strings.Contains(depName, "redis") {
			databases["redis"] = true
		}
		if strings.Contains(depName, "sqlite") {
			databases["sqlite"] = true
		}
	}

	for db := range databases {
		switch db {
		case "postgresql":
			recs = append(recs, Recommendation{
				Type:        "resource",
				Priority:    "high",
				Title:       "PostgreSQL Database Setup",
				Description: "Configure PostgreSQL database with proper connection pooling and backup strategy",
				Action:      "setup_database",
				Resource:    "postgresql",
			})
		case "mysql":
			recs = append(recs, Recommendation{
				Type:        "resource",
				Priority:    "high",
				Title:       "MySQL Database Setup",
				Description: "Configure MySQL database with optimized settings for your workload",
				Action:      "setup_database",
				Resource:    "mysql",
			})
		case "mongodb":
			recs = append(recs, Recommendation{
				Type:        "resource",
				Priority:    "high",
				Title:       "MongoDB Database Setup",
				Description: "Configure MongoDB with proper indexing and replica set configuration",
				Action:      "setup_database",
				Resource:    "mongodb-atlas",
			})
		case "redis":
			recs = append(recs, Recommendation{
				Type:        "resource",
				Priority:    "medium",
				Title:       "Redis Cache Setup",
				Description: "Configure Redis for caching and session management",
				Action:      "setup_cache",
				Resource:    "redis-cache",
			})
		case "sqlite":
			recs = append(recs, Recommendation{
				Type:        "resource",
				Priority:    "low",
				Title:       "Consider Database Upgrade",
				Description: "SQLite detected. Consider upgrading to PostgreSQL or MySQL for production use",
				Action:      "upgrade_database",
			})
		}
	}

	return recs
}

func (pa *ProjectAnalyzer) getGeneralRecommendations(analysis *ProjectAnalysis) []Recommendation {
	var recs []Recommendation

	// Check if Simple Container is already in use
	scInUse := false
	for _, stack := range analysis.TechStacks {
		if strings.Contains(strings.ToLower(stack.Framework), "simple-container") {
			scInUse = true
			break
		}
	}

	if !scInUse {
		recs = append(recs, Recommendation{
			Type:        "setup",
			Priority:    "high",
			Title:       "Initialize Simple Container",
			Description: "Set up Simple Container configuration for streamlined deployment and infrastructure management",
			Action:      "init_simple_container",
		})
	}

	// Check for environment configuration
	if analysis.Resources != nil && len(analysis.Resources.EnvironmentVars) > 0 {
		recs = append(recs, Recommendation{
			Type:        "security",
			Priority:    "high",
			Title:       "Environment Variable Management",
			Description: fmt.Sprintf("Secure %d detected environment variables using Simple Container secrets management", len(analysis.Resources.EnvironmentVars)),
			Action:      "setup_secrets",
		})
	}

	// Docker optimization
	dockerfileExists := false
	if _, err := os.Stat(filepath.Join(analysis.Path, "Dockerfile")); err == nil {
		dockerfileExists = true
	}

	if !dockerfileExists {
		recs = append(recs, Recommendation{
			Type:        "template",
			Priority:    "high",
			Title:       "Add Dockerfile",
			Description: "Generate optimized Dockerfile for containerized deployment",
			Action:      "generate_dockerfile",
		})
	} else {
		recs = append(recs, Recommendation{
			Type:        "optimization",
			Priority:    "medium",
			Title:       "Dockerfile Optimization",
			Description: "Review and optimize existing Dockerfile for smaller image size and faster builds",
			Action:      "optimize_dockerfile",
		})
	}

	// Add infrastructure recommendations
	recs = append(recs, pa.getInfrastructureRecommendations(analysis)...)

	return recs
}
