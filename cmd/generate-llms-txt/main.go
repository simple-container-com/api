package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/docs"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/assistant/config"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/openai"
)

// Config holds the configuration for llms.txt generation
type Config struct {
	OutputDir       string
	Verbose         bool
	DryRun          bool
	UseLLM          bool
	LLMProvider     string // "openai" or "anthropic"
	IncludeAIDocs   bool
	IncludeDesign   bool
	MaxFullTextSize int
}

// NavItem represents a navigation item from mkdocs.yml
type NavItem struct {
	Title string
	Path  string
	Items []NavItem
}

// MkDocsConfig represents the mkdocs.yml structure
type MkDocsConfig struct {
	SiteName string        `yaml:"site_name"`
	Nav      []interface{} `yaml:"nav"`
}

// DocSection represents a documentation section for llms.txt
type DocSection struct {
	Title       string
	Description string
	Links       []DocLink
}

// DocLink represents a documentation link
type DocLink struct {
	Title       string
	Path        string
	Description string
}

func main() {
	cfg := parseFlags()
	log := logger.New()
	ctx := context.Background()

	if cfg.Verbose {
		fmt.Printf("🚀 Starting llms.txt generation\n")
		fmt.Printf("📋 Configuration:\n")
		fmt.Printf("   Output Dir: %s\n", cfg.OutputDir)
		fmt.Printf("   Use LLM: %t\n", cfg.UseLLM)
		if cfg.UseLLM {
			fmt.Printf("   LLM Provider: %s\n", cfg.LLMProvider)
		}
		fmt.Printf("   Include AI Docs: %t\n", cfg.IncludeAIDocs)
		fmt.Printf("   Include Design Docs: %t\n", cfg.IncludeDesign)
		fmt.Printf("   Dry Run: %t\n", cfg.DryRun)
		fmt.Printf("\n")
	}

	// Load mkdocs.yml to get navigation structure
	mkdocsConfig, err := loadMkDocsConfig()
	if err != nil {
		log.Error(ctx, "Failed to load mkdocs.yml: %v", err)
		os.Exit(1)
	}

	if cfg.Verbose {
		fmt.Printf("📚 Loaded navigation from mkdocs.yml\n")
	}

	// Parse navigation into sections
	sections := parseNavigation(mkdocsConfig.Nav, cfg)

	// Load AI Assistant docs if requested
	if cfg.IncludeAIDocs {
		aiSections := loadAIAssistantDocs(cfg.Verbose)
		sections = append(sections, aiSections...)
	}

	if cfg.Verbose {
		fmt.Printf("📂 Parsed %d documentation sections\n", len(sections))
	}

	// Generate llms.txt content
	llmsTxtContent := generateLLMsTxt(mkdocsConfig.SiteName, sections, cfg)

	// Generate llms-full.txt content
	llmsFullContent, err := generateLLMsFullTxt(ctx, mkdocsConfig.SiteName, sections, cfg, log)
	if err != nil {
		log.Error(ctx, "Failed to generate llms-full.txt: %v", err)
		os.Exit(1)
	}

	if cfg.DryRun {
		fmt.Println("🧪 Dry run mode - would generate:")
		fmt.Printf("   - %s/llms.txt (%d bytes)\n", cfg.OutputDir, len(llmsTxtContent))
		fmt.Printf("   - %s/llms-full.txt (%d bytes)\n", cfg.OutputDir, len(llmsFullContent))
		fmt.Println("\n📄 llms.txt preview (first 2000 chars):")
		preview := llmsTxtContent
		if len(preview) > 2000 {
			preview = preview[:2000] + "\n..."
		}
		fmt.Println(preview)
		return
	}

	// Create output directory if needed
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		log.Error(ctx, "Failed to create output directory: %v", err)
		os.Exit(1)
	}

	// Write llms.txt
	llmsTxtPath := filepath.Join(cfg.OutputDir, "llms.txt")
	if err := os.WriteFile(llmsTxtPath, []byte(llmsTxtContent), 0o644); err != nil {
		log.Error(ctx, "Failed to write llms.txt: %v", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Generated %s (%d bytes)\n", llmsTxtPath, len(llmsTxtContent))

	// Write llms-full.txt
	llmsFullPath := filepath.Join(cfg.OutputDir, "llms-full.txt")
	if err := os.WriteFile(llmsFullPath, []byte(llmsFullContent), 0o644); err != nil {
		log.Error(ctx, "Failed to write llms-full.txt: %v", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Generated %s (%d bytes)\n", llmsFullPath, len(llmsFullContent))
}

func parseFlags() Config {
	cfg := Config{
		OutputDir:       "docs",
		LLMProvider:     "anthropic",
		MaxFullTextSize: 500000, // ~500KB max for full text
	}

	flag.StringVar(&cfg.OutputDir, "output", cfg.OutputDir, "Output directory for llms.txt files")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Enable verbose output")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "Show what would be done without writing files")
	flag.BoolVar(&cfg.UseLLM, "use-llm", false, "Use LLM to generate descriptions (requires API key)")
	flag.StringVar(&cfg.LLMProvider, "llm-provider", cfg.LLMProvider, "LLM provider: openai or anthropic")
	flag.BoolVar(&cfg.IncludeAIDocs, "include-ai-docs", true, "Include AI Assistant documentation")
	flag.BoolVar(&cfg.IncludeDesign, "include-design", false, "Include design documentation (internal)")
	flag.IntVar(&cfg.MaxFullTextSize, "max-size", cfg.MaxFullTextSize, "Maximum size for llms-full.txt in bytes")

	flag.Parse()

	return cfg
}

func loadMkDocsConfig() (*MkDocsConfig, error) {
	content, err := docs.EmbeddedDocs.ReadFile("mkdocs.yml")
	if err != nil {
		// Try reading from parent directory
		content, err = os.ReadFile("docs/mkdocs.yml")
		if err != nil {
			return nil, fmt.Errorf("could not read mkdocs.yml: %w", err)
		}
	}

	var cfg MkDocsConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse mkdocs.yml: %w", err)
	}

	return &cfg, nil
}

func parseNavigation(nav []interface{}, cfg Config) []DocSection {
	var sections []DocSection

	for _, item := range nav {
		switch v := item.(type) {
		case map[string]interface{}:
			for title, value := range v {
				section := DocSection{
					Title: title,
				}

				switch items := value.(type) {
				case string:
					// Single file: "Home: index.md"
					section.Links = append(section.Links, DocLink{
						Title: title,
						Path:  items,
					})
				case []interface{}:
					// Section with multiple files
					for _, subItem := range items {
						if subMap, ok := subItem.(map[string]interface{}); ok {
							for subTitle, subValue := range subMap {
								if path, ok := subValue.(string); ok {
									section.Links = append(section.Links, DocLink{
										Title: subTitle,
										Path:  path,
									})
								}
							}
						}
					}
				}

				if len(section.Links) > 0 {
					sections = append(sections, section)
				}
			}
		}
	}

	return sections
}

func loadAIAssistantDocs(verbose bool) []DocSection {
	aiSection := DocSection{
		Title:       "AI Assistant",
		Description: "AI-powered project onboarding and configuration assistant",
	}

	// AI Assistant documentation files in order of importance
	aiDocs := []struct {
		title string
		path  string
	}{
		{"AI Assistant Overview", "ai-assistant/index.md"},
		{"Getting Started", "ai-assistant/getting-started.md"},
		{"Developer Mode", "ai-assistant/developer-mode.md"},
		{"DevOps Mode", "ai-assistant/devops-mode.md"},
		{"Commands Reference", "ai-assistant/commands.md"},
		{"MCP Integration", "ai-assistant/mcp-integration.md"},
		{"Template Requirements", "ai-assistant/templates-config-requirements.md"},
		{"Troubleshooting", "ai-assistant/troubleshooting.md"},
		{"Usage Examples", "ai-assistant/usage-examples.md"},
	}

	for _, doc := range aiDocs {
		// Verify file exists
		fullPath := "docs/" + doc.path
		if _, err := docs.EmbeddedDocs.ReadFile(fullPath); err == nil {
			aiSection.Links = append(aiSection.Links, DocLink{
				Title: doc.title,
				Path:  doc.path,
			})
			if verbose {
				fmt.Printf("   📄 Added AI doc: %s\n", doc.path)
			}
		}
	}

	return []DocSection{aiSection}
}

func generateLLMsTxt(siteName string, sections []DocSection, cfg Config) string {
	var buf bytes.Buffer

	// Header
	buf.WriteString(fmt.Sprintf("# %s\n\n", siteName))

	// Summary blockquote
	buf.WriteString("> Simple Container offers high-level primitives for quick and easy setup of integration and delivery pipelines for microservice applications. Scale from startup to enterprise with 500x faster onboarding and 70% cost reduction.\n\n")

	// Description
	buf.WriteString("Simple Container provides container-native DevOps practices with a separation of concerns philosophy: DevOps manages infrastructure once in parent stacks, developers deploy self-service with simple client configurations. Supports AWS ECS Fargate, Google Cloud GKE Autopilot, and pure Kubernetes deployments.\n\n")

	// Generate sections
	for _, section := range sections {
		buf.WriteString(fmt.Sprintf("## %s\n\n", section.Title))

		if section.Description != "" {
			buf.WriteString(section.Description + "\n\n")
		}

		for _, link := range section.Links {
			description := link.Description
			if description == "" {
				description = generateDefaultDescription(link.Title, link.Path)
			}
			buf.WriteString(fmt.Sprintf("- [%s](docs/%s): %s\n", link.Title, link.Path, description))
		}
		buf.WriteString("\n")
	}

	// Optional section
	buf.WriteString("## Optional\n\n")
	buf.WriteString("- [AI Assistant Examples](docs/ai-assistant/examples/index.md): Detailed AI assistant usage examples\n")
	buf.WriteString("- [Kubernetes Affinity](docs/examples/kubernetes-affinity/README.md): Node affinity and isolation examples\n")
	buf.WriteString("- [Kubernetes VPA](docs/examples/kubernetes-vpa/README.md): Vertical Pod Autoscaler examples\n")
	buf.WriteString("- [Resource Adoption](docs/examples/resource-adoption/README.md): Resource adoption examples\n")

	return buf.String()
}

func generateDefaultDescription(title, path string) string {
	// Generate contextual descriptions based on title and path
	pathLower := strings.ToLower(path)
	titleLower := strings.ToLower(title)

	switch {
	case strings.Contains(pathLower, "getting-started/index"):
		return "Introduction and installation guide for Simple Container"
	case strings.Contains(pathLower, "installation"):
		return "How to install Simple Container CLI"
	case strings.Contains(pathLower, "quick-start"):
		return "Deploy your first app in 15 minutes"
	case strings.Contains(pathLower, "main-concepts"):
		return "Templates, resources, environments fundamentals"
	case strings.Contains(pathLower, "template-placeholders") && strings.Contains(pathLower, "advanced"):
		return "Advanced templating features and resource references"
	case strings.Contains(pathLower, "template-placeholders"):
		return "Basic placeholder syntax and usage"
	case strings.Contains(pathLower, "motivation"):
		return "Quantified scaling advantages over traditional approaches"
	case strings.Contains(pathLower, "ecs-fargate"):
		return "AWS ECS Fargate deployment guide"
	case strings.Contains(pathLower, "gke-autopilot") || strings.Contains(pathLower, "gcp-gke"):
		return "Google Cloud GKE Autopilot deployment guide"
	case strings.Contains(pathLower, "pure-kubernetes"):
		return "Plain Kubernetes deployment guide"
	case strings.Contains(pathLower, "secrets"):
		return "Managing secrets across environments"
	case strings.Contains(pathLower, "migration"):
		return "Migrating from other tools like Terraform or Pulumi"
	case strings.Contains(pathLower, "supported-resources"):
		return "Complete resource support matrix for AWS, GCP, Kubernetes"
	case strings.Contains(pathLower, "deployment-schemas") || strings.Contains(pathLower, "service-available"):
		return "Schema documentation for all deployment types"
	case strings.Contains(pathLower, "use-cases"):
		return "Common use case scenarios"
	case strings.Contains(pathLower, "scaling-advantages"):
		return "Detailed comparison with traditional approaches"
	case strings.Contains(pathLower, "compare"):
		return "Compare with Terraform, Pulumi, Helm, etc."
	case strings.Contains(pathLower, "best-practices"):
		return "Production best practices"
	case strings.Contains(titleLower, "overview") || strings.Contains(pathLower, "index"):
		return fmt.Sprintf("Overview of %s", strings.TrimSuffix(title, " Overview"))
	case strings.Contains(pathLower, "static-websites"):
		return "Static site deployments (landing pages, dashboards, portals)"
	case strings.Contains(pathLower, "ecs-deployments"):
		return "AWS ECS examples (backend services, blockchain, blogs)"
	case strings.Contains(pathLower, "lambda-functions"):
		return "Serverless examples (AI gateway, billing, analytics)"
	case strings.Contains(pathLower, "kubernetes-native"):
		return "Pure Kubernetes deployment examples"
	case strings.Contains(pathLower, "advanced-configs"):
		return "Advanced configuration patterns"
	case strings.Contains(pathLower, "parent-stacks"):
		return "Parent stack examples (AWS multi-region)"
	case strings.Contains(pathLower, "developer-mode"):
		return "Application team workflows for generating client configs"
	case strings.Contains(pathLower, "devops-mode"):
		return "Infrastructure team workflows for server configs"
	case strings.Contains(pathLower, "mcp-integration"):
		return "Model Context Protocol integration for IDE tools like Windsurf and Cursor"
	case strings.Contains(pathLower, "templates-config"):
		return "Template configuration requirements and validation rules"
	case strings.Contains(pathLower, "commands"):
		return "Complete command documentation"
	case strings.Contains(pathLower, "troubleshooting"):
		return "Common issues and solutions"
	case strings.Contains(pathLower, "usage-examples"):
		return "Real-world usage examples and workflows"
	case strings.Contains(pathLower, "ai-assistant") && strings.Contains(pathLower, "index"):
		return "Overview of the AI-powered project onboarding assistant"
	case strings.Contains(pathLower, "ai-assistant") && strings.Contains(pathLower, "getting-started"):
		return "Quick setup guide for AI assistant features"
	case strings.Contains(pathLower, "ai-assistant"):
		return "AI-powered project onboarding assistant"
	default:
		return title
	}
}

func generateLLMsFullTxt(ctx context.Context, siteName string, sections []DocSection, cfg Config, log logger.Logger) (string, error) {
	var buf bytes.Buffer
	totalSize := 0

	// Header
	header := fmt.Sprintf("# %s - Complete Documentation\n\n", siteName)
	header += "> This file contains the complete documentation for Simple Container, optimized for LLM consumption.\n\n"
	buf.WriteString(header)
	totalSize += len(header)

	// Table of contents
	buf.WriteString("## Table of Contents\n\n")
	for i, section := range sections {
		buf.WriteString(fmt.Sprintf("%d. [%s](#%s)\n", i+1, section.Title, strings.ToLower(strings.ReplaceAll(section.Title, " ", "-"))))
	}
	buf.WriteString("\n---\n\n")

	// Process each section
	for _, section := range sections {
		if totalSize > cfg.MaxFullTextSize {
			buf.WriteString("\n\n[Content truncated due to size limits. See llms.txt for links to individual documents.]\n")
			break
		}

		sectionHeader := fmt.Sprintf("# %s\n\n", section.Title)
		buf.WriteString(sectionHeader)
		totalSize += len(sectionHeader)

		for _, link := range section.Links {
			if totalSize > cfg.MaxFullTextSize {
				break
			}

			// Read the document content
			content, err := readDocumentContent(link.Path)
			if err != nil {
				if cfg.Verbose {
					fmt.Printf("   ⚠️  Could not read %s: %v\n", link.Path, err)
				}
				continue
			}

			// Strip YAML frontmatter
			content = stripFrontmatter(content)

			// Add document header
			docHeader := fmt.Sprintf("## %s\n\n", link.Title)
			buf.WriteString(docHeader)
			totalSize += len(docHeader)

			// Check if adding this content would exceed limit
			if totalSize+len(content) > cfg.MaxFullTextSize {
				// Truncate content
				remaining := cfg.MaxFullTextSize - totalSize - 100
				if remaining > 0 && remaining < len(content) {
					content = content[:remaining] + "\n\n[Content truncated...]\n"
				}
			}

			buf.WriteString(content)
			buf.WriteString("\n\n---\n\n")
			totalSize += len(content) + 10

			if cfg.Verbose {
				fmt.Printf("   📄 Added: %s (%d bytes)\n", link.Path, len(content))
			}
		}
	}

	return buf.String(), nil
}

func readDocumentContent(path string) (string, error) {
	// Try embedded docs first
	fullPath := "docs/" + path
	content, err := docs.EmbeddedDocs.ReadFile(fullPath)
	if err != nil {
		// Try filesystem
		content, err = os.ReadFile("docs/docs/" + path)
		if err != nil {
			return "", err
		}
	}
	return string(content), nil
}

func stripFrontmatter(content string) string {
	// Remove YAML frontmatter (content between --- markers at the start)
	if !strings.HasPrefix(content, "---") {
		return content
	}

	// Find the closing ---
	rest := content[3:]
	idx := strings.Index(rest, "---")
	if idx == -1 {
		return content
	}

	// Return content after frontmatter
	return strings.TrimSpace(rest[idx+3:])
}

// LLM-based description generation (optional)
func generateLLMDescription(ctx context.Context, title, content string, cfg Config) (string, error) {
	if !cfg.UseLLM {
		return "", nil
	}

	var llm llms.Model
	var err error

	switch cfg.LLMProvider {
	case "anthropic":
		apiKey := getAnthropicAPIKey()
		if apiKey == "" {
			return "", fmt.Errorf("ANTHROPIC_API_KEY not set")
		}
		llm, err = anthropic.New(anthropic.WithToken(apiKey))
	case "openai":
		apiKey := getOpenAIAPIKey()
		if apiKey == "" {
			return "", fmt.Errorf("OPENAI_API_KEY not set")
		}
		llm, err = openai.New(openai.WithToken(apiKey))
	default:
		return "", fmt.Errorf("unsupported LLM provider: %s", cfg.LLMProvider)
	}

	if err != nil {
		return "", err
	}

	// Truncate content for prompt
	truncatedContent := content
	if len(truncatedContent) > 2000 {
		truncatedContent = truncatedContent[:2000] + "..."
	}

	prompt := fmt.Sprintf(`Generate a concise one-sentence description (max 100 characters) for a documentation page titled "%s".
The description should explain what this page covers for developers looking for documentation.

Page content preview:
%s

Return ONLY the description, no quotes or extra text.`, title, truncatedContent)

	response, err := llms.GenerateFromSinglePrompt(ctx, llm, prompt)
	if err != nil {
		return "", err
	}

	// Clean up response
	description := strings.TrimSpace(response)
	description = strings.Trim(description, "\"'")

	// Ensure it's not too long
	if len(description) > 150 {
		// Find last space before 150 chars
		idx := strings.LastIndex(description[:150], " ")
		if idx > 0 {
			description = description[:idx] + "..."
		}
	}

	return description, nil
}

func getOpenAIAPIKey() string {
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		return apiKey
	}
	if cfg, err := config.Load(); err == nil {
		if apiKey := cfg.GetOpenAIAPIKey(); apiKey != "" {
			return apiKey
		}
	}
	return ""
}

func getAnthropicAPIKey() string {
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		return apiKey
	}
	if cfg, err := config.Load(); err == nil {
		if providerCfg, exists := cfg.GetProviderConfig(config.ProviderAnthropic); exists {
			return providerCfg.APIKey
		}
	}
	return ""
}

