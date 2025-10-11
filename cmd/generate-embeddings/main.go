package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	langchainembeddings "github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms/openai"

	"github.com/simple-container-com/api/docs"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/assistant/config"
	"github.com/simple-container-com/api/pkg/assistant/embeddings"
)

// Embeddings configuration
const (
	DefaultEmbeddingModel = "text-embedding-3-small"
	DefaultBatchSize      = 100
	MaxRetries            = 3
	InitialRetryDelay     = 1 * time.Second
	MaxRetryDelay         = 30 * time.Second
)

// Configuration for embeddings generation
type Config struct {
	OpenAIAPIKey   string
	EmbeddingModel string
	OutputPath     string
	BatchSize      int
	Verbose        bool
	DryRun         bool
	GenerateLocal  bool // Generate local embeddings in addition to or instead of OpenAI
}

// Document represents a document to be embedded
type Document struct {
	ID       string                 `json:"id"`
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata"`
}

// getOpenAIAPIKey retrieves OpenAI API key with proper priority order for CLI usage
func getOpenAIAPIKey() string {
	// 1. First priority: Environment variable (for build/CI contexts)
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		return apiKey
	}

	// 2. Second priority: Assistant configuration (~/.sc/assistant-config.json)
	if cfg, err := config.Load(); err == nil {
		if apiKey := cfg.GetOpenAIAPIKey(); apiKey != "" {
			return apiKey
		}
	}

	// 3. No API key found
	return ""
}

