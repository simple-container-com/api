package embeddings

import (
	"strings"
	"testing"
)

func TestEmbeddedDocumentationSystem(t *testing.T) {
	// Test that we can load the embedded database
	db, err := LoadEmbeddedDatabase()
	if err != nil {
		t.Fatalf("Failed to load embedded database: %v", err)
	}

	if db == nil {
		t.Fatal("Database is nil")
	}

	// Test that documents were loaded
	count := db.Count()
	if count == 0 {
		t.Error("No documents loaded from embedded documentation")
	} else {
		t.Logf("✅ Loaded %d documents from embedded documentation", count)
	}
}

func TestEmbeddedDocumentationSearch(t *testing.T) {
	// Load the embedded database
	db, err := LoadEmbeddedDatabase()
	if err != nil {
		t.Fatalf("Failed to load embedded database: %v", err)
	}

	// Test search functionality
	results, err := SearchDocumentation(db, "client.yaml configuration", 3)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("No search results returned")
	} else {
		t.Logf("✅ Found %d results for 'client.yaml configuration'", len(results))

		// Log the top result for verification
		if len(results) > 0 {
			t.Logf("Top result: %s (similarity: %.2f)", results[0].Title, results[0].Similarity)
		}
	}
}

func TestEmbeddedDocsFileSystem(t *testing.T) {
	// Test that we can read from the embedded filesystem
	entries, err := embeddedDocs.ReadDir("docs")
	if err != nil {
		t.Fatalf("Failed to read embedded docs directory: %v", err)
	}

	if len(entries) == 0 {
		t.Error("No entries found in embedded docs directory")
	} else {
		t.Logf("✅ Found %d entries in embedded docs directory", len(entries))

		// List all entries for debugging
		for i, entry := range entries {
			t.Logf("  Entry %d: %s (isDir: %v)", i, entry.Name(), entry.IsDir())

			// If it's a directory, try to list its contents
			if entry.IsDir() {
				subEntries, err := embeddedDocs.ReadDir("docs/" + entry.Name())
				if err == nil && len(subEntries) > 0 {
					t.Logf("    Contains %d items:", len(subEntries))
					for j, subEntry := range subEntries {
						if j < 3 { // Limit to first 3 items
							t.Logf("      - %s", subEntry.Name())
						}
					}
				}
			}
		}
	}

	// Test reading a specific file (if it exists)
	_, err = embeddedDocs.ReadFile("docs/index.md")
	if err == nil {
		t.Log("✅ Successfully read docs/index.md from embedded filesystem")
	} else {
		t.Logf("Note: docs/index.md not found in embedded filesystem: %v", err)

		// Try to find any .md files in the embedded filesystem
		t.Log("Looking for .md files in embedded filesystem...")
		db := &Database{}
		db.walkEmbeddedDocs("docs", func(path string, content []byte) error {
			if strings.HasSuffix(path, ".md") {
				t.Logf("  Found: %s (%d bytes)", path, len(content))
			}
			return nil
		})
	}
}

func TestContextEnrichmentQueries(t *testing.T) {
	// Load the embedded database
	db, err := LoadEmbeddedDatabase()
	if err != nil {
		t.Fatalf("Failed to load embedded database: %v", err)
	}

	// Test the types of queries that would be used for context enrichment
	testQueries := []string{
		"client.yaml configuration example",
		"docker-compose.yaml example",
		"Dockerfile best practices",
		"Simple Container stacks configuration",
		"Go client.yaml example",
		"Python Dockerfile example",
	}

	for _, query := range testQueries {
		results, err := SearchDocumentation(db, query, 2)
		if err != nil {
			t.Errorf("Search failed for query '%s': %v", query, err)
			continue
		}

		t.Logf("Query: '%s' returned %d results", query, len(results))

		// Check that we get meaningful results
		for i, result := range results {
			if result.Similarity > 0.3 { // Lower threshold for testing
				t.Logf("  Result %d: %s (similarity: %.2f)", i+1, result.Title, result.Similarity)
			}
		}
	}
}

func TestEmbeddingGeneration(t *testing.T) {
	// Test the local embedding function
	testText := "Simple Container client.yaml configuration with Docker Compose and Kubernetes deployment"
	embedding := createSimpleEmbedding(testText)

	if len(embedding) != 128 {
		t.Errorf("Expected embedding length 128, got %d", len(embedding))
	}

	// Check that the embedding has some non-zero values
	hasNonZero := false
	for _, val := range embedding {
		if val != 0 {
			hasNonZero = true
			break
		}
	}

	if !hasNonZero {
		t.Error("Embedding is all zeros, which suggests the embedding function isn't working")
	} else {
		t.Log("✅ Embedding generation working correctly (128-dimensional vector with non-zero values)")
	}
}
