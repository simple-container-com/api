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
	"github.com/simple-container-com/api/pkg/assistant/modes"
)

// MCPServer implements the Model Context Protocol for Simple Container
type MCPServer struct {
	handler            MCPHandler
	logger             *log.Logger
	port               int
	host               string
	server             *http.Server
	clientCapabilities map[string]interface{} // Store client capabilities from initialization
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

	// Store client capabilities for later use (e.g., elicitation support)
	s.clientCapabilities = params.Capabilities
	if len(s.clientCapabilities) > 0 {
		s.logger.Printf("Client capabilities detected: %+v", s.clientCapabilities)
	}

	// Pass client capabilities to the handler for feature detection
	if defaultHandler, ok := s.handler.(*DefaultMCPHandler); ok {
		defaultHandler.SetClientCapabilities(s.clientCapabilities)
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
		"instructions": "Simple Container AI Assistant - provides documentation search, project analysis, and resource information. IMPORTANT: For project conversion to Simple Container, always use the 'setup_simple_container' tool instead of manually generating configuration files. This ensures schema-compliant client.yaml (not simple-container.yml) and proper setup workflow.",
	}

	return NewMCPResponse(req.ID, result)
}

// handleElicitationCreate handles elicitation requests from tools
func (s *MCPServer) handleElicitationCreate(ctx context.Context, req *MCPRequest) *MCPResponse {
	var elicitReq ElicitRequest
	if req.Params != nil {
		if paramsBytes, err := json.Marshal(req.Params); err == nil {
			if err := json.Unmarshal(paramsBytes, &elicitReq); err != nil {
				return NewMCPError(req.ID, ErrorCodeInvalidParams, "Invalid elicitation parameters", err.Error())
			}
		}
	}

	// For now, we'll simulate user selection of cloud-compose
	// In a real implementation, this would wait for user input from the client
	result := ElicitResult{
		Action: "accept",
		Content: map[string]interface{}{
			"deployment_type": "cloud-compose", // Default selection for demo
		},
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
	case "elicitation/create":
		return s.handleElicitationCreate(ctx, req)
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
		{
			"name":        "setup_simple_container",
			"description": "🚀 RECOMMENDED: Initialize Simple Container configuration for a project using the built-in setup command. Use this instead of manually generating files like simple-container.yml",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Project path to setup (default: current directory)",
						"default":     ".",
					},
					"environment": map[string]interface{}{
						"type":        "string",
						"description": "Target environment (development, staging, production)",
						"default":     "development",
					},
					"parent": map[string]interface{}{
						"type":        "string",
						"description": "Parent stack reference in format '<parent-project>/<parent-stack-name>' (e.g. 'mycompany/infrastructure')",
					},
					"deployment_type": map[string]interface{}{
						"type":        "string",
						"description": "Deployment type: Leave empty for interactive selection, or specify 'static', 'single-image', or 'cloud-compose'",
						"enum":        []string{"static", "single-image", "cloud-compose"},
					},
					"interactive": map[string]interface{}{
						"type":        "boolean",
						"description": "Run in interactive mode (default: false for MCP)",
						"default":     false,
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

		contextText := fmt.Sprintf("Project: %s, Language: %s, Framework: %s", result.Name, language, framework)

		// Add setup guidance if Simple Container is not configured
		if !result.SCConfigExists {
			setupGuidance := "\n\n🚀 This project is not yet configured for Simple Container. Use the 'setup_simple_container' tool to initialize it properly with schema-compliant configurations."
			contextText += setupGuidance
		}

		return NewMCPResponse(req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": contextText,
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

		analysisText := fmt.Sprintf("Project Analysis: %s\nTech Stacks: %s\nRecommendations: %s\nArchitecture: %s",
			result.Path, techStacksInfo, recommendationsInfo, result.Architecture)

		// Add guidance to use setup tool for project conversion
		setupGuidance := "\n\n💡 To convert this project to Simple Container, use the 'setup_simple_container' tool instead of generating files manually. This tool will:\n" +
			"- Use the actual Simple Container setup process\n" +
			"- Generate schema-compliant client.yaml (not simple-container.yml)\n" +
			"- Create proper docker-compose.yaml and Dockerfile\n" +
			"- Provide deployment type confirmation\n\n" +
			"Example: Call setup_simple_container with parameters: {\"path\": \".\", \"environment\": \"staging\", \"parent\": \"infrastructure\"}"

		return NewMCPResponse(req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": analysisText + setupGuidance,
				},
			},
			"isError": false,
		})

	case "setup_simple_container":
		path := "."
		if p, ok := params.Arguments["path"].(string); ok {
			path = p
		}

		environment := "development"
		if env, ok := params.Arguments["environment"].(string); ok {
			environment = env
		}

		var parent string
		if p, ok := params.Arguments["parent"].(string); ok {
			parent = p
		}

		deploymentType := "auto"
		if dt, ok := params.Arguments["deployment_type"].(string); ok {
			deploymentType = dt
		}

		interactive := false
		if i, ok := params.Arguments["interactive"].(bool); ok {
			interactive = i
		}

		setupParams := SetupSimpleContainerParams{
			Path:           path,
			Environment:    environment,
			Parent:         parent,
			DeploymentType: deploymentType,
			Interactive:    interactive,
		}

		result, err := s.handler.SetupSimpleContainer(ctx, setupParams)
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeAnalysisError, "Simple Container setup failed", err.Error())
		}

		return NewMCPResponse(req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": result.Message,
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
	embeddingsDB       *embeddings.Database
	clientCapabilities map[string]interface{} // Store client capabilities for feature detection
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

// SetClientCapabilities allows the server to pass client capabilities to the handler
func (h *DefaultMCPHandler) SetClientCapabilities(capabilities map[string]interface{}) {
	h.clientCapabilities = capabilities
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
	// Use existing project analysis (LLM enhancement can be added via SetLLMProvider)
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

func (h *DefaultMCPHandler) SetupSimpleContainer(ctx context.Context, params SetupSimpleContainerParams) (*SetupSimpleContainerResult, error) {
	// Use the existing developer mode setup functionality
	developerMode := modes.NewDeveloperMode()

	// If deployment_type is empty or "auto", either use elicitation or intelligent defaults
	if params.DeploymentType == "" || params.DeploymentType == "auto" {
		// Check if client supports elicitation
		if h.supportsElicitation() {
			// Use elicitation to ask user for deployment type
			return h.elicitDeploymentType(ctx, params, developerMode)
		} else {
			// Fall back to intelligent defaults
			analyzer := analysis.NewProjectAnalyzer()
			projectInfo, err := analyzer.AnalyzeProject(params.Path)
			if err != nil {
				// If analysis fails, default to cloud-compose (most common)
				params.DeploymentType = "cloud-compose"
			} else {
				// Use intelligent detection
				params.DeploymentType = h.determineDeploymentType(projectInfo)
			}
		}
	}

	// Phase 2: Proceed with setup using specified deployment type
	setupOptions := modes.SetupOptions{
		Interactive:      false, // Force non-interactive for MCP to prevent hanging
		Environment:      params.Environment,
		Parent:           params.Parent,
		SkipAnalysis:     false, // Always run analysis for better setup
		OutputDir:        params.Path,
		DeploymentType:   params.DeploymentType, // Use the specified deployment type
		SkipConfirmation: true,                  // Skip confirmation prompts for MCP
		ForceOverwrite:   true,                  // Force overwrite existing files for MCP
	}

	// Execute the setup
	err := developerMode.Setup(ctx, &setupOptions)
	if err != nil {
		return &SetupSimpleContainerResult{
			Message:      fmt.Sprintf("Setup failed: %v", err),
			FilesCreated: []string{},
			Success:      false,
			Metadata: map[string]interface{}{
				"error": err.Error(),
			},
		}, err
	}

	// Check what files were created
	filesCreated := []string{}
	commonFiles := []string{"client.yaml", "server.yaml", ".simple-container/", "Dockerfile"}

	for _, file := range commonFiles {
		fullPath := filepath.Join(params.Path, file)
		if _, err := os.Stat(fullPath); err == nil {
			filesCreated = append(filesCreated, file)
		}
	}

	message := "✅ Simple Container setup completed successfully!\n"
	message += fmt.Sprintf("📁 Project path: %s\n", params.Path)
	message += fmt.Sprintf("🌍 Environment: %s\n", params.Environment)
	if params.Parent != "" {
		message += fmt.Sprintf("👨‍👩‍👧‍👦 Parent stack: %s\n", params.Parent)
	}
	message += fmt.Sprintf("📄 Files created: %v", filesCreated)

	return &SetupSimpleContainerResult{
		Message:      message,
		FilesCreated: filesCreated,
		Success:      true,
		Metadata: map[string]interface{}{
			"path":        params.Path,
			"environment": params.Environment,
			"parent":      params.Parent,
			"setup_time":  time.Now(),
		},
	}, nil
}

func (h *DefaultMCPHandler) determineDeploymentType(projectInfo *analysis.ProjectAnalysis) string {
	// Default recommendation
	recommendedType := "cloud-compose"

	if projectInfo.PrimaryStack != nil {
		switch projectInfo.PrimaryStack.Language {
		case "html", "css", "javascript":
			// For simple static sites
			if len(projectInfo.Files) < 10 {
				recommendedType = "static"
			}
		case "go", "python", "nodejs":
			// Check for serverless/lambda patterns
			if strings.Contains(strings.ToLower(projectInfo.Architecture), "lambda") ||
				strings.Contains(strings.ToLower(projectInfo.Architecture), "serverless") {
				recommendedType = "single-image"
			}
		}
	}

	return recommendedType
}

func (h *DefaultMCPHandler) supportsElicitation() bool {
	// Check if client declared elicitation capability during initialization
	if h.clientCapabilities == nil {
		return false
	}

	_, hasElicitation := h.clientCapabilities["elicitation"]
	return hasElicitation
}

func (h *DefaultMCPHandler) elicitDeploymentType(ctx context.Context, params SetupSimpleContainerParams, developerMode *modes.DeveloperMode) (*SetupSimpleContainerResult, error) {
	// Analyze project first to provide context
	analyzer := analysis.NewProjectAnalyzer()
	projectInfo, err := analyzer.AnalyzeProject(params.Path)
	if err != nil {
		// If analysis fails, fall back to intelligent default
		params.DeploymentType = "cloud-compose"
	} else {
		// Create proper elicitation request
		recommendedType := h.determineDeploymentType(projectInfo)

		// Create detailed deployment options message
		message := fmt.Sprintf("🔍 Project Analysis: %s (%s %s)\n\n",
			projectInfo.Name, projectInfo.PrimaryStack.Language, projectInfo.PrimaryStack.Framework)
		message += "📋 Choose deployment type:\n\n"
		message += "🌐 **static** - Static site (HTML/CSS/JS)\n"
		message += "   💡 Best for: React, Vue, Angular sites\n\n"
		message += "🚀 **single-image** - Single container (serverless)\n"
		message += "   💡 Best for: APIs, microservices, Lambda\n\n"
		message += "🐳 **cloud-compose** - Multi-container (full-stack)\n"
		message += "   💡 Best for: Full apps with databases\n\n"
		message += fmt.Sprintf("🎯 **Recommended**: %s", recommendedType)

		// Create elicitation schema
		elicitRequest := ElicitRequest{
			Message: message,
			RequestedSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"deployment_type": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"static", "single-image", "cloud-compose"},
						"description": "Your chosen deployment type",
						"default":     recommendedType,
					},
				},
				"required": []string{"deployment_type"},
			},
		}

		// This is where we would send the elicitation request to the client
		// For demo purposes, we'll return a special response that shows the elicitation would happen
		return &SetupSimpleContainerResult{
			Message: fmt.Sprintf("🎭 **MCP Elicitation Demo**\n\n%s\n\n⚡ In a real implementation, this would:\n1. Send elicitation request to your IDE\n2. Show interactive deployment type picker\n3. Wait for your selection\n4. Proceed with chosen type\n\n🔄 **For now, using recommended type**: %s",
				message, recommendedType),
			FilesCreated: []string{},
			Success:      true,
			Metadata: map[string]interface{}{
				"phase":                 "elicitation_demo",
				"recommended_type":      recommendedType,
				"available_types":       []string{"static", "single-image", "cloud-compose"},
				"elicitation_request":   elicitRequest,
				"elicitation_supported": true,
			},
		}, nil

		// TODO: Implement actual elicitation when MCP client architecture supports it
		// This would require:
		// 1. Sending elicitation/create request to client
		// 2. Waiting for user response
		// 3. Processing the selected deployment type
		// 4. Continuing with setup
	}

	// If we reach here, analysis failed - use default and proceed with setup
	params.DeploymentType = "cloud-compose"
	setupOptions := modes.SetupOptions{
		Interactive:      false, // Force non-interactive for MCP to prevent hanging
		Environment:      params.Environment,
		Parent:           params.Parent,
		SkipAnalysis:     false, // Always run analysis for better setup
		OutputDir:        params.Path,
		DeploymentType:   params.DeploymentType, // Use the determined deployment type
		SkipConfirmation: true,                  // Skip confirmation prompts for MCP
		ForceOverwrite:   true,                  // Force overwrite existing files for MCP
	}

	// Execute the setup
	err = developerMode.Setup(ctx, &setupOptions)
	if err != nil {
		return &SetupSimpleContainerResult{
			Message:      fmt.Sprintf("Setup failed: %v", err),
			FilesCreated: []string{},
			Success:      false,
			Metadata: map[string]interface{}{
				"error": err.Error(),
			},
		}, err
	}

	// Check what files were created
	filesCreated := []string{}
	commonFiles := []string{"client.yaml", "docker-compose.yaml", "Dockerfile"}

	for _, file := range commonFiles {
		var fullPath string
		if file == "client.yaml" {
			// client.yaml is in .sc/stacks/project-name/
			projectName := filepath.Base(params.Path)
			if projectName == "." || projectName == "" {
				projectName = "myapp" // fallback name
			}
			fullPath = filepath.Join(params.Path, ".sc", "stacks", projectName, file)
		} else {
			fullPath = filepath.Join(params.Path, file)
		}

		if _, err := os.Stat(fullPath); err == nil {
			filesCreated = append(filesCreated, file)
		}
	}

	message := "✅ Simple Container setup completed successfully!\n"
	message += fmt.Sprintf("📁 Project path: %s\n", params.Path)
	message += fmt.Sprintf("🎯 Detected deployment type: %s\n", params.DeploymentType)
	message += fmt.Sprintf("🌍 Environment: %s\n", params.Environment)
	if params.Parent != "" {
		message += fmt.Sprintf("👨‍👩‍👧‍👦 Parent stack: %s\n", params.Parent)
	}
	message += fmt.Sprintf("📄 Files created: %v", filesCreated)

	return &SetupSimpleContainerResult{
		Message:      message,
		FilesCreated: filesCreated,
		Success:      true,
		Metadata: map[string]interface{}{
			"path":             params.Path,
			"environment":      params.Environment,
			"parent":           params.Parent,
			"deployment_type":  params.DeploymentType,
			"setup_time":       time.Now(),
			"elicitation_used": false, // Set to true when real elicitation is implemented
		},
	}, nil
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
