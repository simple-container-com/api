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
	"strconv"
	"strings"
	"time"

	"github.com/simple-container-com/api/docs"
	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/analysis"
	"github.com/simple-container-com/api/pkg/assistant/chat"
	"github.com/simple-container-com/api/pkg/assistant/core"
	"github.com/simple-container-com/api/pkg/assistant/embeddings"
	"github.com/simple-container-com/api/pkg/assistant/modes"
)

// Context key types to avoid collisions
type contextKey string

const (
	contextKeyRequestID  contextKey = "request_id"
	contextKeyUserAgent  contextKey = "user_agent"
	contextKeyRemoteAddr contextKey = "remote_addr"
	contextKeyMCPMethod  contextKey = "mcp_method"
	contextKeyClientID   contextKey = "client_id"
)

// MCPServer implements the Model Context Protocol for Simple Container
type MCPServer struct {
	handler            MCPHandler
	logger             *MCPLogger
	fallbackLogger     *log.Logger
	port               int
	host               string
	server             *http.Server
	clientCapabilities map[string]interface{} // Store client capabilities from initialization
}

// NewMCPServer creates a new MCP server instance with mode-aware logging
func NewMCPServer(host string, port int, mode MCPMode, verboseMode bool, chatInterface *chat.ChatInterface) *MCPServer {
	// Try to create enhanced JSON logger with mode awareness
	mcpLogger, err := NewMCPLogger("mcp-server", mode, verboseMode)
	fallbackLogger := log.New(os.Stderr, "MCP: ", log.LstdFlags)

	if err != nil {
		fallbackLogger.Printf("Warning: Failed to initialize MCP JSON logger: %v - falling back to standard logger", err)
	} else {
		ctx := context.Background()
		mcpLogger.Info(ctx, "MCP Server initializing - host: %s, port: %d, mode: %s, verbose: %v, log file: %s",
			host, port, string(mode), verboseMode, mcpLogger.GetLogFilePath())
	}

	return &MCPServer{
		handler:        NewDefaultMCPHandler(chatInterface),
		logger:         mcpLogger,
		fallbackLogger: fallbackLogger,
		host:           host,
		port:           port,
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
	fmt.Printf("💚 Health check: http://%s/health\n", s.server.Addr)

	// Log server startup
	if s.logger != nil {
		s.logger.Info(ctx, "MCP Server started successfully on %s", s.server.Addr)
		fmt.Printf("📝 Logs: %s\n\n", s.logger.GetLogFilePath())
	} else {
		fmt.Println()
	}

	// Start server in goroutine
	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			if s.logger != nil {
				s.logger.Error(ctx, "MCP Server error: %v", err)
			} else {
				s.fallbackLogger.Printf("MCP Server error: %v", err)
			}
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	fmt.Println("\n🛑 Shutting down MCP server...")
	if s.logger != nil {
		s.logger.Info(ctx, "MCP Server shutdown initiated")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = s.server.Shutdown(shutdownCtx)
	if s.logger != nil {
		if err != nil {
			s.logger.Error(ctx, "MCP Server shutdown error: %v", err)
		} else {
			s.logger.Info(ctx, "MCP Server shutdown completed successfully")
		}
		s.logger.Close()
	}

	return err
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
		if s.logger != nil {
			s.logger.Info(ctx, "Client capabilities detected: %+v", s.clientCapabilities)
		} else {
			s.fallbackLogger.Printf("Client capabilities detected: %+v", s.clientCapabilities)
		}
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

	// Enrich context with HTTP request information for enhanced logging
	ctx := r.Context()

	// Extract request ID as string for context
	requestID := ""
	if id, ok := req.ID.(string); ok {
		requestID = id
	}

	ctx = context.WithValue(ctx, contextKeyRequestID, requestID)
	ctx = context.WithValue(ctx, contextKeyUserAgent, r.UserAgent())
	ctx = context.WithValue(ctx, contextKeyRemoteAddr, r.RemoteAddr)
	ctx = context.WithValue(ctx, contextKeyMCPMethod, req.Method)

	response := s.processRequest(ctx, &req)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// processRequest routes MCP requests to appropriate handlers with enhanced logging
func (s *MCPServer) processRequest(ctx context.Context, req *MCPRequest) *MCPResponse {
	startTime := time.Now()

	// Log incoming request
	if s.logger != nil {
		s.logger.Debug(ctx, "Processing MCP request: %s", req.Method)
	}

	var response *MCPResponse

	switch req.Method {
	// Standard MCP methods only
	case "ping":
		response = s.handlePing(ctx, req)
	case "elicitation/create":
		response = s.handleElicitationCreate(ctx, req)
	case "tools/list":
		response = s.handleListTools(ctx, req)
	case "tools/call":
		response = s.handleCallTool(ctx, req)
	case "resources/list":
		response = s.handleListResources(ctx, req)
	case "resources/read":
		response = s.handleReadResource(ctx, req)
	default:
		response = NewMCPError(req.ID, ErrorCodeMethodNotFound, fmt.Sprintf("Method '%s' not found", req.Method), nil)
	}

	// Log request completion with timing and enhanced context
	duration := time.Since(startTime)
	if s.logger != nil {
		if response.Error != nil {
			s.logger.LogMCPError(req.Method, fmt.Errorf("MCP error: %v", response.Error), map[string]interface{}{
				"request_id": req.ID,
				"duration":   duration.String(),
			})
		} else {
			requestID := ""
			if id, ok := req.ID.(string); ok {
				requestID = id
			}
			s.logger.LogMCPRequest(req.Method, req.Params, duration, requestID)
		}
	}

	return response
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
		{
			"name":        "get_current_config",
			"description": "📖 Read and parse existing Simple Container configuration files (client.yaml or server.yaml)",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"config_type": map[string]interface{}{
						"type":        "string",
						"description": "Type of configuration to read: 'client' or 'server'",
						"enum":        []string{"client", "server"},
					},
					"stack_name": map[string]interface{}{
						"type":        "string",
						"description": "For client.yaml, specific stack name to focus on (optional)",
					},
				},
				"required": []string{"config_type"},
			},
		},
		{
			"name":        "add_environment",
			"description": "🌍 Add new environment/stack to client.yaml (e.g., add 'prod' environment)",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"stack_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the new stack/environment (e.g., 'prod', 'staging')",
					},
					"deployment_type": map[string]interface{}{
						"type":        "string",
						"description": "Deployment type for the new stack",
						"enum":        []string{"static", "single-image", "cloud-compose"},
					},
					"parent": map[string]interface{}{
						"type":        "string",
						"description": "Parent stack reference in format '<parent-project>/<parent-stack-name>'",
					},
					"parent_env": map[string]interface{}{
						"type":        "string",
						"description": "Parent environment to map to (e.g., 'prod', 'staging')",
					},
					"config": map[string]interface{}{
						"type":        "object",
						"description": "Additional configuration for the new stack (optional)",
					},
				},
				"required": []string{"stack_name", "deployment_type", "parent", "parent_env"},
			},
		},
		{
			"name":        "modify_stack_config",
			"description": "⚙️ Modify existing stack configuration in client.yaml (e.g., change deployment type, update scaling)",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"stack_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the stack to modify",
					},
					"changes": map[string]interface{}{
						"type":        "object",
						"description": "Configuration changes to apply (e.g., {'type': 'single-image', 'config.scale.max': 10})",
					},
				},
				"required": []string{"stack_name", "changes"},
			},
		},
		{
			"name":        "add_resource",
			"description": "🗄️ Add new resource to server.yaml (e.g., add MongoDB Atlas cluster, Redis cache)",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"resource_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the resource (e.g., 'mongodb-prod', 'redis-cache')",
					},
					"resource_type": map[string]interface{}{
						"type":        "string",
						"description": "Type of resource (e.g., 'mongodb-atlas', 'redis', 'postgres')",
					},
					"environment": map[string]interface{}{
						"type":        "string",
						"description": "Environment to add the resource to (e.g., 'prod', 'staging')",
					},
					"config": map[string]interface{}{
						"type":        "object",
						"description": "Resource configuration (e.g., {'tier': 'M10', 'region': 'us-east-1'})",
					},
				},
				"required": []string{"resource_name", "resource_type", "environment", "config"},
			},
		},
		{
			"name":        "read_project_file",
			"description": "📄 Read and display a project file (Dockerfile, docker-compose.yaml, etc.) with security obfuscation for sensitive content",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"filename": map[string]interface{}{
						"type":        "string",
						"description": "Name of the file to read (e.g., 'Dockerfile', 'docker-compose.yaml', 'client.yaml')",
					},
				},
				"required": []string{"filename"},
			},
		},
		{
			"name":        "show_stack_config",
			"description": "📋 Show stack configuration (checks both client.yaml and server.yaml) with comprehensive analysis",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"stack_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the stack to show configuration for",
					},
					"config_type": map[string]interface{}{
						"type":        "string",
						"description": "Type of configuration to read: 'client' or 'server' (default: both)",
						"enum":        []string{"client", "server"},
					},
				},
				"required": []string{"stack_name"},
			},
		},
		{
			"name":        "advanced_search_documentation",
			"description": "🔍 Advanced documentation search with LLM integration - allows active search when more context is needed",
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
			"name":        "get_help",
			"description": "❓ Get help information about available tools and their usage",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"tool_name": map[string]interface{}{
						"type":        "string",
						"description": "Specific tool to get help for (optional - shows all tools if not specified)",
					},
				},
			},
		},
		{
			"name":        "get_status",
			"description": "📊 Get current Simple Container project status and diagnostic information",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"detailed": map[string]interface{}{
						"type":        "boolean",
						"description": "Show detailed diagnostic information (default: false)",
						"default":     false,
					},
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Project path to analyze (default: current directory)",
						"default":     ".",
					},
				},
			},
		},
		{
			"name":        "write_project_file",
			"description": "✍️ Write content to a project file (create new or modify existing)",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"filename": map[string]interface{}{
						"type":        "string",
						"description": "File name to write (e.g., Dockerfile, docker-compose.yaml)",
					},
					"content": map[string]interface{}{
						"type":        "string",
						"description": "Content to write to the file",
					},
					"lines": map[string]interface{}{
						"type":        "string",
						"description": "Line range to replace (e.g., '10-20' or '5' for single line) - optional",
					},
					"append": map[string]interface{}{
						"type":        "boolean",
						"description": "Append content to end of file instead of replacing",
						"default":     false,
					},
				},
				"required": []string{"filename", "content"},
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

	// Add timeout for tool calls to prevent hanging (30 seconds for analysis, 15 for others)
	timeout := 15 * time.Second
	if params.Name == "analyze_project" || params.Name == "setup_simple_container" {
		timeout = 30 * time.Second
	}
	
	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Run tool call with timeout protection
	resultChan := make(chan *MCPResponse, 1)
	go func() {
		resultChan <- s.executeToolCall(ctxWithTimeout, req, params.Name, params.Arguments)
	}()

	select {
	case result := <-resultChan:
		return result
	case <-ctxWithTimeout.Done():
		if s.logger != nil {
			s.logger.Error(ctx, "Tool call timeout: %s exceeded %v", params.Name, timeout)
		}
		return NewMCPError(req.ID, ErrorCodeTimeout, 
			fmt.Sprintf("Tool call '%s' exceeded timeout of %v. This may indicate a performance issue.", params.Name, timeout),
			map[string]interface{}{
				"tool":    params.Name,
				"timeout": timeout.String(),
				"hint":    "Try using cached mode or check system resources",
			})
	}
}