func main() {
	config := parseFlags()

	log := logger.New()
	ctx := context.Background()

	if config.Verbose {
		fmt.Printf("🚀 Starting Simple Container AI Assistant embeddings generation\n")
		fmt.Printf("📋 Configuration:\n")
		fmt.Printf("   Model: %s\n", config.EmbeddingModel)
		fmt.Printf("   Output: %s\n", config.OutputPath)
		fmt.Printf("   Batch Size: %d\n", config.BatchSize)
		fmt.Printf("   Dry Run: %t\n", config.DryRun)
		fmt.Printf("\n")
	}

	// Load documents from embedded documentation
	documents, err := loadDocuments(ctx, log, config.Verbose)
	if err != nil {
		log.Error(ctx, "Failed to load documents: %v", err)
		os.Exit(1)
	}

	if config.Verbose {
		fmt.Printf("📚 Loaded %d documents for embedding\n", len(documents))
	}

	if len(documents) == 0 {
		fmt.Println("⚠️  No documents found to embed")
		os.Exit(0)
	}

	if config.DryRun {
		fmt.Println("🧪 Dry run mode - would generate embeddings for:")
		for _, doc := range documents {
			fmt.Printf("   - %s (%d chars)\n", doc.ID, len(doc.Content))
		}

		hasOpenAI := config.OpenAIAPIKey != ""
		generateOpenAI := hasOpenAI                        // Generate OpenAI if key is available
		generateLocal := config.GenerateLocal || hasOpenAI // Generate local as fallback (always when OpenAI available) or when explicitly requested

		if generateOpenAI {
			cost := estimateEmbeddingCost(config.EmbeddingModel, documents)
			fmt.Printf("📊 OpenAI Embeddings: %d documents, estimated cost: $%.4f using %s\n", len(documents), cost, config.EmbeddingModel)
		}
		if generateLocal {
			fmt.Printf("📊 Local Embeddings: %d documents, cost: $0.00 using enhanced vocabulary analysis\n", len(documents))
		}
		return
	}

	// Determine what to generate
	hasOpenAI := config.OpenAIAPIKey != ""
	generateOpenAI := hasOpenAI                        // Generate OpenAI if key is available
	generateLocal := config.GenerateLocal || hasOpenAI // Generate local as fallback (always when OpenAI available) or when explicitly requested

	// Generate OpenAI embeddings if requested
	if generateOpenAI {
		fmt.Printf("🚀 Generating OpenAI embeddings using %s...\n", config.EmbeddingModel)
		embeddedDocs, err := generateOpenAIEmbeddings(ctx, config, documents, log)
		if err != nil {
			log.Error(ctx, "Failed to generate OpenAI embeddings: %v", err)
			os.Exit(1)
		}

		// Save OpenAI embeddings
		openaiPath := "pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json"
		if err := saveEmbeddings(openaiPath, embeddedDocs, config.Verbose); err != nil {
			log.Error(ctx, "Failed to save OpenAI embeddings: %v", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Successfully generated %d OpenAI embeddings using %s\n", len(embeddedDocs), config.EmbeddingModel)
		fmt.Printf("💾 Saved to: %s\n", openaiPath)
	}

	// Generate local embeddings if requested
	if generateLocal {
		fmt.Println("🏠 Generating enhanced local embeddings...")
		localEmbeddedDocs, err := generateLocalEmbeddings(documents, config.Verbose)
		if err != nil {
			log.Error(ctx, "Failed to generate local embeddings: %v", err)
			os.Exit(1)
		}

		// Save local embeddings
		localPath := "pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json"
		if err := saveEmbeddings(localPath, localEmbeddedDocs, config.Verbose); err != nil {
			log.Error(ctx, "Failed to save local embeddings: %v", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Successfully generated %d local embeddings using enhanced vocabulary analysis\n", len(localEmbeddedDocs))
		fmt.Printf("💾 Saved to: %s\n", localPath)
	}
}

func parseFlags() Config {
	config := Config{
		EmbeddingModel: DefaultEmbeddingModel,
		OutputPath:     "pkg/assistant/embeddings/vectors/prebuilt_embeddings.json",
		BatchSize:      DefaultBatchSize,
	}

	flag.StringVar(&config.OpenAIAPIKey, "openai-key", "", "OpenAI API key (defaults to OPENAI_API_KEY env var, then ~/.sc/assistant-config.json)")
	flag.StringVar(&config.EmbeddingModel, "model", config.EmbeddingModel, "OpenAI embedding model to use")
	flag.StringVar(&config.OutputPath, "output", config.OutputPath, "Output path for generated embeddings")
	flag.IntVar(&config.BatchSize, "batch-size", config.BatchSize, "Number of documents to process in each batch")
	flag.BoolVar(&config.Verbose, "verbose", false, "Enable verbose output")
	flag.BoolVar(&config.DryRun, "dry-run", false, "Show what would be done without making API calls")
	flag.BoolVar(&config.GenerateLocal, "local", false, "Generate local embeddings (in addition to or instead of OpenAI)")

	flag.Parse()

	// Get API key with proper priority order if not provided via command line
	if config.OpenAIAPIKey == "" {
		config.OpenAIAPIKey = getOpenAIAPIKey()
	}

	// Check if we can generate embeddings
	hasOpenAI := config.OpenAIAPIKey != ""
	generateOpenAI := hasOpenAI                        // Generate OpenAI if key is available
	generateLocal := config.GenerateLocal || hasOpenAI // Generate local as fallback (always when OpenAI available) or when explicitly requested

	if !generateOpenAI && !generateLocal && !config.DryRun {
		fmt.Fprintf(os.Stderr, "❌ Error: No embedding generation method available\n")
		fmt.Fprintf(os.Stderr, "   Provide OpenAI API key or use -local flag for local embeddings\n")
		os.Exit(1)
	}

	// Validate embedding model
	if err := validateEmbeddingModel(config.EmbeddingModel); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}

	return config
}

func validateEmbeddingModel(model string) error {
	validModels := map[string]bool{
		"text-embedding-3-small": true,
		"text-embedding-3-large": true,
		"text-embedding-ada-002": true,
	}

	if !validModels[model] {
		return fmt.Errorf("unsupported embedding model: %s. Supported models: text-embedding-3-small, text-embedding-3-large, text-embedding-ada-002", model)
	}

	return nil
}

func loadDocuments(ctx context.Context, log logger.Logger, verbose bool) ([]Document, error) {
	var documents []Document

	// Walk through embedded documentation
	err := walkEmbeddedDocs(ctx, log, "docs", func(path string, content []byte) error {
		// Handle both markdown files and YAML configuration files
		pathLower := strings.ToLower(path)
		isMarkdown := strings.HasSuffix(pathLower, ".md")
		isYAML := strings.HasSuffix(pathLower, ".yaml") || strings.HasSuffix(pathLower, ".yml")

		if !isMarkdown && !isYAML {
			return nil
		}

		// Create relative path for ID
		id := strings.TrimPrefix(path, "docs/")
		id = strings.ReplaceAll(id, "\\", "/") // Normalize path separators

		// Skip empty files
		if len(content) == 0 {
			return nil
		}

		var doc Document
		var err error

		if isYAML {
			// Handle YAML configuration files with README context
			doc, err = processYAMLDocument(path, id, content)
			if err != nil {
				if verbose {
					fmt.Printf("⚠️  Failed to process YAML %s: %v\n", path, err)
				}
				return nil
			}
		} else {
			// Handle regular markdown files
			doc, err = processMarkdownDocument(path, id, content)
			if err != nil {
				if verbose {
					fmt.Printf("⚠️  Failed to process markdown %s: %v\n", path, err)
				}
				return nil
			}

			// Extract YAML code blocks from markdown and create separate embeddings
			yamlDocs := extractYAMLCodeBlocks(path, id, content, verbose)
			documents = append(documents, yamlDocs...)

			// Show extracted YAML blocks in verbose mode
			if verbose && len(yamlDocs) > 0 {
				fmt.Printf("   🔧 Extracted %d YAML configuration blocks from this document\n", len(yamlDocs))
			}
		}

		documents = append(documents, doc)

		if verbose {
			docType := "documentation"
			if isYAML {
				docType = "configuration"
			}
			fmt.Printf("📄 Loaded: %s (%d chars) - %s [%s]\n", doc.ID, len(doc.Content), doc.Metadata["title"], docType)
		}

		return nil
	})

	return documents, err
}

func walkEmbeddedDocs(ctx context.Context, log logger.Logger, root string, fn func(path string, content []byte) error) error {
	entries, err := docs.EmbeddedDocs.ReadDir(root)
	if err != nil {
		return fmt.Errorf("error reading embedded docs dir %s: %w", root, err)
	}

	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())

		if entry.IsDir() {
			// Recursively walk subdirectories
			if err := walkEmbeddedDocs(ctx, log, path, fn); err != nil {
				return err
			}
		} else {
			// Read file content
			content, err := docs.EmbeddedDocs.ReadFile(path)
			if err != nil {
				if log != nil {
					log.Warn(ctx, "Failed to read embedded file %s: %v", path, err)
				}
				continue
			}

			if err := fn(path, content); err != nil {
				return err
			}
		}
	}

	return nil
}

func extractTitleFromMarkdown(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}

// processMarkdownDocument processes a regular markdown file
func processMarkdownDocument(path, id string, content []byte) (Document, error) {
	// Extract title from first heading or use filename
	title := extractTitleFromMarkdown(string(content))
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), ".md")
	}

	// Create metadata
	metadata := map[string]interface{}{
		"title": title,
		"path":  id,
		"type":  "documentation",
	}

	// Handle long documents - OpenAI can handle much more content than 8000 chars
	contentStr := string(content)
	// OpenAI text-embedding models can handle up to ~8192 tokens (roughly 30,000+ characters)
	// Only truncate extremely large documents to avoid hitting API limits
	if len(contentStr) > 25000 {
		// For very large docs, take first part + last part to preserve both intro and conclusion
		firstPart := contentStr[:12000]
		lastPart := contentStr[len(contentStr)-12000:]
		contentStr = firstPart + "\n\n[...middle content truncated for embedding...]\n\n" + lastPart
	}

	return Document{
		ID:       id,
		Content:  contentStr,
		Metadata: metadata,
	}, nil
}

