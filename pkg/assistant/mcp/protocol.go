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
	Message   string          `json:"message,omitempty"` // Optional message for errors or info
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

// Configuration modification types

type GetCurrentConfigParams struct {
	ConfigType string `json:"config_type"`          // "client" or "server"
	StackName  string `json:"stack_name,omitempty"` // For client.yaml, optional stack name
}

type GetCurrentConfigResult struct {
	ConfigType string                 `json:"config_type"`
	FilePath   string                 `json:"file_path"`
	Content    map[string]interface{} `json:"content"`
	Message    string                 `json:"message"`
	Success    bool                   `json:"success"`
}

type AddEnvironmentParams struct {
	StackName      string                 `json:"stack_name"`       // Name of new environment/stack
	DeploymentType string                 `json:"deployment_type"`  // "static", "single-image", "cloud-compose"
	Parent         string                 `json:"parent"`           // Parent stack reference (project/stack)
	ParentEnv      string                 `json:"parent_env"`       // Parent environment to map to
	Config         map[string]interface{} `json:"config,omitempty"` // Additional configuration
}

type AddEnvironmentResult struct {
	StackName   string                 `json:"stack_name"`
	FilePath    string                 `json:"file_path"`
	Message     string                 `json:"message"`
	Success     bool                   `json:"success"`
	ConfigAdded map[string]interface{} `json:"config_added"`
}

type ModifyStackConfigParams struct {
	StackName string                 `json:"stack_name"` // Which stack to modify
	Changes   map[string]interface{} `json:"changes"`    // What to change
}

type ModifyStackConfigResult struct {
	StackName      string                 `json:"stack_name"`
	FilePath       string                 `json:"file_path"`
	Message        string                 `json:"message"`
	Success        bool                   `json:"success"`
	ChangesApplied map[string]interface{} `json:"changes_applied"`
}

type AddResourceParams struct {
	ResourceName string                 `json:"resource_name"` // Name of the resource
	ResourceType string                 `json:"resource_type"` // "mongodb-atlas", "redis", "postgres", etc.
	Environment  string                 `json:"environment"`   // Which environment to add it to
	Config       map[string]interface{} `json:"config"`        // Resource configuration
}

type AddResourceResult struct {
	ResourceName string                 `json:"resource_name"`
	Environment  string                 `json:"environment"`
	FilePath     string                 `json:"file_path"`
	Message      string                 `json:"message"`
	Success      bool                   `json:"success"`
	ConfigAdded  map[string]interface{} `json:"config_added"`
}

type ProviderInfo struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Resources   []string `json:"resources"`
	Description string   `json:"description,omitempty"`
}

// New tool parameter types
type ReadProjectFileParams struct {
	Filename string `json:"filename"` // Name of the file to read
}

type ReadProjectFileResult struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
	Message  string `json:"message"`
	Success  bool   `json:"success"`
}

type ShowStackConfigParams struct {
	StackName  string `json:"stack_name"`            // Stack name to show
	ConfigType string `json:"config_type,omitempty"` // "client" or "server" (optional)
}

type ShowStackConfigResult struct {
	StackName  string `json:"stack_name"`
	ConfigType string `json:"config_type"`
	FilePath   string `json:"file_path"`
	Content    string `json:"content"`
	Message    string `json:"message"`
	Success    bool   `json:"success"`
}

type AdvancedSearchDocumentationParams struct {
	Query string `json:"query"` // Search query
	Limit int    `json:"limit,omitempty"`
}

type AdvancedSearchDocumentationResult struct {
	Query   string          `json:"query"`
	Results []DocumentChunk `json:"results"`
	Total   int             `json:"total"`
	Message string          `json:"message"`
	Success bool            `json:"success"`
}

type GetHelpParams struct {
	ToolName string `json:"tool_name,omitempty"` // Specific tool to get help for (optional)
}

type GetHelpResult struct {
	ToolName string `json:"tool_name,omitempty"`
	Message  string `json:"message"`
	Success  bool   `json:"success"`
}

type GetStatusParams struct {
	Detailed bool   `json:"detailed,omitempty"` // Show detailed diagnostic information
	Path     string `json:"path,omitempty"`     // Project path to analyze (default: current directory)
}

type GetStatusResult struct {
	Status  string                 `json:"status"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
	Success bool                   `json:"success"`
}

type WriteProjectFileParams struct {
	Filename string `json:"filename"`         // File name to write
	Content  string `json:"content"`          // Content to write to the file
	Lines    string `json:"lines,omitempty"`  // Line range to replace (e.g., '10-20' or '5' for single line)
	Append   bool   `json:"append,omitempty"` // Append content to end of file instead of replacing
}

type WriteProjectFileResult struct {
	Message string   `json:"message"`
	Files   []string `json:"files,omitempty"`
	Success bool     `json:"success"`
}

// MCP method handler interface
type MCPHandler interface {
	SearchDocumentation(ctx context.Context, params SearchDocumentationParams) (*DocumentationSearchResult, error)
	GetProjectContext(ctx context.Context, params GetProjectContextParams) (*ProjectContext, error)
	GenerateConfiguration(ctx context.Context, params GenerateConfigurationParams) (*GeneratedConfiguration, error)
	AnalyzeProject(ctx context.Context, params AnalyzeProjectParams) (*ProjectAnalysis, error)
	GetSupportedResources(ctx context.Context) (*SupportedResourcesResult, error)
	SetupSimpleContainer(ctx context.Context, params SetupSimpleContainerParams) (*SetupSimpleContainerResult, error)

	// Configuration modification methods
	GetCurrentConfig(ctx context.Context, params GetCurrentConfigParams) (*GetCurrentConfigResult, error)
	AddEnvironment(ctx context.Context, params AddEnvironmentParams) (*AddEnvironmentResult, error)
	ModifyStackConfig(ctx context.Context, params ModifyStackConfigParams) (*ModifyStackConfigResult, error)
	AddResource(ctx context.Context, params AddResourceParams) (*AddResourceResult, error)

	// New chat command equivalent methods
	ReadProjectFile(ctx context.Context, params ReadProjectFileParams) (*ReadProjectFileResult, error)
	ShowStackConfig(ctx context.Context, params ShowStackConfigParams) (*ShowStackConfigResult, error)
	AdvancedSearchDocumentation(ctx context.Context, params AdvancedSearchDocumentationParams) (*AdvancedSearchDocumentationResult, error)
	GetHelp(ctx context.Context, params GetHelpParams) (*GetHelpResult, error)
	GetStatus(ctx context.Context, params GetStatusParams) (*GetStatusResult, error)
	WriteProjectFile(ctx context.Context, params WriteProjectFileParams) (*WriteProjectFileResult, error)

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
	ErrorCodeFileOperationError = -32006
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
