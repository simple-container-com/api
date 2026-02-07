package sbom

import (
	"strings"
	"testing"
)

func TestFormatIsValid(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		want   bool
	}{
		{"CycloneDX JSON valid", FormatCycloneDXJSON, true},
		{"CycloneDX XML valid", FormatCycloneDXXML, true},
		{"SPDX JSON valid", FormatSPDXJSON, true},
		{"SPDX tag-value valid", FormatSPDXTagValue, true},
		{"Syft JSON valid", FormatSyftJSON, true},
		{"Invalid format", Format("invalid"), false},
		{"Empty format", Format(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.format.IsValid(); got != tt.want {
				t.Errorf("Format.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatPredicateType(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		want   string
	}{
		{"CycloneDX JSON", FormatCycloneDXJSON, "https://cyclonedx.org/bom"},
		{"CycloneDX XML", FormatCycloneDXXML, "https://cyclonedx.org/bom"},
		{"SPDX JSON", FormatSPDXJSON, "https://spdx.dev/Document"},
		{"SPDX tag-value", FormatSPDXTagValue, "https://spdx.dev/Document"},
		{"Syft JSON", FormatSyftJSON, "https://syft.dev/bom"},
		{"Unknown format defaults to CycloneDX", Format("unknown"), "https://cyclonedx.org/bom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.format.PredicateType(); got != tt.want {
				t.Errorf("Format.PredicateType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatAttestationType(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		want   string
	}{
		{"CycloneDX JSON", FormatCycloneDXJSON, "cyclonedx"},
		{"CycloneDX XML", FormatCycloneDXXML, "cyclonedx"},
		{"SPDX JSON", FormatSPDXJSON, "spdx"},
		{"SPDX tag-value", FormatSPDXTagValue, "spdx"},
		{"Syft JSON", FormatSyftJSON, "custom"},
		{"Unknown format defaults to CycloneDX", Format("unknown"), "cyclonedx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.format.AttestationType(); got != tt.want {
				t.Errorf("Format.AttestationType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatIsCycloneDX(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		want   bool
	}{
		{"CycloneDX JSON is CycloneDX", FormatCycloneDXJSON, true},
		{"CycloneDX XML is CycloneDX", FormatCycloneDXXML, true},
		{"SPDX JSON not CycloneDX", FormatSPDXJSON, false},
		{"Syft JSON not CycloneDX", FormatSyftJSON, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.format.IsCycloneDX(); got != tt.want {
				t.Errorf("Format.IsCycloneDX() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatIsSPDX(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		want   bool
	}{
		{"SPDX JSON is SPDX", FormatSPDXJSON, true},
		{"SPDX tag-value is SPDX", FormatSPDXTagValue, true},
		{"CycloneDX JSON not SPDX", FormatCycloneDXJSON, false},
		{"Syft JSON not SPDX", FormatSyftJSON, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.format.IsSPDX(); got != tt.want {
				t.Errorf("Format.IsSPDX() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Format
		wantErr bool
	}{
		{"Parse cyclonedx-json", "cyclonedx-json", FormatCycloneDXJSON, false},
		{"Parse CYCLONEDX-JSON uppercase", "CYCLONEDX-JSON", FormatCycloneDXJSON, false},
		{"Parse with spaces", "  cyclonedx-json  ", FormatCycloneDXJSON, false},
		{"Parse cyclonedx-xml", "cyclonedx-xml", FormatCycloneDXXML, false},
		{"Parse spdx-json", "spdx-json", FormatSPDXJSON, false},
		{"Parse spdx-tag-value", "spdx-tag-value", FormatSPDXTagValue, false},
		{"Parse syft-json", "syft-json", FormatSyftJSON, false},
		{"Invalid format", "invalid-format", "", true},
		{"Empty string", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFormat(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFormat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Valid cyclonedx-json", "cyclonedx-json", false},
		{"Valid spdx-json", "spdx-json", false},
		{"Invalid format", "invalid", true},
		{"Empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFormat(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFormat() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAllFormats(t *testing.T) {
	formats := AllFormats()
	if len(formats) != 5 {
		t.Errorf("AllFormats() returned %d formats, want 5", len(formats))
	}

	// Check all expected formats are present
	expected := map[Format]bool{
		FormatCycloneDXJSON: false,
		FormatCycloneDXXML:  false,
		FormatSPDXJSON:      false,
		FormatSPDXTagValue:  false,
		FormatSyftJSON:      false,
	}

	for _, f := range formats {
		if _, ok := expected[f]; !ok {
			t.Errorf("Unexpected format in AllFormats(): %v", f)
		}
		expected[f] = true
	}

	for f, found := range expected {
		if !found {
			t.Errorf("Format %v missing from AllFormats()", f)
		}
	}
}

func TestAllFormatStrings(t *testing.T) {
	formatStrings := AllFormatStrings()
	if len(formatStrings) != 5 {
		t.Errorf("AllFormatStrings() returned %d formats, want 5", len(formatStrings))
	}

	for _, s := range formatStrings {
		if !strings.Contains(s, "-") {
			t.Errorf("Format string %q doesn't look like a valid format", s)
		}
	}
}
