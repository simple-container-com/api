package mcp

import (
	"context"
	"encoding/json"
	"time"
)

// MCP (Model Context Protocol) implementation for Simple Container
// Provides JSON-RPC interface for external LLM tools like Windsurf, Cursor, etc.

// Protocol version and constants
const (
	MCPVersion = "1.0"
	MCPName    = "simple-container-mcp"
)

// Core MCP request/response structures following JSON-RPC 2.0 specification
type MCPRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      interface{} `json:"id,omitempty"`
}

type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
	ID      interface{} `json:"id,omitempty"`
}

type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCP method parameter structures
type SearchDocumentationParams struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
	Type  string `json:"type,omitempty"` // "docs", "examples", "schemas", or empty for all
}

type GetProjectContextParams struct {
	Path string `json:"path,omitempty"`
}

type GenerateConfigurationParams struct {
	ProjectPath   string                 `json:"project_path"`
	ProjectType   string                 `json:"project_type,omitempty"`   // "node", "python", "go", etc.
	CloudProvider string                 `json:"cloud_provider,omitempty"` // "aws", "gcp", "azure"
	ConfigType    string                 `json:"config_type"`              // "dockerfile", "compose", "sc-structure"
	Options       map[string]interface{} `json:"options,omitempty"`
}

type AnalyzeProjectParams struct {
	Path string `json:"path"`
}

// MCP response data structures
type DocumentationSearchResult struct {
	Documents []DocumentChunk `json:"documents"`
	Total     int             `json:"total"`
	Query     string          `json:"query"`
	Timestamp time.Time       `json:"timestamp"`
}

type DocumentChunk struct {
	ID         string            `json:"id"`
	Content    string            `json:"content"`
	Path       string            `json:"path"`
	Type       string            `json:"type"`
	Similarity float32           `json:"similarity,omitempty"`
	Metadata   map[string]string `json:"metadata"`
}

type ProjectContext struct {
	Path            string                 `json:"path"`
	Name            string                 `json:"name"`
	SCConfigExists  bool                   `json:"sc_config_exists"`
	SCConfigPath    string                 `json:"sc_config_path,omitempty"`
	TechStack       *TechStackInfo         `json:"tech_stack,omitempty"`
	Resources       []ResourceInfo         `json:"resources,omitempty"`
	Recommendations []string               `json:"recommendations,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

type TechStackInfo struct {
	Language     string            `json:"language,omitempty"`
	Framework    string            `json:"framework,omitempty"`
	Runtime      string            `json:"runtime,omitempty"`
	Dependencies []string          `json:"dependencies,omitempty"`
	Architecture string            `json:"architecture,omitempty"`
	Confidence   float32           `json:"confidence"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type ResourceInfo struct {
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	Provider    string            `json:"provider"`
	Description string            `json:"description,omitempty"`
	Properties  map[string]string `json:"properties,omitempty"`
	SchemaURL   string            `json:"schema_url,omitempty"`
}

type GeneratedConfiguration struct {
	ConfigType string                 `json:"config_type"`
	Files      []GeneratedFile        `json:"files"`
	Messages   []string               `json:"messages,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

type GeneratedFile struct {
	Path        string `json:"path"`
	Content     string `json:"content"`
	ContentType string `json:"content_type"` // "yaml", "dockerfile", "json", etc.
	Description string `json:"description,omitempty"`
}

type ProjectAnalysis struct {
	Path            string                 `json:"path"`
	TechStacks      []TechStackInfo        `json:"tech_stacks"`
	Architecture    string                 `json:"architecture,omitempty"`
	Recommendations []Recommendation       `json:"recommendations"`
	Files           []FileInfo             `json:"files,omitempty"`
	Timestamp       time.Time              `json:"timestamp"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

type Recommendation struct {
	Type        string `json:"type"`     // "resource", "template", "configuration"
	Category    string `json:"category"` // "database", "storage", "compute", etc.
	Priority    string `json:"priority"` // "high", "medium", "low"
	Title       string `json:"title"`
	Description string `json:"description"`
	Action      string `json:"action,omitempty"`
}

type FileInfo struct {
	Path      string            `json:"path"`
	Type      string            `json:"type"`
	Size      int64             `json:"size"`
	Language  string            `json:"language,omitempty"`
	Framework string            `json:"framework,omitempty"`
	Purpose   string            `json:"purpose,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type SupportedResourcesResult struct {
	Resources []ResourceInfo `json:"resources"`
	Providers []ProviderInfo `json:"providers"`
	Total     int            `json:"total"`
}

type SetupSimpleContainerParams struct {
	Path           string `json:"path"`
	Environment    string `json:"environment,omitempty"`
	Parent         string `json:"parent,omitempty"`
	DeploymentType string `json:"deployment_type,omitempty"`
	Interactive    bool   `json:"interactive,omitempty"`
}

type SetupSimpleContainerResult struct {
	Message      string                 `json:"message"`
	FilesCreated []string               `json:"files_created"`
	Success      bool                   `json:"success"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// MCP Elicitation types
type ElicitRequest struct {
	Message         string                 `json:"message"`
	RequestedSchema map[string]interface{} `json:"requestedSchema"`
}

type ElicitResult struct {
	Action  string                 `json:"action"` // "accept", "decline", "cancel"
	Content map[string]interface{} `json:"content,omitempty"`
}

type ProviderInfo struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Resources   []string `json:"resources"`
	Description string   `json:"description,omitempty"`
}

// MCP method handler interface
type MCPHandler interface {
	SearchDocumentation(ctx context.Context, params SearchDocumentationParams) (*DocumentationSearchResult, error)
	GetProjectContext(ctx context.Context, params GetProjectContextParams) (*ProjectContext, error)
	GenerateConfiguration(ctx context.Context, params GenerateConfigurationParams) (*GeneratedConfiguration, error)
	AnalyzeProject(ctx context.Context, params AnalyzeProjectParams) (*ProjectAnalysis, error)
	GetSupportedResources(ctx context.Context) (*SupportedResourcesResult, error)
	SetupSimpleContainer(ctx context.Context, params SetupSimpleContainerParams) (*SetupSimpleContainerResult, error)
	GetCapabilities(ctx context.Context) (map[string]interface{}, error)
	Ping(ctx context.Context) (string, error)
}

// Standard MCP error codes (following JSON-RPC specification)
const (
	ErrorCodeParseError     = -32700
	ErrorCodeInvalidRequest = -32600
	ErrorCodeMethodNotFound = -32601
	ErrorCodeInvalidParams  = -32602
	ErrorCodeInternalError  = -32603

	// Simple Container specific error codes
	ErrorCodeProjectNotFound    = -32001
	ErrorCodeConfigurationError = -32002
	ErrorCodeAnalysisError      = -32003
	ErrorCodeEmbeddingError     = -32004
	ErrorCodeGenerationError    = -32005
)

// Helper functions for creating MCP responses
func NewMCPResponse(id interface{}, result interface{}) *MCPResponse {
	return &MCPResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
}

func NewMCPError(id interface{}, code int, message string, data interface{}) *MCPResponse {
	return &MCPResponse{
		JSONRPC: "2.0",
		Error: &MCPError{
			Code:    code,
			Message: message,
			Data:    data,
		},
		ID: id,
	}
}

// Utility functions for MCP protocol
func ParseMCPRequest(data []byte) (*MCPRequest, error) {
	var req MCPRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *MCPResponse) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

func (r *MCPRequest) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}
