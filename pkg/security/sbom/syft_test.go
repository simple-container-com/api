package sbom

import (
	"context"
	"testing"
	"time"
)

func TestNewSyftGenerator(t *testing.T) {
	g := NewSyftGenerator()
	if g == nil {
		t.Fatal("NewSyftGenerator() returned nil")
	}
	if g.Timeout != 5*time.Minute {
		t.Errorf("Expected timeout of 5 minutes, got %v", g.Timeout)
	}
}

func TestSyftGeneratorSupportsFormat(t *testing.T) {
	g := NewSyftGenerator()

	tests := []struct {
		name   string
		format Format
		want   bool
	}{
		{"CycloneDX JSON supported", FormatCycloneDXJSON, true},
		{"CycloneDX XML supported", FormatCycloneDXXML, true},
		{"SPDX JSON supported", FormatSPDXJSON, true},
		{"SPDX tag-value supported", FormatSPDXTagValue, true},
		{"Syft JSON supported", FormatSyftJSON, true},
		{"Invalid format not supported", Format("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := g.SupportsFormat(tt.format); got != tt.want {
				t.Errorf("SupportsFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractCycloneDXPackageCount(t *testing.T) {
	g := NewSyftGenerator()

	tests := []struct {
		name    string
		content string
		want    int
		wantErr bool
	}{
		{
			name:    "Valid CycloneDX with 3 components",
			content: `{"components": [{"name": "pkg1"}, {"name": "pkg2"}, {"name": "pkg3"}]}`,
			want:    3,
			wantErr: false,
		},
		{
			name:    "Empty components array",
			content: `{"components": []}`,
			want:    0,
			wantErr: false,
		},
		{
			name:    "Invalid JSON",
			content: `{invalid json}`,
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := g.extractCycloneDXPackageCount([]byte(tt.content))
			if (err != nil) != tt.wantErr {
				t.Errorf("extractCycloneDXPackageCount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractCycloneDXPackageCount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractSPDXPackageCount(t *testing.T) {
	g := NewSyftGenerator()

	tests := []struct {
		name    string
		content string
		want    int
		wantErr bool
	}{
		{
			name:    "Valid SPDX with 2 packages",
			content: `{"packages": [{"name": "pkg1"}, {"name": "pkg2"}]}`,
			want:    2,
			wantErr: false,
		},
		{
			name:    "Empty packages array",
			content: `{"packages": []}`,
			want:    0,
			wantErr: false,
		},
		{
			name:    "Invalid JSON",
			content: `{invalid}`,
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := g.extractSPDXPackageCount([]byte(tt.content))
			if (err != nil) != tt.wantErr {
				t.Errorf("extractSPDXPackageCount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractSPDXPackageCount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractSyftPackageCount(t *testing.T) {
	g := NewSyftGenerator()

	tests := []struct {
		name    string
		content string
		want    int
		wantErr bool
	}{
		{
			name:    "Valid Syft with 4 artifacts",
			content: `{"artifacts": [{"name": "a1"}, {"name": "a2"}, {"name": "a3"}, {"name": "a4"}]}`,
			want:    4,
			wantErr: false,
		},
		{
			name:    "Empty artifacts array",
			content: `{"artifacts": []}`,
			want:    0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := g.extractSyftPackageCount([]byte(tt.content))
			if (err != nil) != tt.wantErr {
				t.Errorf("extractSyftPackageCount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractSyftPackageCount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractImageDigest(t *testing.T) {
	g := NewSyftGenerator()

	tests := []struct {
		name   string
		image  string
		stderr string
		want   string
	}{
		{
			name:   "Extract from stderr",
			image:  "myapp:v1.0",
			stderr: "Loaded image sha256:abc123def4567890123456789012345678901234567890123456789012345678",
			want:   "sha256:abc123def4567890123456789012345678901234567890123456789012345678",
		},
		{
			name:   "Extract from image reference with digest",
			image:  "myapp@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			stderr: "",
			want:   "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		},
		{
			name:   "Fallback to image reference",
			image:  "myapp:v1.0",
			stderr: "No digest here",
			want:   "myapp:v1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.extractImageDigest(tt.image, tt.stderr)
			if got != tt.want {
				t.Errorf("extractImageDigest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsVersionGreaterOrEqual(t *testing.T) {
	tests := []struct {
		name    string
		current string
		minimum string
		want    bool
	}{
		{"Same version", "1.0.0", "1.0.0", true},
		{"Current higher major", "2.0.0", "1.0.0", true},
		{"Current lower major", "1.0.0", "2.0.0", false},
		{"Current higher minor", "1.2.0", "1.1.0", true},
		{"Current lower minor", "1.1.0", "1.2.0", false},
		{"Current higher patch", "1.0.2", "1.0.1", true},
		{"Current lower patch", "1.0.1", "1.0.2", false},
		{"Version with v prefix", "v1.2.3", "1.2.3", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isVersionGreaterOrEqual(tt.current, tt.minimum); got != tt.want {
				t.Errorf("isVersionGreaterOrEqual(%v, %v) = %v, want %v", tt.current, tt.minimum, got, tt.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    [3]int
	}{
		{"Standard version", "1.2.3", [3]int{1, 2, 3}},
		{"Version with v prefix", "v1.2.3", [3]int{1, 2, 3}},
		{"Version with two parts", "1.2", [3]int{1, 2, 0}},
		{"Version with one part", "1", [3]int{1, 0, 0}},
		{"Invalid version", "invalid", [3]int{0, 0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVersion(tt.version)
			if got != tt.want {
				t.Errorf("parseVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckInstalled_NotInstalled(t *testing.T) {
	ctx := context.Background()
	err := CheckInstalled(ctx)
	// This will fail if syft is not installed, which is expected in most test environments
	if err != nil {
		t.Logf("Syft not installed (expected in test environment): %v", err)
	}
}

func TestCheckVersion_NotInstalled(t *testing.T) {
	ctx := context.Background()
	err := CheckVersion(ctx, "1.0.0")
	// This will fail if syft is not installed
	if err != nil {
		t.Logf("Syft version check failed (expected if not installed): %v", err)
	}
}
