package embeddings

import (
	"context"
	"strings"
	"testing"

	"github.com/philippgille/chromem-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddingBasicFunctionality(t *testing.T) {
	// Create an in-memory test database
	db := chromem.NewDB()

	// Create a test collection
	collection, err := db.CreateCollection("test-docs", nil, nil)
	require.NoError(t, err)

	// Add some test documents
	testDocs := []struct {
		id       string
		content  string
		metadata map[string]string
	}{
		{
			id:      "aws-s3-doc",
			content: "AWS S3 bucket configuration with Simple Container. Use type s3-bucket to create storage resources.",
			metadata: map[string]string{
				"type":     "docs",
				"provider": "aws",
				"resource": "s3-bucket",
			},
		},
		{
			id:      "gcp-gke-doc",
			content: "Google Kubernetes Engine setup with GKE Autopilot. Configure your cluster with gcp-gke-autopilot template.",
			metadata: map[string]string{
				"type":     "docs",
				"provider": "gcp",
				"resource": "gke-autopilot",
			},
		},
		{
			id:      "postgres-doc",
			content: "PostgreSQL database configuration. Use aws-rds-postgres for AWS or gcp-cloudsql-postgres for Google Cloud.",
			metadata: map[string]string{
				"type":     "docs",
				"provider": "multi",
				"resource": "postgres",
			},
		},
	}

	ctx := context.Background()
	for _, doc := range testDocs {
		err := collection.Add(ctx, doc.id, doc.content, doc.metadata)
		require.NoError(t, err)
	}

	// Test semantic search
	t.Run("search for S3 bucket", func(t *testing.T) {
		results, err := collection.Query(ctx, "how to configure AWS storage bucket", 2, nil, nil)
		require.NoError(t, err)
		assert.True(t, len(results) > 0, "Should find relevant results")

		// The S3 document should be the most relevant
		if len(results) > 0 {
			assert.Contains(t, results[0].Content, "S3 bucket", "Most relevant result should contain S3")
		}
	})

	t.Run("search for Kubernetes", func(t *testing.T) {
		results, err := collection.Query(ctx, "kubernetes cluster setup", 2, nil, nil)
		require.NoError(t, err)
		assert.True(t, len(results) > 0, "Should find relevant results")

		// The GKE document should be most relevant
		if len(results) > 0 {
			assert.Contains(t, results[0].Content, "Kubernetes", "Most relevant result should contain Kubernetes")
		}
	})

	t.Run("search for database", func(t *testing.T) {
		results, err := collection.Query(ctx, "database configuration postgres", 1, nil, nil)
		require.NoError(t, err)
		assert.True(t, len(results) > 0, "Should find relevant results")

		// The PostgreSQL document should be most relevant
		if len(results) > 0 {
			assert.Contains(t, results[0].Content, "PostgreSQL", "Most relevant result should contain PostgreSQL")
		}
	})

	// Test collection info
	count := collection.Count()
	assert.Equal(t, 3, count, "Should have 3 documents in collection")
}

func TestSearchDocumentationFunction(t *testing.T) {
	// This test would normally use the embedded database
	// For now, we'll skip it since we don't have pre-generated embeddings
	t.Skip("Skipping test that requires pre-generated embeddings - run after 'welder run generate-embeddings'")

	db, err := LoadEmbeddedDatabase()
	if err != nil {
		t.Skipf("No embedded database found: %v", err)
	}

	results, err := SearchDocumentation(db, "AWS S3 bucket configuration", 3)
	require.NoError(t, err)
	assert.True(t, len(results) > 0, "Should find relevant documentation")

	info, err := GetCollectionInfo(db)
	require.NoError(t, err)
	assert.Contains(t, info, "collection_name")
	assert.Contains(t, info, "document_count")
}

func TestChunkingStrategy(t *testing.T) {
	// Test the chunking logic that would be used in cmd/embed-docs
	content := `# Simple Container Documentation

This is the first paragraph of documentation.

## Section 1

This is a longer paragraph that explains how to use Simple Container with AWS. It includes details about S3 buckets, RDS databases, and ECS deployments.

## Section 2

Another section with information about GCP integration. This covers GKE clusters, Cloud SQL, and other Google Cloud services.

### Subsection

Final section with examples and code snippets.`

	// Simple chunking simulation
	chunks := createSimpleChunks(content, 150) // Small chunk size for testing

	assert.True(t, len(chunks) >= 2, "Should create multiple chunks")

	// Verify chunks don't exceed size limit too much
	for i, chunk := range chunks {
		assert.True(t, len(chunk) <= 200, "Chunk %d should not be too large: %d chars", i, len(chunk))
		assert.True(t, len(chunk) > 0, "Chunk %d should not be empty", i)
	}
}

// Helper function to simulate chunking
func createSimpleChunks(content string, maxSize int) []string {
	var chunks []string
	paragraphs := strings.Split(content, "\n\n")

	currentChunk := ""
	for _, paragraph := range paragraphs {
		if strings.TrimSpace(paragraph) == "" {
			continue
		}

		if len(currentChunk)+len(paragraph) > maxSize && currentChunk != "" {
			chunks = append(chunks, strings.TrimSpace(currentChunk))
			currentChunk = paragraph
		} else {
			if currentChunk != "" {
				currentChunk += "\n\n" + paragraph
			} else {
				currentChunk = paragraph
			}
		}
	}

	if currentChunk != "" {
		chunks = append(chunks, strings.TrimSpace(currentChunk))
	}

	return chunks
}