// processYAMLDocument processes a YAML configuration file with README context
func processYAMLDocument(path, id string, content []byte) (Document, error) {
	filename := filepath.Base(path)
	dirPath := filepath.Dir(path)

	// Try to find README.md in the same directory
	readmePath := filepath.Join(dirPath, "README.md")
	var readmeContent string

	// Read the README if it exists
	if readmeData, err := docs.EmbeddedDocs.ReadFile(readmePath); err == nil {
		readmeContent = string(readmeData)
	}

	// Extract title from README or use a descriptive name
	var title string
	if readmeContent != "" {
		title = extractTitleFromMarkdown(readmeContent)
	}
	if title == "" {
		// Create a descriptive title based on the file and directory
		dirName := filepath.Base(dirPath)
		configType := strings.TrimSuffix(filename, filepath.Ext(filename))
		if len(configType) > 0 {
			title = fmt.Sprintf("%s - %s Configuration", dirName, strings.ToUpper(configType[:1])+configType[1:])
		} else {
			title = fmt.Sprintf("%s - Configuration", dirName)
		}
	}

	// Create enhanced content combining YAML and README context
	var enhancedContent strings.Builder

	// Add title and description from README
	if readmeContent != "" {
		enhancedContent.WriteString(fmt.Sprintf("# %s\n\n", title))

		// Extract the first paragraph from README as description
		lines := strings.Split(readmeContent, "\n")
		inDescription := false
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "# ") {
				continue // Skip title
			}
			if line == "" && inDescription {
				break // End of first paragraph
			}
			if line != "" {
				inDescription = true
				enhancedContent.WriteString(line + " ")
			}
		}
		enhancedContent.WriteString("\n\n")
	}

	// Add YAML configuration with clear labeling
	configType := strings.TrimSuffix(filename, filepath.Ext(filename))
	configTitle := "Configuration"
	if len(configType) > 0 {
		configTitle = strings.ToUpper(configType[:1]) + configType[1:] + " Configuration"
	}
	enhancedContent.WriteString(fmt.Sprintf("## %s\n\n", configTitle))
	enhancedContent.WriteString("```yaml\n")
	enhancedContent.WriteString(string(content))
	enhancedContent.WriteString("\n```\n")

	// Add additional context from README
	if readmeContent != "" {
		// Look for configuration or features sections
		lines := strings.Split(readmeContent, "\n")
		inRelevantSection := false
		for _, line := range lines {
			trimmedLine := strings.TrimSpace(line)

			// Check for relevant section headers
			if strings.Contains(strings.ToLower(trimmedLine), "configuration") ||
				strings.Contains(strings.ToLower(trimmedLine), "features") ||
				strings.Contains(strings.ToLower(trimmedLine), "setup") ||
				strings.Contains(strings.ToLower(trimmedLine), "usage") {
				inRelevantSection = true
				enhancedContent.WriteString("\n" + line + "\n")
				continue
			}

			// Stop at next major section
			if strings.HasPrefix(trimmedLine, "# ") || strings.HasPrefix(trimmedLine, "## ") {
				inRelevantSection = false
			}

			if inRelevantSection && trimmedLine != "" {
				enhancedContent.WriteString(line + "\n")
			}
		}
	}

	// Create metadata with enhanced information
	metadata := map[string]interface{}{
		"title":        title,
		"path":         id,
		"type":         "configuration",
		"config_type":  strings.TrimSuffix(filename, filepath.Ext(filename)),
		"example_type": filepath.Base(strings.TrimSuffix(dirPath, filepath.Base(dirPath))),
		"filename":     filename,
	}

	finalContent := enhancedContent.String()

	// Handle long YAML documents with same logic as markdown
	if len(finalContent) > 25000 {
		// For very large docs, take first part + last part to preserve both intro and conclusion
		firstPart := finalContent[:12000]
		lastPart := finalContent[len(finalContent)-12000:]
		finalContent = firstPart + "\n\n[...middle content truncated for embedding...]\n\n" + lastPart
	}

	return Document{
		ID:       id,
		Content:  finalContent,
		Metadata: metadata,
	}, nil
}

