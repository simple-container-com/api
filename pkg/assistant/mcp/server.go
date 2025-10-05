package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/analysis"
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
		logger:  log.New(os.Stderr, "MCP: ", log.LstdFlags),
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
	mux.HandleFunc("/capabilities", func(w http.ResponseWriter, r *http.Request) {
		capabilities := map[string]interface{}{
			"name":        MCPName,
			"version":     MCPVersion,
			"description": "Simple Container AI Assistant - provides documentation search, project analysis, and resource information",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(capabilities)
	})

	s.server = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", s.host, s.port),
		Handler: mux,
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

// StartStdio starts the MCP server in stdio mode for IDE integration
func (s *MCPServer) StartStdio(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)

	// Create a channel to handle stdin input
	inputCh := make(chan string, 1)
	errCh := make(chan error, 1)

	// Track initialization state
	initialized := false

	// Start goroutine to read from stdin
	go func() {
		defer close(inputCh)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			case inputCh <- scanner.Text():
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("error reading from stdin: %w", err)
		}
	}()

	// Main processing loop
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		case line, ok := <-inputCh:
			if !ok {
				// stdin closed
				return nil
			}

			if line == "" {
				continue
			}

			// Parse JSON-RPC request
			var req MCPRequest
			if err := json.Unmarshal([]byte(line), &req); err != nil {
				response := NewMCPError(nil, ErrorCodeParseError, "Invalid JSON", err.Error())
				s.sendStdioResponse(response)
				continue
			}

			// Handle initialization sequence
			if req.Method == "initialize" && !initialized {
				response := s.handleInitialize(ctx, &req)
				s.sendStdioResponse(response)
				continue
			}

			if req.Method == "notifications/initialized" {
				initialized = true
				continue // No response needed for notifications
			}

			// Reject non-ping requests before initialization
			if !initialized && req.Method != "ping" {
				response := NewMCPError(req.ID, ErrorCodeInvalidRequest, "Server not initialized", nil)
				s.sendStdioResponse(response)
				continue
			}

			// Process request
			response := s.processRequest(ctx, &req)
			s.sendStdioResponse(response)
		}
	}
}

// sendStdioResponse sends a response via stdout with proper formatting
func (s *MCPServer) sendStdioResponse(response *MCPResponse) {
	if responseJSON, err := response.ToJSON(); err == nil {
		// Ensure no embedded newlines (MCP requirement)
		jsonStr := strings.ReplaceAll(string(responseJSON), "\n", "")
		fmt.Println(jsonStr)
	}
}

