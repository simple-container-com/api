package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/embeddings"
)

// MCPServer implements the Model Context Protocol for Simple Container
type MCPServer struct {
	handler MCPHandler
	logger  *log.Logger
	port    int
	host    string
	server  *http.Server
}

// NewMCPServer creates a new MCP server instance
func NewMCPServer(host string, port int) *MCPServer {
	return &MCPServer{
		handler: NewDefaultMCPHandler(),
		logger:  log.New(os.Stdout, "MCP: ", log.LstdFlags),
		host:    host,
		port:    port,
	}
}

// Start starts the MCP JSON-RPC server
func (s *MCPServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Main MCP endpoint
	mux.HandleFunc("/mcp", s.handleMCPRequest)

	// Health check endpoint
	mux.HandleFunc("/health", s.handleHealthCheck)

	// Capabilities endpoint
	mux.HandleFunc("/capabilities", s.handleCapabilities)

	s.server = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", s.host, s.port),
		Handler: s.corsMiddleware(mux),
	}

	listener, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	fmt.Printf("🌐 MCP Server starting on %s\n", color.CyanFmt(s.server.Addr))
	fmt.Printf("📖 Documentation search available at: http://%s/mcp\n", s.server.Addr)
	fmt.Printf("🔍 Capabilities endpoint: http://%s/capabilities\n", s.server.Addr)
	fmt.Printf("💚 Health check: http://%s/health\n\n", s.server.Addr)

	// Start server in goroutine
	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.logger.Printf("MCP Server error: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	fmt.Println("\n🛑 Shutting down MCP server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.server.Shutdown(shutdownCtx)
}

// handleMCPRequest processes JSON-RPC requests
func (s *MCPServer) handleMCPRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, nil, ErrorCodeInvalidRequest, "Method not allowed", nil)
		return
	}

	var req MCPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, nil, ErrorCodeParseError, "Invalid JSON", err.Error())
		return
	}

	ctx := r.Context()
	response := s.processRequest(ctx, &req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// processRequest routes MCP requests to appropriate handlers
func (s *MCPServer) processRequest(ctx context.Context, req *MCPRequest) *MCPResponse {
	switch req.Method {
	case "search_documentation":
		return s.handleSearchDocumentation(ctx, req)
	case "get_project_context":
		return s.handleGetProjectContext(ctx, req)
	case "generate_configuration":
		return s.handleGenerateConfiguration(ctx, req)
	case "analyze_project":
		return s.handleAnalyzeProject(ctx, req)
	case "get_supported_resources":
		return s.handleGetSupportedResources(ctx, req)
	case "get_capabilities":
		return s.handleGetCapabilities(ctx, req)
	case "ping":
		return s.handlePing(ctx, req)
	default:
		return NewMCPError(req.ID, ErrorCodeMethodNotFound, fmt.Sprintf("Method '%s' not found", req.Method), nil)
	}
}

func (s *MCPServer) handleSearchDocumentation(ctx context.Context, req *MCPRequest) *MCPResponse {
	var params SearchDocumentationParams
	if err := s.parseParams(req.Params, &params); err != nil {
		return NewMCPError(req.ID, ErrorCodeInvalidParams, "Invalid parameters", err.Error())
	}

	result, err := s.handler.SearchDocumentation(ctx, params)
	if err != nil {
		return NewMCPError(req.ID, ErrorCodeEmbeddingError, "Documentation search failed", err.Error())
	}

	return NewMCPResponse(req.ID, result)
}

func (s *MCPServer) handleGetProjectContext(ctx context.Context, req *MCPRequest) *MCPResponse {
	var params GetProjectContextParams
	if err := s.parseParams(req.Params, &params); err != nil {
		return NewMCPError(req.ID, ErrorCodeInvalidParams, "Invalid parameters", err.Error())
	}

	result, err := s.handler.GetProjectContext(ctx, params)
	if err != nil {
		return NewMCPError(req.ID, ErrorCodeProjectNotFound, "Project context retrieval failed", err.Error())
	}

	return NewMCPResponse(req.ID, result)
}

func (s *MCPServer) handleGenerateConfiguration(ctx context.Context, req *MCPRequest) *MCPResponse {
	var params GenerateConfigurationParams
	if err := s.parseParams(req.Params, &params); err != nil {
		return NewMCPError(req.ID, ErrorCodeInvalidParams, "Invalid parameters", err.Error())
	}

	result, err := s.handler.GenerateConfiguration(ctx, params)
	if err != nil {
		return NewMCPError(req.ID, ErrorCodeGenerationError, "Configuration generation failed", err.Error())
	}

	return NewMCPResponse(req.ID, result)
}

func (s *MCPServer) handleAnalyzeProject(ctx context.Context, req *MCPRequest) *MCPResponse {
	var params AnalyzeProjectParams
	if err := s.parseParams(req.Params, &params); err != nil {
		return NewMCPError(req.ID, ErrorCodeInvalidParams, "Invalid parameters", err.Error())
	}

	result, err := s.handler.AnalyzeProject(ctx, params)
	if err != nil {
		return NewMCPError(req.ID, ErrorCodeAnalysisError, "Project analysis failed", err.Error())
	}

	return NewMCPResponse(req.ID, result)
}

func (s *MCPServer) handleGetSupportedResources(ctx context.Context, req *MCPRequest) *MCPResponse {
	result, err := s.handler.GetSupportedResources(ctx)
	if err != nil {
		return NewMCPError(req.ID, ErrorCodeInternalError, "Failed to get supported resources", err.Error())
	}

	return NewMCPResponse(req.ID, result)
}

func (s *MCPServer) handleGetCapabilities(ctx context.Context, req *MCPRequest) *MCPResponse {
	result, err := s.handler.GetCapabilities(ctx)
	if err != nil {
		return NewMCPError(req.ID, ErrorCodeInternalError, "Failed to get capabilities", err.Error())
	}

	return NewMCPResponse(req.ID, result)
}

func (s *MCPServer) handlePing(ctx context.Context, req *MCPRequest) *MCPResponse {
	result, err := s.handler.Ping(ctx)
	if err != nil {
		return NewMCPError(req.ID, ErrorCodeInternalError, "Ping failed", err.Error())
	}

	return NewMCPResponse(req.ID, result)
}

// HTTP handlers for non-JSON-RPC endpoints
func (s *MCPServer) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"version":   MCPVersion,
		"name":      MCPName,
	})
}