// extractYAMLCodeBlocks extracts YAML code blocks from markdown with surrounding context
func extractYAMLCodeBlocks(path, parentID string, content []byte, verbose bool) []Document {
	var documents []Document
	contentStr := string(content)

	// Regular expression to find YAML code blocks
	yamlBlockRegex := regexp.MustCompile("(?s)```(?:yaml|yml)(?:\n|\r\n)(.*?)(?:\n|\r\n)```")
	matches := yamlBlockRegex.FindAllStringSubmatch(contentStr, -1)

	if len(matches) == 0 {
		return documents
	}

	// Split content into lines for context extraction
	lines := strings.Split(contentStr, "\n")

	for i, match := range matches {
		yamlContent := strings.TrimSpace(match[1])

		// Check if this YAML block is relevant (contains server.yaml, secrets.yaml, or client.yaml patterns)
		if !isRelevantYAMLBlock(yamlContent) {
			continue
		}

		// Find the position of this YAML block in the content
		blockStartIndex := strings.Index(contentStr, match[0])
		if blockStartIndex == -1 {
			continue
		}

		// Extract surrounding context
		context := extractSurroundingContext(lines, match[0], blockStartIndex, contentStr)

		// Determine the YAML type based on content patterns
		yamlType := determineYAMLType(yamlContent)

		// Create enhanced document with context
		doc := createYAMLEmbeddingDocument(parentID, yamlContent, context, yamlType, i+1)
		documents = append(documents, doc)

		if verbose {
			fmt.Printf("     📋 %s block (%d chars)\n", yamlType, len(doc.Content))
		}
	}

	return documents
}

// isRelevantYAMLBlock checks if a YAML block contains Simple Container configuration patterns
func isRelevantYAMLBlock(yamlContent string) bool {
	yamlLower := strings.ToLower(yamlContent)

	// Check for Simple Container configuration patterns
	relevantPatterns := []string{
		// Server.yaml patterns
		"provisioner:", "resources:", "templates:", "environments:",

		// Client.yaml patterns
		"name:", "type:", "uses:", "secrets:", "environment:", "deployment:",

		// Secrets.yaml patterns
		"mongodb_atlas", "redis", "aws_", "gcp_", "discord", "telegram",
		"api_key", "secret_key", "private_key", "webhook", "token",

		// General Simple Container patterns
		"simple-container", "${resource:", "${secret:", "mongodb-atlas", "redis-cache",
	}

	// Must contain at least 2 relevant patterns to be considered relevant
	patternCount := 0
	for _, pattern := range relevantPatterns {
		if strings.Contains(yamlLower, pattern) {
			patternCount++
			if patternCount >= 2 {
				return true
			}
		}
	}

	// Also check if it's a substantial YAML block (more than just a few lines)
	lines := strings.Split(strings.TrimSpace(yamlContent), "\n")
	return len(lines) >= 5 && patternCount >= 1
}

// determineYAMLType determines the type of YAML configuration based on content
func determineYAMLType(yamlContent string) string {
	yamlLower := strings.ToLower(yamlContent)

	// Server.yaml indicators
	if strings.Contains(yamlLower, "provisioner:") ||
		strings.Contains(yamlLower, "resources:") ||
		strings.Contains(yamlLower, "templates:") {
		return "server.yaml"
	}

	// Secrets.yaml indicators
	if strings.Contains(yamlLower, "mongodb_atlas") ||
		strings.Contains(yamlLower, "api_key") ||
		strings.Contains(yamlLower, "secret_key") ||
		strings.Contains(yamlLower, "private_key") ||
		strings.Contains(yamlLower, "webhook") {
		return "secrets.yaml"
	}

	// Client.yaml indicators
	if strings.Contains(yamlLower, "uses:") ||
		strings.Contains(yamlLower, "deployment:") ||
		(strings.Contains(yamlLower, "name:") && strings.Contains(yamlLower, "type:")) {
		return "client.yaml"
	}

	// Docker compose indicators
	if strings.Contains(yamlLower, "version:") && strings.Contains(yamlLower, "services:") {
		return "docker-compose.yaml"
	}

	return "configuration.yaml"
}

