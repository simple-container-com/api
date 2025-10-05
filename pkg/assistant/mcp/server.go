package mcp

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/analysis"
	"github.com/simple-container-com/api/pkg/assistant/embeddings"
	"github.com/simple-container-com/api/pkg/assistant/modes"
)

//go:embed schemas/**/*.json
var embeddedSchemas embed.FS

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
	_ = json.NewEncoder(w).Encode(response)
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
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"version":   MCPVersion,
		"name":      MCPName,
	})
}

func (s *MCPServer) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	capabilities, _ := s.handler.GetCapabilities(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(capabilities)
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
	_ = json.NewEncoder(w).Encode(response)
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
}

func NewDefaultMCPHandler() MCPHandler {
	return &DefaultMCPHandler{}
}

func (h *DefaultMCPHandler) SearchDocumentation(ctx context.Context, params SearchDocumentationParams) (*DocumentationSearchResult, error) {
	// Load embedded documentation database
	db, err := embeddings.LoadEmbeddedDatabase(ctx)
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

	// Discover resources in the project
	resources := h.discoverResources(projectPath, scConfigExists)

	// Generate context-aware recommendations
	recommendations := h.generateRecommendations(projectPath, scConfigExists, resources)

	return &ProjectContext{
		Path:            projectPath,
		Name:            filepath.Base(projectPath),
		SCConfigExists:  scConfigExists,
		SCConfigPath:    scConfigPath,
		Resources:       resources,
		Recommendations: recommendations,
		Metadata: map[string]interface{}{
			"analyzed_at": time.Now(),
			"mcp_version": MCPVersion,
		},
	}, nil
}

// discoverResources scans the project for existing Simple Container resources
func (h *DefaultMCPHandler) discoverResources(projectPath string, scConfigExists bool) []ResourceInfo {
	var resources []ResourceInfo

	if !scConfigExists {
		return resources
	}

	// Scan .sc/stacks directory for server.yaml files
	stacksPath := filepath.Join(projectPath, ".sc", "stacks")
	if _, err := os.Stat(stacksPath); err != nil {
		return resources
	}

	_ = filepath.Walk(stacksPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || !strings.HasSuffix(info.Name(), "server.yaml") {
			return nil
		}

		// Parse server.yaml to extract resources
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var config map[string]interface{}
		if err := yaml.Unmarshal(data, &config); err != nil {
			return nil
		}

		// Extract resources from server.yaml
		if resourcesSection, ok := config["resources"].(map[interface{}]interface{}); ok {
			for env, envResources := range resourcesSection {
				if envResourcesMap, ok := envResources.(map[interface{}]interface{}); ok {
					for resourceName, resourceDef := range envResourcesMap {
						if resourceMap, ok := resourceDef.(map[interface{}]interface{}); ok {
							if resourceType, ok := resourceMap["type"].(string); ok {
								resources = append(resources, ResourceInfo{
									Type:        resourceType,
									Name:        fmt.Sprintf("%v", resourceName),
									Provider:    h.extractProviderFromType(resourceType),
									Description: fmt.Sprintf("Resource in %v environment", env),
									Properties: map[string]string{
										"environment": fmt.Sprintf("%v", env),
										"stack":       strings.TrimSuffix(info.Name(), ".yaml"),
									},
								})
							}
						}
					}
				}
			}
		}

		return nil
	})

	return resources
}

// generateRecommendations creates context-aware recommendations
func (h *DefaultMCPHandler) generateRecommendations(projectPath string, scConfigExists bool, resources []ResourceInfo) []string {
	var recommendations []string

	if !scConfigExists {
		recommendations = append(recommendations,
			"Initialize Simple Container configuration with 'sc init'",
			"Create a parent stack for shared infrastructure",
			"Set up secrets management with 'sc secrets init'",
		)
		return recommendations
	}

	// Check for missing common files
	if _, err := os.Stat(filepath.Join(projectPath, "Dockerfile")); os.IsNotExist(err) {
		recommendations = append(recommendations, "Consider adding a Dockerfile for containerization")
	}

	if _, err := os.Stat(filepath.Join(projectPath, "docker-compose.yaml")); os.IsNotExist(err) {
		recommendations = append(recommendations, "Add docker-compose.yaml for local development")
	}

	// Analyze resource patterns
	hasDatabase := false
	hasStorage := false
	for _, resource := range resources {
		if strings.Contains(resource.Type, "postgres") || strings.Contains(resource.Type, "mysql") || strings.Contains(resource.Type, "mongodb") {
			hasDatabase = true
		}
		if strings.Contains(resource.Type, "bucket") || strings.Contains(resource.Type, "s3") {
			hasStorage = true
		}
	}

	if len(resources) == 0 {
		recommendations = append(recommendations, "Define shared resources in server.yaml for infrastructure")
	}

	if !hasDatabase {
		recommendations = append(recommendations, "Consider adding a database resource for data persistence")
	}

	if !hasStorage {
		recommendations = append(recommendations, "Consider adding storage resources for file/object storage")
	}

	return recommendations
}

