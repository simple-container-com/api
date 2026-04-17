package tools

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
)

func TestNewToolInstaller(t *testing.T) {
	RegisterTestingT(t)

	installer := NewToolInstaller()
	Expect(installer).ToNot(BeNil())
	Expect(installer.registry).ToNot(BeNil())
}

func TestToolInstallerListAvailableTools(t *testing.T) {
	RegisterTestingT(t)

	installer := NewToolInstaller()
	tools := installer.ListAvailableTools()

	Expect(tools).ToNot(BeEmpty())

	// Check for expected tools
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	expectedTools := []string{"cosign", "syft", "grype", "trivy"}
	for _, expected := range expectedTools {
		Expect(toolNames[expected]).To(BeTrue(), "Expected tool %s to be in available tools", expected)
	}
}

func TestToolInstallerGetInstallURL(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			url, err := installer.GetInstallURL(tt.toolName)
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(url).ToNot(BeEmpty())
			}
		})
	}
}

func TestToolInstallerIsToolAvailable(t *testing.T) {
	RegisterTestingT(t)

	installer := NewToolInstaller()
	ctx := context.Background()

	// Unknown tool should never be available
	Expect(installer.IsToolAvailable(ctx, "nonexistent-tool-that-does-not-exist")).To(BeFalse())

	// Known tools: result depends on system state, but should not panic
	_ = installer.IsToolAvailable(ctx, "cosign")
}

func TestToolInstallerGetToolCommand(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			cmd, err := installer.GetToolCommand(tt.toolName)
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(cmd).To(Equal(tt.want))
			}
		})
	}
}

func TestToolRegistryGetTool(t *testing.T) {
	RegisterTestingT(t)

	registry := NewToolRegistry()

	tool, err := registry.GetTool("cosign")
	Expect(err).ToNot(HaveOccurred())
	Expect(tool.Name).To(Equal("cosign"))
	Expect(tool.Command).ToNot(BeEmpty())
	Expect(tool.MinVersion).ToNot(BeEmpty())
	Expect(tool.InstallURL).ToNot(BeEmpty())
}

func TestToolRegistryHasTool(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			Expect(registry.HasTool(tt.name)).To(Equal(tt.want))
		})
	}
}

func TestToolRegistryCount(t *testing.T) {
	RegisterTestingT(t)

	registry := NewToolRegistry()
	Expect(registry.Count()).To(BeNumerically(">=", 4))
}

func TestToolRegistryGetToolsByCategory(t *testing.T) {
	RegisterTestingT(t)

	registry := NewToolRegistry()

	tests := []struct {
		category    string
		expectedMin int
		shouldHave  []string
	}{
		{"signing", 1, []string{"cosign"}},
		{"sbom", 1, []string{"syft"}},
		{"scan", 2, []string{"grype", "trivy"}},
	}

	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			RegisterTestingT(t)
			tools := registry.GetToolsByCategory(tt.category)
			Expect(len(tools)).To(BeNumerically(">=", tt.expectedMin))

			toolNames := make(map[string]bool)
			for _, tool := range tools {
				toolNames[tool.Name] = true
			}

			for _, expected := range tt.shouldHave {
				Expect(toolNames[expected]).To(BeTrue(), "Expected tool %s in category %s", expected, tt.category)
			}
		})
	}
}

func TestToolRegistryRegisterAndUnregister(t *testing.T) {
	RegisterTestingT(t)

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
	Expect(registry.HasTool("custom-tool")).To(BeTrue())

	// Unregister
	registry.Unregister("custom-tool")
	Expect(registry.HasTool("custom-tool")).To(BeFalse())
}

func TestInstallScriptVersionValidation(t *testing.T) {
	RegisterTestingT(t)

	validVersions := []string{"2.4.1", "0.98.0", "1.0.0-rc1", "3.2.1-beta.2"}
	invalidVersions := []string{"1.0; rm -rf /", "$(whoami)", "v1.0.0", "latest", ""}

	for _, v := range validVersions {
		_, err := installScript("cosign", v, "/tmp")
		Expect(err).ToNot(HaveOccurred(), "installScript(cosign, %q) should succeed", v)
	}
	for _, v := range invalidVersions {
		_, err := installScript("cosign", v, "/tmp")
		Expect(err).To(HaveOccurred(), "installScript(cosign, %q) should fail", v)
	}
}

func TestInstallScriptPlatformDetection(t *testing.T) {
	RegisterTestingT(t)

	// Verify syft/grype scripts detect OS/arch at runtime
	for _, tool := range []string{"syft", "grype"} {
		script, err := installScript(tool, "0.98.0", "/usr/local/bin")
		Expect(err).ToNot(HaveOccurred())
		Expect(containsAll(script, "uname -s", "uname -m", "${OS}", "${ARCH}")).To(BeTrue(),
			"installScript(%s) should detect OS/arch at runtime", tool)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