// extractSurroundingContext extracts context around a YAML block
func extractSurroundingContext(lines []string, yamlBlock string, blockStartIndex int, fullContent string) string {
	// Find which line the YAML block starts on
	beforeBlock := fullContent[:blockStartIndex]
	blockStartLine := strings.Count(beforeBlock, "\n")

	// Extract context: 10 lines before and 5 lines after the YAML block
	contextStart := blockStartLine - 10
	if contextStart < 0 {
		contextStart = 0
	}

	// Find where the YAML block ends
	yamlEndIndex := blockStartIndex + len(yamlBlock)
	afterBlock := fullContent[:yamlEndIndex]
	blockEndLine := strings.Count(afterBlock, "\n")

	contextEnd := blockEndLine + 5
	if contextEnd >= len(lines) {
		contextEnd = len(lines) - 1
	}

	// Build context content
	var contextBuilder strings.Builder

	// Add preceding context
	if contextStart < blockStartLine {
		contextBuilder.WriteString("## Context\n\n")
		for i := contextStart; i < blockStartLine; i++ {
			line := strings.TrimSpace(lines[i])
			if line != "" {
				contextBuilder.WriteString(lines[i] + "\n")
			}
		}
		contextBuilder.WriteString("\n")
	}

	// Add following context
	if blockEndLine < contextEnd {
		contextBuilder.WriteString("\n## Additional Context\n\n")
		for i := blockEndLine + 1; i <= contextEnd && i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			if line != "" {
				contextBuilder.WriteString(lines[i] + "\n")
			}
		}
	}

	return contextBuilder.String()
}

// createYAMLEmbeddingDocument creates a specialized embedding document for YAML blocks
func createYAMLEmbeddingDocument(parentID, yamlContent, context, yamlType string, blockNumber int) Document {
	// Create unique ID for this YAML block
	id := fmt.Sprintf("%s#yaml-block-%d-%s", parentID, blockNumber, yamlType)

	// Create enhanced content with context and YAML
	var contentBuilder strings.Builder

	// Create title caser for proper capitalization
	titleCaser := cases.Title(language.English)

	// Add title based on YAML type
	configTypeName := titleCaser.String(strings.TrimSuffix(yamlType, ".yaml"))
	title := fmt.Sprintf("Simple Container %s Example", configTypeName)
	contentBuilder.WriteString(fmt.Sprintf("# %s\n\n", title))

	// Add description based on type
	description := getYAMLTypeDescription(yamlType)
	if description != "" {
		contentBuilder.WriteString(description + "\n\n")
	}

	// Add surrounding context
	if context != "" {
		contentBuilder.WriteString(context)
		contentBuilder.WriteString("\n")
	}

	// Add the YAML configuration with proper formatting
	contentBuilder.WriteString(fmt.Sprintf("## %s Configuration\n\n", configTypeName))
	contentBuilder.WriteString("```yaml\n")
	contentBuilder.WriteString(yamlContent)
	contentBuilder.WriteString("\n```\n")

	// Create metadata
	metadata := map[string]interface{}{
		"title":       title,
		"path":        parentID,
		"type":        "yaml_example",
		"yaml_type":   yamlType,
		"block_index": blockNumber,
		"parent_doc":  parentID,
	}

	return Document{
		ID:       id,
		Content:  contentBuilder.String(),
		Metadata: metadata,
	}
}

// getYAMLTypeDescription returns a description for the YAML type
func getYAMLTypeDescription(yamlType string) string {
	switch yamlType {
	case "server.yaml":
		return "Server configuration defines infrastructure resources, provisioners, and deployment templates for Simple Container stacks."
	case "client.yaml":
		return "Client configuration defines application deployment settings, resource usage, and environment-specific configurations."
	case "secrets.yaml":
		return "Secrets configuration manages sensitive data like API keys, database credentials, and authentication tokens."
	case "docker-compose.yaml":
		return "Docker Compose configuration for container orchestration and service definitions."
	default:
		return "Simple Container YAML configuration example with deployment and infrastructure settings."
	}
}