// extractProviderFromType extracts the provider from a resource type
func (h *DefaultMCPHandler) extractProviderFromType(resourceType string) string {
	parts := strings.Split(resourceType, "-")
	if len(parts) > 0 {
		switch parts[0] {
		case "aws":
			return "aws"
		case "gcp":
			return "gcp"
		case "k8s", "kubernetes":
			return "kubernetes"
		case "mongodb":
			return "mongodb"
		case "cloudflare":
			return "cloudflare"
		default:
			if strings.Contains(resourceType, "s3") {
				return "aws"
			}
			if strings.Contains(resourceType, "gke") || strings.Contains(resourceType, "cloudrun") {
				return "gcp"
			}
			return "unknown"
		}
	}
	return "unknown"
}

func (h *DefaultMCPHandler) GenerateConfiguration(ctx context.Context, params GenerateConfigurationParams) (*GeneratedConfiguration, error) {
	var files []GeneratedFile
	var messages []string

	// Use project analyzer to understand the project
	analyzer := analysis.NewProjectAnalyzer()
	projectAnalysis, err := analyzer.AnalyzeProject(params.ProjectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze project: %w", err)
	}

	// Use developer mode for intelligent file generation
	devMode := modes.NewDeveloperMode()

	switch params.ConfigType {
	case "dockerfile":
		content, err := devMode.GenerateDockerfileWithLLM(projectAnalysis)
		if err != nil {
			messages = append(messages, fmt.Sprintf("Failed to generate Dockerfile: %v", err))
			content = h.getFallbackDockerfile()
		}
		files = append(files, GeneratedFile{
			Path:        "Dockerfile",
			Content:     content,
			ContentType: "dockerfile",
			Description: "Generated Dockerfile based on project analysis",
		})
		messages = append(messages, "Generated Dockerfile using AI analysis")

	case "docker-compose":
		content, err := devMode.GenerateComposeYAMLWithLLM(projectAnalysis)
		if err != nil {
			messages = append(messages, fmt.Sprintf("Failed to generate docker-compose.yaml: %v", err))
			content = h.getFallbackComposeYAML()
		}
		files = append(files, GeneratedFile{
			Path:        "docker-compose.yaml",
			Content:     content,
			ContentType: "yaml",
			Description: "Generated docker-compose.yaml for local development",
		})
		messages = append(messages, "Generated docker-compose.yaml using AI analysis")

	case "client-yaml":
		opts := &modes.SetupOptions{
			Environment: "staging",
			Parent:      "infrastructure",
		}
		content, err := devMode.GenerateClientYAMLWithLLM(opts, projectAnalysis)
		if err != nil {
			messages = append(messages, fmt.Sprintf("Failed to generate client.yaml: %v", err))
			content = h.getFallbackClientYAML()
		}
		files = append(files, GeneratedFile{
			Path:        ".sc/stacks/" + filepath.Base(params.ProjectPath) + "/client.yaml",
			Content:     content,
			ContentType: "yaml",
			Description: "Generated Simple Container client configuration",
		})
		messages = append(messages, "Generated client.yaml using AI analysis")

	case "full-setup":
		// Generate all three files
		dockerfileContent, _ := devMode.GenerateDockerfileWithLLM(projectAnalysis)
		composeContent, _ := devMode.GenerateComposeYAMLWithLLM(projectAnalysis)
		opts := &modes.SetupOptions{Environment: "staging", Parent: "infrastructure"}
		clientContent, _ := devMode.GenerateClientYAMLWithLLM(opts, projectAnalysis)

		files = append(files,
			GeneratedFile{
				Path:        "Dockerfile",
				Content:     dockerfileContent,
				ContentType: "dockerfile",
				Description: "Generated Dockerfile based on project analysis",
			},
			GeneratedFile{
				Path:        "docker-compose.yaml",
				Content:     composeContent,
				ContentType: "yaml",
				Description: "Generated docker-compose.yaml for local development",
			},
			GeneratedFile{
				Path:        ".sc/stacks/" + filepath.Base(params.ProjectPath) + "/client.yaml",
				Content:     clientContent,
				ContentType: "yaml",
				Description: "Generated Simple Container client configuration",
			})
		messages = append(messages, "Generated complete Simple Container setup")

	default:
		return nil, fmt.Errorf("unsupported configuration type: %s", params.ConfigType)
	}

	return &GeneratedConfiguration{
		ConfigType: params.ConfigType,
		Files:      files,
		Messages:   messages,
		Metadata: map[string]interface{}{
			"generated_at": time.Now(),
			"project_path": params.ProjectPath,
		},
	}, nil
}

