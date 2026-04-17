package sbom

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// SBOM represents a Software Bill of Materials
type SBOM struct {
	// Format is the SBOM format
	Format Format `json:"format"`

	// Content is the raw SBOM content
	Content []byte `json:"content"`

	// Digest is the SHA256 hash of the SBOM content
	Digest string `json:"digest"`

	// ImageDigest is the digest of the image this SBOM was generated for
	ImageDigest string `json:"imageDigest"`

	// GeneratedAt is when the SBOM was generated
	GeneratedAt time.Time `json:"generatedAt"`

	// Metadata contains information about the generation
	Metadata *Metadata `json:"metadata"`
}

// Metadata contains SBOM generation metadata
type Metadata struct {
	// ToolName is the name of the tool used to generate the SBOM
	ToolName string `json:"toolName"`

	// ToolVersion is the version of the tool
	ToolVersion string `json:"toolVersion"`

	// PackageCount is the number of packages found
	PackageCount int `json:"packageCount"`
}

// Generator is the interface for SBOM generators
type Generator interface {
	// Generate generates an SBOM for the given image
	Generate(ctx context.Context, image string, format Format) (*SBOM, error)

	// SupportsFormat checks if the generator supports the given format
	SupportsFormat(format Format) bool

	// Version returns the version of the generator tool
	Version(ctx context.Context) (string, error)
}

// NewSBOM creates a new SBOM
func NewSBOM(format Format, content []byte, imageDigest string, metadata *Metadata) *SBOM {
	hash := sha256.Sum256(content)
	digest := hex.EncodeToString(hash[:])

	return &SBOM{
		Format:      format,
		Content:     content,
		Digest:      digest,
		ImageDigest: imageDigest,
		GeneratedAt: time.Now(),
		Metadata:    metadata,
	}
}

// ValidateDigest validates the SBOM content against its digest
func (s *SBOM) ValidateDigest() bool {
	hash := sha256.Sum256(s.Content)
	expectedDigest := hex.EncodeToString(hash[:])
	return s.Digest == expectedDigest
}

// Size returns the size of the SBOM content in bytes
func (s *SBOM) Size() int {
	return len(s.Content)
}