func (s *MCPServer) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	capabilities, _ := s.handler.GetCapabilities(r.Context())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(capabilities)
}

// Utility methods
func (s *MCPServer) parseParams(params interface{}, target interface{}) error {
	if params == nil {
		return nil
	}

	data, err := json.Marshal(params)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, target)
}

func (s *MCPServer) writeError(w http.ResponseWriter, id interface{}, code int, message string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	response := NewMCPError(id, code, message, data)
	json.NewEncoder(w).Encode(response)
}

func (s *MCPServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// DefaultMCPHandler provides default implementations of MCP methods
type DefaultMCPHandler struct {
	embeddings *embeddings.DB // This will be chromem-go DB instance
}

func NewDefaultMCPHandler() MCPHandler {
	return &DefaultMCPHandler{}
}

func (h *DefaultMCPHandler) SearchDocumentation(ctx context.Context, params SearchDocumentationParams) (*DocumentationSearchResult, error) {
	// Load embedded documentation database
	db, err := embeddings.LoadEmbeddedDatabase()
	if err != nil {
		return nil, fmt.Errorf("failed to load documentation database: %w", err)
	}

	// Set default limit
	limit := params.Limit
	if limit <= 0 {
		limit = 10
	}

	// Perform search
	results, err := embeddings.SearchDocumentation(db, params.Query, limit)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Convert results to MCP format
	documents := make([]DocumentChunk, len(results))
	for i, result := range results {
		// Type assertions for metadata
		path, _ := result.Metadata["path"].(string)
		docType, _ := result.Metadata["type"].(string)

		// Convert metadata map
		metadata := make(map[string]string)
		for k, v := range result.Metadata {
			if str, ok := v.(string); ok {
				metadata[k] = str
			}
		}

		documents[i] = DocumentChunk{
			ID:         result.ID,
			Content:    result.Content,
			Path:       path,
			Type:       docType,
			Similarity: float32(result.Similarity),
			Metadata:   metadata,
		}
	}

	return &DocumentationSearchResult{
		Documents: documents,
		Total:     len(documents),
		Query:     params.Query,
		Timestamp: time.Now(),
	}, nil
}

func (h *DefaultMCPHandler) GetProjectContext(ctx context.Context, params GetProjectContextParams) (*ProjectContext, error) {
	projectPath := params.Path
	if projectPath == "" {
		projectPath = "."
	}

	// Check if path exists
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("project path does not exist: %s", projectPath)
	}

	// Check for Simple Container configuration
	scConfigPath := filepath.Join(projectPath, ".sc")
	scConfigExists := false
	if _, err := os.Stat(scConfigPath); err == nil {
		scConfigExists = true
	}

	return &ProjectContext{
		Path:           projectPath,
		Name:           filepath.Base(projectPath),
		SCConfigExists: scConfigExists,
		SCConfigPath:   scConfigPath,
		Resources:      []ResourceInfo{}, // TODO: Implement resource discovery
		Recommendations: []string{
			"Consider adding Simple Container configuration",
			"Review documentation for best practices",
		},
		Metadata: map[string]interface{}{
			"analyzed_at": time.Now(),
			"mcp_version": MCPVersion,
		},
	}, nil
}