func (s *MCPServer) executeToolCall(ctx context.Context, req *MCPRequest, toolName string, arguments map[string]interface{}) *MCPResponse {
	switch toolName {
	case "search_documentation":
		// Convert arguments to SearchDocumentationParams
		query, _ := arguments["query"].(string)
		limit := 5
		if l, ok := arguments["limit"].(float64); ok {
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

		// Prepare response text
		responseText := fmt.Sprintf("Found %d documentation results for '%s'", result.Total, query)
		if result.Message != "" {
			responseText = result.Message
		}

		return NewMCPResponse(req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": responseText,
				},
			},
			"isError": false,
		})

	case "get_project_context":
		path := "."
		if p, ok := arguments["path"].(string); ok {
			path = p
		}

		// Resolve path to absolute path to ensure we work in the correct directory
		absPath, err := filepath.Abs(path)
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeProjectNotFound, "Invalid path", fmt.Sprintf("Failed to resolve path '%s': %v", path, err))
		}
		path = absPath

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

		// Return full structured data for tools, content format for display
		return NewMCPResponse(req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Simple Container supports %d resource types across %d providers", result.Total, len(result.Providers)),
				},
			},
			"isError":   false,
			"resources": result.Resources,
			"providers": result.Providers,
			"total":     result.Total,
		})

	case "analyze_project":
		path := "."
		if p, ok := arguments["path"].(string); ok {
			path = p
		}

		// Resolve path to absolute path to ensure we work in the correct directory
		absPath, err := filepath.Abs(path)
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeAnalysisError, "Invalid path", fmt.Sprintf("Failed to resolve path '%s': %v", path, err))
		}
		path = absPath

		analyzeParams := AnalyzeProjectParams{
			Path: path,
		}

		result, err := s.handler.AnalyzeProject(ctx, analyzeParams)
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeAnalysisError, "Project analysis failed", err.Error())
		}

		// Build detailed analysis text with comprehensive information
		var analysisText strings.Builder

		// Project Overview
		analysisText.WriteString(fmt.Sprintf("# 📊 Project Analysis: %s\n", filepath.Base(result.Path)))
		analysisText.WriteString(fmt.Sprintf("**Path:** `%s`\n", result.Path))
		analysisText.WriteString(fmt.Sprintf("**Architecture:** %s\n\n", result.Architecture))

		// Tech Stack Details
		analysisText.WriteString("## 🔧 Technology Stacks\n")
		if len(result.TechStacks) == 0 {
			analysisText.WriteString("❌ No technology stacks detected\n\n")
		} else {
			for i, stack := range result.TechStacks {
				analysisText.WriteString(fmt.Sprintf("### Stack %d\n", i+1))
				analysisText.WriteString(fmt.Sprintf("- **Language:** %s\n", stack.Language))
				analysisText.WriteString(fmt.Sprintf("- **Framework:** %s\n", stack.Framework))
				if stack.Runtime != "" {
					analysisText.WriteString(fmt.Sprintf("- **Runtime:** %s\n", stack.Runtime))
				}
				analysisText.WriteString(fmt.Sprintf("- **Confidence:** %.1f%%\n", stack.Confidence*100))

				if len(stack.Dependencies) > 0 {
					analysisText.WriteString("- **Dependencies:** ")
					for j, dep := range stack.Dependencies {
						if j > 0 {
							analysisText.WriteString(", ")
						}
						analysisText.WriteString(fmt.Sprintf("`%s`", dep))
					}
					analysisText.WriteString("\n")
				}
				analysisText.WriteString("\n")
			}
		}

		// Recommendations
		analysisText.WriteString("## 💡 Recommendations\n")
		if len(result.Recommendations) == 0 {
			analysisText.WriteString("✅ No specific recommendations at this time\n\n")
		} else {
			for i, rec := range result.Recommendations {
				analysisText.WriteString(fmt.Sprintf("### %d. %s\n", i+1, rec.Title))
				analysisText.WriteString(fmt.Sprintf("**Priority:** %s | **Category:** %s\n", rec.Priority, rec.Category))
				analysisText.WriteString(fmt.Sprintf("**Description:** %s\n", rec.Description))
				if rec.Action != "" {
					analysisText.WriteString(fmt.Sprintf("**Action:** %s\n", rec.Action))
				}
				analysisText.WriteString("\n")
			}
		}

		// Files Information
		if len(result.Files) > 0 {
			analysisText.WriteString("## 📁 Project Files\n")
			analysisText.WriteString(fmt.Sprintf("**Total Files:** %d\n", len(result.Files)))

			// Group files by type
			fileTypes := make(map[string]int)
			for _, file := range result.Files {
				ext := filepath.Ext(file.Path)
				if ext == "" {
					ext = "no extension"
				}
				fileTypes[ext]++
			}

			analysisText.WriteString("**File Types:** ")
			first := true
			for ext, count := range fileTypes {
				if !first {
					analysisText.WriteString(", ")
				}
				analysisText.WriteString(fmt.Sprintf("%s (%d)", ext, count))
				first = false
			}
			analysisText.WriteString("\n\n")
		}

		// Metadata
		if len(result.Metadata) > 0 {
			analysisText.WriteString("## 🔍 Analysis Details\n")
			if analyzedAt, ok := result.Metadata["analyzed_at"]; ok {
				analysisText.WriteString(fmt.Sprintf("**Analyzed:** %v\n", analyzedAt))
			}
			if totalFiles, ok := result.Metadata["total_files"]; ok {
				analysisText.WriteString(fmt.Sprintf("**Total Files Scanned:** %v\n", totalFiles))
			}
			if version, ok := result.Metadata["analyzer_version"]; ok {
				analysisText.WriteString(fmt.Sprintf("**Analyzer Version:** %v\n", version))
			}
			analysisText.WriteString("\n")
		}

		// Setup guidance
		analysisText.WriteString("## 🚀 Next Steps\n")
		analysisText.WriteString("To convert this project to Simple Container, use the **setup_simple_container** tool:\n\n")
		analysisText.WriteString("```json\n")
		analysisText.WriteString("{\n")
		analysisText.WriteString("  \"path\": \".\",\n")
		analysisText.WriteString("  \"environment\": \"staging\",\n")
		analysisText.WriteString("  \"parent\": \"infrastructure\"\n")
		analysisText.WriteString("}\n")
		analysisText.WriteString("```\n\n")
		analysisText.WriteString("This will:\n")
		analysisText.WriteString("- ✅ Use the actual Simple Container setup process\n")
		analysisText.WriteString("- ✅ Generate schema-compliant client.yaml\n")
		analysisText.WriteString("- ✅ Create proper docker-compose.yaml and Dockerfile\n")
		analysisText.WriteString("- ✅ Provide deployment type recommendations\n")

		return NewMCPResponse(req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": analysisText.String(),
				},
			},
			"isError":         false,
			"analysis_data":   result, // Include full structured data for programmatic access
			"tech_stacks":     result.TechStacks,
			"recommendations": result.Recommendations,
			"architecture":    result.Architecture,
			"files":           result.Files,
			"metadata":        result.Metadata,
		})

	case "setup_simple_container":
		path := "."
		if p, ok := arguments["path"].(string); ok {
			path = p
		}

		// Resolve path to absolute path to ensure we work in the correct directory
		absPath, err := filepath.Abs(path)
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeAnalysisError, "Invalid path", fmt.Sprintf("Failed to resolve path '%s': %v", path, err))
		}
		path = absPath

		environment := "development"
		if env, ok := arguments["environment"].(string); ok {
			environment = env
		}

		var parent string
		if p, ok := arguments["parent"].(string); ok {
			parent = p
		}

		deploymentType := "auto"
		if dt, ok := arguments["deployment_type"].(string); ok {
			deploymentType = dt
		}

		interactive := false
		if i, ok := arguments["interactive"].(bool); ok {
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

	case "get_current_config":
		configType := "client"
		var stackName string

		if ct, ok := arguments["config_type"].(string); ok {
			// Validate config_type - must be "client" or "server"
			switch ct {
			case "client", "server":
				configType = ct
			default:
				// If invalid config_type provided, treat it as potential stack_name
				// and default config_type to "client"
				if stackName == "" {
					stackName = ct
				}
				configType = "client" // Default to client for invalid config_type
			}
		}

		// Override with explicit stack_name if provided
		if sn, ok := arguments["stack_name"].(string); ok {
			stackName = sn
		}

		configParams := GetCurrentConfigParams{
			ConfigType: configType,
			StackName:  stackName,
		}

		result, err := s.handler.GetCurrentConfig(ctx, configParams)
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeAnalysisError, "Failed to read configuration", err.Error())
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

	case "add_environment":
		stackName, ok := arguments["stack_name"].(string)
		if !ok {
			return NewMCPError(req.ID, ErrorCodeInvalidParams, "stack_name is required", nil)
		}

		deploymentType, ok := arguments["deployment_type"].(string)
		if !ok {
			return NewMCPError(req.ID, ErrorCodeInvalidParams, "deployment_type is required", nil)
		}

		parent, ok := arguments["parent"].(string)
		if !ok {
			return NewMCPError(req.ID, ErrorCodeInvalidParams, "parent is required", nil)
		}

		parentEnv, ok := arguments["parent_env"].(string)
		if !ok {
			return NewMCPError(req.ID, ErrorCodeInvalidParams, "parent_env is required", nil)
		}

		var config map[string]interface{}
		if c, ok := arguments["config"].(map[string]interface{}); ok {
			config = c
		}

		envParams := AddEnvironmentParams{
			StackName:      stackName,
			DeploymentType: deploymentType,
			Parent:         parent,
			ParentEnv:      parentEnv,
			Config:         config,
		}

		result, err := s.handler.AddEnvironment(ctx, envParams)
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeAnalysisError, "Failed to add environment", err.Error())
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

	case "modify_stack_config":
		stackName, ok := arguments["stack_name"].(string)
		if !ok {
			return NewMCPError(req.ID, ErrorCodeInvalidParams, "stack_name is required", nil)
		}

		changes, ok := arguments["changes"].(map[string]interface{})
		if !ok {
			return NewMCPError(req.ID, ErrorCodeInvalidParams, "changes is required", nil)
		}

		modifyParams := ModifyStackConfigParams{
			StackName: stackName,
			Changes:   changes,
		}

		result, err := s.handler.ModifyStackConfig(ctx, modifyParams)
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeAnalysisError, "Failed to modify stack configuration", err.Error())
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

	case "add_resource":
		resourceName, ok := arguments["resource_name"].(string)
		if !ok {
			return NewMCPError(req.ID, ErrorCodeInvalidParams, "resource_name is required", nil)
		}

		resourceType, ok := arguments["resource_type"].(string)
		if !ok {
			return NewMCPError(req.ID, ErrorCodeInvalidParams, "resource_type is required", nil)
		}

		environment, ok := arguments["environment"].(string)
		if !ok {
			return NewMCPError(req.ID, ErrorCodeInvalidParams, "environment is required", nil)
		}

		config, ok := arguments["config"].(map[string]interface{})
		if !ok {
			return NewMCPError(req.ID, ErrorCodeInvalidParams, "config is required", nil)
		}

		resourceParams := AddResourceParams{
			ResourceName: resourceName,
			ResourceType: resourceType,
			Environment:  environment,
			Config:       config,
		}

		result, err := s.handler.AddResource(ctx, resourceParams)
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeAnalysisError, "Failed to add resource", err.Error())
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

	case "read_project_file":
		filename, ok := arguments["filename"].(string)
		if !ok || filename == "" {
			return NewMCPError(req.ID, ErrorCodeInvalidParams, "filename is required", nil)
		}

		result, err := s.handler.ReadProjectFile(ctx, ReadProjectFileParams{Filename: filename})
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeAnalysisError, "Failed to read project file", err.Error())
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

	case "show_stack_config":
		stackName, ok := arguments["stack_name"].(string)
		if !ok || stackName == "" {
			return NewMCPError(req.ID, ErrorCodeInvalidParams, "stack_name is required", nil)
		}

		configType := ""
		if ct, ok := arguments["config_type"].(string); ok {
			configType = ct
		}

		result, err := s.handler.ShowStackConfig(ctx, ShowStackConfigParams{
			StackName:  stackName,
			ConfigType: configType,
		})
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeAnalysisError, "Failed to show stack configuration", err.Error())
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

	case "advanced_search_documentation":
		query, ok := arguments["query"].(string)
		if !ok || query == "" {
			return NewMCPError(req.ID, ErrorCodeInvalidParams, "query is required", nil)
		}

		limit := 5
		if l, ok := arguments["limit"].(float64); ok {
			limit = int(l)
		}

		result, err := s.handler.AdvancedSearchDocumentation(ctx, AdvancedSearchDocumentationParams{
			Query: query,
			Limit: limit,
		})
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeEmbeddingError, "Advanced documentation search failed", err.Error())
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

	case "get_help":
		toolName := ""
		if tn, ok := arguments["tool_name"].(string); ok {
			toolName = tn
		}

		result, err := s.handler.GetHelp(ctx, GetHelpParams{ToolName: toolName})
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeAnalysisError, "Failed to get help", err.Error())
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

	case "get_status":
		detailed := false
		if d, ok := arguments["detailed"].(bool); ok {
			detailed = d
		}

		path := "."
		if p, ok := arguments["path"].(string); ok {
			path = p
		}

		result, err := s.handler.GetStatus(ctx, GetStatusParams{Detailed: detailed, Path: path})
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeAnalysisError, "Failed to get status", err.Error())
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

	case "write_project_file":
		filename, ok := arguments["filename"].(string)
		if !ok || filename == "" {
			return NewMCPError(req.ID, ErrorCodeInvalidParams, "filename is required", nil)
		}

		content, ok := arguments["content"].(string)
		if !ok {
			return NewMCPError(req.ID, ErrorCodeInvalidParams, "content is required", nil)
		}

		// Parse optional parameters
		lines := ""
		if l, ok := arguments["lines"].(string); ok {
			lines = l
		}

		append := false
		if a, ok := arguments["append"].(bool); ok {
			append = a
		}

		result, err := s.handler.WriteProjectFile(ctx, WriteProjectFileParams{
			Filename: filename,
			Content:  content,
			Lines:    lines,
			Append:   append,
		})
		if err != nil {
			return NewMCPError(req.ID, ErrorCodeFileOperationError, "File write failed", err.Error())
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
		return NewMCPError(req.ID, ErrorCodeMethodNotFound, fmt.Sprintf("Tool '%s' not found", toolName), nil)
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
	commandHandler     *core.UnifiedCommandHandler
	chat               *chat.ChatInterface
	clientCapabilities map[string]interface{} // Store client capabilities for feature detection
}

