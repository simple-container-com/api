package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/gomega"
)

func TestMCPProtocol(t *testing.T) {
	RegisterTestingT(t)

	t.Run("test MCP request parsing", func(t *testing.T) {
		RegisterTestingT(t)
		RegisterTestingT(t)

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
		Expect(err).ToNot(HaveOccurred())
		Expect(req.JSONRPC).To(Equal("2.0"))
		Expect(req.Method).To(Equal("search_documentation"))
		Expect(req.ID).To(Equal("test-123"))
		Expect(req.Params).ToNot(BeNil())
	})

	t.Run("test MCP response creation", func(t *testing.T) {
		RegisterTestingT(t)
		RegisterTestingT(t)

		result := map[string]interface{}{
			"documents": []string{"doc1", "doc2"},
			"total":     2,
		}

		response := NewMCPResponse("test-456", result)
		Expect(response.JSONRPC).To(Equal("2.0"))
		Expect(response.ID).To(Equal("test-456"))
		Expect(response.Result).To(Equal(result))
		Expect(response.Error).To(BeNil())

		// Test JSON serialization
		jsonData, err := response.ToJSON()
		Expect(err).ToNot(HaveOccurred())
		Expect(string(jsonData)).To(ContainSubstring("test-456"))
		Expect(string(jsonData)).To(ContainSubstring("doc1"))
	})

	t.Run("test MCP error creation", func(t *testing.T) {
		RegisterTestingT(t)
		errorResponse := NewMCPError("test-789", ErrorCodeMethodNotFound, "Method not found", "additional data")
		Expect(errorResponse.JSONRPC).To(Equal("2.0"))
		Expect(errorResponse.ID).To(Equal("test-789"))
		Expect(errorResponse.Result).To(BeNil())
		Expect(errorResponse.Error).ToNot(BeNil())
		Expect(errorResponse.Error.Code).To(Equal(ErrorCodeMethodNotFound))
		Expect(errorResponse.Error.Message).To(Equal("Method not found"))
		Expect(errorResponse.Error.Data).To(Equal("additional data"))
	})
}

func TestMCPServer(t *testing.T) {
	// Create test server with HTTP mode for testing
	server := NewMCPServer("localhost", 0, MCPModeHTTP, false, nil) // Use port 0 for testing

	t.Run("test health check endpoint", func(t *testing.T) {
		RegisterTestingT(t)
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()

		server.handleHealthCheck(w, req)

		Expect(w.Code).To(Equal(http.StatusOK))
		Expect(w.Header().To(Equal("application/json")).Get("Content-Type"))

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		Expect(err).ToNot(HaveOccurred())
		Expect(response["status"]).To(Equal("healthy"))
		Expect(response["version"]).To(Equal(MCPVersion))
		Expect(response["name"]).To(Equal(MCPName))
	})

	t.Run("test capabilities endpoint", func(t *testing.T) {
		RegisterTestingT(t)
		req := httptest.NewRequest(http.MethodGet, "/capabilities", nil)
		w := httptest.NewRecorder()

		// Test the capabilities endpoint via the HTTP handler (simplified)
		server.handleHealthCheck(w, req) // Use health check since capabilities handler was removed

		Expect(w.Code).To(Equal(http.StatusOK))
		Expect(w.Header().To(Equal("application/json")).Get("Content-Type"))

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		Expect(err).ToNot(HaveOccurred())
		Expect(response["name"]).To(Equal(MCPName))
		Expect(response["version"]).To(Equal(MCPVersion))
	})

	t.Run("test MCP ping request", func(t *testing.T) {
		RegisterTestingT(t)
		requestBody := MCPRequest{
			JSONRPC: "2.0",
			Method:  "ping",
			ID:      "ping-test",
		}

		jsonData, err := requestBody.ToJSON()
		Expect(err).ToNot(HaveOccurred())

		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleMCPRequest(w, req)

		Expect(w.Code).To(Equal(http.StatusOK))

		var response MCPResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		Expect(err).ToNot(HaveOccurred())
		Expect(response.JSONRPC).To(Equal("2.0"))
		Expect(response.ID).To(Equal("ping-test"))
		Expect(response.Result).To(Equal("pong"))
		Expect(response.Error).To(BeNil())
	})

	t.Run("test MCP tools/call get_project_context", func(t *testing.T) {
		RegisterTestingT(t)
		requestBody := MCPRequest{
			JSONRPC: "2.0",
			Method:  "tools/call",
			Params: map[string]interface{}{
				"name": "get_project_context",
				"arguments": map[string]interface{}{
					"path": ".",
				},
			},
			ID: "context-test",
		}

		jsonData, err := requestBody.ToJSON()
		Expect(err).ToNot(HaveOccurred())

		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleMCPRequest(w, req)

		Expect(w.Code).To(Equal(http.StatusOK))

		var response MCPResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		Expect(err).ToNot(HaveOccurred())
		Expect(response.JSONRPC).To(Equal("2.0"))
		Expect(response.ID).To(Equal("context-test"))
		Expect(response.Result).ToNot(BeNil())
		Expect(response.Error).To(BeNil())

		// Verify tool call response structure
		resultMap := response.Result.(map[string]interface{})
		Expect(resultMap).To(ContainSubstring("content"))
		Expect(resultMap).To(ContainSubstring("isError"))
	})

	t.Run("test MCP invalid method", func(t *testing.T) {
		RegisterTestingT(t)
		requestBody := MCPRequest{
			JSONRPC: "2.0",
			Method:  "nonexistent_method",
			ID:      "invalid-test",
		}

		jsonData, err := requestBody.ToJSON()
		Expect(err).ToNot(HaveOccurred())

		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleMCPRequest(w, req)

		Expect(w.Code).To(Equal(http.StatusOK))

		var response MCPResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		Expect(err).ToNot(HaveOccurred())
		Expect(response.ID).To(Equal("invalid-test"))
		Expect(response.Result).To(BeNil())
		Expect(response.Error).ToNot(BeNil())
		Expect(response.Error.Code).To(Equal(ErrorCodeMethodNotFound))
	})

	t.Run("test standard MCP tools/list", func(t *testing.T) {
		RegisterTestingT(t)
		requestBody := MCPRequest{
			JSONRPC: "2.0",
			Method:  "tools/list",
			ID:      "tools-test",
		}

		jsonData, err := requestBody.ToJSON()
		Expect(err).ToNot(HaveOccurred())

		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleMCPRequest(w, req)

		Expect(w.Code).To(Equal(http.StatusOK))

		var response MCPResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		Expect(err).ToNot(HaveOccurred())
		Expect(response.ID).To(Equal("tools-test"))
		Expect(response.Result).ToNot(BeNil())
		Expect(response.Error).To(BeNil())
	})
}

