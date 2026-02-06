package tools

import (
	"context"
	"testing"
)

func TestNewToolInstaller(t *testing.T) {
	installer := NewToolInstaller()
	if installer == nil {
		t.Fatal("NewToolInstaller() returned nil")
	}
	if installer.registry == nil {
		t.Error("Expected registry to be initialized")
	}
}

func TestToolInstallerListAvailableTools(t *testing.T) {
	installer := NewToolInstaller()
	tools := installer.ListAvailableTools()

	if len(tools) == 0 {
		t.Error("Expected at least some tools to be registered")
	}

	// Check for expected tools
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	expectedTools := []string{"cosign", "syft", "grype", "trivy"}
	for _, expected := range expectedTools {
		if !toolNames[expected] {
			t.Errorf("Expected tool %s to be in available tools", expected)
		}
	}
}

func TestToolInstallerGetInstallURL(t *testing.T) {
	installer := NewToolInstaller()

	tests := []struct {
		toolName string
		wantErr  bool
	}{
		{"cosign", false},
		{"syft", false},
		{"grype", false},
		{"trivy", false},
		{"nonexistent", true},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			url, err := installer.GetInstallURL(tt.toolName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetInstallURL(%s) error = %v, wantErr %v", tt.toolName, err, tt.wantErr)
			}
			if !tt.wantErr && url == "" {
				t.Errorf("Expected non-empty install URL for %s", tt.toolName)
			}
		})
	}
}

func TestToolInstallerIsToolAvailable(t *testing.T) {
	installer := NewToolInstaller()
	ctx := context.Background()

	// This test will depend on what's actually installed on the system
	// We can only test that it doesn't panic
	_ = installer.IsToolAvailable(ctx, "cosign")
	_ = installer.IsToolAvailable(ctx, "nonexistent-tool")
}

func TestToolInstallerGetToolCommand(t *testing.T) {
	installer := NewToolInstaller()

	tests := []struct {
		toolName string
		want     string
		wantErr  bool
	}{
		{"cosign", "cosign", false},
		{"syft", "syft", false},
		{"nonexistent", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			cmd, err := installer.GetToolCommand(tt.toolName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetToolCommand(%s) error = %v, wantErr %v", tt.toolName, err, tt.wantErr)
			}
			if !tt.wantErr && cmd != tt.want {
				t.Errorf("GetToolCommand(%s) = %s, want %s", tt.toolName, cmd, tt.want)
			}
		})
	}
}

func TestToolRegistryGetTool(t *testing.T) {
	registry := NewToolRegistry()

	tool, err := registry.GetTool("cosign")
	if err != nil {
		t.Fatalf("GetTool(cosign) failed: %v", err)
	}

	if tool.Name != "cosign" {
		t.Errorf("Expected tool name 'cosign', got '%s'", tool.Name)
	}

	if tool.Command == "" {
		t.Error("Expected non-empty command")
	}

	if tool.MinVersion == "" {
		t.Error("Expected non-empty minimum version")
	}

	if tool.InstallURL == "" {
		t.Error("Expected non-empty install URL")
	}
}

func TestToolRegistryHasTool(t *testing.T) {
	registry := NewToolRegistry()

	tests := []struct {
		name string
		want bool
	}{
		{"cosign", true},
		{"syft", true},
		{"grype", true},
		{"trivy", true},
		{"nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := registry.HasTool(tt.name)
			if got != tt.want {
				t.Errorf("HasTool(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestToolRegistryCount(t *testing.T) {
	registry := NewToolRegistry()
	count := registry.Count()

	// Should have at least the default tools
	if count < 4 {
		t.Errorf("Expected at least 4 tools, got %d", count)
	}
}

func TestToolRegistryGetToolsByCategory(t *testing.T) {
	registry := NewToolRegistry()

	tests := []struct {
		category     string
		expectedMin  int
		shouldHave   []string
	}{
		{"signing", 1, []string{"cosign"}},
		{"sbom", 1, []string{"syft"}},
		{"scan", 2, []string{"grype", "trivy"}},
	}

	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			tools := registry.GetToolsByCategory(tt.category)
			if len(tools) < tt.expectedMin {
				t.Errorf("Expected at least %d tools for category %s, got %d",
					tt.expectedMin, tt.category, len(tools))
			}

			toolNames := make(map[string]bool)
			for _, tool := range tools {
				toolNames[tool.Name] = true
			}

			for _, expected := range tt.shouldHave {
				if !toolNames[expected] {
					t.Errorf("Expected tool %s in category %s", expected, tt.category)
				}
			}
		})
	}
}

func TestToolRegistryRegisterAndUnregister(t *testing.T) {
	registry := NewToolRegistry()

	customTool := ToolMetadata{
		Name:        "custom-tool",
		Command:     "custom",
		MinVersion:  "1.0.0",
		InstallURL:  "https://example.com/install",
		Description: "Custom security tool",
		VersionFlag: "version",
	}

	// Register
	registry.Register(customTool)

	if !registry.HasTool("custom-tool") {
		t.Error("Expected custom tool to be registered")
	}

	// Unregister
	registry.Unregister("custom-tool")

	if registry.HasTool("custom-tool") {
		t.Error("Expected custom tool to be unregistered")
	}
}
