package tools

// ToolMetadata contains metadata about a security tool
type ToolMetadata struct {
	Name       string
	Command    string
	MinVersion string
	InstallURL string
	Required   bool
}

// ToolRegistry contains metadata for all supported security tools
var ToolRegistry = map[string]ToolMetadata{
	"cosign": {
		Name:       "Cosign",
		Command:    "cosign",
		MinVersion: "v3.0.2",
		InstallURL: "https://docs.sigstore.dev/cosign/installation/",
		Required:   true,
	},
	"syft": {
		Name:       "Syft",
		Command:    "syft",
		MinVersion: "v1.41.0",
		InstallURL: "https://github.com/anchore/syft#installation",
		Required:   true,
	},
	"grype": {
		Name:       "Grype",
		Command:    "grype",
		MinVersion: "v0.106.0",
		InstallURL: "https://github.com/anchore/grype#installation",
		Required:   true,
	},
	"trivy": {
		Name:       "Trivy",
		Command:    "trivy",
		MinVersion: "v0.68.2",
		InstallURL: "https://aquasecurity.github.io/trivy/latest/getting-started/installation/",
		Required:   false,
	},
}
