package embeddings

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	chromem "github.com/philippgille/chromem-go"
	langchainembeddings "github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms/openai"

	"github.com/simple-container-com/api/docs"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/assistant/config"
)

//go:embed vectors
var embeddedVectors embed.FS

// EmbeddingType represents the type of embeddings used
type EmbeddingType string

const (
	EmbeddingTypeOpenAI EmbeddingType = "openai"
	EmbeddingTypeLocal  EmbeddingType = "local"
)

// Database represents an embedded vector database using chromem-go
type Database struct {
	collection    *chromem.Collection
	db            *chromem.DB
	embeddingType EmbeddingType
	dimensions    int
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

	// Determine which embeddings to use and create appropriate database
	embeddingType, dimensions := detectAvailableEmbeddings(ctx, log)

	// Initialize chromem-go database
	db := chromem.NewDB()

	// Create embedding function matching the detected type
	var embeddingFunc func(context.Context, string) ([]float32, error)
	var collectionName string

	switch embeddingType {
	case EmbeddingTypeOpenAI:
		// For OpenAI embeddings, use OpenAI API for queries if available, otherwise use enhanced local
		embeddingFunc = func(ctx context.Context, text string) ([]float32, error) {
			// Try to use OpenAI for query embedding to match document embeddings
			if apiKey := getOpenAIAPIKey(); apiKey != "" {
				return generateOpenAIQueryEmbedding(text, apiKey, "text-embedding-3-small")
			}
			// Fallback: use enhanced local embedding expanded to match dimensions
			localEmbed := createEnhancedLocalEmbedding(text)
			return expandLocalEmbeddingToOpenAI(localEmbed, dimensions), nil
		}
		collectionName = "simple-container-docs-openai"
		log.Debug(ctx, "Using OpenAI embeddings (%dD) with smart query embedding", dimensions)
	case EmbeddingTypeLocal:
		embeddingFunc = func(ctx context.Context, text string) ([]float32, error) {
			return createEnhancedLocalEmbedding(text), nil
		}
		collectionName = "simple-container-docs-local"
		log.Debug(ctx, "Using local embeddings (%dD)", dimensions)
	}

	// Create documentation collection with appropriate embedding function
	collection, err := db.GetOrCreateCollection(collectionName, nil, embeddingFunc)
	if err != nil {
		return nil, fmt.Errorf("failed to create collection: %w", err)
	}

	database := &Database{
		collection:    collection,
		db:            db,
		embeddingType: embeddingType,
		dimensions:    dimensions,
	}

	// Load pre-built embeddings based on detected type
	if err := database.loadPrebuiltEmbeddings(ctx, log); err != nil {
		log.Debug(ctx, "Failed to load pre-built embeddings, building from embedded docs: %v", err)
	}

	// If we have no documents, load from embedded docs using appropriate method
	if database.Count() == 0 {
		if embeddingType == EmbeddingTypeLocal {
			if err := database.loadFromEmbeddedDocs(ctx, log); err != nil {
				log.Error(ctx, "Failed to initialize documentation database: %v", err)
				return database, nil
			}
			log.Debug(ctx, "Initialized documentation database with %d documents using local embeddings", database.Count())
		} else {
			log.Debug(ctx, "No OpenAI embeddings available and cannot generate them at runtime")
			// Return empty database - will still work for other features
			return database, nil
		}
	} else {
		log.Debug(ctx, "Successfully loaded %s pre-built embeddings with %d documents", embeddingType, database.Count())
	}

	return database, nil
}

