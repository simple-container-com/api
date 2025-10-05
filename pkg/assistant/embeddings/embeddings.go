package embeddings

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strings"

	chromem "github.com/philippgille/chromem-go"

	"github.com/simple-container-com/api/docs"
	"github.com/simple-container-com/api/pkg/api/logger"
)

//go:embed vectors/prebuilt_embeddings.json
var embeddedVectors []byte

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
	Title      string                 `json:"title"`
}

// EmbeddedDocument represents a pre-embedded document
type EmbeddedDocument struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata"`
	Embedding []float32              `json:"embedding"`
}

// PrebuiltEmbeddings represents the embedded vectors data
type PrebuiltEmbeddings struct {
	Version   string             `json:"version"`
	Documents []EmbeddedDocument `json:"documents"`
}

// LoadEmbeddedDatabase loads the pre-built documentation database from embedded data
func LoadEmbeddedDatabase(ctx context.Context) (*Database, error) {
	log := logger.New()
	// Initialize chromem-go database
	db := chromem.NewDB()

	// Create a simple local embedding function (for new documents if needed)
	embeddingFunc := func(ctx context.Context, text string) ([]float32, error) {
		return createSimpleEmbedding(text), nil
	}

	// Create documentation collection with local embedding function
	collection, err := db.GetOrCreateCollection("simple-container-docs", nil, embeddingFunc)
	if err != nil {
		return nil, fmt.Errorf("failed to create collection: %w", err)
	}

	database := &Database{
		collection: collection,
		db:         db,
	}

	// Load pre-built embeddings if available, otherwise build from embedded docs
	if err := database.loadPrebuiltEmbeddings(ctx, log); err != nil {
		// This is expected when no pre-built vectors exist - only log in debug mode
		log.Debug(ctx, "Failed to load pre-built embeddings, building from embedded docs: %v", err)
	}

	// If we have no documents (either pre-built failed or was empty), load from embedded docs
	if database.Count() == 0 {
		if err := database.loadFromEmbeddedDocs(ctx, log); err != nil {
			log.Error(ctx, "Failed to initialize documentation database: %v", err)
			return database, nil // Return empty database instead of failing
		}
		// Only show count in debug mode - users don't need to see internal details
		log.Debug(ctx, "Initialized documentation database with %d documents", database.Count())
	} else {
		log.Debug(ctx, "Successfully loaded pre-built embeddings with %d documents", database.Count())
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

		// Extract title for easy access
		title := result.ID
		if titleVal, ok := metadata["title"]; ok {
			if titleStr, ok := titleVal.(string); ok {
				title = titleStr
			}
		}

		searchResults[i] = SearchResult{
			ID:         result.ID,
			Content:    result.Content,
			Score:      float64(result.Similarity),
			Similarity: float64(result.Similarity),
			Metadata:   metadata,
			Title:      title,
		}
	}

	return searchResults, nil
}

// loadPrebuiltEmbeddings loads pre-built embeddings from embedded data
func (db *Database) loadPrebuiltEmbeddings(ctx context.Context, log logger.Logger) error {
	if len(embeddedVectors) == 0 {
		return fmt.Errorf("no embedded vectors data available")
	}

	var prebuilt PrebuiltEmbeddings
	if err := json.Unmarshal(embeddedVectors, &prebuilt); err != nil {
		return fmt.Errorf("failed to unmarshal embedded vectors: %w", err)
	}

	// Add each pre-built document to the collection
	for _, doc := range prebuilt.Documents {
		// Convert metadata to string map for chromem
		stringMetadata := make(map[string]string)
		for k, v := range doc.Metadata {
			if str, ok := v.(string); ok {
				stringMetadata[k] = str
			} else {
				stringMetadata[k] = fmt.Sprintf("%v", v)
			}
		}

		// Add document with pre-computed embedding
		err := db.collection.AddDocument(context.Background(), chromem.Document{
			ID:        doc.ID,
			Content:   doc.Content,
			Metadata:  stringMetadata,
			Embedding: doc.Embedding,
		})
		if err != nil {
			log.Warn(ctx, "Failed to add pre-built document %s: %v", doc.ID, err)
		}
	}

	log.Debug(ctx, "Loaded %d pre-built embeddings", len(prebuilt.Documents))
	return nil
}

// loadFromEmbeddedDocs builds embeddings from embedded markdown files
func (db *Database) loadFromEmbeddedDocs(ctx context.Context, log logger.Logger) error {
	log.Debug(ctx, "Starting to load documents from embedded docs...")
	// Walk through embedded documentation files
	docCount := 0
	err := db.walkEmbeddedDocs(ctx, log, "docs", func(path string, content []byte) error {
		// Skip non-markdown files
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}

		// Create relative path for ID
		id := strings.TrimPrefix(path, "docs/")
		id = strings.ReplaceAll(id, "\\", "/") // Normalize path separators

		// Extract title from first heading or use filename
		title := extractTitleFromMarkdown(string(content))
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(path), ".md")
		}

		// Create metadata
		metadata := map[string]string{
			"title": title,
			"path":  id,
			"type":  "documentation",
		}

		// Add document to collection (embedding will be computed automatically)
		err := db.collection.AddDocument(context.Background(), chromem.Document{
			ID:       id,
			Content:  string(content),
			Metadata: metadata,
		})
		if err != nil {
			log.Warn(ctx, "Failed to add document %s: %v", id, err)
		} else {
			docCount++
			log.Debug(ctx, "Added document: %s (%s)", id, title)
		}

		return nil
	})

	log.Debug(ctx, "Successfully loaded %d documents from embedded docs", docCount)
	return err
}

// walkEmbeddedDocs walks through embedded documentation files
func (db *Database) walkEmbeddedDocs(ctx context.Context, log logger.Logger, root string, fn func(path string, content []byte) error) error {
	if log != nil {
		log.Debug(ctx, "Walking embedded docs starting from root: %s", root)
	}
	entries, err := docs.EmbeddedDocs.ReadDir(root)
	if err != nil {
		if log != nil {
			log.Error(ctx, "Error reading embedded docs dir %s: %v", root, err)
		}
		return err
	}
	if log != nil {
		log.Debug(ctx, "Found %d entries in %s", len(entries), root)
	}

	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())

		if entry.IsDir() {
			// Recursively walk subdirectories
			if err := db.walkEmbeddedDocs(ctx, log, path, fn); err != nil {
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

// Count returns the number of documents in the database
func (db *Database) Count() int {
	if db == nil || db.collection == nil {
		return 0
	}
	return db.collection.Count()
}

// DB alias for backward compatibility
type DB = Database
