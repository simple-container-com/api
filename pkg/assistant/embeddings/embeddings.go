package embeddings

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	chromem "github.com/philippgille/chromem-go"
)

// Database represents an embedded vector database using chromem-go
type Database struct {
	collection *chromem.Collection
	db         *chromem.DB
}

// SearchResult represents a search result from the documentation
type SearchResult struct {
	ID         string                 `json:"id"`
	Content    string                 `json:"content"`
	Score      float64                `json:"score"`
	Similarity float64                `json:"similarity"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// LoadEmbeddedDatabase loads or creates the documentation database
func LoadEmbeddedDatabase() (*Database, error) {
	// Initialize chromem-go database
	db := chromem.NewDB()

	// Create a simple local embedding function (basic word count vectors)
	embeddingFunc := func(ctx context.Context, text string) ([]float32, error) {
		// Simple embedding based on text characteristics
		// This is a placeholder - in production you'd want a proper embedding model
		return createSimpleEmbedding(text), nil
	}

	// Create or get documentation collection with local embedding function
	collection, err := db.GetOrCreateCollection("simple-container-docs", nil, embeddingFunc)
	if err != nil {
		return nil, fmt.Errorf("failed to create collection: %w", err)
	}

	database := &Database{
		collection: collection,
		db:         db,
	}

	// Initialize with documentation if empty
	if err := database.initializeIfEmpty(); err != nil {
		log.Printf("Warning: Failed to initialize documentation database: %v", err)
	}

	return database, nil
}

// SearchDocumentation searches the documentation using semantic search
func SearchDocumentation(db *Database, query string, limit int) ([]SearchResult, error) {
	if db == nil || db.collection == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Perform similarity search using chromem-go
	results, err := db.collection.Query(context.Background(), query, limit, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Convert chromem results to SearchResult format
	searchResults := make([]SearchResult, len(results))
	for i, result := range results {
		// Extract metadata - chromem returns map[string]string
		metadata := make(map[string]interface{})
		if result.Metadata != nil {
			for k, v := range result.Metadata {
				metadata[k] = v
			}
		}

		// Ensure we have basic metadata
		if _, ok := metadata["title"]; !ok {
			metadata["title"] = result.ID
		}
		if _, ok := metadata["type"]; !ok {
			metadata["type"] = "documentation"
		}
		if _, ok := metadata["path"]; !ok {
			metadata["path"] = result.ID
		}

		searchResults[i] = SearchResult{
			ID:         result.ID,
			Content:    result.Content,
			Score:      float64(result.Similarity),
			Similarity: float64(result.Similarity),
			Metadata:   metadata,
		}
	}

	return searchResults, nil
}

// initializeIfEmpty loads documentation into the database if it's empty
func (db *Database) initializeIfEmpty() error {
	// Check if collection already has documents
	count := db.collection.Count()

	if count > 0 {
		// Already initialized
		return nil
	}

	// Find documentation directory
	docsPath := findDocsPath()
	if docsPath == "" {
		return fmt.Errorf("documentation directory not found")
	}

	// Index documentation files
	return db.indexDocumentation(docsPath)
}

// findDocsPath attempts to find the documentation directory
func findDocsPath() string {
	// Common locations for Simple Container documentation
	possiblePaths := []string{
		"docs/docs",
		"../docs/docs",
		"../../docs/docs",
		"./docs",
		"../docs",
	}

	for _, path := range possiblePaths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}

	return ""
}

// indexDocumentation recursively indexes markdown files in the documentation directory
func (db *Database) indexDocumentation(docsPath string) error {
	return filepath.Walk(docsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only process markdown files
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Warning: Failed to read %s: %v", path, err)
			return nil // Continue processing other files
		}

		// Create relative path for ID
		relPath, _ := filepath.Rel(docsPath, path)
		id := strings.ReplaceAll(relPath, "\\", "/") // Normalize path separators

		// Extract title from first heading or use filename
		title := extractTitleFromMarkdown(string(content))
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(path), ".md")
		}

		// Create metadata
		metadata := map[string]string{
			"title": title,
			"path":  relPath,
			"type":  "documentation",
		}

		// Add document to collection
		err = db.collection.AddDocument(context.Background(), chromem.Document{
			ID:       id,
			Content:  string(content),
			Metadata: metadata,
		})
		if err != nil {
			log.Printf("Warning: Failed to add document %s: %v", id, err)
			return nil // Continue processing other files
		}

		return nil
	})
}

// extractTitleFromMarkdown extracts the first heading from markdown content
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

// createSimpleEmbedding creates a basic embedding vector based on text characteristics
// This is a simple local implementation that doesn't require external APIs
func createSimpleEmbedding(text string) []float32 {
	// Normalize and clean text
	text = strings.ToLower(text)
	text = regexp.MustCompile(`[^\w\s]`).ReplaceAllString(text, " ")
	words := strings.Fields(text)

	// Create a feature vector based on text characteristics
	embedding := make([]float32, 128) // 128-dimensional vector

	if len(words) == 0 {
		return embedding
	}

	// Feature 1-10: Common Simple Container terms
	scTerms := []string{"docker", "kubernetes", "aws", "gcp", "postgres", "redis", "mongo", "yaml", "stack", "resource"}
	for i, term := range scTerms {
		if i < 10 {
			embedding[i] = float32(countTermOccurrences(words, term))
		}
	}

	// Feature 11-20: Technical concepts
	techTerms := []string{"deployment", "configuration", "service", "database", "api", "server", "client", "secret", "template", "scale"}
	for i, term := range techTerms {
		if i < 10 {
			embedding[10+i] = float32(countTermOccurrences(words, term))
		}
	}

	// Feature 21-30: Document structure indicators
	structureTerms := []string{"example", "guide", "tutorial", "reference", "concept", "advanced", "getting", "started", "install", "setup"}
	for i, term := range structureTerms {
		if i < 10 {
			embedding[20+i] = float32(countTermOccurrences(words, term))
		}
	}

	// Feature 31-40: Action words
	actionTerms := []string{"create", "deploy", "configure", "manage", "setup", "install", "run", "build", "test", "update"}
	for i, term := range actionTerms {
		if i < 10 {
			embedding[30+i] = float32(countTermOccurrences(words, term))
		}
	}

	// Feature 41-50: Cloud providers and services
	cloudTerms := []string{"fargate", "lambda", "s3", "rds", "gke", "cloudrun", "pubsub", "mongodb", "atlas", "cloudflare"}
	for i, term := range cloudTerms {
		if i < 10 {
			embedding[40+i] = float32(countTermOccurrences(words, term))
		}
	}

	// Feature 51-60: Programming languages and frameworks
	langTerms := []string{"nodejs", "python", "golang", "javascript", "typescript", "react", "express", "fastapi", "gin", "nest"}
	for i, term := range langTerms {
		if i < 10 {
			embedding[50+i] = float32(countTermOccurrences(words, term))
		}
	}

	// Feature 61-70: File types and formats
	fileTerms := []string{"dockerfile", "compose", "yaml", "json", "config", "env", "secret", "key", "cert", "ssl"}
	for i, term := range fileTerms {
		if i < 10 {
			embedding[60+i] = float32(countTermOccurrences(words, term))
		}
	}

	// Feature 71-80: Operations and DevOps
	opsTerms := []string{"provision", "destroy", "scale", "monitor", "log", "debug", "troubleshoot", "backup", "restore", "migrate"}
	for i, term := range opsTerms {
		if i < 10 {
			embedding[70+i] = float32(countTermOccurrences(words, term))
		}
	}

	// Feature 81-90: Text statistics
	embedding[80] = float32(len(words))                 // Word count
	embedding[81] = float32(len(text))                  // Character count
	embedding[82] = float32(averageWordLength(words))   // Average word length
	embedding[83] = float32(countCodeBlocks(text))      // Code blocks
	embedding[84] = float32(countLinks(text))           // Links
	embedding[85] = float32(countHeadings(text))        // Headings
	embedding[86] = float32(countListItems(text))       // List items
	embedding[87] = float32(countCamelCaseWords(words)) // CamelCase words
	embedding[88] = float32(countUppercaseWords(words)) // UPPERCASE words
	embedding[89] = float32(countNumericTerms(words))   // Numbers

	// Feature 91-100: Sentiment and style indicators
	positiveTerms := []string{"easy", "simple", "quick", "efficient", "powerful", "flexible", "reliable", "secure", "fast", "clean"}
	for i, term := range positiveTerms {
		if i < 10 {
			embedding[90+i] = float32(countTermOccurrences(words, term))
		}
	}

	// Feature 101-110: Problem/solution indicators
	problemTerms := []string{"error", "issue", "problem", "troubleshoot", "debug", "fix", "solve", "warning", "fail", "broken"}
	for i, term := range problemTerms {
		if i < 10 {
			embedding[100+i] = float32(countTermOccurrences(words, term))
		}
	}

	// Feature 111-120: Command and CLI terms
	cliTerms := []string{"command", "cli", "flag", "option", "argument", "parameter", "execute", "run", "invoke", "call"}
	for i, term := range cliTerms {
		if i < 10 {
			embedding[110+i] = float32(countTermOccurrences(words, term))
		}
	}

	// Feature 121-128: Additional context
	embedding[120] = float32(countCodeSnippets(text))        // Code snippets
	embedding[121] = float32(countCommands(text))            // Shell commands
	embedding[122] = float32(countPaths(text))               // File paths
	embedding[123] = float32(countUrls(text))                // URLs
	embedding[124] = float32(countVersions(text))            // Version numbers
	embedding[125] = float32(countEnvironments(text))        // Environment references
	embedding[126] = float32(countConfigKeys(text))          // Configuration keys
	embedding[127] = float32(len(strings.Split(text, "\n"))) // Line count

	// Normalize the embedding vector
	return normalizeVector(embedding)
}

// Helper functions for embedding creation
func countTermOccurrences(words []string, term string) int {
	count := 0
	for _, word := range words {
		if strings.Contains(word, term) {
			count++
		}
	}
	return count
}

func averageWordLength(words []string) float64 {
	if len(words) == 0 {
		return 0
	}
	totalLen := 0
	for _, word := range words {
		totalLen += len(word)
	}
	return float64(totalLen) / float64(len(words))
}

func countCodeBlocks(text string) int {
	return strings.Count(text, "```")
}

