package sbom

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestDefaultConfig(t *testing.T) {
	RegisterTestingT(t)

	cfg := DefaultConfig()
	Expect(cfg).ToNot(BeNil())
	Expect(cfg.Enabled).To(BeFalse())
	Expect(cfg.Format).To(Equal(FormatCycloneDXJSON))
	Expect(cfg.Generator).To(Equal("syft"))
	Expect(cfg.CacheEnabled).To(BeTrue())
	Expect(cfg.Attach).To(BeFalse())
	Expect(cfg.Required).To(BeFalse())
	Expect(cfg.Output).ToNot(BeNil())
	Expect(cfg.Output.Local).To(Equal(""))
	Expect(cfg.Output.Registry).To(BeFalse())
}

func TestConfigValidate(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		// assert applies post-validation expectations on the (possibly mutated) config.
		assert func(c *Config)
	}{
		{
			name:    "Disabled config skips validation",
			config:  &Config{Enabled: false, Format: Format("garbage"), Generator: "bogus"},
			wantErr: false,
			assert: func(c *Config) {
				// Validate returns early; nothing is normalized.
				Expect(c.Format).To(Equal(Format("garbage")))
				Expect(c.Generator).To(Equal("bogus"))
			},
		},
		{
			name:    "Enabled with empty format defaults to cyclonedx-json",
			config:  &Config{Enabled: true},
			wantErr: false,
			assert: func(c *Config) {
				Expect(c.Format).To(Equal(FormatCycloneDXJSON))
				Expect(c.Generator).To(Equal("syft"))
				Expect(c.Output).ToNot(BeNil())
			},
		},
		{
			name:    "Enabled with valid explicit format and generator",
			config:  &Config{Enabled: true, Format: FormatSPDXJSON, Generator: "syft"},
			wantErr: false,
			assert: func(c *Config) {
				Expect(c.Format).To(Equal(FormatSPDXJSON))
			},
		},
		{
			name:    "Enabled with invalid format errors",
			config:  &Config{Enabled: true, Format: Format("not-a-format")},
			wantErr: true,
		},
		{
			name:    "Enabled with invalid generator errors",
			config:  &Config{Enabled: true, Format: FormatCycloneDXJSON, Generator: "trivy"},
			wantErr: true,
		},
		{
			name:    "Enabled with nil output gets initialized",
			config:  &Config{Enabled: true, Format: FormatCycloneDXJSON, Generator: "syft", Output: nil},
			wantErr: false,
			assert: func(c *Config) {
				Expect(c.Output).ToNot(BeNil())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tt.config.Validate()
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
				if tt.assert != nil {
					tt.assert(tt.config)
				}
			}
		})
	}
}

func TestConfigValidateInvalidFormatMessage(t *testing.T) {
	RegisterTestingT(t)

	c := &Config{Enabled: true, Format: Format("xml-of-doom")}
	err := c.Validate()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("invalid SBOM format"))
	Expect(err.Error()).To(ContainSubstring("xml-of-doom"))
}

func TestConfigShouldCache(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		cfg  *Config
		want bool
	}{
		{"Cache enabled", &Config{CacheEnabled: true}, true},
		{"Cache disabled", &Config{CacheEnabled: false}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(tt.cfg.ShouldCache()).To(Equal(tt.want))
		})
	}
}

func TestConfigShouldSaveLocal(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		cfg  *Config
		want bool
	}{
		{"Nil output", &Config{Output: nil}, false},
		{"Empty local path", &Config{Output: &OutputConfig{Local: ""}}, false},
		{"Non-empty local path", &Config{Output: &OutputConfig{Local: "/tmp/sbom.json"}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(tt.cfg.ShouldSaveLocal()).To(Equal(tt.want))
		})
	}
}

func TestConfigShouldAttach(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		cfg  *Config
		want bool
	}{
		{"Attach flag set", &Config{Attach: true}, true},
		{"Registry output set", &Config{Output: &OutputConfig{Registry: true}}, true},
		{"Neither attach nor registry", &Config{Attach: false, Output: &OutputConfig{Registry: false}}, false},
		{"Nil output and attach false", &Config{Attach: false, Output: nil}, false},
		{"Attach true and registry true", &Config{Attach: true, Output: &OutputConfig{Registry: true}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(tt.cfg.ShouldAttach()).To(Equal(tt.want))
		})
	}
}

func TestConfigIsRequired(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		cfg  *Config
		want bool
	}{
		{"Required true", &Config{Required: true}, true},
		{"Required false", &Config{Required: false}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(tt.cfg.IsRequired()).To(Equal(tt.want))
		})
	}
}