// handleInitialize handles the MCP initialization request
func (s *MCPServer) handleInitialize(ctx context.Context, req *MCPRequest) *MCPResponse {
	// Parse initialize parameters
	var params struct {
		ProtocolVersion string                 `json:"protocolVersion"`
		Capabilities    map[string]interface{} `json:"capabilities"`
		ClientInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"clientInfo"`
	}

	if req.Params != nil {
		if paramsBytes, err := json.Marshal(req.Params); err == nil {
			_ = json.Unmarshal(paramsBytes, &params)
		}
	}

	// Respond with server capabilities
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{
				"listChanged": false,
			},
			"resources": map[string]interface{}{
				"subscribe":   false,
				"listChanged": false,
			},
			"logging": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "simple-container-mcp",
			"version": "1.0.0",
		},
		"instructions": "Simple Container AI Assistant - provides documentation search, project analysis, and resource information",
	}

	return NewMCPResponse(req.ID, result)
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
	_ = json.NewEncoder(w).Encode(response)
}

// processRequest routes MCP requests to appropriate handlers
func (s *MCPServer) processRequest(ctx context.Context, req *MCPRequest) *MCPResponse {
	switch req.Method {
	// Standard MCP methods only
	case "ping":
		return s.handlePing(ctx, req)
	case "tools/list":
		return s.handleListTools(ctx, req)
	case "tools/call":
		return s.handleCallTool(ctx, req)
	case "resources/list":
		return s.handleListResources(ctx, req)
	case "resources/read":
		return s.handleReadResource(ctx, req)
	default:
		return NewMCPError(req.ID, ErrorCodeMethodNotFound, fmt.Sprintf("Method '%s' not found", req.Method), nil)
	}
}

func (s *MCPServer) handlePing(ctx context.Context, req *MCPRequest) *MCPResponse {
	return NewMCPResponse(req.ID, "pong")
}

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
	w.WriteHeader(http.StatusOK) // MCP uses 200 OK even for errors
	_ = json.NewEncoder(w).Encode(NewMCPError(id, code, message, data))
}

func (s *MCPServer) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"name":      MCPName,
		"status":    "healthy",
		"timestamp": time.Now(),
		"version":   MCPVersion,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(health)
}

// Standard MCP method handlers

func (s *MCPServer) handleListTools(ctx context.Context, req *MCPRequest) *MCPResponse {
	tools := []map[string]interface{}{
		{
			"name":        "search_documentation",
			"description": "Search Simple Container documentation using semantic similarity",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Search query for documentation",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of results (default: 5)",
						"default":     5,
					},
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "get_project_context",
			"description": "Analyze project structure and get Simple Container context",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Project path to analyze (default: current directory)",
						"default":     ".",
					},
				},
			},
		},
		{
			"name":        "get_supported_resources",
			"description": "Get list of all supported Simple Container resources",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "analyze_project",
			"description": "Perform detailed project analysis with tech stack detection and recommendations",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Project path to analyze (default: current directory)",
						"default":     ".",
					},
				},
			},
		},
	}

	result := map[string]interface{}{
		"tools": tools,
	}

	return NewMCPResponse(req.ID, result)
}

func (s *MCPServer) handleCallTool(ctx context.Context, req *MCPRequest) *MCPResponse {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}

	if err := s.parseParams(req.Params, &params); err != nil {
		return NewMCPError(req.ID, ErrorCodeInvalidParams, "Invalid parameters", err.Error())
	}

	switch params.Name {
	case "search_documentation":
		// Convert arguments to SearchDocumentationParams
		query, _ := params.Arguments["query"].(string)
		limit := 5
		if l, ok := params.Arguments["limit"].(float64); ok {
			limit = int(l)
		}

		searchParams := SearchDocumentationParams{
			Query: query,
			Limit: limit,
		}

		result, err := s.handler.SearchDocumentation(ctx, searchParams)
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeEmbeddingError, "Documentation search failed", err.Error())
		}

		return NewMCPResponse(req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Found %d documentation results for '%s'", result.Total, query),
				},
			},
			"isError": false,
		})

	case "get_project_context":
		path := "."
		if p, ok := params.Arguments["path"].(string); ok {
			path = p
		}

		contextParams := GetProjectContextParams{
			Path: path,
		}

		result, err := s.handler.GetProjectContext(ctx, contextParams)
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeProjectNotFound, "Project context retrieval failed", err.Error())
		}

		language := "unknown"
		framework := "unknown"
		if result.TechStack != nil {
			language = result.TechStack.Language
			framework = result.TechStack.Framework
		}

		return NewMCPResponse(req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Project: %s, Language: %s, Framework: %s", result.Name, language, framework),
				},
			},
			"isError": false,
		})

	case "get_supported_resources":
		result, err := s.handler.GetSupportedResources(ctx)
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeAnalysisError, "Failed to get supported resources", err.Error())
		}

		return NewMCPResponse(req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Simple Container supports %d resource types across %d providers", result.Total, len(result.Providers)),
				},
			},
			"isError": false,
		})

	case "analyze_project":
		path := "."
		if p, ok := params.Arguments["path"].(string); ok {
			path = p
		}

		analyzeParams := AnalyzeProjectParams{
			Path: path,
		}

		result, err := s.handler.AnalyzeProject(ctx, analyzeParams)
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeAnalysisError, "Project analysis failed", err.Error())
		}

		// Format the analysis results
		techStacksInfo := "No tech stacks detected"
		if len(result.TechStacks) > 0 {
			techStacksInfo = fmt.Sprintf("Detected %d tech stacks", len(result.TechStacks))
		}

		recommendationsInfo := "No recommendations"
		if len(result.Recommendations) > 0 {
			recommendationsInfo = fmt.Sprintf("%d recommendations available", len(result.Recommendations))
		}

		return NewMCPResponse(req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Project Analysis: %s\nTech Stacks: %s\nRecommendations: %s\nArchitecture: %s",
						result.Path, techStacksInfo, recommendationsInfo, result.Architecture),
				},
			},
			"isError": false,
		})

	default:
		return NewMCPError(req.ID, ErrorCodeMethodNotFound, fmt.Sprintf("Tool '%s' not found", params.Name), nil)
	}
}

func (s *MCPServer) handleListResources(ctx context.Context, req *MCPRequest) *MCPResponse {
	resources := []map[string]interface{}{
		{
			"uri":         "simple-container://documentation",
			"name":        "Simple Container Documentation",
			"description": "Searchable Simple Container documentation and examples",
			"mimeType":    "text/plain",
		},
		{
			"uri":         "simple-container://resources",
			"name":        "Supported Resources",
			"description": "List of all supported Simple Container cloud resources",
			"mimeType":    "application/json",
		},
	}

	result := map[string]interface{}{
		"resources": resources,
	}

	return NewMCPResponse(req.ID, result)
}

func (s *MCPServer) handleReadResource(ctx context.Context, req *MCPRequest) *MCPResponse {
	var params struct {
		URI string `json:"uri"`
	}

	if err := s.parseParams(req.Params, &params); err != nil {
		return NewMCPError(req.ID, ErrorCodeInvalidParams, "Invalid parameters", err.Error())
	}

	switch params.URI {
	case "simple-container://documentation":
		result := map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"uri":      params.URI,
					"mimeType": "text/plain",
					"text":     "Simple Container documentation is available via search_documentation tool. Use semantic search to find specific topics.",
				},
			},
		}
		return NewMCPResponse(req.ID, result)

	case "simple-container://resources":
		resourceResult, err := s.handler.GetSupportedResources(ctx)
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeAnalysisError, "Failed to get supported resources", err.Error())
		}

		resourcesJSON, _ := json.Marshal(resourceResult)
		result := map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"uri":      params.URI,
					"mimeType": "application/json",
					"text":     string(resourcesJSON),
				},
			},
		}
		return NewMCPResponse(req.ID, result)

	default:
		return NewMCPError(req.ID, ErrorCodeInvalidParams, fmt.Sprintf("Unknown resource URI: %s", params.URI), nil)
	}
}

// DefaultMCPHandler implements MCPHandler interface with only essential functionality
type DefaultMCPHandler struct {
	embeddingsDB *embeddings.Database
}

// NewDefaultMCPHandler creates a new default MCP handler
func NewDefaultMCPHandler() MCPHandler {
	// Initialize embeddings database
	db, err := embeddings.LoadEmbeddedDatabase(context.Background())
	if err != nil {
		// Log error but continue - server will work without embeddings
		log.Printf("Warning: Failed to load embeddings database: %v", err)
		db = nil
	}

	return &DefaultMCPHandler{
		embeddingsDB: db,
	}
}

func (h *DefaultMCPHandler) SearchDocumentation(ctx context.Context, params SearchDocumentationParams) (*DocumentationSearchResult, error) {
	if h.embeddingsDB == nil {
		return &DocumentationSearchResult{
			Documents: []DocumentChunk{},
			Total:     0,
		}, nil
	}

	// Perform semantic search
	results, err := embeddings.SearchDocumentation(h.embeddingsDB, params.Query, params.Limit)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Convert results to MCP format
	documents := make([]DocumentChunk, len(results))
	for i, result := range results {
		// Convert metadata map
		metadata := make(map[string]string)
		for k, v := range result.Metadata {
			if str, ok := v.(string); ok {
				metadata[k] = str
			}
		}

		documents[i] = DocumentChunk{
			Content:    result.Content,
			Similarity: float32(result.Similarity),
			Metadata:   metadata,
		}
	}

	return &DocumentationSearchResult{
		Documents: documents,
		Total:     len(documents),
	}, nil
}

func (h *DefaultMCPHandler) GetProjectContext(ctx context.Context, params GetProjectContextParams) (*ProjectContext, error) {
	// Analyze project at given path
	analyzer := analysis.NewProjectAnalyzer()
	projectInfo, err := analyzer.AnalyzeProject(params.Path)
	if err != nil {
		return nil, fmt.Errorf("project analysis failed: %w", err)
	}

	// Check for Simple Container configuration
	scConfigPath := filepath.Join(params.Path, ".sc")
	scConfigExists := false
	if _, err := os.Stat(scConfigPath); err == nil {
		scConfigExists = true
	}

	// Convert to MCP format
	context := &ProjectContext{
		Path:           params.Path,
		Name:           projectInfo.Name,
		SCConfigExists: scConfigExists,
		SCConfigPath:   scConfigPath,
		Metadata:       make(map[string]interface{}),
	}

	// Add tech stack info if available
	if projectInfo.PrimaryStack != nil {
		// Convert dependencies to strings
		deps := make([]string, len(projectInfo.PrimaryStack.Dependencies))
		for i, dep := range projectInfo.PrimaryStack.Dependencies {
			deps[i] = dep.Name
		}

		context.TechStack = &TechStackInfo{
			Language:     projectInfo.PrimaryStack.Language,
			Framework:    projectInfo.PrimaryStack.Framework,
			Runtime:      projectInfo.PrimaryStack.Runtime,
			Dependencies: deps,
			Architecture: projectInfo.Architecture,
			Confidence:   projectInfo.PrimaryStack.Confidence,
			Metadata:     projectInfo.PrimaryStack.Metadata,
		}
	}

	return context, nil
}

func (h *DefaultMCPHandler) GetSupportedResources(ctx context.Context) (*SupportedResourcesResult, error) {
	// Simplified implementation - return basic resource info
	resources := []*ResourceInfo{
		{Type: "s3-bucket", Name: "S3Bucket", Provider: "aws", Description: "AWS S3 bucket configuration"},
		{Type: "gcp-bucket", Name: "GcpBucket", Provider: "gcp", Description: "Google Cloud Platform bucket configuration"},
		{Type: "kubernetes", Name: "KubernetesConfig", Provider: "kubernetes", Description: "Kubernetes configuration"},
		{Type: "mongodb-atlas", Name: "AtlasConfig", Provider: "mongodb", Description: "MongoDB Atlas configuration"},
	}

	providers := []*ProviderInfo{
		{Name: "aws", DisplayName: "Amazon Web Services", Resources: []string{"s3-bucket"}},
		{Name: "gcp", DisplayName: "Google Cloud Platform", Resources: []string{"gcp-bucket"}},
		{Name: "kubernetes", DisplayName: "Kubernetes", Resources: []string{"kubernetes"}},
		{Name: "mongodb", DisplayName: "MongoDB Atlas", Resources: []string{"mongodb-atlas"}},
	}

	// Convert to slices (not pointers)
	resourceSlice := make([]ResourceInfo, len(resources))
	for i, r := range resources {
		resourceSlice[i] = *r
	}

	providerSlice := make([]ProviderInfo, len(providers))
	for i, p := range providers {
		providerSlice[i] = *p
	}

	return &SupportedResourcesResult{
		Resources: resourceSlice,
		Providers: providerSlice,
		Total:     len(resources),
	}, nil
}

// Simplified interface - remove methods we don't need
func (h *DefaultMCPHandler) GenerateConfiguration(ctx context.Context, params GenerateConfigurationParams) (*GeneratedConfiguration, error) {
	return nil, fmt.Errorf("method not implemented")
}

func (h *DefaultMCPHandler) AnalyzeProject(ctx context.Context, params AnalyzeProjectParams) (*ProjectAnalysis, error) {
	// Use existing project analysis
	analyzer := analysis.NewProjectAnalyzer()
	projectInfo, err := analyzer.AnalyzeProject(params.Path)
	if err != nil {
		return nil, fmt.Errorf("project analysis failed: %w", err)
	}

	// Convert to MCP ProjectAnalysis format
	result := &ProjectAnalysis{
		Path:            params.Path,
		TechStacks:      []TechStackInfo{}, // Convert from projectInfo.TechStacks
		Architecture:    projectInfo.Architecture,
		Recommendations: []Recommendation{}, // Generate recommendations based on analysis
		Files:           []FileInfo{},       // Convert from projectInfo.Files if needed
		Timestamp:       time.Now(),
		Metadata:        make(map[string]interface{}),
	}

	// Add primary tech stack if available
	if projectInfo.PrimaryStack != nil {
		// Convert dependencies to strings
		deps := make([]string, len(projectInfo.PrimaryStack.Dependencies))
		for i, dep := range projectInfo.PrimaryStack.Dependencies {
			deps[i] = dep.Name
		}

		techStack := TechStackInfo{
			Language:     projectInfo.PrimaryStack.Language,
			Framework:    projectInfo.PrimaryStack.Framework,
			Runtime:      projectInfo.PrimaryStack.Runtime,
			Dependencies: deps,
			Architecture: projectInfo.Architecture,
			Confidence:   projectInfo.PrimaryStack.Confidence,
			Metadata:     projectInfo.PrimaryStack.Metadata,
		}
		result.TechStacks = append(result.TechStacks, techStack)
	}

	// Add all tech stacks
	for _, stack := range projectInfo.TechStacks {
		deps := make([]string, len(stack.Dependencies))
		for i, dep := range stack.Dependencies {
			deps[i] = dep.Name
		}

		techStack := TechStackInfo{
			Language:     stack.Language,
			Framework:    stack.Framework,
			Runtime:      stack.Runtime,
			Dependencies: deps,
			Architecture: projectInfo.Architecture,
			Confidence:   stack.Confidence,
			Metadata:     stack.Metadata,
		}
		result.TechStacks = append(result.TechStacks, techStack)
	}

	// Convert recommendations
	for _, rec := range projectInfo.Recommendations {
		recommendation := Recommendation{
			Type:        rec.Type,
			Category:    rec.Category,
			Priority:    rec.Priority,
			Title:       rec.Title,
			Description: rec.Description,
			Action:      rec.Action,
		}
		result.Recommendations = append(result.Recommendations, recommendation)
	}

	// Add metadata
	result.Metadata["analyzed_at"] = time.Now()
	result.Metadata["analyzer_version"] = "1.0"
	result.Metadata["total_files"] = len(projectInfo.Files)

	return result, nil
}

func (h *DefaultMCPHandler) GetCapabilities(ctx context.Context) (map[string]interface{}, error) {
	return map[string]interface{}{
		"name":        MCPName,
		"version":     MCPVersion,
		"description": "Simple Container AI Assistant - provides documentation search, project analysis, and resource information",
		"methods": []string{
			"ping",
			"tools/list",
			"tools/call",
			"resources/list",
			"resources/read",
		},
		"features": map[string]interface{}{
			"documentation_search": true,
			"project_analysis":     true,
			"resource_catalog":     true,
		},
	}, nil
}

func (h *DefaultMCPHandler) Ping(ctx context.Context) (string, error) {
	return "pong", nil
}