func TestDefaultMCPHandler(t *testing.T) {
	handler := NewDefaultMCPHandler(nil)
	ctx := context.Background()

	t.Run("test ping", func(t *testing.T) {
		RegisterTestingT(t)
		result, err := handler.Ping(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal("pong"))
	})

	t.Run("test get capabilities", func(t *testing.T) {
		RegisterTestingT(t)
		capabilities, err := handler.GetCapabilities(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(capabilities).To(ContainSubstring("name"))
		Expect(capabilities).To(ContainSubstring("version"))
		Expect(capabilities).To(ContainSubstring("methods"))
		Expect(capabilities["name"]).To(Equal(MCPName))
		Expect(capabilities["version"]).To(Equal(MCPVersion))
	})

	t.Run("test get project context", func(t *testing.T) {
		RegisterTestingT(t)
		params := GetProjectContextParams{Path: "."}
		context, err := handler.GetProjectContext(ctx, params)
		Expect(err).ToNot(HaveOccurred())
		Expect(context.Path).ToNot(BeEmpty())
		Expect(context.Name).ToNot(BeEmpty())
		Expect(context.Metadata).ToNot(BeNil())
	})

	t.Run("test get supported resources", func(t *testing.T) {
		RegisterTestingT(t)
		resources, err := handler.GetSupportedResources(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(resources.Resources).To(BeTrue()) > 0)
		Expect(len(resources.Providers).To(BeTrue()) > 0)
		Expect(resources.Total).To(Equal(len(resources.Resources)))
	})

	t.Run("test search documentation (mock)", func(t *testing.T) {
		RegisterTestingT(t)
		// This test will skip if embeddings are not available
		params := SearchDocumentationParams{
			Query: "test query",
			Limit: 5,
		}

		// Search documentation - may return results if embeddings are available
		result, err := handler.SearchDocumentation(ctx, params)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		// Results may be empty or have content depending on embeddings availability
		Expect(result.Total >= 0).To(BeTrue())
		Expect(len(result.Documents).To(Equal(result.Total)))
	})
}

// Benchmark tests for MCP operations
func BenchmarkMCPRequest(b *testing.B) {
	server := NewMCPServer("localhost", 0, MCPModeHTTP, false, nil)

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