// NewDefaultMCPHandler creates a new default MCP handler with optional chat interface
func NewDefaultMCPHandler(chatInterface *chat.ChatInterface) MCPHandler {
	// Initialize unified command handler (should not fail with new robust implementation)
	commandHandler, err := core.NewUnifiedCommandHandler()
	if err != nil {
		// This should rarely happen now, but handle gracefully
		log.Printf("Warning: Failed to initialize command handler: %v", err)
		commandHandler = nil
	}

	// Create a default chat interface if none provided
	if chatInterface == nil {
		// Initialize with a default chat interface if needed
		// This ensures we always have a valid chat interface, even if it's a no-op implementation
		defaultChat := chat.ChatInterface{}
		chatInterface = &defaultChat
	}

	return &DefaultMCPHandler{
		commandHandler:     commandHandler,
		chat:               chatInterface,
		clientCapabilities: make(map[string]interface{}),
	}
}

// SetClientCapabilities allows the server to pass client capabilities to the handler
func (h *DefaultMCPHandler) SetClientCapabilities(capabilities map[string]interface{}) {
	h.clientCapabilities = capabilities
}

func (h *DefaultMCPHandler) SearchDocumentation(ctx context.Context, params SearchDocumentationParams) (*DocumentationSearchResult, error) {
	if h.commandHandler == nil {
		return &DocumentationSearchResult{
			Documents: []DocumentChunk{},
			Total:     0,
		}, nil
	}

	// Use unified command handler
	result, err := h.commandHandler.SearchDocumentation(ctx, params.Query, params.Limit)
	if err != nil {
		// If embeddings database is not available, return empty results with helpful message
		return &DocumentationSearchResult{
			Documents: []DocumentChunk{},
			Total:     0,
			Query:     params.Query,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("⚠️ Documentation search is not available - embeddings database not loaded. Error: %v", err),
		}, nil
	}

	// Convert unified result to MCP format
	documents := []DocumentChunk{}
	// First try the correct type: []embeddings.SearchResult
	if resultsData, ok := result.Data["results"].([]embeddings.SearchResult); ok {
		documents = make([]DocumentChunk, len(resultsData))
		for i, searchResult := range resultsData {
			// Convert metadata map[string]interface{} to map[string]string
			metadata := make(map[string]string)
			for k, v := range searchResult.Metadata {
				if str, ok := v.(string); ok {
					metadata[k] = str
				} else {
					metadata[k] = fmt.Sprintf("%v", v)
				}
			}

			documents[i] = DocumentChunk{
				Content:    searchResult.Content,
				Similarity: float32(searchResult.Score),
				Metadata:   metadata,
			}
		}
	} else if resultsData, ok := result.Data["results"].([]interface{}); ok {
		// Fallback for interface{} conversion
		documents = make([]DocumentChunk, len(resultsData))
		for i, res := range resultsData {
			if resultMap, ok := res.(map[string]interface{}); ok {
				// Convert metadata map
				metadata := make(map[string]string)
				if metaData, ok := resultMap["metadata"].(map[string]interface{}); ok {
					for k, v := range metaData {
						if str, ok := v.(string); ok {
							metadata[k] = str
						}
					}
				}

				documents[i] = DocumentChunk{
					Content:    fmt.Sprintf("%v", resultMap["content"]),
					Similarity: float32(0.8), // Default similarity since unified handler doesn't expose it
					Metadata:   metadata,
				}
			}
		}
	}

	return &DocumentationSearchResult{
		Documents: documents,
		Total:     len(documents),
	}, nil
}

