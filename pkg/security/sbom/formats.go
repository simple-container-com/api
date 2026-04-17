// Package sbom provides Software Bill of Materials (SBOM) generation and attestation functionality
package sbom

import (
	"fmt"
	"strings"
)

// Format represents an SBOM format
type Format string

const (
	// FormatCycloneDXJSON is the CycloneDX JSON format (default)
	FormatCycloneDXJSON Format = "cyclonedx-json"
	// FormatCycloneDXXML is the CycloneDX XML format
	FormatCycloneDXXML Format = "cyclonedx-xml"
	// FormatSPDXJSON is the SPDX JSON format
	FormatSPDXJSON Format = "spdx-json"
	// FormatSPDXTagValue is the SPDX tag-value format
	FormatSPDXTagValue Format = "spdx-tag-value"
	// FormatSyftJSON is the Syft native JSON format
	FormatSyftJSON Format = "syft-json"
)

// AllFormats returns all supported SBOM formats
func AllFormats() []Format {
	return []Format{
		FormatCycloneDXJSON,
		FormatCycloneDXXML,
		FormatSPDXJSON,
		FormatSPDXTagValue,
		FormatSyftJSON,
	}
}

// AllFormatStrings returns all supported SBOM format strings
func AllFormatStrings() []string {
	formats := AllFormats()
	result := make([]string, len(formats))
	for i, f := range formats {
		result[i] = string(f)
	}
	return result
}

// IsValid checks if the format is valid
func (f Format) IsValid() bool {
	for _, valid := range AllFormats() {
		if f == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the format
func (f Format) String() string {
	return string(f)
}

// PredicateType returns the predicate type for cosign attestation
func (f Format) PredicateType() string {
	switch f {
	case FormatCycloneDXJSON, FormatCycloneDXXML:
		return "https://cyclonedx.org/bom"
	case FormatSPDXJSON, FormatSPDXTagValue:
		return "https://spdx.dev/Document"
	case FormatSyftJSON:
		return "https://syft.dev/bom"
	default:
		return "https://cyclonedx.org/bom" // default
	}
}

// AttestationType returns the attestation type for cosign
func (f Format) AttestationType() string {
	switch f {
	case FormatCycloneDXJSON, FormatCycloneDXXML:
		return "cyclonedx"
	case FormatSPDXJSON, FormatSPDXTagValue:
		return "spdx"
	case FormatSyftJSON:
		return "custom"
	default:
		return "cyclonedx" // default
	}
}

// IsCycloneDX checks if the format is CycloneDX
func (f Format) IsCycloneDX() bool {
	return f == FormatCycloneDXJSON || f == FormatCycloneDXXML
}

// IsSPDX checks if the format is SPDX
func (f Format) IsSPDX() bool {
	return f == FormatSPDXJSON || f == FormatSPDXTagValue
}

// ParseFormat parses a format string
func ParseFormat(s string) (Format, error) {
	f := Format(strings.ToLower(strings.TrimSpace(s)))
	if !f.IsValid() {
		return "", fmt.Errorf("invalid SBOM format: %s (supported: %s)", s, strings.Join(AllFormatStrings(), ", "))
	}
	return f, nil
}

// ValidateFormat validates a format string
func ValidateFormat(s string) error {
	_, err := ParseFormat(s)
	return err
}