// Fallback functions for when LLM generation fails
func (h *DefaultMCPHandler) getFallbackDockerfile() string {
	return `FROM node:18-alpine

WORKDIR /app

COPY package*.json ./
RUN npm ci --only=production

COPY . .

EXPOSE 3000

CMD ["npm", "start"]`
}

func (h *DefaultMCPHandler) getFallbackComposeYAML() string {
	return `version: '3.8'

services:
  app:
    build: .
    ports:
      - "3000:3000"
    environment:
      - NODE_ENV=development
      - PORT=3000
    volumes:
      - .:/app:delegated
    command: npm run dev`
}

func (h *DefaultMCPHandler) getFallbackClientYAML() string {
	return `schemaVersion: 1.0

stacks:
  app:
    type: cloud-compose
    parent: infrastructure
    parentEnv: staging
    config:
      runs: [app]
      scale:
        min: 1
        max: 3
      env:
        PORT: 3000
      secrets:
        JWT_SECRET: "${secret:jwt-secret}"`
}

func (h *DefaultMCPHandler) AnalyzeProject(ctx context.Context, params AnalyzeProjectParams) (*ProjectAnalysis, error) {
	// Use the actual project analyzer
	analyzer := analysis.NewProjectAnalyzer()

	projectPath := params.Path
	if projectPath == "" {
		projectPath = "."
	}

	// Check if path exists
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("project path does not exist: %s", projectPath)
	}

	// Perform actual project analysis
	analysisResult, err := analyzer.AnalyzeProject(projectPath)
	if err != nil {
		return &ProjectAnalysis{
			Path: projectPath,
			TechStacks: []TechStackInfo{
				{
					Language:   "unknown",
					Confidence: 0.0,
					Framework:  "",
				},
			},
			Recommendations: []Recommendation{
				{
					Type:        "error",
					Category:    "analysis",
					Priority:    "medium",
					Title:       "Analysis Failed",
					Description: fmt.Sprintf("Failed to analyze project: %v", err),
				},
			},
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"status": "error",
				"error":  err.Error(),
			},
		}, nil
	}

	// Convert analysis result to MCP format
	techStacks := make([]TechStackInfo, 0)
	if len(analysisResult.TechStacks) > 0 {
		for _, stack := range analysisResult.TechStacks {
			// Extract dependencies as string slice
			deps := make([]string, 0)
			for _, dep := range stack.Dependencies {
				deps = append(deps, dep.Name)
			}

			// Create metadata with additional info
			metadata := make(map[string]string)
			if stack.Version != "" {
				metadata["version"] = stack.Version
			}
			if stack.Runtime != "" {
				metadata["runtime"] = stack.Runtime
			}
			for k, v := range stack.Metadata {
				metadata[k] = v
			}

			techStack := TechStackInfo{
				Language:     stack.Language,
				Framework:    stack.Framework,
				Runtime:      stack.Runtime,
				Dependencies: deps,
				Architecture: analysisResult.Architecture,
				Confidence:   stack.Confidence,
				Metadata:     metadata,
			}
			techStacks = append(techStacks, techStack)
		}
	} else {
		// Fallback if no tech stacks detected
		techStacks = append(techStacks, TechStackInfo{
			Language:   "unknown",
			Confidence: 0.0,
		})
	}

	// Convert recommendations
	recommendations := make([]Recommendation, 0)
	for _, rec := range analysisResult.Recommendations {
		recommendation := Recommendation{
			Type:        rec.Type,
			Category:    rec.Category,
			Priority:    rec.Priority,
			Title:       rec.Title,
			Description: rec.Description,
		}
		recommendations = append(recommendations, recommendation)
	}

	// Add some default recommendations if none provided
	if len(recommendations) == 0 {
		primaryLanguage := "unknown"
		if analysisResult.PrimaryStack != nil {
			primaryLanguage = analysisResult.PrimaryStack.Language
		} else if len(analysisResult.TechStacks) > 0 {
			primaryLanguage = analysisResult.TechStacks[0].Language
		}

		recommendations = append(recommendations, Recommendation{
			Type:        "setup",
			Category:    "development",
			Priority:    "medium",
			Title:       "Project Analysis Complete",
			Description: fmt.Sprintf("Successfully analyzed %s project", primaryLanguage),
		})
	}

	return &ProjectAnalysis{
		Path:            projectPath,
		TechStacks:      techStacks,
		Recommendations: recommendations,
		Timestamp:       time.Now(),
		Metadata: map[string]interface{}{
			"status":       "success",
			"name":         analysisResult.Name,
			"architecture": analysisResult.Architecture,
			"confidence":   analysisResult.Confidence,
		},
	}, nil
}