// SearchDocumentation searches the documentation using semantic search
func SearchDocumentation(db *Database, query string, limit int) ([]SearchResult, error) {
	if db == nil || db.collection == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Perform similarity search using chromem-go (get more results for better filtering)
	results, err := db.collection.Query(context.Background(), query, limit*2, nil, nil)
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

		// Calculate enhanced similarity score with term-specific boosting
		baseSimilarity := float64(result.Similarity)
		enhancedScore := calculateEnhancedScore(query, result.Content, baseSimilarity)

		searchResults[i] = SearchResult{
			ID:         result.ID,
			Content:    result.Content,
			Score:      enhancedScore,
			Similarity: enhancedScore,
			Metadata:   metadata,
			Title:      title,
		}
	}

	// Sort by enhanced score and return top results
	sort.Slice(searchResults, func(i, j int) bool {
		return searchResults[i].Similarity > searchResults[j].Similarity
	})

	// Return only requested limit
	if len(searchResults) > limit {
		searchResults = searchResults[:limit]
	}

	return searchResults, nil
}

// calculateEnhancedScore boosts similarity scores for specific term matches
func calculateEnhancedScore(query, content string, baseSimilarity float64) float64 {
	queryLower := strings.ToLower(query)
	contentLower := strings.ToLower(content)

	boostFactor := 1.0

	// Strong boost for exact compound term matches (most important for Simple Container)
	compoundTerms := map[string]float64{
		"cloud-compose":   1.5, // Highest boost for this critical deployment type
		"single-image":    1.4,
		"multi-container": 1.3,
		"docker-compose":  1.2,
	}

	for term, boost := range compoundTerms {
		if strings.Contains(queryLower, term) && strings.Contains(contentLower, term) {
			boostFactor *= boost
		}
	}

	// Boost for configuration-related content with deployment types
	if strings.Contains(queryLower, "configuration") || strings.Contains(queryLower, "deployment") {
		if strings.Contains(contentLower, "type:") && strings.Contains(contentLower, "stack") {
			boostFactor *= 1.3
		}
	}

	// Boost for YAML examples that show actual configurations
	if strings.Contains(contentLower, "```yaml") || strings.Contains(contentLower, "```yml") {
		boostFactor *= 1.2
	}

	// Ensure we don't exceed reasonable similarity bounds
	enhancedScore := baseSimilarity * boostFactor
	if enhancedScore > 1.0 {
		enhancedScore = 1.0
	}

	return enhancedScore
}

// loadPrebuiltEmbeddings loads pre-built embeddings from embedded data
func (db *Database) loadPrebuiltEmbeddings(ctx context.Context, log logger.Logger) error {
	// Determine which embeddings file to use based on the database type
	var filename string
	switch db.embeddingType {
	case EmbeddingTypeOpenAI:
		filename = "vectors/prebuilt_embeddings_openai.json"
	case EmbeddingTypeLocal:
		filename = "vectors/prebuilt_embeddings_local.json"
	default:
		// Fallback to original filename for backward compatibility
		filename = "vectors/prebuilt_embeddings.json"
	}

	// Try to read the prebuilt embeddings file
	data, err := embeddedVectors.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("no embedded vectors data available for %s: %w", db.embeddingType, err)
	}

	var prebuilt PrebuiltEmbeddings
	if err := json.Unmarshal(data, &prebuilt); err != nil {
		return fmt.Errorf("failed to unmarshal embedded vectors: %w", err)
	}

	// Add each pre-built document to the collection
	successCount := 0
	failCount := 0

	for i, doc := range prebuilt.Documents {
		// Convert metadata to string map for chromem
		stringMetadata := make(map[string]string)
		for k, v := range doc.Metadata {
			if str, ok := v.(string); ok {
				stringMetadata[k] = str
			} else {
				stringMetadata[k] = fmt.Sprintf("%v", v)
			}
		}

		// Validate embedding dimensions
		if len(doc.Embedding) == 0 {
			log.Warn(ctx, "Document %s has empty embedding, skipping", doc.ID)
			failCount++
			continue
		}

		// Add document with pre-computed embedding
		err := db.collection.AddDocument(context.Background(), chromem.Document{
			ID:        doc.ID,
			Content:   doc.Content,
			Metadata:  stringMetadata,
			Embedding: doc.Embedding,
		})
		if err != nil {
			log.Error(ctx, "CRITICAL: Failed to add pre-built document %s (doc %d/%d): %v", doc.ID, i+1, len(prebuilt.Documents), err)
			failCount++
		} else {
			successCount++
			if i < 10 || (i+1)%20 == 0 || i+1 == len(prebuilt.Documents) {
				log.Debug(ctx, "Successfully added document %d/%d: %s (%d dims)", i+1, len(prebuilt.Documents), doc.ID, len(doc.Embedding))
			}
		}
	}

	if failCount > 0 {
		log.Error(ctx, "EMBEDDING LOAD ISSUE: %d documents failed to load, %d succeeded", failCount, successCount)
	}
	log.Debug(ctx, "Embedding load complete: %d successful, %d failed, collection size: %d", successCount, failCount, db.collection.Count())
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