func countLinks(text string) int {
	linkPattern := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	return len(linkPattern.FindAllString(text, -1))
}

func countHeadings(text string) int {
	headingPattern := regexp.MustCompile(`(?m)^#+\s`)
	return len(headingPattern.FindAllString(text, -1))
}

func countListItems(text string) int {
	listPattern := regexp.MustCompile(`(?m)^[\s]*[-*+]\s`)
	return len(listPattern.FindAllString(text, -1))
}

func countCamelCaseWords(words []string) int {
	count := 0
	camelCasePattern := regexp.MustCompile(`^[a-z]+[A-Z][a-zA-Z]*$`)
	for _, word := range words {
		if camelCasePattern.MatchString(word) {
			count++
		}
	}
	return count
}

func countUppercaseWords(words []string) int {
	count := 0
	for _, word := range words {
		if len(word) > 1 && strings.ToUpper(word) == word && !regexp.MustCompile(`^\d+$`).MatchString(word) {
			count++
		}
	}
	return count
}

func countNumericTerms(words []string) int {
	count := 0
	numericPattern := regexp.MustCompile(`\d+`)
	for _, word := range words {
		if numericPattern.MatchString(word) {
			count++
		}
	}
	return count
}

func countCodeSnippets(text string) int {
	codePattern := regexp.MustCompile("`[^`]+`")
	return len(codePattern.FindAllString(text, -1))
}