func (h *DefaultMCPHandler) GetProjectContext(ctx context.Context, params GetProjectContextParams) (*ProjectContext, error) {
	if h.commandHandler == nil {
		return nil, fmt.Errorf("command handler not initialized")
	}

	// Use unified command handler
	result, err := h.commandHandler.GetProjectContext(ctx, params.Path)
	if err != nil {
		return nil, err
	}

	// Convert unified result to MCP format
	projectInfo, ok := result.Data["project_info"]
	if !ok {
		return nil, fmt.Errorf("invalid project context data")
	}

	// Cast to the correct type - it's actually *analysis.ProjectAnalysis
	projAnalysis, ok := projectInfo.(*analysis.ProjectAnalysis)
	if !ok {
		return nil, fmt.Errorf("invalid project info type: expected *analysis.ProjectAnalysis")
	}

	context := &ProjectContext{
		Path:           result.Data["absolute_path"].(string),
		Name:           projAnalysis.Name,
		SCConfigExists: result.Data["has_client_config"].(bool) || result.Data["has_server_config"].(bool),
		SCConfigPath:   filepath.Join(params.Path, ".sc"),
		Metadata:       make(map[string]interface{}),
	}

	// Add tech stack info if available
	if projAnalysis.PrimaryStack != nil {
		// Convert dependencies from analysis format to MCP format
		var dependencies []string
		for _, dep := range projAnalysis.PrimaryStack.Dependencies {
			if dep.Version != "" {
				dependencies = append(dependencies, fmt.Sprintf("%s@%s", dep.Name, dep.Version))
			} else {
				dependencies = append(dependencies, dep.Name)
			}
		}

		context.TechStack = &TechStackInfo{
			Language:     projAnalysis.PrimaryStack.Language,
			Framework:    projAnalysis.PrimaryStack.Framework,
			Runtime:      projAnalysis.PrimaryStack.Runtime,
			Dependencies: dependencies,
			Architecture: projAnalysis.Architecture,
			Confidence:   projAnalysis.PrimaryStack.Confidence,
			Metadata:     projAnalysis.PrimaryStack.Metadata,
		}
	}

	return context, nil
}

func (h *DefaultMCPHandler) GetSupportedResources(ctx context.Context) (*SupportedResourcesResult, error) {
	// Try to load resources dynamically from embedded schemas first
	result, err := h.loadResourcesFromEmbeddedSchemas(ctx)
	if err != nil {
		// If schema loading fails, fall back to hardcoded resource list to prevent server crash
		fmt.Fprintf(os.Stderr, "MCP Warning: Failed to load embedded schemas, using fallback: %v\n", err)
		return h.getFallbackSupportedResources(), nil
	}
	return result, nil
}

// loadResourcesFromEmbeddedSchemas loads resources from the embedded schema files
func (h *DefaultMCPHandler) loadResourcesFromEmbeddedSchemas(ctx context.Context) (result *SupportedResourcesResult, err error) {
	// Add panic recovery to prevent MCP server crashes
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in loadResourcesFromEmbeddedSchemas: %v", r)
			result = nil
		}
	}()

	var allResources []ResourceInfo
	var allProviders []ProviderInfo

	// First, read the main index to get provider information
	indexData, err := docs.EmbeddedSchemas.ReadFile("schemas/index.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read schemas index: %w", err)
	}

	var mainIndex struct {
		Providers map[string]struct {
			Count       int    `json:"count"`
			Description string `json:"description"`
		} `json:"providers"`
	}

	if err := json.Unmarshal(indexData, &mainIndex); err != nil {
		return nil, fmt.Errorf("failed to parse schemas index: %w", err)
	}

	// Load resources from each provider
	for providerName, providerInfo := range mainIndex.Providers {
		// Skip core schemas as they're not user-facing resources
		if providerName == "core" || providerName == "fs" {
			continue
		}

		providerResources, err := h.loadProviderResources(ctx, providerName)
		if err != nil {
			// Log warning but continue - partial resource loading is acceptable
			fmt.Fprintf(os.Stderr, "MCP Warning: Failed to load resources for provider %s: %v\n", providerName, err)
			continue
		}

		// Extract resource types for this provider
		var resourceTypes []string
		for _, resource := range providerResources {
			resourceTypes = append(resourceTypes, resource.Type)
			allResources = append(allResources, resource)
		}

		// Create provider info
		if len(resourceTypes) > 0 {
			allProviders = append(allProviders, ProviderInfo{
				Name:        providerName,
				DisplayName: h.getProviderDisplayName(providerName),
				Resources:   resourceTypes,
				Description: providerInfo.Description,
			})
		}
	}

	return &SupportedResourcesResult{
		Resources: allResources,
		Providers: allProviders,
		Total:     len(allResources),
	}, nil
}

// loadProviderResources loads resources for a specific provider from embedded schemas
func (h *DefaultMCPHandler) loadProviderResources(ctx context.Context, providerName string) (resources []ResourceInfo, err error) {
	// Add panic recovery to prevent MCP server crashes
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in loadProviderResources for %s: %v", providerName, r)
			resources = nil
		}
	}()

	indexPath := fmt.Sprintf("schemas/%s/index.json", providerName)
	indexData, err := docs.EmbeddedSchemas.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read provider index %s: %w", indexPath, err)
	}

	var providerIndex struct {
		Provider  string `json:"provider"`
		Resources []struct {
			Name         string `json:"name"`
			Type         string `json:"type"`
			Provider     string `json:"provider"`
			Description  string `json:"description"`
			ResourceType string `json:"resourceType"`
		} `json:"resources"`
	}

	if err := json.Unmarshal(indexData, &providerIndex); err != nil {
		return nil, fmt.Errorf("failed to parse provider index %s: %w", indexPath, err)
	}

	var resourceList []ResourceInfo
	for _, res := range providerIndex.Resources {
		// Only include actual resources (not auth, secrets, provisioners, or templates)
		if res.Type == "resource" {
			resourceList = append(resourceList, ResourceInfo{
				Type:        res.ResourceType,
				Name:        res.Name,
				Provider:    res.Provider,
				Description: res.Description,
				Properties:  make(map[string]string), // Could be enhanced by reading actual schema files
			})
		}
	}

	return resourceList, nil
}

// getProviderDisplayName returns a human-readable display name for providers
func (h *DefaultMCPHandler) getProviderDisplayName(providerName string) string {
	displayNames := map[string]string{
		"aws":        "Amazon Web Services",
		"gcp":        "Google Cloud Platform",
		"azure":      "Microsoft Azure",
		"kubernetes": "Kubernetes",
		"mongodb":    "MongoDB Atlas",
		"cloudflare": "Cloudflare",
		"github":     "GitHub",
	}

	if displayName, exists := displayNames[providerName]; exists {
		return displayName
	}

	// Fallback: capitalize first letter
	if len(providerName) > 0 {
		return strings.ToUpper(providerName[:1]) + providerName[1:]
	}
	return providerName
}

