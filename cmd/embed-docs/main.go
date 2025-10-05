package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/philippgille/chromem-go"
	
)

type Config struct {
	DocsPath     string
	ExamplesPath string
	SchemasPath  string
	OutputPath   string
	EmbedModel   string
	ChunkSize    int
	Verbose      bool
}

type DocumentChunk struct {
	ID       string
	Content  string
	Path     string
	Type     string // "docs", "examples", "schemas"
	Metadata map[string]string
}

func main() {
	var config Config

	rootCmd := &cobra.Command{
		Use:   "embed-docs",
		Short: "Generate embeddings for Simple Container documentation",
		Long:  "Build-time tool to create vector embeddings for docs, examples, and schemas",
		RunE: func(cmd *cobra.Command, args []string) error {
			return generateEmbeddings(config)
		},
	}

	rootCmd.Flags().StringVar(&config.DocsPath, "docs-path", "./docs", "Path to documentation directory")
	rootCmd.Flags().StringVar(&config.ExamplesPath, "examples-path", "./docs/docs/examples", "Path to examples directory")
	rootCmd.Flags().StringVar(&config.SchemasPath, "schemas-path", "./docs/schemas", "Path to schemas directory")
	rootCmd.Flags().StringVar(&config.OutputPath, "output", "./pkg/assistant/embeddings/embedded_docs.go", "Output file path")
	rootCmd.Flags().StringVar(&config.EmbedModel, "embed-model", "text-embedding-3-small", "OpenAI embedding model")
	rootCmd.Flags().IntVar(&config.ChunkSize, "chunk-size", 1000, "Maximum characters per chunk")
	rootCmd.Flags().BoolVarP(&config.Verbose, "verbose", "v", false, "Verbose output")

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func generateEmbeddings(config Config) error {
	fmt.Println("🚀 Simple Container Documentation Embedding Generator")
	fmt.Printf("📖 Processing docs from: %s\n", config.DocsPath)
	fmt.Printf("📋 Processing examples from: %s\n", config.ExamplesPath)
	fmt.Printf("🔧 Processing schemas from: %s\n", config.SchemasPath)

	// Initialize chromem-go database
	db := chromem.NewDB()

	// Create collection for documentation
	collection, err := db.CreateCollection("simple-container-docs", nil, nil)
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	var allChunks []DocumentChunk

	// Process documentation files
	if err := processDirectory(config.DocsPath, "docs", config, &allChunks); err != nil {
		return fmt.Errorf("failed to process docs: %w", err)
	}

	// Process examples
	if err := processDirectory(config.ExamplesPath, "examples", config, &allChunks); err != nil {
		return fmt.Errorf("failed to process examples: %w", err)
	}

	// Process schemas
	if err := processDirectory(config.SchemasPath, "schemas", config, &allChunks); err != nil {
		return fmt.Errorf("failed to process schemas: %w", err)
	}

	fmt.Printf("📊 Total chunks processed: %d\n", len(allChunks))

	// Add documents to collection (this will generate embeddings)
	ctx := context.Background()
	for i, chunk := range allChunks {
		if config.Verbose {
			fmt.Printf("Processing chunk %d/%d: %s\n", i+1, len(allChunks), chunk.Path)
		}

		err := collection.Add(ctx, chunk.ID, chunk.Content, chunk.Metadata)
		if err != nil {
			return fmt.Errorf("failed to add chunk %s: %w", chunk.ID, err)
		}

		// Rate limiting for API calls
		if i%10 == 0 && i > 0 {
			fmt.Printf("Processed %d/%d chunks...\n", i, len(allChunks))
			time.Sleep(100 * time.Millisecond) // Avoid rate limits
		}
	}

	// Export the database to a file
	fmt.Printf("💾 Exporting embeddings to: %s\n", config.OutputPath)
	if err := exportEmbeddings(db, config.OutputPath); err != nil {
		return fmt.Errorf("failed to export embeddings: %w", err)
	}

	fmt.Println("✅ Documentation embedding generation complete!")
	return nil
}

func processDirectory(dirPath, docType string, config Config, chunks *[]DocumentChunk) error {
	return filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		// Process markdown, YAML, and JSON files
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".yaml" && ext != ".yml" && ext != ".json" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}

		// Create chunks from content
		fileChunks := createChunks(string(content), path, docType, config.ChunkSize)
		*chunks = append(*chunks, fileChunks...)

		if config.Verbose {
			fmt.Printf("Processed %s: %d chunks\n", path, len(fileChunks))
		}

		return nil
	})
}