func (h *DefaultMCPHandler) GenerateConfiguration(ctx context.Context, params GenerateConfigurationParams) (*GeneratedConfiguration, error) {
	// TODO: Implement configuration generation
	return &GeneratedConfiguration{
		ConfigType: params.ConfigType,
		Files: []GeneratedFile{
			{
				Path:        "Dockerfile",
				Content:     "# Generated Dockerfile\n# TODO: Implement generation logic",
				ContentType: "dockerfile",
				Description: "Basic Dockerfile for the project",
			},
		},
		Messages: []string{
			"Configuration generation is not yet implemented",
			"This will be available in Phase 2 of the implementation",
		},
		Metadata: map[string]interface{}{
			"generated_at": time.Now(),
			"status":       "placeholder",
		},
	}, nil
}

func (h *DefaultMCPHandler) AnalyzeProject(ctx context.Context, params AnalyzeProjectParams) (*ProjectAnalysis, error) {
	// TODO: Implement project analysis
	return &ProjectAnalysis{
		Path: params.Path,
		TechStacks: []TechStackInfo{
			{
				Language:   "unknown",
				Confidence: 0.0,
			},
		},
		Recommendations: []Recommendation{
			{
				Type:        "analysis",
				Category:    "setup",
				Priority:    "medium",
				Title:       "Project Analysis Not Implemented",
				Description: "Project analysis will be available in Phase 2",
			},
		},
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"status": "placeholder",
		},
	}, nil
}

func (h *DefaultMCPHandler) GetSupportedResources(ctx context.Context) (*SupportedResourcesResult, error) {
	// TODO: Load from schema files or registry
	return &SupportedResourcesResult{
		Resources: []ResourceInfo{
			{Type: "s3-bucket", Name: "S3 Bucket", Provider: "aws", Description: "Amazon S3 storage bucket"},
			{Type: "gcp-bucket", Name: "GCS Bucket", Provider: "gcp", Description: "Google Cloud Storage bucket"},
			{Type: "aws-rds-postgres", Name: "PostgreSQL RDS", Provider: "aws", Description: "Amazon RDS PostgreSQL database"},
		},
		Providers: []ProviderInfo{
			{Name: "aws", DisplayName: "Amazon Web Services", Resources: []string{"s3-bucket", "aws-rds-postgres"}},
			{Name: "gcp", DisplayName: "Google Cloud Platform", Resources: []string{"gcp-bucket"}},
		},
		Total: 3,
	}, nil
}

func (h *DefaultMCPHandler) GetCapabilities(ctx context.Context) (map[string]interface{}, error) {
	return map[string]interface{}{
		"name":    MCPName,
		"version": MCPVersion,
		"methods": []string{
			"search_documentation",
			"get_project_context",
			"generate_configuration",
			"analyze_project",
			"get_supported_resources",
			"get_capabilities",
			"ping",
		},
		"features": map[string]interface{}{
			"documentation_search":     true,
			"project_analysis":         false, // Phase 2
			"configuration_generation": false, // Phase 2
			"interactive_chat":         false, // Phase 3
		},
		"documentation": map[string]interface{}{
			"indexed_documents": 0, // TODO: Get actual count
			"providers":         []string{"docs", "examples", "schemas"},
			"embedding_model":   "openai/text-embedding-3-small",
		},
		"endpoints": map[string]string{
			"mcp":          "/mcp",
			"health":       "/health",
			"capabilities": "/capabilities",
		},
	}, nil
}

func (h *DefaultMCPHandler) Ping(ctx context.Context) (string, error) {
	return "pong", nil
}