// getFallbackSupportedResources provides hardcoded resource list when schema loading fails
func (h *DefaultMCPHandler) getFallbackSupportedResources() *SupportedResourcesResult {
	// Hardcoded resource list to prevent MCP server crashes
	fallbackResources := []ResourceInfo{
		// AWS Resources
		{Type: "s3-bucket", Name: "S3 Bucket", Provider: "aws", Description: "Amazon S3 storage bucket", Properties: map[string]string{}},
		{Type: "ecr-repository", Name: "ECR Repository", Provider: "aws", Description: "Amazon ECR container registry", Properties: map[string]string{}},
		{Type: "aws-rds-postgres", Name: "RDS PostgreSQL", Provider: "aws", Description: "Amazon RDS PostgreSQL database", Properties: map[string]string{}},
		{Type: "aws-rds-mysql", Name: "RDS MySQL", Provider: "aws", Description: "Amazon RDS MySQL database", Properties: map[string]string{}},

		// GCP Resources
		{Type: "gcp-bucket", Name: "Cloud Storage Bucket", Provider: "gcp", Description: "Google Cloud Storage bucket", Properties: map[string]string{}},
		{Type: "gcp-redis", Name: "Memorystore Redis", Provider: "gcp", Description: "Google Cloud Memorystore Redis", Properties: map[string]string{}},
		{Type: "gcp-cloudsql-postgres", Name: "Cloud SQL PostgreSQL", Provider: "gcp", Description: "Google Cloud SQL PostgreSQL", Properties: map[string]string{}},
		{Type: "gcp-artifact-registry", Name: "Artifact Registry", Provider: "gcp", Description: "Google Artifact Registry", Properties: map[string]string{}},

		// MongoDB Atlas
		{Type: "mongodb-atlas", Name: "MongoDB Atlas", Provider: "mongodb", Description: "MongoDB Atlas managed cluster", Properties: map[string]string{}},

		// Kubernetes Resources
		{Type: "helm-postgres", Name: "Helm PostgreSQL", Provider: "kubernetes", Description: "PostgreSQL via Helm chart", Properties: map[string]string{}},
		{Type: "helm-redis", Name: "Helm Redis", Provider: "kubernetes", Description: "Redis via Helm chart", Properties: map[string]string{}},
		{Type: "helm-rabbitmq", Name: "Helm RabbitMQ", Provider: "kubernetes", Description: "RabbitMQ via Helm chart", Properties: map[string]string{}},

		// Cloudflare
		{Type: "cloudflare-registrar", Name: "Domain Registrar", Provider: "cloudflare", Description: "Cloudflare domain management", Properties: map[string]string{}},
	}

	fallbackProviders := []ProviderInfo{
		{Name: "aws", DisplayName: "Amazon Web Services", Resources: []string{"s3-bucket", "ecr-repository", "aws-rds-postgres", "aws-rds-mysql"}, Description: "AWS cloud services"},
		{Name: "gcp", DisplayName: "Google Cloud Platform", Resources: []string{"gcp-bucket", "gcp-redis", "gcp-cloudsql-postgres", "gcp-artifact-registry"}, Description: "Google Cloud services"},
		{Name: "mongodb", DisplayName: "MongoDB Atlas", Resources: []string{"mongodb-atlas"}, Description: "MongoDB Atlas managed database"},
		{Name: "kubernetes", DisplayName: "Kubernetes", Resources: []string{"helm-postgres", "helm-redis", "helm-rabbitmq"}, Description: "Kubernetes resources"},
		{Name: "cloudflare", DisplayName: "Cloudflare", Resources: []string{"cloudflare-registrar"}, Description: "Cloudflare services"},
	}

	return &SupportedResourcesResult{
		Resources: fallbackResources,
		Providers: fallbackProviders,
		Total:     len(fallbackResources),
	}
}

// Simplified interface - remove methods we don't need
func (h *DefaultMCPHandler) GenerateConfiguration(ctx context.Context, params GenerateConfigurationParams) (*GeneratedConfiguration, error) {
	return nil, fmt.Errorf("method not implemented")
}

func (h *DefaultMCPHandler) AnalyzeProject(ctx context.Context, params AnalyzeProjectParams) (*ProjectAnalysis, error) {
	// Use existing project analysis with progress reporting for MCP clients
	analyzer := analysis.NewProjectAnalyzer()

	// Set up JSON progress reporter for structured MCP output
	progressReporter := analysis.NewJSONProgressReporter(os.Stderr)
	analyzer.SetProgressReporter(progressReporter)

	// Set analysis mode to FullMode for comprehensive analysis (matching chat behavior)
	// This ensures resource detection, git analysis, and all other detectors run
	analyzer.SetAnalysisMode(analysis.FullMode)

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

	// Inform the LLM about the generated analysis report
	analysisReportPath := filepath.Join(params.Path, ".sc", "analysis-report.md")
	if _, err := os.Stat(analysisReportPath); err == nil {
		result.Metadata["analysis_report"] = analysisReportPath
		result.Metadata["analysis_report_suggestion"] = "A comprehensive analysis report was generated at .sc/analysis-report.md. Consider reading this file for detailed project insights, tech stack evidence, Git analysis, resource detection results, and actionable recommendations."
	}

	return result, nil
}

func (h *DefaultMCPHandler) SetupSimpleContainer(ctx context.Context, params SetupSimpleContainerParams) (*SetupSimpleContainerResult, error) {
	// Resolve path to absolute path to ensure we work in the correct directory
	path := params.Path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return &SetupSimpleContainerResult{
			Message:      fmt.Sprintf("Failed to resolve path '%s': %v", path, err),
			FilesCreated: []string{},
			Success:      false,
		}, err
	}
	path = absPath

	// Use the existing developer mode setup functionality
	developerMode := modes.NewDeveloperMode()

	// Determine if we need to run analysis to detect deployment type
	needsAnalysisForDeploymentType := params.DeploymentType == "" || params.DeploymentType == "auto"

	// If deployment_type is empty or "auto", either use elicitation or intelligent defaults
	if needsAnalysisForDeploymentType {
		// Check if client supports elicitation
		if h.supportsElicitation() {
			// Use elicitation to ask user for deployment type
			return h.elicitDeploymentType(ctx, params, developerMode)
		} else {
			// Fall back to intelligent defaults
			analyzer := analysis.NewProjectAnalyzer()
			// Set analysis mode for consistent behavior with chat interface
			analyzer.SetAnalysisMode(analysis.FullMode)
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
		Interactive:      false,                         // Force non-interactive for MCP to prevent hanging
		Environment:      params.Environment,
		Parent:           params.Parent,
		SkipAnalysis:     needsAnalysisForDeploymentType, // Skip only if we already analyzed for deployment type detection
		OutputDir:        params.Path,
		DeploymentType:   params.DeploymentType, // Use the specified deployment type
		SkipConfirmation: true,                  // Skip confirmation prompts for MCP
		ForceOverwrite:   true,                  // Force overwrite existing files for MCP
	}

	// Change working directory temporarily to ensure all file operations happen in the correct location
	originalWd, err := os.Getwd()
	if err != nil {
		return &SetupSimpleContainerResult{
			Message:      fmt.Sprintf("Failed to get current directory: %v", err),
			FilesCreated: []string{},
			Success:      false,
		}, err
	}

	// Change to the target directory
	if err := os.Chdir(path); err != nil {
		return &SetupSimpleContainerResult{
			Message:      fmt.Sprintf("Cannot change to directory '%s': %v", path, err),
			FilesCreated: []string{},
			Success:      false,
		}, err
	}

	// Ensure we restore the working directory
	defer func() {
		if restoreErr := os.Chdir(originalWd); restoreErr != nil {
			fmt.Printf("Warning: Failed to restore working directory: %v\n", restoreErr)
		}
	}()

	// Now use "." as the path since we're in the correct directory
	setupOptions.OutputDir = "."

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

	// Check what files were created - Simple Container uses .sc/stacks/ directory structure
	filesCreated := []string{}

	// Discover all available stacks (both client and parent stacks)
	availableStacks := h.discoverAvailableStacks()

	// Check for common project files
	commonFiles := []string{"docker-compose.yaml", "Dockerfile"}
	for _, file := range commonFiles {
		if _, err := os.Stat(file); err == nil {
			filesCreated = append(filesCreated, file)
		}
	}

	// Add discovered stacks to created files
	if len(availableStacks.ClientStacks) > 0 {
		filesCreated = append(filesCreated, fmt.Sprintf("%d client stack(s)", len(availableStacks.ClientStacks)))
	}
	if len(availableStacks.ParentStacks) > 0 {
		filesCreated = append(filesCreated, fmt.Sprintf("%d parent stack(s)", len(availableStacks.ParentStacks)))
	}

	message := "✅ Simple Container setup completed successfully!\n"
	message += fmt.Sprintf("📁 Project path: %s\n", path)
	message += fmt.Sprintf("🌍 Environment: %s\n", params.Environment)
	if params.Parent != "" {
		message += fmt.Sprintf("👨‍👩‍👧‍👦 Parent stack: %s\n", params.Parent)
	}
	message += fmt.Sprintf("📄 Files created: %v\n", filesCreated)

	// Add detailed information about discovered stacks
	if len(availableStacks.ClientStacks) > 0 || len(availableStacks.ParentStacks) > 0 {
		message += "\n📂 Discovered Stack Structure:\n"
		message += "├── .sc/stacks/\n"

		// Show parent stacks (DevOps infrastructure)
		for _, parentStack := range availableStacks.ParentStacks {
			message += fmt.Sprintf("│   ├── %s/ (parent infrastructure)\n", parentStack)
			message += "│   │   ├── server.yaml\n"
			if _, err := os.Stat(filepath.Join(".sc", "stacks", parentStack, "secrets.yaml")); err == nil {
				message += "│   │   └── secrets.yaml\n"
			}
		}

		// Show client stacks (Developer applications)
		for _, clientStack := range availableStacks.ClientStacks {
			message += fmt.Sprintf("│   ├── %s/ (client application)\n", clientStack)
			message += "│   │   ├── client.yaml\n"
			if _, err := os.Stat(filepath.Join(".sc", "stacks", clientStack, "secrets.yaml")); err == nil {
				message += "│   │   └── secrets.yaml\n"
			}
		}

		// Show project files
		if _, err := os.Stat("docker-compose.yaml"); err == nil {
			message += "├── docker-compose.yaml (local development)\n"
		}
		if _, err := os.Stat("Dockerfile"); err == nil {
			message += "└── Dockerfile (container image)\n"
		}

		// Add helpful usage information
		if len(availableStacks.ClientStacks) > 0 {
			message += fmt.Sprintf("\n💡 **Available Client Stacks**: %v\n", availableStacks.ClientStacks)
			message += "   Use: `/show <stack-name>` to view configuration\n"
		}
		if len(availableStacks.ParentStacks) > 0 {
			message += fmt.Sprintf("\n🏗️ **Available Parent Stacks**: %v\n", availableStacks.ParentStacks)
			message += "   Use: `/show <stack-name>` to view infrastructure\n"
		}
	}

	// Add schema context for LLM guidance on future modifications
	message += "\n\n" + h.getStackConfigSchemaContext()

	return &SetupSimpleContainerResult{
		Message:      message,
		FilesCreated: filesCreated,
		Success:      true,
		Metadata: map[string]interface{}{
			"path":          path,
			"environment":   params.Environment,
			"parent":        params.Parent,
			"setup_time":    time.Now(),
			"client_stacks": availableStacks.ClientStacks,
			"parent_stacks": availableStacks.ParentStacks,
		},
	}, nil
}

