package analysis

import (
	"strings"
)

// generateEnhancedRecommendations creates enhanced recommendations based on detected resources
func (pa *ProjectAnalyzer) generateEnhancedRecommendations(analysis *ProjectAnalysis) []Recommendation {
	var recs []Recommendation

	// Start with original recommendations
	recs = append(recs, analysis.Recommendations...)

	// Add Git-based recommendations
	if analysis.Git != nil && analysis.Git.IsGitRepo {
		recs = append(recs, pa.getGitBasedRecommendations(analysis.Git)...)
	}

	// Add resource-based recommendations
	if analysis.Resources != nil {
		recs = append(recs, pa.getResourceBasedRecommendations(analysis.Resources)...)
	}

	// Add complexity-based recommendations
	if len(analysis.Files) > 0 {
		recs = append(recs, pa.getComplexityBasedRecommendations(analysis.Files)...)
	}

	return recs
}

// getInfrastructureRecommendations provides recommendations based on existing IaC tools
func (pa *ProjectAnalyzer) getInfrastructureRecommendations(analysis *ProjectAnalysis) []Recommendation {
	var recs []Recommendation

	terraformDetected := false
	pulumiDetected := false
	simpleContainerDetected := false

	// Check what IaC tools are detected
	for _, stack := range analysis.TechStacks {
		switch strings.ToLower(stack.Framework) {
		case "terraform":
			terraformDetected = true
		case "pulumi":
			pulumiDetected = true
		case "simple-container":
			simpleContainerDetected = true
		}
	}

	// Provide contextual recommendations based on detected tools
	if terraformDetected && !simpleContainerDetected {
		recs = append(recs, Recommendation{
			Type:        "migration",
			Priority:    "medium",
			Title:       "Consider Simple Container Migration",
			Description: "Terraform detected. Evaluate migrating to Simple Container for simpler infrastructure management with built-in best practices",
			Action:      "evaluate_migration_from_terraform",
		})

		recs = append(recs, Recommendation{
			Type:        "integration",
			Priority:    "low",
			Title:       "Hybrid Infrastructure Approach",
			Description: "Consider using Simple Container for application deployment while keeping Terraform for complex infrastructure provisioning",
			Action:      "setup_hybrid_infrastructure",
		})
	}

	if pulumiDetected && !simpleContainerDetected {
		recs = append(recs, Recommendation{
			Type:        "migration",
			Priority:    "medium",
			Title:       "Pulumi to Simple Container Migration",
			Description: "Pulumi detected. Simple Container provides similar infrastructure-as-code benefits with less complexity",
			Action:      "evaluate_migration_from_pulumi",
		})
	}

	if simpleContainerDetected {
		// Already using Simple Container - suggest enhancements
		recs = append(recs, Recommendation{
			Type:        "enhancement",
			Priority:    "medium",
			Title:       "Simple Container Advanced Features",
			Description: "Explore advanced Simple Container features like multi-environment deployments and resource optimization",
			Action:      "explore_advanced_features",
		})

		recs = append(recs, Recommendation{
			Type:        "optimization",
			Priority:    "low",
			Title:       "Simple Container Configuration Review",
			Description: "Review current Simple Container configuration for optimization opportunities",
			Action:      "review_configuration",
		})
	}

	// If no IaC tool detected, strongly recommend Simple Container
	if !terraformDetected && !pulumiDetected && !simpleContainerDetected {
		recs = append(recs, Recommendation{
			Type:        "setup",
			Priority:    "high",
			Title:       "Infrastructure as Code Setup",
			Description: "No infrastructure management detected. Simple Container provides easy infrastructure-as-code with built-in best practices",
			Action:      "setup_infrastructure_as_code",
		})
	}

	return recs
}