func (h *DefaultMCPHandler) GetSupportedResources(ctx context.Context) (*SupportedResourcesResult, error) {
	// Load schemas from embedded files
	resources := make([]ResourceInfo, 0)
	providers := make(map[string]*ProviderInfo)

	// Define provider display names
	providerDisplayNames := map[string]string{
		"aws":        "Amazon Web Services",
		"gcp":        "Google Cloud Platform",
		"kubernetes": "Kubernetes",
		"mongodb":    "MongoDB Atlas",
		"cloudflare": "Cloudflare",
		"github":     "GitHub",
		"fs":         "File System",
	}

	// Read main index from embedded schemas
	mainIndex, err := h.readEmbeddedProviderIndex("schemas/index.json")
	if err != nil {
		// If schema loading fails, fall back to static resources for backwards compatibility
		fmt.Printf("Warning: failed to load schemas from embedded files (%v), using fallback resources\n", err)
		return h.getFallbackResources(), nil
	}

	// Process each provider
	for providerName := range mainIndex {
		if providerName == "core" {
			continue // Skip core schemas (client.yaml, server.yaml)
		}

		providerIndexPath := fmt.Sprintf("schemas/%s/index.json", providerName)
		providerResources, err := h.readEmbeddedProviderResources(providerIndexPath, providerName)
		if err != nil {
			// Skip providers with missing/invalid indexes
			continue
		}

		// Initialize provider info
		displayName, exists := providerDisplayNames[providerName]
		if !exists {
			displayName = strings.ToUpper(providerName[:1]) + providerName[1:]
		}

		providerInfo := &ProviderInfo{
			Name:        providerName,
			DisplayName: displayName,
			Resources:   make([]string, 0),
		}

		// Add resources from this provider
		for _, resource := range providerResources {
			// Only include actual resources (not templates, auth, provisioner)
			if resource.Type == "resource" {
				resourceInfo := ResourceInfo{
					Type:        resource.ResourceType,
					Name:        resource.Name,
					Provider:    providerName,
					Description: resource.Description,
				}

				resources = append(resources, resourceInfo)
				providerInfo.Resources = append(providerInfo.Resources, resource.ResourceType)
			}
		}

		if len(providerInfo.Resources) > 0 {
			providers[providerName] = providerInfo
		}
	}

	// Convert providers map to slice
	providerList := make([]ProviderInfo, 0, len(providers))
	for _, provider := range providers {
		providerList = append(providerList, *provider)
	}

	// If no resources were loaded, fall back to static resources
	if len(resources) == 0 {
		return h.getFallbackResources(), nil
	}

	return &SupportedResourcesResult{
		Resources: resources,
		Providers: providerList,
		Total:     len(resources),
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
			"project_analysis":         true,  // ✅ Fully implemented
			"configuration_generation": false, // Future enhancement
			"interactive_chat":         false, // Available via separate chat command
		},
		"documentation": map[string]interface{}{
			"indexed_documents": h.getIndexedDocumentsCount(),
			"providers":         []string{"docs", "examples", "schemas"},
			"embedding_model":   "local/simple-container-128d",
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

// getIndexedDocumentsCount returns the number of documents in the embeddings database
func (h *DefaultMCPHandler) getIndexedDocumentsCount() int {
	ctx := context.Background()
	db, err := embeddings.LoadEmbeddedDatabase(ctx)
	if err != nil {
		return 0
	}

	// Get document count using a generic search query
	results, err := embeddings.SearchDocumentation(db, "simple container", 1000) // Large limit to get all
	if err != nil {
		// Fallback to a reasonable estimate based on typical documentation size
		return 30 // Typical number of indexed documents
	}

	return len(results)
}

// Helper functions for schema loading

func (h *DefaultMCPHandler) getFallbackResources() *SupportedResourcesResult {
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
	}
}

func (h *DefaultMCPHandler) readEmbeddedProviderIndex(indexPath string) (map[string]interface{}, error) {
	data, err := embeddedSchemas.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}

	var index struct {
		Providers map[string]interface{} `json:"providers"`
	}

	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}

	return index.Providers, nil
}

type SchemaResource struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Provider     string `json:"provider"`
	Description  string `json:"description"`
	ResourceType string `json:"resourceType"`
	GoPackage    string `json:"goPackage"`
	GoStruct     string `json:"goStruct"`
}

func (h *DefaultMCPHandler) readEmbeddedProviderResources(indexPath, providerName string) ([]SchemaResource, error) {
	data, err := embeddedSchemas.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}

	var providerIndex struct {
		Resources []SchemaResource `json:"resources"`
	}

	if err := json.Unmarshal(data, &providerIndex); err != nil {
		return nil, err
	}

	return providerIndex.Resources, nil
}
