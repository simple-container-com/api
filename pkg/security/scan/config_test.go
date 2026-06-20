package scan

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestDefaultConfig(t *testing.T) {
	RegisterTestingT(t)

	cfg := DefaultConfig()
	Expect(cfg).ToNot(BeNil())
	Expect(cfg.Enabled).To(BeTrue())
	Expect(cfg.Tools).To(ConsistOf(ScanToolGrype))
	Expect(cfg.FailOn).To(Equal(Severity("")))
	Expect(cfg.WarnOn).To(Equal(SeverityHigh))
	Expect(cfg.Required).To(BeFalse())
	Expect(cfg.Output).ToNot(BeNil())
	Expect(cfg.Cache).ToNot(BeNil())
	Expect(cfg.Cache.Enabled).To(BeTrue())
	Expect(cfg.Cache.TTL).To(Equal(6))
}

func TestConfig_Validate(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name      string
		config    *Config
		shouldErr bool
		errSubstr string
	}{
		{
			name:      "disabled config skips validation",
			config:    &Config{Enabled: false, Tools: nil},
			shouldErr: false,
		},
		{
			name:      "disabled config skips invalid tool",
			config:    &Config{Enabled: false, Tools: []ScanTool{"bogus"}},
			shouldErr: false,
		},
		{
			name:      "no tools is an error",
			config:    &Config{Enabled: true, Tools: nil},
			shouldErr: true,
			errSubstr: "at least one scanning tool",
		},
		{
			name:      "empty tools slice is an error",
			config:    &Config{Enabled: true, Tools: []ScanTool{}},
			shouldErr: true,
			errSubstr: "at least one scanning tool",
		},
		{
			name:      "invalid tool is an error",
			config:    &Config{Enabled: true, Tools: []ScanTool{ScanToolGrype, "snyk"}},
			shouldErr: true,
			errSubstr: "invalid scan tool: snyk",
		},
		{
			name:      "all valid tools accepted",
			config:    &Config{Enabled: true, Tools: []ScanTool{ScanToolGrype, ScanToolTrivy, ScanToolAll}},
			shouldErr: false,
		},
		{
			name:      "invalid failOn severity is an error",
			config:    &Config{Enabled: true, Tools: []ScanTool{ScanToolGrype}, FailOn: "severe"},
			shouldErr: true,
			errSubstr: "invalid failOn severity: severe",
		},
		{
			name:      "invalid warnOn severity is an error",
			config:    &Config{Enabled: true, Tools: []ScanTool{ScanToolGrype}, WarnOn: "kinda-bad"},
			shouldErr: true,
			errSubstr: "invalid warnOn severity: kinda-bad",
		},
		{
			name:      "empty failOn and warnOn are allowed",
			config:    &Config{Enabled: true, Tools: []ScanTool{ScanToolGrype}, FailOn: "", WarnOn: ""},
			shouldErr: false,
		},
		{
			name:      "valid failOn and warnOn accepted",
			config:    &Config{Enabled: true, Tools: []ScanTool{ScanToolTrivy}, FailOn: SeverityCritical, WarnOn: SeverityHigh},
			shouldErr: false,
		},
		{
			name:      "default config is valid",
			config:    DefaultConfig(),
			shouldErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tt.config.Validate()
			if tt.shouldErr {
				Expect(err).To(HaveOccurred())
				if tt.errSubstr != "" {
					Expect(err.Error()).To(ContainSubstring(tt.errSubstr))
				}
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestIsValidSeverity(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		input Severity
		valid bool
	}{
		{SeverityCritical, true},
		{SeverityHigh, true},
		{SeverityMedium, true},
		{SeverityLow, true},
		{SeverityUnknown, true},
		{"", false},
		{"CRITICAL", false}, // case-sensitive; canonical form is lowercase
		{"severe", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			RegisterTestingT(t)
			Expect(isValidSeverity(tt.input)).To(Equal(tt.valid))
		})
	}
}

func TestConfig_ShouldCache(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name   string
		config *Config
		want   bool
	}{
		{
			name:   "nil cache returns false",
			config: &Config{Cache: nil},
			want:   false,
		},
		{
			name:   "cache disabled returns false",
			config: &Config{Cache: &CacheConfig{Enabled: false}},
			want:   false,
		},
		{
			name:   "cache enabled returns true",
			config: &Config{Cache: &CacheConfig{Enabled: true, TTL: 6}},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(tt.config.ShouldCache()).To(Equal(tt.want))
		})
	}
}

func TestConfig_ShouldSaveLocal(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name   string
		config *Config
		want   bool
	}{
		{
			name:   "nil output returns false",
			config: &Config{Output: nil},
			want:   false,
		},
		{
			name:   "empty local path returns false",
			config: &Config{Output: &OutputConfig{Local: ""}},
			want:   false,
		},
		{
			name:   "local path set returns true",
			config: &Config{Output: &OutputConfig{Local: "/tmp/reports"}},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(tt.config.ShouldSaveLocal()).To(Equal(tt.want))
		})
	}
}