func createChunks(content, path, docType string, maxSize int) []DocumentChunk {
	var chunks []DocumentChunk

	// Simple chunking strategy: split by paragraphs, then by size
	paragraphs := strings.Split(content, "\n\n")

	currentChunk := ""
	chunkIndex := 0

	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}

		// If adding this paragraph would exceed max size, create a chunk
		if len(currentChunk)+len(paragraph) > maxSize && currentChunk != "" {
			chunk := DocumentChunk{
				ID:      fmt.Sprintf("%s_chunk_%d", filepath.Base(path), chunkIndex),
				Content: strings.TrimSpace(currentChunk),
				Path:    path,
				Type:    docType,
				Metadata: map[string]string{
					"path":      path,
					"type":      docType,
					"chunk_id":  fmt.Sprintf("%d", chunkIndex),
					"file_name": filepath.Base(path),
					"file_ext":  filepath.Ext(path),
				},
			}
			chunks = append(chunks, chunk)

			currentChunk = paragraph
			chunkIndex++
		} else {
			if currentChunk != "" {
				currentChunk += "\n\n" + paragraph
			} else {
				currentChunk = paragraph
			}
		}
	}

	// Add final chunk if there's content
	if currentChunk != "" {
		chunk := DocumentChunk{
			ID:      fmt.Sprintf("%s_chunk_%d", filepath.Base(path), chunkIndex),
			Content: strings.TrimSpace(currentChunk),
			Path:    path,
			Type:    docType,
			Metadata: map[string]string{
				"path":      path,
				"type":      docType,
				"chunk_id":  fmt.Sprintf("%d", chunkIndex),
				"file_name": filepath.Base(path),
				"file_ext":  filepath.Ext(path),
			},
		}
		chunks = append(chunks, chunk)
	}

	return chunks
}

func exportEmbeddings(db *chromem.DB, outputPath string) error {
	// Create output directory if it doesn't exist
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Export to gob file (chromem-go's native format)
	gobPath := strings.Replace(outputPath, ".go", ".gob", 1)
	file, err := os.Create(gobPath)
	if err != nil {
		return fmt.Errorf("failed to create gob file: %w", err)
	}
	defer file.Close()

	if err := db.Export(file, false); err != nil {
		return fmt.Errorf("failed to export database: %w", err)
	}

	// Generate Go code that embeds the gob file
	return generateGoEmbeddingFile(outputPath, gobPath)
}

func generateGoEmbeddingFile(goPath, gobPath string) error {
	gobFile := filepath.Base(gobPath)

	goCode := fmt.Sprintf(`// Code generated by embed-docs tool. DO NOT EDIT.
package embeddings

import (
	_ "embed"
	"bytes"
	"io"
	
	"github.com/philippgille/chromem-go"
)

//go:embed %s
var embeddedDocsData []byte

// LoadEmbeddedDatabase loads the pre-generated documentation embeddings
func LoadEmbeddedDatabase() (*chromem.DB, error) {
	db := chromem.NewDB()
	reader := bytes.NewReader(embeddedDocsData)
	
	if err := db.Import(reader, false); err != nil {
		return nil, err
	}
	
	return db, nil
}

// SearchDocumentation performs semantic search on the embedded documentation
func SearchDocumentation(db *chromem.DB, query string, limit int) ([]chromem.Result, error) {
	collection, err := db.GetCollection("simple-container-docs", nil)
	if err != nil {
		return nil, err
	}
	
	results, err := collection.Query(nil, query, limit, nil, nil)
	if err != nil {
		return nil, err
	}
	
	return results, nil
}

// GetCollectionInfo returns information about the embedded documentation collection
func GetCollectionInfo(db *chromem.DB) (map[string]interface{}, error) {
	collection, err := db.GetCollection("simple-container-docs", nil)
	if err != nil {
		return nil, err
	}
	
	count := collection.Count()
	
	return map[string]interface{}{
		"collection_name": "simple-container-docs",
		"document_count": count,
		"embedded_size_bytes": len(embeddedDocsData),
	}, nil
}
`, gobFile)

	return os.WriteFile(goPath, []byte(goCode), 0644)
}