// createEnhancedLocalEmbedding creates a high-quality local embedding using vocabulary analysis
func createEnhancedLocalEmbedding(text string) []float32 {
	return createVocabularyBasedEmbedding(text, 512) // 512-dimensional local embeddings
}

// createSimpleEmbedding creates a basic embedding vector based on text characteristics
// This is a simple local implementation that doesn't require external APIs
func createSimpleEmbedding(text string) []float32 {
	// Keep original text for compound term matching
	originalText := strings.ToLower(text)

	// Normalize and clean text for word-based analysis
	cleanText := regexp.MustCompile(`[^\w\s-]`).ReplaceAllString(originalText, " ") // Keep hyphens for compound terms
	words := strings.Fields(cleanText)

	// Create a feature vector based on text characteristics
	embedding := make([]float32, 128) // 128-dimensional vector

	if len(words) == 0 {
		return embedding
	}

	// Feature 1-10: Common Simple Container terms
	scTerms := []string{"docker", "kubernetes", "aws", "gcp", "postgres", "redis", "mongo", "yaml", "stack", "compose"}
	for i, term := range scTerms {
		if i < 10 {
			embedding[i] = float32(countTermOccurrences(words, term))
		}
	}

	// Feature 11-20: Deployment types and technical concepts
	// Use full text search for compound terms
	compoundTerms := []string{"cloud-compose", "single-image"}
	regularTerms := []string{"deployment", "configuration", "static", "database", "api", "server", "client", "template"}

	for i, term := range compoundTerms {
		if i < 10 {
			embedding[10+i] = float32(countTermInText(originalText, term))
		}
	}
	for i, term := range regularTerms {
		if i+2 < 10 { // Offset by 2 since we used first 2 slots for compound terms
			embedding[10+i+2] = float32(countTermOccurrences(words, term))
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

// countTermInText counts occurrences of a term in the full text (handles compound terms like "cloud-compose")
func countTermInText(text, term string) int {
	text = strings.ToLower(text)
	term = strings.ToLower(term)
	return strings.Count(text, term)
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

// CreateTestEmbedding exposes the embedding function for testing
func CreateTestEmbedding(text string) []float32 {
	return createSimpleEmbedding(text)
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

// createVocabularyBasedEmbedding creates enhanced local embeddings using vocabulary analysis
func createVocabularyBasedEmbedding(text string, dimensions int) []float32 {
	// Enhanced vocabulary-based embedding generation
	words := extractWords(text)
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

	// Base features with better discrimination (first 20 dimensions)
	docType := strings.ToLower(text)

	// Document-specific discriminative features (VERY high weights for key terms)
	if dimensions > 0 {
		gkeAutopilot := float32(strings.Count(docType, "gke") + strings.Count(docType, "autopilot"))
		if gkeAutopilot > 0 {
			embedding[0] = gkeAutopilot / float32(totalWords+1) * 5000.0 // Much higher weight
		}
	}
	if dimensions > 1 {
		lambdaFunc := float32(strings.Count(docType, "lambda") + strings.Count(docType, "function"))
		if lambdaFunc > 0 {
			embedding[1] = lambdaFunc / float32(totalWords+1) * 5000.0
		}
	}
	if dimensions > 2 {
		staticWeb := float32(strings.Count(docType, "static") + strings.Count(docType, "website"))
		if staticWeb > 0 {
			embedding[2] = staticWeb / float32(totalWords+1) * 5000.0
		}
	}
	if dimensions > 3 {
		dockerComp := float32(strings.Count(docType, "docker") + strings.Count(docType, "compose"))
		if dockerComp > 0 {
			embedding[3] = dockerComp / float32(totalWords+1) * 5000.0
		}
	}
	if dimensions > 4 {
		mongoDb := float32(strings.Count(docType, "mongodb") + strings.Count(docType, "database"))
		if mongoDb > 0 {
			embedding[4] = mongoDb / float32(totalWords+1) * 5000.0
		}
	}
	if dimensions > 5 {
		comprSetup := float32(strings.Count(docType, "comprehensive") + strings.Count(docType, "setup"))
		if comprSetup > 0 {
			embedding[5] = comprSetup / float32(totalWords+1) * 5000.0
		}
	}
	if dimensions > 6 {
		billing := float32(strings.Count(docType, "billing") + strings.Count(docType, "payment"))
		if billing > 0 {
			embedding[6] = billing / float32(totalWords+1) * 5000.0
		}
	}
	if dimensions > 7 {
		ecsFargate := float32(strings.Count(docType, "ecs") + strings.Count(docType, "fargate"))
		if ecsFargate > 0 {
			embedding[7] = ecsFargate / float32(totalWords+1) * 5000.0
		}
	}
	if dimensions > 8 {
		serverYaml := float32(strings.Count(docType, "server.yaml") + strings.Count(docType, "parent"))
		if serverYaml > 0 {
			embedding[8] = serverYaml / float32(totalWords+1) * 5000.0
		}
	}
	if dimensions > 9 {
		clientYaml := float32(strings.Count(docType, "client.yaml") + strings.Count(docType, "stack"))
		if clientYaml > 0 {
			embedding[9] = clientYaml / float32(totalWords+1) * 5000.0
		}
	}

	// Statistical features (dimensions 10-15)
	if dimensions > 10 {
		embedding[10] = float32(totalWords) / 5000.0 // Document length, better scaled
	}
	if dimensions > 11 {
		embedding[11] = float32(uniqueWords) / 2500.0 // Vocabulary size, better scaled
	}
	if dimensions > 12 {
		embedding[12] = float32(uniqueWords) / float32(totalWords) // vocabulary richness
	}
	if dimensions > 13 {
		embedding[13] = averageWordLengthFloat(words) / 25.0 // Average word length
	}
	if dimensions > 14 {
		// Document structure features
		embedding[14] = float32(strings.Count(text, "##")+strings.Count(text, "###")) / float32(totalWords+1) * 100.0
	}
	if dimensions > 15 {
		// Code block density
		embedding[15] = float32(strings.Count(text, "```")) / float32(totalWords+1) * 100.0
	}

	// Technical term detection with better weighting (dimensions 16-80)
	technicalTerms := []string{
		// Core containerization terms
		"docker", "container", "kubernetes", "k8s", "pod", "deployment", "service",
		"ingress", "namespace", "volume", "configmap", "secret",

		// Cloud providers
		"aws", "gcp", "azure", "cloud", "ec2", "s3", "rds", "lambda", "fargate", "ecs", "eks", "gke",
		"compute", "storage", "network", "vpc", "subnet", "firewall", "gateway",

		// Simple Container specific
		"simple-container", "client.yaml", "server.yaml", "welder", "template", "resource", "stack",
		"provisioner", "pulumi", "terraform", "parent", "environment", "staging", "production",

		// Configuration and data
		"yaml", "json", "config", "configuration", "metadata", "schema", "api", "endpoint",
		"database", "postgres", "mysql", "mongodb", "redis", "elasticsearch", "kafka",

		// Infrastructure tools
		"nginx", "apache", "caddy", "traefik", "load", "balancer", "proxy", "cdn",
		"ansible", "helm", "kustomize", "gitops", "ci", "cd", "pipeline",

		// Monitoring and security
		"monitoring", "logging", "prometheus", "grafana", "jaeger", "zipkin", "alert",
		"security", "tls", "ssl", "oauth", "jwt", "rbac", "policy", "auth",
	}

	for i, term := range technicalTerms {
		if i+16 >= dimensions {
			break
		}
		// Use better TF-IDF style weighting with square root normalization
		termFreq := float32(countWordOccurrences(words, term))
		if termFreq > 0 {
			embedding[i+16] = float32(math.Sqrt(float64(termFreq))) / float32(totalWords) * 10.0
		}
	}

	// Language and framework detection (dimensions 80-150)
	langFrameworks := []string{
		"javascript", "typescript", "nodejs", "npm", "yarn", "react", "vue", "angular",
		"python", "pip", "django", "flask", "fastapi", "pandas", "numpy", "pytorch",
		"java", "maven", "gradle", "spring", "hibernate", "junit", "scala", "kotlin",
		"go", "golang", "gin", "echo", "fiber", "gorm", "cobra", "viper",
		"rust", "cargo", "tokio", "serde", "diesel", "actix", "warp", "rocket",
		"php", "composer", "laravel", "symfony", "wordpress", "magento", "drupal",
		"ruby", "rails", "sinatra", "rspec", "bundler", "rake", "sidekiq",
	}

	startIdx := 80
	for i, term := range langFrameworks {
		if startIdx+i >= dimensions {
			break
		}
		termFreq := float32(countWordOccurrences(words, term))
		if termFreq > 0 {
			embedding[startIdx+i] = float32(math.Sqrt(float64(termFreq))) / float32(totalWords) * 8.0
		}
	}

	// Infrastructure and cloud terms (dimensions 150-250)
	infraTerms := []string{
		"server", "cluster", "node", "pod", "namespace", "volume", "storage",
		"network", "subnet", "vpc", "firewall", "gateway", "proxy", "cdn",
		"lambda", "function", "serverless", "fargate", "ecs", "eks", "gke", "autopilot",
		"s3", "bucket", "rds", "dynamodb", "cloudformation", "cloudwatch", "artifact", "registry",
		"instance", "vm", "container", "image", "docker-compose", "dockerfile",
		"backup", "snapshot", "restore", "migration", "scaling", "autoscaling", "replicas",
		"availability", "zone", "region", "latency", "throughput", "performance",
		"comprehensive", "setup", "example", "configuration", "deployment", "guide",
		"pubsub", "topic", "subscription", "kms", "encryption", "bucket-state",
		"caddy", "reverse-proxy", "dns", "cloudflare", "mongodb-atlas", "redis-cache",
	}

	startIdx = 150
	for i, term := range infraTerms {
		if startIdx+i >= dimensions {
			break
		}
		termFreq := float32(countWordOccurrences(words, term))
		if termFreq > 0 {
			embedding[startIdx+i] = float32(math.Sqrt(float64(termFreq))) / float32(totalWords) * 12.0
		}
	}

	// Configuration and deployment terms (dimensions 250-350)
	configTerms := []string{
		"environment", "staging", "production", "development", "test", "dev", "prod",
		"configuration", "config", "env", "variable", "secret", "key", "value",
		"port", "host", "url", "endpoint", "path", "route", "domain", "subdomain",
		"version", "tag", "branch", "commit", "release", "deploy", "rollback",
		"health", "check", "status", "ready", "live", "probe", "metric", "log",
	}

	startIdx = 250
	for i, term := range configTerms {
		if startIdx+i >= dimensions {
			break
		}
		termFreq := float32(countWordOccurrences(words, term))
		if termFreq > 0 {
			embedding[startIdx+i] = float32(math.Sqrt(float64(termFreq))) / float32(totalWords) * 6.0
		}
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
				hash := simpleStringHash(words[i%len(words)])
				embedding[i] = float32(hash%1000) / 1000.0
			} else {
				// Fill with derived features
				embedding[i] = embedding[i%300] * 0.1
			}
		}
	}

	return normalizeVector(embedding)
}

// detectAvailableEmbeddings determines which type of embeddings are available
func detectAvailableEmbeddings(ctx context.Context, log logger.Logger) (EmbeddingType, int) {
	log.Debug(ctx, "Checking for available embedding types...")

	// Check for OpenAI embeddings first (preferred)
	if _, err := embeddedVectors.ReadFile("vectors/prebuilt_embeddings_openai.json"); err == nil {
		log.Debug(ctx, "Found prebuilt_embeddings_openai.json file")
		// Try to determine dimensions from the first embedding
		data, err := embeddedVectors.ReadFile("vectors/prebuilt_embeddings_openai.json")
		if err == nil {
			var prebuilt PrebuiltEmbeddings
			if err := json.Unmarshal(data, &prebuilt); err == nil && len(prebuilt.Documents) > 0 {
				dimensions := len(prebuilt.Documents[0].Embedding)
				log.Debug(ctx, "Successfully detected OpenAI embeddings with %d documents, %d dimensions", len(prebuilt.Documents), dimensions)
				return EmbeddingTypeOpenAI, dimensions
			} else {
				log.Debug(ctx, "Failed to parse OpenAI embeddings: unmarshal error or no documents")
			}
		} else {
			log.Debug(ctx, "Failed to read OpenAI embeddings file: %v", err)
		}
	} else {
		log.Debug(ctx, "No OpenAI embeddings file found: %v", err)
	}

	// Check for local embeddings
	if _, err := embeddedVectors.ReadFile("vectors/prebuilt_embeddings_local.json"); err == nil {
		log.Debug(ctx, "Found prebuilt_embeddings_local.json file")
		// Try to determine dimensions from the first embedding
		data, err := embeddedVectors.ReadFile("vectors/prebuilt_embeddings_local.json")
		if err == nil {
			var prebuilt PrebuiltEmbeddings
			if err := json.Unmarshal(data, &prebuilt); err == nil && len(prebuilt.Documents) > 0 {
				dimensions := len(prebuilt.Documents[0].Embedding)
				log.Debug(ctx, "Successfully detected local embeddings with %d documents, %d dimensions", len(prebuilt.Documents), dimensions)
				return EmbeddingTypeLocal, dimensions
			} else {
				log.Debug(ctx, "Failed to parse local embeddings: unmarshal error or no documents")
			}
		} else {
			log.Debug(ctx, "Failed to read local embeddings file: %v", err)
		}
	} else {
		log.Debug(ctx, "No local embeddings file found: %v", err)
	}

	// Fallback: use local embeddings with our enhanced algorithm
	log.Debug(ctx, "No pre-built embeddings found, will generate local embeddings at runtime")
	return EmbeddingTypeLocal, 512 // Default dimensions for enhanced local embeddings
}

// Helper functions for vocabulary-based embedding
func extractWords(text string) []string {
	// Convert to lowercase and extract words
	text = strings.ToLower(text)
	// Keep hyphens and underscores as part of words (important for technical terms)
	re := regexp.MustCompile(`[a-z0-9_-]+`)
	return re.FindAllString(text, -1)
}

func countWordOccurrences(words []string, target string) int {
	count := 0
	for _, word := range words {
		if strings.Contains(word, target) {
			count++
		}
	}
	return count
}

func averageWordLengthFloat(words []string) float32 {
	if len(words) == 0 {
		return 0
	}
	totalLen := 0
	for _, word := range words {
		totalLen += len(word)
	}
	return float32(totalLen) / float32(len(words))
}

func simpleStringHash(s string) int {
	hash := 0
	for _, char := range s {
		hash = hash*31 + int(char)
	}
	if hash < 0 {
		hash = -hash
	}
	return hash
}

// getOpenAIAPIKey retrieves OpenAI API key with proper priority order for runtime usage
func getOpenAIAPIKey() string {
	// 1. First priority: User's assistant configuration (~/.sc/assistant-config.json)
	if cfg, err := config.Load(); err == nil {
		if apiKey := cfg.GetOpenAIAPIKey(); apiKey != "" {
			return apiKey
		}
	}

	// 2. Second priority: Environment variable (for development/testing)
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		return apiKey
	}

	// 3. No API key found
	return ""
}

// generateOpenAIQueryEmbedding generates an embedding using OpenAI API via langchain-go
func generateOpenAIQueryEmbedding(text, apiKey, model string) ([]float32, error) {
	// Create OpenAI LLM client
	llm, err := openai.New(
		openai.WithToken(apiKey),
		openai.WithEmbeddingModel(model),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI LLM: %w", err)
	}

	// Create embedder using the LLM client (which implements EmbedderClient)
	embedder, err := langchainembeddings.NewEmbedder(llm)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedder: %w", err)
	}

	// Generate embedding for the query text
	vectors, err := embedder.EmbedQuery(context.Background(), text)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	return vectors, nil
}

// expandLocalEmbeddingToOpenAI expands a local embedding to match OpenAI dimensions with better semantic preservation
func expandLocalEmbeddingToOpenAI(localEmbed []float32, targetDims int) []float32 {
	if len(localEmbed) == 0 {
		return make([]float32, targetDims)
	}

	expanded := make([]float32, targetDims)
	localLen := len(localEmbed)

	// Strategy: Use sophisticated interpolation and feature mapping to better match OpenAI's embedding space

	// 1. Core features - direct mapping of most important dimensions
	coreFeatures := localLen
	if coreFeatures > 512 {
		coreFeatures = 512 // Limit to reasonable size
	}

	// Map local features to dispersed positions in OpenAI space (not consecutive)
	for i := 0; i < coreFeatures && i < targetDims; i++ {
		// Spread local features across the OpenAI embedding space
		targetIdx := (i * targetDims) / coreFeatures
		if targetIdx < targetDims {
			expanded[targetIdx] = localEmbed[i]
		}
	}

	// 2. Generate harmonic features - create relationships between dimensions
	for i := 0; i < localLen && i+512 < targetDims; i++ {
		expanded[i+512] = localEmbed[i] * 0.7 // Harmonic of original features
	}

	// 3. Statistical features - capture global properties
	if targetDims > 1024 {
		var mean, variance float32
		for _, val := range localEmbed {
			mean += val
		}
		mean /= float32(localLen)

		for _, val := range localEmbed {
			variance += (val - mean) * (val - mean)
		}
		variance /= float32(localLen)

		expanded[1024] = mean
		expanded[1025] = variance
		if targetDims > 1026 {
			expanded[1026] = float32(localLen) / 512.0 // Embedding richness indicator
		}
	}

	// 4. Fill remaining with intelligent interpolation
	for i := 0; i < targetDims; i++ {
		if expanded[i] == 0 { // Only fill empty positions
			// Use weighted combination of nearby features
			sourceIdx := (i * localLen) / targetDims
			weight := float32(i%17) / 17.0 // Varied weights

			if sourceIdx < localLen {
				expanded[i] = localEmbed[sourceIdx] * weight
			} else if sourceIdx-1 < localLen && sourceIdx+1 < localLen {
				// Interpolate between neighbors
				expanded[i] = (localEmbed[sourceIdx-1] + localEmbed[(sourceIdx+1)%localLen]) * 0.5 * weight
			}
		}
	}

	return normalizeVector(expanded)
}

// DB alias for backward compatibility
type DB = Database