// getGitBasedRecommendations provides recommendations based on Git analysis
func (pa *ProjectAnalyzer) getGitBasedRecommendations(git *GitAnalysis) []Recommendation {
	var recs []Recommendation

	// CI/CD recommendations
	if !git.HasCI {
		recs = append(recs, Recommendation{
			Type:        "devops",
			Priority:    "high",
			Title:       "CI/CD Pipeline Setup",
			Description: "No CI/CD detected. Set up automated testing and deployment pipeline for better development workflow",
			Action:      "setup_cicd",
		})
	} else {
		recs = append(recs, Recommendation{
			Type:        "optimization",
			Priority:    "medium",
			Title:       "CI/CD Optimization",
			Description: "Optimize existing CI/CD pipeline for faster builds and more reliable deployments",
			Action:      "optimize_cicd",
		})
	}

	// Versioning recommendations
	if len(git.Tags) == 0 {
		recs = append(recs, Recommendation{
			Type:        "devops",
			Priority:    "medium",
			Title:       "Version Tagging Strategy",
			Description: "No version tags detected. Implement semantic versioning for better release management",
			Action:      "setup_versioning",
		})
	}

	// Collaboration recommendations
	if len(git.Contributors) == 1 {
		recs = append(recs, Recommendation{
			Type:        "process",
			Priority:    "low",
			Title:       "Code Review Process",
			Description: "Single contributor detected. Consider setting up code review process for better code quality",
			Action:      "setup_code_review",
		})
	} else if len(git.Contributors) > 5 {
		recs = append(recs, Recommendation{
			Type:        "process",
			Priority:    "medium",
			Title:       "Branch Protection Rules",
			Description: "Multiple contributors detected. Set up branch protection and review requirements",
			Action:      "setup_branch_protection",
		})
	}

	return recs
}

// getResourceBasedRecommendations provides recommendations based on detected resources
func (pa *ProjectAnalyzer) getResourceBasedRecommendations(resources *ResourceAnalysis) []Recommendation {
	var recs []Recommendation

	// Security recommendations for secrets
	if len(resources.Secrets) > 0 {
		recs = append(recs, Recommendation{
			Type:        "security",
			Priority:    "critical",
			Title:       "Secrets Management",
			Description: "Potential secrets detected in code. Move sensitive data to secure secrets management",
			Action:      "secure_secrets",
		})
	}

	// Database recommendations
	if len(resources.Databases) > 3 {
		recs = append(recs, Recommendation{
			Type:        "architecture",
			Priority:    "medium",
			Title:       "Database Architecture Review",
			Description: "Multiple databases detected. Review data architecture for potential consolidation opportunities",
			Action:      "review_database_architecture",
		})
	}

	// API management recommendations
	if len(resources.ExternalAPIs) > 5 {
		recs = append(recs, Recommendation{
			Type:        "integration",
			Priority:    "medium",
			Title:       "API Management Strategy",
			Description: "Many external APIs detected. Consider API gateway for better management and monitoring",
			Action:      "implement_api_gateway",
		})
	}

	// Environment variable recommendations
	if len(resources.EnvironmentVars) > 20 {
		recs = append(recs, Recommendation{
			Type:        "config",
			Priority:    "medium",
			Title:       "Configuration Management",
			Description: "Many environment variables detected. Consider configuration management strategy",
			Action:      "organize_configuration",
		})
	}

	return recs
}

// getComplexityBasedRecommendations provides recommendations based on code complexity
func (pa *ProjectAnalyzer) getComplexityBasedRecommendations(files []FileInfo) []Recommendation {
	var recs []Recommendation

	highComplexityFiles := 0
	totalLOC := 0
	sourceFiles := 0

	for _, file := range files {
		if file.Type == "source" && file.Complexity != nil {
			sourceFiles++
			totalLOC += file.Complexity.LinesOfCode
			if file.Complexity.ComplexityLevel == "high" || file.Complexity.ComplexityLevel == "very_high" {
				highComplexityFiles++
			}
		}
	}

	// High complexity recommendations
	if highComplexityFiles > 0 {
		recs = append(recs, Recommendation{
			Type:        "refactoring",
			Priority:    "medium",
			Title:       "Code Complexity Reduction",
			Description: "High complexity files detected. Consider refactoring for better maintainability",
			Action:      "reduce_complexity",
		})
	}

	// Large codebase recommendations
	if totalLOC > 50000 {
		recs = append(recs, Recommendation{
			Type:        "architecture",
			Priority:    "medium",
			Title:       "Large Codebase Management",
			Description: "Large codebase detected. Consider modular architecture and automated testing strategies",
			Action:      "implement_modular_architecture",
		})
	}

	// Testing recommendations for complex code
	if highComplexityFiles > sourceFiles/4 { // More than 25% high complexity
		recs = append(recs, Recommendation{
			Type:        "testing",
			Priority:    "high",
			Title:       "Automated Testing Strategy",
			Description: "High complexity files require comprehensive testing. Implement automated testing in your CI/CD pipeline before deployment",
			Action:      "implement_testing_strategy",
		})
	}

	return recs
}