func countCommands(text string) int {
	commandPattern := regexp.MustCompile(`(?m)^\s*\$\s+.*|(?m)^\s*sc\s+`)
	return len(commandPattern.FindAllString(text, -1))
}

func countPaths(text string) int {
	pathPattern := regexp.MustCompile(`[./][a-zA-Z0-9_/-]+\.[a-zA-Z0-9]+|/[a-zA-Z0-9_/-]+`)
	return len(pathPattern.FindAllString(text, -1))
}

func countUrls(text string) int {
	urlPattern := regexp.MustCompile(`https?://[^\s]+`)
	return len(urlPattern.FindAllString(text, -1))
}

func countVersions(text string) int {
	versionPattern := regexp.MustCompile(`v?\d+\.\d+(\.\d+)?`)
	return len(versionPattern.FindAllString(text, -1))
}

func countEnvironments(text string) int {
	envPattern := regexp.MustCompile(`(?i)(staging|production|development|dev|prod|test)`)
	return len(envPattern.FindAllString(text, -1))
}

func countConfigKeys(text string) int {
	configPattern := regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_]*:\s*`)
	return len(configPattern.FindAllString(text, -1))
}

func normalizeVector(vec []float32) []float32 {
	// Calculate magnitude
	var magnitude float64
	for _, v := range vec {
		magnitude += float64(v * v)
	}
	magnitude = math.Sqrt(magnitude)

	// Avoid division by zero
	if magnitude == 0 {
		return vec
	}

	// Normalize
	normalized := make([]float32, len(vec))
	for i, v := range vec {
		normalized[i] = float32(float64(v) / magnitude)
	}

	return normalized
}

// DB alias for backward compatibility
type DB = Database
