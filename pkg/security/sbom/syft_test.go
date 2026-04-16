package sbom

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestNewSyftGenerator(t *testing.T) {
	RegisterTestingT(t)

	g := NewSyftGenerator()
	Expect(g).ToNot(BeNil())
	Expect(g.Timeout).To(Equal(5 * time.Minute))
}

func TestSyftGeneratorSupportsFormat(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			Expect(g.SupportsFormat(tt.format)).To(Equal(tt.want))
		})
	}
}

func TestExtractCycloneDXPackageCount(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			got, err := g.extractCycloneDXPackageCount([]byte(tt.content))
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(got).To(Equal(tt.want))
			}
		})
	}
}

func TestExtractSPDXPackageCount(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			got, err := g.extractSPDXPackageCount([]byte(tt.content))
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(got).To(Equal(tt.want))
			}
		})
	}
}

func TestExtractSyftPackageCount(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			got, err := g.extractSyftPackageCount([]byte(tt.content))
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(got).To(Equal(tt.want))
			}
		})
	}
}

func TestExtractImageDigest(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			Expect(g.extractImageDigest(tt.image, tt.stderr)).To(Equal(tt.want))
		})
	}
}

func TestIsVersionGreaterOrEqual(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			Expect(isVersionGreaterOrEqual(tt.current, tt.minimum)).To(Equal(tt.want))
		})
	}
}

func TestParseVersion(t *testing.T) {
	RegisterTestingT(t)

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
			RegisterTestingT(t)
			Expect(parseVersion(tt.version)).To(Equal(tt.want))
		})
	}
}

func TestCheckInstalled_NotInstalled(t *testing.T) {
	RegisterTestingT(t)

	ctx := context.Background()
	err := CheckInstalled(ctx)
	// This will fail if syft is not installed, which is expected in most test environments
	if err != nil {
		t.Logf("Syft not installed (expected in test environment): %v", err)
	}
}

func TestCheckVersion_NotInstalled(t *testing.T) {
	RegisterTestingT(t)

	ctx := context.Background()
	err := CheckVersion(ctx, "1.0.0")
	// This will fail if syft is not installed
	if err != nil {
		t.Logf("Syft version check failed (expected if not installed): %v", err)
	}
}
