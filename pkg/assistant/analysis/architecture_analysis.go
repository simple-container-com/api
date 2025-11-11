package analysis

import (
	"os"
	"path/filepath"
	"strings"
)

// detectArchitecture determines the architectural pattern
func (pa *ProjectAnalyzer) detectArchitecture(stacks []TechStackInfo, projectPath string) string {
	// Check for microservice indicators
	if pa.hasMicroserviceIndicators(stacks, projectPath) {
		return "microservice"
	}

	// Check for serverless indicators
	if pa.hasServerlessIndicators(stacks, projectPath) {
		return "serverless"
	}

	// Check for static site indicators
	if pa.hasStaticSiteIndicators(stacks, projectPath) {
		return "static-site"
	}

	// Check for monolith indicators
	if pa.hasMonolithIndicators(stacks, projectPath) {
		return "monolith"
	}

	return "standard-web-app"
}

func (pa *ProjectAnalyzer) hasMicroserviceIndicators(stacks []TechStackInfo, projectPath string) bool {
	indicators := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"kubernetes",
		"k8s",
	}

	for _, indicator := range indicators {
		if _, err := os.Stat(filepath.Join(projectPath, indicator)); err == nil {
			return true
		}
	}

	// Check for multiple service directories
	serviceCount := 0
	entries, err := os.ReadDir(projectPath)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() && pa.looksLikeServiceDir(entry.Name()) {
				serviceCount++
			}
		}
	}

	return serviceCount >= 2
}

func (pa *ProjectAnalyzer) hasMonolithIndicators(stacks []TechStackInfo, projectPath string) bool {
	// Large single-language projects tend to be monoliths
	for _, stack := range stacks {
		if len(stack.Dependencies) > 20 {
			return true
		}
	}

	// Check for typical monolith structure
	monolithFiles := []string{
		"app.js", "server.js", "main.py", "app.py",
		"main.go", "cmd/main.go", "src/main/java",
	}

	for _, file := range monolithFiles {
		if _, err := os.Stat(filepath.Join(projectPath, file)); err == nil {
			return true
		}
	}

	return false
}

func (pa *ProjectAnalyzer) hasServerlessIndicators(stacks []TechStackInfo, projectPath string) bool {
	serverlessFiles := []string{
		"serverless.yml",
		"serverless.yaml",
		"sam.yaml",
		"sam.yml",
		"template.yaml", // AWS SAM
		"template.yml",
		"functions", // Directory
		"lambda",    // Directory
	}

	for _, file := range serverlessFiles {
		if _, err := os.Stat(filepath.Join(projectPath, file)); err == nil {
			return true
		}
	}

	// Check for serverless frameworks in dependencies
	for _, stack := range stacks {
		for _, dep := range stack.Dependencies {
			serverlessKeywords := []string{
				"serverless", "lambda", "functions", "azure-functions",
				"google-cloud-functions", "vercel", "netlify-functions",
			}
			for _, keyword := range serverlessKeywords {
				if strings.Contains(strings.ToLower(dep.Name), keyword) {
					return true
				}
			}
		}
	}

	return false
}

func (pa *ProjectAnalyzer) hasStaticSiteIndicators(stacks []TechStackInfo, projectPath string) bool {
	staticSiteFiles := []string{
		"index.html",
		"_config.yml", // Jekyll
		"_config.yaml",
		"gatsby-config.js",     // Gatsby
		"next.config.js",       // Next.js
		"nuxt.config.js",       // Nuxt.js
		"gridsome.config.js",   // Gridsome
		"vuepress",             // VuePress directory
		"docusaurus.config.js", // Docusaurus
	}

	for _, file := range staticSiteFiles {
		if _, err := os.Stat(filepath.Join(projectPath, file)); err == nil {
			return true
		}
	}

	// Check for static site generators in dependencies
	for _, stack := range stacks {
		for _, dep := range stack.Dependencies {
			staticGenerators := []string{
				"gatsby", "next", "nuxt", "gridsome", "vuepress",
				"jekyll", "hugo", "hexo", "docusaurus", "eleventy",
			}
			for _, generator := range staticGenerators {
				if strings.Contains(strings.ToLower(dep.Name), generator) {
					return true
				}
			}
		}
	}

	return false
}

func (pa *ProjectAnalyzer) looksLikeServiceDir(name string) bool {
	serviceIndicators := []string{
		"service", "api", "app", "backend", "frontend",
		"auth", "user", "payment", "notification", "gateway",
		"admin", "web", "mobile", "worker", "processor",
	}

	lowerName := strings.ToLower(name)
	for _, indicator := range serviceIndicators {
		if strings.Contains(lowerName, indicator) {
			return true
		}
	}

	return false
}