func generateOpenAIEmbeddings(ctx context.Context, config Config, documents []Document, log logger.Logger) ([]embeddings.EmbeddedDocument, error) {
	var allEmbeddings []embeddings.EmbeddedDocument
	totalTokens := 0

	// Process documents in batches to respect rate limits
	for i := 0; i < len(documents); i += config.BatchSize {
		end := i + config.BatchSize
		if end > len(documents) {
			end = len(documents)
		}

		batch := documents[i:end]
		if config.Verbose {
			fmt.Printf("🔄 Processing batch %d/%d (%d documents)\n",
				i/config.BatchSize+1,
				(len(documents)+config.BatchSize-1)/config.BatchSize,
				len(batch))
		}

		batchEmbeddings, tokens, err := generateBatchEmbeddings(ctx, config, batch)
		if err != nil {
			return nil, fmt.Errorf("failed to generate batch embeddings: %w", err)
		}

		allEmbeddings = append(allEmbeddings, batchEmbeddings...)
		totalTokens += tokens

		if config.Verbose {
			fmt.Printf("   ✅ Generated %d embeddings (%d tokens)\n", len(batchEmbeddings), tokens)
		}

		// Rate limiting: wait between batches to be respectful to OpenAI API
		if end < len(documents) {
			time.Sleep(1 * time.Second)
		}
	}

	if config.Verbose {
		cost := calculateEmbeddingCost(config.EmbeddingModel, totalTokens)
		fmt.Printf("📊 Total tokens used: %d (estimated cost: $%.4f)\n", totalTokens, cost)
	}

	return allEmbeddings, nil
}

func generateLocalEmbeddings(documents []Document, verbose bool) ([]embeddings.EmbeddedDocument, error) {
	var allEmbeddings []embeddings.EmbeddedDocument

	if verbose {
		fmt.Printf("🔄 Generating local embeddings for %d documents...\n", len(documents))
	}

	for i, doc := range documents {
		// Generate local embedding using the enhanced vocabulary-based approach
		embedding := createEnhancedLocalEmbedding(doc.Content)

		embeddedDoc := embeddings.EmbeddedDocument{
			ID:        doc.ID,
			Content:   doc.Content,
			Metadata:  doc.Metadata,
			Embedding: embedding,
		}

		allEmbeddings = append(allEmbeddings, embeddedDoc)

		if verbose && (i+1)%10 == 0 {
			fmt.Printf("   ✅ Processed %d/%d documents\n", i+1, len(documents))
		}
	}

	if verbose {
		fmt.Printf("📊 Generated %d local embeddings (%d dimensions each)\n", len(allEmbeddings), len(allEmbeddings[0].Embedding))
	}

	return allEmbeddings, nil
}

// createEnhancedLocalEmbedding replicates the function from embeddings.go
func createEnhancedLocalEmbedding(text string) []float32 {
	return createVocabularyBasedEmbedding(text, 512) // 512-dimensional local embeddings
}

func generateBatchEmbeddings(ctx context.Context, config Config, documents []Document) ([]embeddings.EmbeddedDocument, int, error) {
	// Create OpenAI LLM client
	llm, err := openai.New(
		openai.WithToken(config.OpenAIAPIKey),
		openai.WithEmbeddingModel(config.EmbeddingModel),
	)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create OpenAI LLM: %w", err)
	}

	// Create embedder using the LLM client
	embedder, err := langchainembeddings.NewEmbedder(llm)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create embedder: %w", err)
	}

	// Prepare input texts
	texts := make([]string, len(documents))
	for i, doc := range documents {
		texts[i] = doc.Content
	}

	// Generate embeddings with retry logic
	vectors, err := generateEmbeddingsWithRetry(ctx, embedder, texts, config.Verbose)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to generate embeddings: %w", err)
	}

	if len(vectors) != len(documents) {
		return nil, 0, fmt.Errorf("received %d embeddings for %d documents", len(vectors), len(documents))
	}

	// Convert to our format
	var embeddedDocs []embeddings.EmbeddedDocument
	totalTokens := 0

	for i, doc := range documents {
		embeddedDoc := embeddings.EmbeddedDocument{
			ID:        doc.ID,
			Content:   doc.Content,
			Metadata:  doc.Metadata,
			Embedding: vectors[i],
		}
		embeddedDocs = append(embeddedDocs, embeddedDoc)

		// Estimate tokens (rough approximation: 1 token ≈ 4 chars)
		totalTokens += len(doc.Content) / 4
	}

	return embeddedDocs, totalTokens, nil
}

