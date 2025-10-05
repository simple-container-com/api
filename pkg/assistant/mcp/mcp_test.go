package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPProtocol(t *testing.T) {
	t.Run("test MCP request parsing", func(t *testing.T) {
		requestJSON := `{
			"jsonrpc": "2.0",
			"method": "search_documentation",
			"params": {
				"query": "AWS S3 bucket",
				"limit": 5
			},
			"id": "test-123"
		}`

		req, err := ParseMCPRequest([]byte(requestJSON))
		require.NoError(t, err)
		assert.Equal(t, "2.0", req.JSONRPC)
		assert.Equal(t, "search_documentation", req.Method)
		assert.Equal(t, "test-123", req.ID)
		assert.NotNil(t, req.Params)
	})

	t.Run("test MCP response creation", func(t *testing.T) {
		result := map[string]interface{}{
			"documents": []string{"doc1", "doc2"},
			"total":     2,
		}

		response := NewMCPResponse("test-456", result)
		assert.Equal(t, "2.0", response.JSONRPC)
		assert.Equal(t, "test-456", response.ID)
		assert.Equal(t, result, response.Result)
		assert.Nil(t, response.Error)

		// Test JSON serialization
		jsonData, err := response.ToJSON()
		require.NoError(t, err)
		assert.Contains(t, string(jsonData), "test-456")
		assert.Contains(t, string(jsonData), "doc1")
	})

	t.Run("test MCP error creation", func(t *testing.T) {
		errorResponse := NewMCPError("test-789", ErrorCodeMethodNotFound, "Method not found", "additional data")
		assert.Equal(t, "2.0", errorResponse.JSONRPC)
		assert.Equal(t, "test-789", errorResponse.ID)
		assert.Nil(t, errorResponse.Result)
		require.NotNil(t, errorResponse.Error)
		assert.Equal(t, ErrorCodeMethodNotFound, errorResponse.Error.Code)
		assert.Equal(t, "Method not found", errorResponse.Error.Message)
		assert.Equal(t, "additional data", errorResponse.Error.Data)
	})
}

func TestMCPServer(t *testing.T) {
	// Create test server
	server := NewMCPServer("localhost", 0) // Use port 0 for testing

	t.Run("test health check endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()

		server.handleHealthCheck(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "healthy", response["status"])
		assert.Equal(t, MCPVersion, response["version"])
		assert.Equal(t, MCPName, response["name"])
	})

	t.Run("test capabilities endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/capabilities", nil)
		w := httptest.NewRecorder()

		server.handleCapabilities(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, MCPName, response["name"])
		assert.Equal(t, MCPVersion, response["version"])
		assert.Contains(t, response, "methods")
		assert.Contains(t, response, "features")
	})

	t.Run("test MCP ping request", func(t *testing.T) {
		requestBody := MCPRequest{
			JSONRPC: "2.0",
			Method:  "ping",
			ID:      "ping-test",
		}

		jsonData, err := requestBody.ToJSON()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleMCPRequest(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response MCPResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "2.0", response.JSONRPC)
		assert.Equal(t, "ping-test", response.ID)
		assert.Equal(t, "pong", response.Result)
		assert.Nil(t, response.Error)
	})

	t.Run("test MCP get_project_context request", func(t *testing.T) {
		requestBody := MCPRequest{
			JSONRPC: "2.0",
			Method:  "get_project_context",
			Params: map[string]interface{}{
				"path": ".",
			},
			ID: "context-test",
		}

		jsonData, err := requestBody.ToJSON()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleMCPRequest(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response MCPResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "2.0", response.JSONRPC)
		assert.Equal(t, "context-test", response.ID)
		assert.NotNil(t, response.Result)
		assert.Nil(t, response.Error)

		// Verify project context structure
		resultMap := response.Result.(map[string]interface{})
		assert.Contains(t, resultMap, "path")
		assert.Contains(t, resultMap, "name")
		assert.Contains(t, resultMap, "sc_config_exists")
	})

	t.Run("test MCP invalid method", func(t *testing.T) {
		requestBody := MCPRequest{
			JSONRPC: "2.0",
			Method:  "nonexistent_method",
			ID:      "invalid-test",
		}

		jsonData, err := requestBody.ToJSON()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleMCPRequest(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response MCPResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "invalid-test", response.ID)
		assert.Nil(t, response.Result)
		require.NotNil(t, response.Error)
		assert.Equal(t, ErrorCodeMethodNotFound, response.Error.Code)
	})

	t.Run("test CORS headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
		w := httptest.NewRecorder()

		// Test CORS middleware
		handler := server.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
	})
}

func TestDefaultMCPHandler(t *testing.T) {
	handler := NewDefaultMCPHandler()
	ctx := context.Background()

	t.Run("test ping", func(t *testing.T) {
		result, err := handler.Ping(ctx)
		require.NoError(t, err)
		assert.Equal(t, "pong", result)
	})

	t.Run("test get capabilities", func(t *testing.T) {
		capabilities, err := handler.GetCapabilities(ctx)
		require.NoError(t, err)
		assert.Contains(t, capabilities, "name")
		assert.Contains(t, capabilities, "version")
		assert.Contains(t, capabilities, "methods")
		assert.Equal(t, MCPName, capabilities["name"])
		assert.Equal(t, MCPVersion, capabilities["version"])
	})

	t.Run("test get project context", func(t *testing.T) {
		params := GetProjectContextParams{Path: "."}
		context, err := handler.GetProjectContext(ctx, params)
		require.NoError(t, err)
		assert.NotEmpty(t, context.Path)
		assert.NotEmpty(t, context.Name)
		assert.Contains(t, context.Metadata, "analyzed_at")
	})

	t.Run("test get supported resources", func(t *testing.T) {
		resources, err := handler.GetSupportedResources(ctx)
		require.NoError(t, err)
		assert.True(t, len(resources.Resources) > 0)
		assert.True(t, len(resources.Providers) > 0)
		assert.Equal(t, len(resources.Resources), resources.Total)
	})

	t.Run("test search documentation (mock)", func(t *testing.T) {
		// This test will skip if embeddings are not available
		params := SearchDocumentationParams{
			Query: "test query",
			Limit: 5,
		}

		// For now, this will likely return an error since we don't have embedded docs
		result, err := handler.SearchDocumentation(ctx, params)
		if err != nil {
			t.Logf("Expected error (embeddings not available): %v", err)
			assert.Contains(t, err.Error(), "documentation database")
		} else {
			assert.NotNil(t, result)
			assert.Equal(t, "test query", result.Query)
		}
	})
}

// Benchmark tests for MCP operations
func BenchmarkMCPRequest(b *testing.B) {
	server := NewMCPServer("localhost", 0)

	requestBody := MCPRequest{
		JSONRPC: "2.0",
		Method:  "ping",
		ID:      "benchmark-test",
	}

	jsonData, _ := requestBody.ToJSON()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleMCPRequest(w, req)

		if w.Code != http.StatusOK {
			b.Errorf("Expected status 200, got %d", w.Code)
		}
	}
}

func BenchmarkJSONParsing(b *testing.B) {
	requestJSON := `{
		"jsonrpc": "2.0",
		"method": "search_documentation",
		"params": {
			"query": "AWS S3 bucket configuration example",
			"limit": 10,
			"type": "docs"
		},
		"id": "benchmark-456"
	}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseMCPRequest([]byte(requestJSON))
		if err != nil {
			b.Errorf("Parse error: %v", err)
		}
	}
}
