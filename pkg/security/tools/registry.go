package tools

import (
	"fmt"
	"sync"
)

// ToolMetadata contains metadata about a security tool
type ToolMetadata struct {
	Name        string // Tool name (e.g., "cosign", "syft")
	Command     string // Command to execute (usually same as name)
	MinVersion  string // Minimum required version (e.g., "3.0.2")
	InstallURL  string // URL with installation instructions
	Description string // Brief description
	VersionFlag string // Flag to get version (e.g., "version" or "--version")
}

// ToolRegistry maintains a registry of available security tools
type ToolRegistry struct {
	tools map[string]ToolMetadata
	mu    sync.RWMutex
}

// NewToolRegistry creates a new tool registry with default tools
func NewToolRegistry() *ToolRegistry {
	registry := &ToolRegistry{
		tools: make(map[string]ToolMetadata),
	}

	// Register default tools
	registry.registerDefaultTools()

	return registry
}

// registerDefaultTools registers the default security tools
func (r *ToolRegistry) registerDefaultTools() {
	// Cosign - Image signing and verification
	r.Register(ToolMetadata{
		Name:        "cosign",
		Command:     "cosign",
		MinVersion:  "3.0.2",
		InstallURL:  "https://docs.sigstore.dev/cosign/installation/",
		Description: "Container image signing and verification tool",
		VersionFlag: "version",
	})

	// Syft - SBOM generation
	r.Register(ToolMetadata{
		Name:        "syft",
		Command:     "syft",
		MinVersion:  "1.41.0",
		InstallURL:  "https://github.com/anchore/syft#installation",
		Description: "SBOM generation tool for container images",
		VersionFlag: "version",
	})

	// Grype - Vulnerability scanning
	r.Register(ToolMetadata{
		Name:        "grype",
		Command:     "grype",
		MinVersion:  "0.106.0",
		InstallURL:  "https://github.com/anchore/grype#installation",
		Description: "Vulnerability scanner for container images",
		VersionFlag: "version",
	})

	// Trivy - Multi-purpose security scanner
	r.Register(ToolMetadata{
		Name:        "trivy",
		Command:     "trivy",
		MinVersion:  "0.68.2",
		InstallURL:  "https://aquasecurity.github.io/trivy/latest/getting-started/installation/",
		Description: "Comprehensive security scanner for containers",
		VersionFlag: "version",
	})
}

// Register adds or updates a tool in the registry
func (r *ToolRegistry) Register(tool ToolMetadata) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tools[tool.Name] = tool
}

// GetTool retrieves tool metadata by name
func (r *ToolRegistry) GetTool(name string) (ToolMetadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	if !exists {
		return ToolMetadata{}, fmt.Errorf("tool '%s' not found in registry", name)
	}

	return tool, nil
}

// ListTools returns all registered tools
func (r *ToolRegistry) ListTools() []ToolMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]ToolMetadata, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}

	return tools
}

// HasTool checks if a tool is registered
func (r *ToolRegistry) HasTool(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.tools[name]
	return exists
}

// GetToolsByCategory returns tools that match a category
// Category can be: "signing", "sbom", "scan", "provenance"
func (r *ToolRegistry) GetToolsByCategory(category string) []ToolMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var tools []ToolMetadata

	switch category {
	case "signing":
		if tool, exists := r.tools["cosign"]; exists {
			tools = append(tools, tool)
		}
	case "sbom":
		if tool, exists := r.tools["syft"]; exists {
			tools = append(tools, tool)
		}
	case "scan":
		if tool, exists := r.tools["grype"]; exists {
			tools = append(tools, tool)
		}
		if tool, exists := r.tools["trivy"]; exists {
			tools = append(tools, tool)
		}
	}

	return tools
}

// GetRequiredTools returns tools required for a given operation
func (r *ToolRegistry) GetRequiredTools(operations []string) []ToolMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	toolMap := make(map[string]ToolMetadata)

	for _, op := range operations {
		switch op {
		case "signing", "sign", "verify":
			if tool, exists := r.tools["cosign"]; exists {
				toolMap["cosign"] = tool
			}
		case "sbom":
			if tool, exists := r.tools["syft"]; exists {
				toolMap["syft"] = tool
			}
		case "scan", "grype":
			if tool, exists := r.tools["grype"]; exists {
				toolMap["grype"] = tool
			}
		case "trivy":
			if tool, exists := r.tools["trivy"]; exists {
				toolMap["trivy"] = tool
			}
		case "provenance":
			// Provenance uses cosign for attestation
			if tool, exists := r.tools["cosign"]; exists {
				toolMap["cosign"] = tool
			}
		}
	}

	// Convert map to slice
	tools := make([]ToolMetadata, 0, len(toolMap))
	for _, tool := range toolMap {
		tools = append(tools, tool)
	}

	return tools
}

// Unregister removes a tool from the registry
func (r *ToolRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.tools, name)
}

// Count returns the number of registered tools
func (r *ToolRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.tools)
}

// GetMinVersion returns the minimum version for a tool
func (r *ToolRegistry) GetMinVersion(name string) (string, error) {
	tool, err := r.GetTool(name)
	if err != nil {
		return "", err
	}
	return tool.MinVersion, nil
}

// GetInstallURL returns the installation URL for a tool
func (r *ToolRegistry) GetInstallURL(name string) (string, error) {
	tool, err := r.GetTool(name)
	if err != nil {
		return "", err
	}
	return tool.InstallURL, nil
}