func saveEmbeddings(outputPath string, embeddedDocs []embeddings.EmbeddedDocument, verbose bool) error {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create embeddings data structure
	embeddingsData := embeddings.PrebuiltEmbeddings{
		Version:   "1.0",
		Documents: embeddedDocs,
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(embeddingsData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal embeddings: %w", err)
	}

	// Write to file
	if err := os.WriteFile(outputPath, jsonData, 0o644); err != nil {
		return fmt.Errorf("failed to write embeddings file: %w", err)
	}

	if verbose {
		fileInfo, _ := os.Stat(outputPath)
		fmt.Printf("💾 Embeddings saved: %s (%.2f MB)\n", outputPath, float64(fileInfo.Size())/(1024*1024))
	}

	return nil
}

func estimateEmbeddingCost(model string, documents []Document) float64 {
	totalTokens := 0
	for _, doc := range documents {
		// Rough estimation: ~4 characters per token
		totalTokens += len(doc.Content) / 4
	}

	return calculateEmbeddingCost(model, totalTokens)
}

func calculateEmbeddingCost(model string, tokens int) float64 {
	var costPer1KTokens float64

	switch model {
	case "text-embedding-3-small":
		costPer1KTokens = 0.00002 // $0.00002 per 1K tokens
	case "text-embedding-3-large":
		costPer1KTokens = 0.00013 // $0.00013 per 1K tokens
	case "text-embedding-ada-002":
		costPer1KTokens = 0.0001 // $0.0001 per 1K tokens
	default:
		costPer1KTokens = 0.00002 // Default to cheapest
	}

	return (float64(tokens) / 1000.0) * costPer1KTokens
}

// Local embedding functions (duplicated from embeddings.go for standalone tool)
func createVocabularyBasedEmbedding(text string, dimensions int) []float32 {
	// Enhanced vocabulary-based embedding generation
	words := extractWordsLocal(text)
	embedding := make([]float32, dimensions)

	if len(words) == 0 {
		return embedding
	}

	// Build word frequency map
	wordFreq := make(map[string]int)
	for _, word := range words {
		wordFreq[word]++
	}

	// Calculate features based on word analysis
	totalWords := len(words)
	uniqueWords := len(wordFreq)

	// Base features (first 100 dimensions)
	if dimensions > 0 {
		embedding[0] = float32(totalWords) / 1000.0
	}
	if dimensions > 1 {
		embedding[1] = float32(uniqueWords) / 500.0
	}
	if dimensions > 2 {
		embedding[2] = float32(uniqueWords) / float32(totalWords)
	} // vocabulary richness
	if dimensions > 3 {
		embedding[3] = averageWordLengthFloatLocal(words) / 20.0
	}

	// Technical term detection (dimensions 4-50)
	technicalTerms := []string{
		"docker", "kubernetes", "aws", "gcp", "azure", "cloud", "container", "deployment",
		"yaml", "json", "config", "template", "resource", "stack", "service", "api",
		"database", "postgres", "mysql", "mongodb", "redis", "elasticsearch", "kafka",
		"nginx", "apache", "caddy", "traefik", "ingress", "load", "balancer",
		"terraform", "pulumi", "ansible", "helm", "kustomize", "gitops", "ci", "cd",
		"monitoring", "logging", "prometheus", "grafana", "jaeger", "zipkin", "alert",
		"security", "tls", "ssl", "oauth", "jwt", "rbac", "policy", "firewall",
	}

	for i, term := range technicalTerms {
		if i+4 >= dimensions {
			break
		}
		embedding[i+4] = float32(countWordOccurrencesLocal(words, term)) / float32(totalWords)
	}

	// Language and framework detection (dimensions 51-100)
	langFrameworks := []string{
		"javascript", "typescript", "nodejs", "npm", "yarn", "react", "vue", "angular",
		"python", "pip", "django", "flask", "fastapi", "pandas", "numpy", "pytorch",
		"java", "maven", "gradle", "spring", "hibernate", "junit", "scala", "kotlin",
		"go", "golang", "gin", "echo", "fiber", "gorm", "cobra", "viper",
		"rust", "cargo", "tokio", "serde", "diesel", "actix", "warp", "rocket",
		"php", "composer", "laravel", "symfony", "wordpress", "magento", "drupal",
		"ruby", "rails", "sinatra", "rspec", "bundler", "rake", "sidekiq",
	}

	startIdx := 51
	for i, term := range langFrameworks {
		if startIdx+i >= dimensions {
			break
		}
		embedding[startIdx+i] = float32(countWordOccurrencesLocal(words, term)) / float32(totalWords)
	}

	// Infrastructure and cloud terms (dimensions 101-200)
	infraTerms := []string{
		"server", "cluster", "node", "pod", "namespace", "volume", "storage",
		"network", "subnet", "vpc", "firewall", "gateway", "proxy", "cdn",
		"lambda", "function", "serverless", "fargate", "ecs", "eks", "gke",
		"s3", "bucket", "rds", "dynamodb", "cloudformation", "cloudwatch",
		"instance", "vm", "container", "image", "registry", "artifact",
		"backup", "snapshot", "restore", "migration", "scaling", "autoscaling",
		"availability", "zone", "region", "latency", "throughput", "performance",
	}

	startIdx = 101
	for i, term := range infraTerms {
		if startIdx+i >= dimensions {
			break
		}
		embedding[startIdx+i] = float32(countWordOccurrencesLocal(words, term)) / float32(totalWords)
	}

	// Configuration and deployment terms (dimensions 201-300)
	configTerms := []string{
		"environment", "staging", "production", "development", "test", "dev", "prod",
		"configuration", "config", "env", "variable", "secret", "key", "value",
		"port", "host", "url", "endpoint", "path", "route", "domain", "subdomain",
		"version", "tag", "branch", "commit", "release", "deploy", "rollback",
		"health", "check", "status", "ready", "live", "probe", "metric", "log",
	}

	startIdx = 201
	for i, term := range configTerms {
		if startIdx+i >= dimensions {
			break
		}
		embedding[startIdx+i] = float32(countWordOccurrencesLocal(words, term)) / float32(totalWords)
	}

	// Fill remaining dimensions with statistical features
	if dimensions > 300 {
		// Text statistics
		sentences := strings.Count(text, ".") + strings.Count(text, "!") + strings.Count(text, "?")
		codeBlocks := strings.Count(text, "```")
		yamlBlocks := strings.Count(text, "```yaml") + strings.Count(text, "```yml")

		embedding[300] = float32(sentences) / 100.0
		embedding[301] = float32(codeBlocks) / 20.0
		embedding[302] = float32(yamlBlocks) / 10.0

		// Character-level features
		if dimensions > 303 {
			embedding[303] = float32(len(text)) / 10000.0
		}
		if dimensions > 304 {
			embedding[304] = float32(strings.Count(text, "\n")) / 500.0
		}

		// Fill remaining with word n-grams and patterns
		for i := 305; i < dimensions; i++ {
			if i < len(words) {
				// Use word hash as feature
				hash := simpleStringHashLocal(words[i%len(words)])
				embedding[i] = float32(hash%1000) / 1000.0
			} else {
				// Fill with derived features
				embedding[i] = embedding[i%300] * 0.1
			}
		}
	}

	return normalizeVectorLocal(embedding)
}

func extractWordsLocal(text string) []string {
	// Convert to lowercase and extract words
	text = strings.ToLower(text)
	// Keep hyphens and underscores as part of words (important for technical terms)
	re := regexp.MustCompile(`[a-z0-9_-]+`)
	return re.FindAllString(text, -1)
}

func countWordOccurrencesLocal(words []string, target string) int {
	count := 0
	for _, word := range words {
		if strings.Contains(word, target) {
			count++
		}
	}
	return count
}

func averageWordLengthFloatLocal(words []string) float32 {
	if len(words) == 0 {
		return 0
	}
	totalLen := 0
	for _, word := range words {
		totalLen += len(word)
	}
	return float32(totalLen) / float32(len(words))
}

func simpleStringHashLocal(s string) int {
	hash := 0
	for _, char := range s {
		hash = hash*31 + int(char)
	}
	if hash < 0 {
		hash = -hash
	}
	return hash
}

func normalizeVectorLocal(embedding []float32) []float32 {
	// Calculate magnitude
	var magnitude float32
	for _, val := range embedding {
		magnitude += val * val
	}
	magnitude = float32(math.Sqrt(float64(magnitude)))

	// Avoid division by zero
	if magnitude == 0 {
		return embedding
	}

	// Normalize
	for i := range embedding {
		embedding[i] /= magnitude
	}

	return embedding
}

// generateEmbeddingsWithRetry implements retry logic with exponential backoff for OpenAI API calls
func generateEmbeddingsWithRetry(ctx context.Context, embedder *langchainembeddings.EmbedderImpl, texts []string, verbose bool) ([][]float32, error) {
	var lastErr error
	retryDelay := InitialRetryDelay

	for attempt := 0; attempt <= MaxRetries; attempt++ {
		if attempt > 0 {
			if verbose {
				fmt.Printf("   ⚠️  Retry attempt %d/%d after %v (previous error: %v)\n", attempt, MaxRetries, retryDelay, lastErr)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDelay):
				// Continue with retry
			}
			// Exponential backoff with jitter
			retryDelay = time.Duration(float64(retryDelay) * 1.5)
			if retryDelay > MaxRetryDelay {
				retryDelay = MaxRetryDelay
			}
		}

		vectors, err := embedder.EmbedDocuments(ctx, texts)
		if err == nil {
			if attempt > 0 && verbose {
				fmt.Printf("   ✅ Successfully generated embeddings after %d retries\n", attempt)
			}
			return vectors, nil
		}

		lastErr = err
		// Check if this is a retryable error
		if !isRetryableError(err) {
			return nil, fmt.Errorf("non-retryable error: %w", err)
		}

		if attempt == MaxRetries {
			break
		}
	}

	return nil, fmt.Errorf("failed after %d retries, last error: %w", MaxRetries, lastErr)
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	errorStr := strings.ToLower(err.Error())
	// Common retryable errors
	return strings.Contains(errorStr, "eof") ||
		strings.Contains(errorStr, "timeout") ||
		strings.Contains(errorStr, "connection reset") ||
		strings.Contains(errorStr, "rate limit") ||
		strings.Contains(errorStr, "too many requests") ||
		strings.Contains(errorStr, "internal server error") ||
		strings.Contains(errorStr, "bad gateway") ||
		strings.Contains(errorStr, "service unavailable") ||
		strings.Contains(errorStr, "gateway timeout")
}