// StackDiscovery holds information about discovered stacks
type StackDiscovery struct {
	ClientStacks []string // Stacks with client.yaml (developer applications)
	ParentStacks []string // Stacks with server.yaml (infrastructure/parent stacks)
}

// discoverAvailableStacks scans .sc/stacks/ directory to find all available stacks
func (h *DefaultMCPHandler) discoverAvailableStacks() StackDiscovery {
	discovery := StackDiscovery{
		ClientStacks: []string{},
		ParentStacks: []string{},
	}

	stacksDir := filepath.Join(".sc", "stacks")
	if _, err := os.Stat(stacksDir); os.IsNotExist(err) {
		return discovery
	}

	entries, err := os.ReadDir(stacksDir)
	if err != nil {
		return discovery
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		stackName := entry.Name()
		stackPath := filepath.Join(stacksDir, stackName)

		// Check for client.yaml (developer/application stack)
		clientYamlPath := filepath.Join(stackPath, "client.yaml")
		if _, err := os.Stat(clientYamlPath); err == nil {
			discovery.ClientStacks = append(discovery.ClientStacks, stackName)
		}

		// Check for server.yaml (infrastructure/parent stack)
		serverYamlPath := filepath.Join(stackPath, "server.yaml")
		if _, err := os.Stat(serverYamlPath); err == nil {
			discovery.ParentStacks = append(discovery.ParentStacks, stackName)
		}
	}

	return discovery
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
	// Set analysis mode for consistent behavior with chat interface
	analyzer.SetAnalysisMode(analysis.FullMode)
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

		// FUTURE: Implement actual MCP elicitation when protocol extensions support it
		// Current status: MCP protocol doesn't have standardized elicitation mechanism
		// This would require MCP protocol extension for:
		// 1. Client capability declaration for UI interactions
		// 2. Server-initiated elicitation requests (not in current MCP spec)
		// 3. Bidirectional communication for user input collection
		// 4. Standardized UI component schemas (dropdowns, forms, etc.)
		//
		// For now, we provide intelligent defaults based on project analysis.
		// IDEs can implement custom UI for deployment type selection if desired.
	}

	// If we reach here, analysis failed - use default and proceed with setup
	params.DeploymentType = "cloud-compose"
	setupOptions := modes.SetupOptions{
		Interactive:      false, // Force non-interactive for MCP to prevent hanging
		Environment:      params.Environment,
		Parent:           params.Parent,
		SkipAnalysis:     true,  // Skip analysis - we already analyzed above (even if it failed)
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

// Configuration modification methods

func (h *DefaultMCPHandler) GetCurrentConfig(ctx context.Context, params GetCurrentConfigParams) (*GetCurrentConfigResult, error) {
	if h.commandHandler == nil {
		return &GetCurrentConfigResult{
			Success: false,
			Message: "❌ Command handler not initialized",
		}, fmt.Errorf("command handler not initialized")
	}

	// Use unified command handler
	result, err := h.commandHandler.GetCurrentConfig(ctx, params.ConfigType, params.StackName)

	// Choose appropriate schema context based on config type
	var schemaContext string
	if params.ConfigType == "server" {
		schemaContext = h.getResourceSchemaContext()
	} else {
		schemaContext = h.getStackConfigSchemaContext()
	}

	if err != nil {
		errorMessage := result.Message + "\n\n" + schemaContext
		return &GetCurrentConfigResult{
			ConfigType: params.ConfigType,
			Success:    false,
			Message:    errorMessage,
		}, err
	}

	// Convert unified result to MCP format
	successMessage := result.Message + "\n\n" + schemaContext

	return &GetCurrentConfigResult{
		ConfigType: params.ConfigType,
		FilePath:   result.Data["file_path"].(string),
		Content:    result.Data["content"].(map[string]interface{}),
		Message:    successMessage,
		Success:    result.Success,
	}, nil
}

func (h *DefaultMCPHandler) AddEnvironment(ctx context.Context, params AddEnvironmentParams) (*AddEnvironmentResult, error) {
	if h.commandHandler == nil {
		return &AddEnvironmentResult{
			Success: false,
			Message: "❌ Command handler not initialized",
		}, fmt.Errorf("command handler not initialized")
	}

	// Use unified command handler
	result, err := h.commandHandler.AddEnvironment(ctx, params.StackName, params.DeploymentType, params.Parent, params.ParentEnv, params.Config)

	// Always include schema context in response for LLM guidance
	stackContext := h.getStackConfigSchemaContext()

	if err != nil {
		errorMessage := result.Message + "\n\n" + stackContext
		return &AddEnvironmentResult{
			StackName: params.StackName,
			Success:   false,
			Message:   errorMessage,
		}, err
	}

	// Convert unified result to MCP format
	successMessage := result.Message + "\n\n" + stackContext

	return &AddEnvironmentResult{
		StackName:   result.Data["stack_name"].(string),
		FilePath:    result.Data["file_path"].(string),
		Message:     successMessage,
		Success:     result.Success,
		ConfigAdded: result.Data["config_added"].(map[string]interface{}),
	}, nil
}

func (h *DefaultMCPHandler) ModifyStackConfig(ctx context.Context, params ModifyStackConfigParams) (*ModifyStackConfigResult, error) {
	if h.commandHandler == nil {
		return &ModifyStackConfigResult{
			Success: false,
			Message: "❌ Command handler not initialized",
		}, fmt.Errorf("command handler not initialized")
	}

	// For backward compatibility, use current directory as stack and StackName as environment
	// This maintains existing MCP API behavior where StackName was the environment name
	currentDir, _ := os.Getwd()
	inferredStackName := filepath.Base(currentDir)
	if inferredStackName == "." || inferredStackName == "" {
		inferredStackName = "myapp" // fallback default
	}
	result, err := h.commandHandler.ModifyStackConfig(ctx, inferredStackName, params.StackName, params.Changes)

	// Always include schema context in response for LLM guidance
	schemaContext := h.getStackConfigSchemaContext()

	if err != nil {
		errorMessage := fmt.Sprintf("❌ Failed to modify stack configuration: %v", err)
		if result != nil {
			errorMessage = result.Message
		}

		// Include schema guidance in error message
		errorMessage += "\n\n" + schemaContext

		return &ModifyStackConfigResult{
			StackName: params.StackName,
			Success:   false,
			Message:   errorMessage,
		}, err
	}

	// Convert unified result to MCP format
	stackName := params.StackName
	if sn, ok := result.Data["stack_name"].(string); ok {
		stackName = sn
	}

	filePath := ""
	if fp, ok := result.Data["file_path"].(string); ok {
		filePath = fp
	}

	changesApplied := make(map[string]interface{})
	if ca, ok := result.Data["changes_applied"].(map[string]interface{}); ok {
		changesApplied = ca
	}

	// Include schema context in success message
	successMessage := result.Message + "\n\n" + schemaContext

	return &ModifyStackConfigResult{
		StackName:      stackName,
		FilePath:       filePath,
		Message:        successMessage,
		Success:        result.Success,
		ChangesApplied: changesApplied,
	}, nil
}

func (h *DefaultMCPHandler) AddResource(ctx context.Context, params AddResourceParams) (*AddResourceResult, error) {
	if h.commandHandler == nil {
		return &AddResourceResult{
			Success: false,
			Message: "❌ Command handler not initialized",
		}, fmt.Errorf("command handler not initialized")
	}

	// Use unified command handler
	result, err := h.commandHandler.AddResource(ctx, params.ResourceName, params.ResourceType, params.Environment, params.Config)

	// Always include schema context in response for LLM guidance
	resourceContext := h.getResourceSchemaContext()

	if err != nil {
		errorMessage := result.Message + "\n\n" + resourceContext
		return &AddResourceResult{
			ResourceName: params.ResourceName,
			Environment:  params.Environment,
			Success:      false,
			Message:      errorMessage,
		}, err
	}

	// Convert unified result to MCP format
	successMessage := result.Message + "\n\n" + resourceContext

	return &AddResourceResult{
		ResourceName: result.Data["resource_name"].(string),
		Environment:  result.Data["environment"].(string),
		FilePath:     result.Data["file_path"].(string),
		Message:      successMessage,
		Success:      result.Success,
		ConfigAdded:  result.Data["config_added"].(map[string]interface{}),
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

// getStackConfigSchemaContext provides schema guidance for stack configuration modifications
func (h *DefaultMCPHandler) getStackConfigSchemaContext() string {
	return `## 📋 Simple Container Stack Configuration Schema

**IMPORTANT**: Use only these validated properties. Do NOT invent new properties.

### ✅ Valid Stack Properties:
- **type**: "cloud-compose", "single-image", "static"
- **parent**: Parent stack reference (e.g., "infrastructure")  
- **parentEnv**: Environment in parent (e.g., "staging", "production")
- **config**: Configuration section (see below)

### ✅ Valid Config Properties:
#### For cloud-compose type:
- **dockerComposeFile**: "docker-compose.yaml" (REQUIRED)
- **runs**: ["service-name"] (REQUIRED - containers from docker-compose)
- **env**: {"KEY": "value"} (environment variables)
- **secrets**: {"KEY": "${secret:name}"} (sensitive values)
- **scale**: {"min": 1, "max": 5} (scaling configuration)
- **uses**: ["resource-name"] (consume parent resources)
- **dependencies**: ["other-stack"] (stack dependencies)

#### For single-image type:
- **image**: {"dockerfile": "${git:root}/Dockerfile"} (REQUIRED)
- **env**: Environment variables
- **secrets**: Sensitive values
- **scale**: Scaling configuration

#### For static type:
- **bundleDir**: "build/" or "dist/" (static files directory)
- **indexDocument**: "index.html" (default page)
- **errorDocument**: "error.html" (error page)

### ❌ FORBIDDEN Properties (will cause errors):
- ~~compose.file~~ → Use **dockerComposeFile**
- ~~scaling~~ → Use **scale**  
- ~~minCapacity/maxCapacity~~ → Use **scale.min/scale.max**
- ~~environment~~ → Use **env**
- ~~version~~ → Use **schemaVersion** (top-level only)
- ~~connectionString~~ → Auto-injected by resources

### 💡 Example Valid Configuration:
` + "```yaml" + `
schemaVersion: 1.0
stacks:
  myapp:
    type: cloud-compose
    parent: infrastructure
    parentEnv: staging
    config:
      dockerComposeFile: docker-compose.yaml
      runs: [app, worker]
      scale:
        min: 1
        max: 3
      env:
        NODE_ENV: production
        PORT: 3000
      secrets:
        JWT_SECRET: "${secret:jwt-secret}"
      uses: [postgres-db, redis-cache]
` + "```" + `

**🔍 For complete schema details, search documentation with: "client.yaml schema" or "stack configuration"**`
}

// getResourceSchemaContext provides schema guidance for resource configurations
func (h *DefaultMCPHandler) getResourceSchemaContext() string {
	return `## 🗄️ Simple Container Resource Schema

**IMPORTANT**: Use only validated resource types and properties from schemas.

### ✅ Valid Resource Types:
#### AWS Resources:
- **s3-bucket**: S3 storage bucket
- **ecr-repository**: Container registry  
- **aws-rds-postgres**: PostgreSQL database
- **aws-rds-mysql**: MySQL database

#### GCP Resources:
- **gcp-bucket**: Cloud Storage bucket
- **gcp-redis**: Memorystore Redis
- **gcp-cloudsql-postgres**: Cloud SQL PostgreSQL
- **gcp-artifact-registry**: Container registry

#### MongoDB Atlas:
- **mongodb-atlas**: Managed MongoDB cluster

#### Kubernetes:
- **helm-postgres**: PostgreSQL via Helm
- **helm-redis**: Redis via Helm
- **helm-rabbitmq**: RabbitMQ via Helm

### 💡 Example Valid Resource:
` + "```yaml" + `
resources:
  staging:
    postgres-db:
      type: aws-rds-postgres
      name: myapp-staging-db
      instanceClass: db.t3.micro
      allocatedStorage: 20
      username: postgres
      password: "${secret:postgres-password}"
      databaseName: myapp
` + "```" + `

**🔍 Search documentation for specific resource schemas: "aws s3 schema", "mongodb atlas configuration", etc.**`
}

// New chat command equivalent handlers

func (h *DefaultMCPHandler) ReadProjectFile(ctx context.Context, params ReadProjectFileParams) (*ReadProjectFileResult, error) {
	// TODO: Implement file reading functionality
	// For now, return a helpful error message
	return &ReadProjectFileResult{
		Filename: params.Filename,
		Success:  false,
		Message:  "❌ File reading functionality is not yet implemented",
	}, nil
}

func (h *DefaultMCPHandler) ShowStackConfig(ctx context.Context, params ShowStackConfigParams) (*ShowStackConfigResult, error) {
	// TODO: Implement stack configuration display functionality
	// For now, return a helpful error message
	return &ShowStackConfigResult{
		StackName:  params.StackName,
		ConfigType: params.ConfigType,
		Success:    false,
		Message:    "❌ Stack configuration display is not yet implemented",
	}, nil
}

func (h *DefaultMCPHandler) AdvancedSearchDocumentation(ctx context.Context, params AdvancedSearchDocumentationParams) (*AdvancedSearchDocumentationResult, error) {
	// Reuse the existing SearchDocumentation method but with different formatting
	searchParams := SearchDocumentationParams{
		Query: params.Query,
		Limit: params.Limit,
	}

	result, err := h.SearchDocumentation(ctx, searchParams)
	if err != nil {
		return &AdvancedSearchDocumentationResult{
			Query:   params.Query,
			Success: false,
			Message: fmt.Sprintf("❌ Advanced documentation search failed: %v", err),
		}, nil
	}

	// Convert to the advanced result format
	var docChunks []DocumentChunk
	docChunks = append(docChunks, result.Documents...)

	// Format enhanced message for LLM tool calling context
	message := fmt.Sprintf("📚 **Found %d documentation results for \"%s\"**\n\n", result.Total, params.Query)
	for i, doc := range docChunks {
		score := int(doc.Similarity * 100)
		title := "Unknown"
		if t, exists := doc.Metadata["title"]; exists {
			title = t
		}
		message += fmt.Sprintf("**%d. %s** (%d%% relevance)\n", i+1, title, score)

		// Truncate content for tool response (600 chars)
		content := doc.Content
		if len(content) > 600 {
			content = content[:600] + "..."
		}
		message += fmt.Sprintf("```\n%s\n```\n\n", content)
	}
	message += "💡 **Use this information to provide accurate, specific guidance based on Simple Container documentation.**"

	return &AdvancedSearchDocumentationResult{
		Query:   params.Query,
		Results: docChunks,
		Total:   result.Total,
		Message: message,
		Success: true,
	}, nil
}

func (h *DefaultMCPHandler) GetHelp(ctx context.Context, params GetHelpParams) (*GetHelpResult, error) {
	var helpMessage string

	if params.ToolName != "" {
		// Provide help for specific tool
		switch params.ToolName {
		case "search_documentation":
			helpMessage = `📚 **search_documentation** - Search Simple Container documentation using semantic similarity

**Usage:** Provide a query to search for relevant documentation
**Parameters:**
- query (required): Search terms for documentation
- limit (optional): Maximum results (default: 5)

**Example:** Search for "mongodb configuration" to find MongoDB setup examples`

		case "get_project_context":
			helpMessage = `📊 **get_project_context** - Analyze project structure and get Simple Container context

**Usage:** Analyze a project directory to understand its tech stack
**Parameters:**
- path (optional): Project path to analyze (default: current directory)

**Returns:** Project name, detected languages/frameworks, and SC configuration status`

		case "setup_simple_container":
			helpMessage = `🚀 **setup_simple_container** - Initialize Simple Container configuration

**Usage:** Set up Simple Container files for a project
**Parameters:**
- path (optional): Project path (default: current directory)
- environment (optional): Target environment (default: development)  
- parent (optional): Parent stack reference (e.g., 'mycompany/infrastructure')
- deployment_type (optional): 'static', 'single-image', or 'cloud-compose'
- interactive (optional): Run in interactive mode (default: false)

**Creates:** client.yaml, docker-compose.yaml, and other configuration files`

		case "read_project_file":
			helpMessage = `📄 **read_project_file** - Read and display project files securely

**Usage:** Read project files with automatic credential obfuscation
**Parameters:**
- filename (required): File to read (e.g., 'Dockerfile', 'client.yaml')

**Security:** Automatically masks sensitive credentials in configuration files`

		case "show_stack_config":
			helpMessage = `📋 **show_stack_config** - Display stack configuration

**Usage:** Show comprehensive stack configuration from client.yaml and server.yaml
**Parameters:**
- stack_name (required): Name of the stack to display
- config_type (optional): 'client' or 'server' (default: both)

**Returns:** Complete configuration with analysis and guidance`

		default:
			helpMessage = fmt.Sprintf("❓ Tool '%s' not found. Use get_help without parameters to see all available tools.", params.ToolName)
		}
	} else {
		// Provide general help with all available tools
		helpMessage = `# 🛠️ Simple Container MCP Tools

## 📚 Documentation & Analysis
- **search_documentation** - Search documentation with semantic similarity
- **advanced_search_documentation** - Enhanced search with LLM integration
- **get_project_context** - Analyze project tech stack and structure
- **analyze_project** - Detailed project analysis with recommendations

## ⚙️ Configuration Management  
- **get_current_config** - Read client.yaml or server.yaml configuration
- **setup_simple_container** - Initialize Simple Container for a project
- **read_project_file** - Read project files with credential protection
- **show_stack_config** - Display comprehensive stack configuration

## 🏗️ Stack & Environment Management
- **add_environment** - Add new environments to client.yaml
- **modify_stack_config** - Modify existing stack configurations  
- **add_resource** - Add resources to server.yaml

## 📊 Information & Support
- **get_supported_resources** - List all supported cloud resources
- **get_help** - This help system (use with tool_name for specific help)
- **get_status** - Show project and system status

**💡 Pro Tip:** Use search_documentation to find specific examples for any Simple Container feature!`
	}

	return &GetHelpResult{
		ToolName: params.ToolName,
		Message:  helpMessage,
		Success:  true,
	}, nil
}

func (h *DefaultMCPHandler) GetStatus(ctx context.Context, params GetStatusParams) (*GetStatusResult, error) {
	// Use provided path or current directory
	path := params.Path
	if path == "" {
		path = "."
	}

	// Get absolute path for display
	currentDir, err := filepath.Abs(path)
	if err != nil {
		currentDir = path
	}

	// Change to the target directory for analysis
	originalDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %w", err)
	}

	if err := os.Chdir(currentDir); err != nil {
		return nil, fmt.Errorf("failed to change to directory %s: %w", currentDir, err)
	}
	defer func() {
		_ = os.Chdir(originalDir) // Restore original directory - ignore error in defer
	}()

	// Check if Simple Container is configured
	scConfigured := false
	scConfigPath := ""

	// Look for .sc directory
	if _, err := os.Stat(".sc"); err == nil {
		scConfigured = true
		scConfigPath = ".sc/"

		// Check for specific config files
		if _, err := os.Stat(".sc/stacks"); err == nil {
			scConfigPath += "stacks/"
		}
	}

	statusMessage := "# 📊 Simple Container Project Status\n\n"
	statusMessage += fmt.Sprintf("**📁 Current Directory:** `%s`\n", currentDir)
	statusMessage += fmt.Sprintf("**🔧 Simple Container Configured:** %t\n", scConfigured)

	if scConfigured {
		statusMessage += fmt.Sprintf("**📂 Configuration Path:** `%s`\n", scConfigPath)
	}

	details := make(map[string]interface{})
	details["current_directory"] = currentDir
	details["sc_configured"] = scConfigured
	details["sc_config_path"] = scConfigPath

	if params.Detailed {
		// Add detailed diagnostic information
		statusMessage += "\n## 🔍 Detailed Diagnostics\n"

		// Check for common project files
		commonFiles := []string{"package.json", "requirements.txt", "go.mod", "Dockerfile", "docker-compose.yaml", "client.yaml", "server.yaml"}
		foundFiles := []string{}

		for _, file := range commonFiles {
			if _, err := os.Stat(file); err == nil {
				foundFiles = append(foundFiles, file)
			}
		}

		if len(foundFiles) > 0 {
			statusMessage += fmt.Sprintf("**📄 Project Files Found:** %s\n", strings.Join(foundFiles, ", "))
			details["project_files"] = foundFiles
		}

		// Check for git repository
		if _, err := os.Stat(".git"); err == nil {
			statusMessage += "**📦 Git Repository:** ✅ Present\n"
			details["git_repository"] = true
		} else {
			details["git_repository"] = false
		}

		// MCP server status
		statusMessage += "**🌐 MCP Server:** ✅ Running and responding\n"
		details["mcp_server_status"] = "running"
	}

	status := "healthy"
	if !scConfigured {
		status = "not_configured"
		statusMessage += "\n💡 **Next Steps:** Use `setup_simple_container` tool to initialize Simple Container configuration"
	}

	return &GetStatusResult{
		Status:  status,
		Message: statusMessage,
		Details: details,
		Success: true,
	}, nil
}

func (h *DefaultMCPHandler) WriteProjectFile(ctx context.Context, params WriteProjectFileParams) (*WriteProjectFileResult, error) {
	filename := params.Filename
	content := params.Content

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %w", err)
	}

	// Build full file path and validate security
	filePath := filepath.Join(cwd, filename)

	// Basic security check - prevent writing outside project directory
	if !strings.HasPrefix(filepath.Clean(filePath), filepath.Clean(cwd)) {
		return &WriteProjectFileResult{
			Message: "❌ Security error: Cannot write files outside the project directory",
			Success: false,
		}, nil
	}

	var finalContent []byte
	var mode string

	if params.Lines != "" {
		// Line range replacement mode
		mode = "line-range replacement"
		finalContent, err = h.replaceFileLines(filePath, content, params.Lines)
		if err != nil {
			return &WriteProjectFileResult{
				Message: fmt.Sprintf("❌ Failed to replace lines in %s: %v", filename, err),
				Success: false,
			}, nil
		}
	} else if params.Append {
		// Append mode
		mode = "append"
		finalContent, err = h.appendToFile(filePath, content)
		if err != nil {
			return &WriteProjectFileResult{
				Message: fmt.Sprintf("❌ Failed to append to %s: %v", filename, err),
				Success: false,
			}, nil
		}
	} else {
		// Full file replacement mode (default)
		mode = "full replacement"
		finalContent = []byte(content)
	}

	// Write the file
	err = os.WriteFile(filePath, finalContent, 0o644)
	if err != nil {
		return &WriteProjectFileResult{
			Message: fmt.Sprintf("❌ Failed to write file %s: %v", filename, err),
			Success: false,
		}, nil
	}

	// Create success message
	message := fmt.Sprintf("✅ **File written successfully**: %s (%s)\n", filename, mode)
	message += fmt.Sprintf("📁 **Location**: %s\n", filePath)
	message += fmt.Sprintf("📊 **File Info**: %d bytes written", len(finalContent))

	return &WriteProjectFileResult{
		Message: message,
		Files:   []string{filename},
		Success: true,
	}, nil
}

// Helper functions for file operations
func (h *DefaultMCPHandler) replaceFileLines(filePath, newContent, lineRange string) ([]byte, error) {
	// Parse line range (e.g., "10-20" or "5")
	var startLine, endLine int
	var err error

	if strings.Contains(lineRange, "-") {
		parts := strings.Split(lineRange, "-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid line range format. Use 'start-end' or single line number")
		}
		startLine, err = strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("invalid start line number: %v", err)
		}
		endLine, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid end line number: %v", err)
		}
	} else {
		// Single line replacement
		startLine, err = strconv.Atoi(strings.TrimSpace(lineRange))
		if err != nil {
			return nil, fmt.Errorf("invalid line number: %v", err)
		}
		endLine = startLine
	}

	// Convert to 0-based indexing
	startLine--
	endLine--

	// Read existing file content
	var existingContent []byte
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		existingContent, err = os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read existing file: %v", err)
		}
	}

	// Split into lines
	lines := strings.Split(string(existingContent), "\n")

	// Validate line range
	if startLine < 0 || startLine >= len(lines) {
		return nil, fmt.Errorf("start line %d is out of range (file has %d lines)", startLine+1, len(lines))
	}
	if endLine < 0 || endLine >= len(lines) {
		return nil, fmt.Errorf("end line %d is out of range (file has %d lines)", endLine+1, len(lines))
	}
	if startLine > endLine {
		return nil, fmt.Errorf("start line (%d) cannot be greater than end line (%d)", startLine+1, endLine+1)
	}

	// Replace the specified lines
	newLines := strings.Split(newContent, "\n")

	// Build the final content
	var result []string
	result = append(result, lines[:startLine]...) // Lines before replacement
	result = append(result, newLines...)          // New content
	result = append(result, lines[endLine+1:]...) // Lines after replacement

	return []byte(strings.Join(result, "\n")), nil
}

func (h *DefaultMCPHandler) appendToFile(filePath, content string) ([]byte, error) {
	var existingContent []byte

	// Read existing file if it exists
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		var err error
		existingContent, err = os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read existing file: %v", err)
		}
	}

	// Ensure there's a newline before appending (if file has content)
	finalContent := string(existingContent)
	if len(existingContent) > 0 && !strings.HasSuffix(finalContent, "\n") {
		finalContent += "\n"
	}

	finalContent += content

	return []byte(finalContent), nil
}
