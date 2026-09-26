// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"testing"

	"gopkg.in/yaml.v3"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

// detectedRegistrarConfig stands in for a real registrar config so DetectRegistrarType
// can be observed without pulling a cloud provider into pkg/api's tests.
type detectedRegistrarConfig struct {
	detected bool
}

func init() {
	RegisterProviderConfig(ConfigRegisterMap{
		"test-dns": func(config *Config) (Config, error) {
			return Config{Config: &detectedRegistrarConfig{detected: true}}, nil
		},
		"test-dns-broken": func(config *Config) (Config, error) {
			return *config, errors.New("cannot read config")
		},
	})
}

func TestRegistrarDescriptorIsConfigured(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		desc RegistrarDescriptor
		want bool
	}{
		{name: "zero value is not configured", desc: RegistrarDescriptor{}, want: false},
		{name: "type set", desc: RegistrarDescriptor{Type: "cloudflare"}, want: true},
		{name: "inherited", desc: RegistrarDescriptor{Inherit: Inherit{Inherit: "common"}}, want: true},
		{name: "config only", desc: RegistrarDescriptor{Config: Config{Config: "anything"}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(tt.desc.IsConfigured()).To(Equal(tt.want))
		})
	}
}

// TestAllRegistrarsBackwardsCompatibility pins the four shapes a server.yaml may use.
// The first three predate `registrars:` and must normalise to exactly what they always
// meant, under the empty name, so nothing derived from a registrar's name changes.
func TestAllRegistrarsBackwardsCompatibility(t *testing.T) {
	RegisterTestingT(t)

	cloudflare := RegistrarDescriptor{Type: "cloudflare", Config: Config{Config: "cf"}}
	inherited := RegistrarDescriptor{Inherit: Inherit{Inherit: "common"}}
	yandex := RegistrarDescriptor{Type: "yc-dns", Config: Config{Config: "yc"}}

	tests := []struct {
		name      string
		resources PerStackResourcesDescriptor
		want      map[string]RegistrarDescriptor
		wantErr   string
	}{
		{
			name:      "no registrar block at all",
			resources: PerStackResourcesDescriptor{},
			want:      map[string]RegistrarDescriptor{},
		},
		{
			name:      "legacy single registrar",
			resources: PerStackResourcesDescriptor{Registrar: cloudflare},
			want:      map[string]RegistrarDescriptor{DefaultRegistrarName: cloudflare},
		},
		{
			name:      "legacy inherited registrar",
			resources: PerStackResourcesDescriptor{Registrar: inherited},
			want:      map[string]RegistrarDescriptor{DefaultRegistrarName: inherited},
		},
		{
			name:      "single entry in the registrars map",
			resources: PerStackResourcesDescriptor{Registrars: map[string]RegistrarDescriptor{"cf": cloudflare}},
			want:      map[string]RegistrarDescriptor{"cf": cloudflare},
		},
		{
			name: "several registrars",
			resources: PerStackResourcesDescriptor{
				Registrars: map[string]RegistrarDescriptor{"cf": cloudflare, "yc": yandex},
			},
			want: map[string]RegistrarDescriptor{"cf": cloudflare, "yc": yandex},
		},
		{
			name: "both forms at once is an error, not a precedence rule",
			resources: PerStackResourcesDescriptor{
				Registrar:  cloudflare,
				Registrars: map[string]RegistrarDescriptor{"yc": yandex},
			},
			wantErr: "cannot declare both",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := tt.resources.AllRegistrars()
			if tt.wantErr != "" {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(tt.wantErr))
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(tt.want))
		})
	}
}

// TestPerStackResourcesCopyPreservesNilRegistrars guards the copy path: turning a nil
// map into an empty one would make every pre-`registrars:` descriptor compare unequal
// to its own copy.
func TestPerStackResourcesCopyPreservesNilRegistrars(t *testing.T) {
	RegisterTestingT(t)

	original := PerStackResourcesDescriptor{Registrar: RegistrarDescriptor{Type: "cloudflare"}}
	Expect(original.Copy().Registrar).To(Equal(original.Registrar))
	Expect(original.Copy().Registrars).To(BeNil())

	withMap := PerStackResourcesDescriptor{
		Registrars: map[string]RegistrarDescriptor{"cf": {Type: "cloudflare"}},
	}
	Expect(withMap.Copy().Registrars).To(Equal(withMap.Registrars))
}

func TestDetectRegistrarTypeAcrossBothForms(t *testing.T) {
	RegisterTestingT(t)

	t.Run("legacy single registrar still detected", func(t *testing.T) {
		RegisterTestingT(t)
		p := &PerStackResourcesDescriptor{Registrar: RegistrarDescriptor{Type: "test-dns"}}
		got, err := DetectRegistrarType(p)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Registrar.Config.Config).To(Equal(&detectedRegistrarConfig{detected: true}))
	})

	t.Run("every entry of the registrars map is detected", func(t *testing.T) {
		RegisterTestingT(t)
		p := &PerStackResourcesDescriptor{Registrars: map[string]RegistrarDescriptor{
			"cf":        {Type: "test-dns"},
			"yc":        {Type: "test-dns"},
			"inherited": {Inherit: Inherit{Inherit: "common"}},
			"untyped":   {Config: Config{Config: "left alone"}},
		}}
		got, err := DetectRegistrarType(p)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Registrars["cf"].Config.Config).To(Equal(&detectedRegistrarConfig{detected: true}))
		Expect(got.Registrars["yc"].Config.Config).To(Equal(&detectedRegistrarConfig{detected: true}))
		// inherited entries are resolved later, untyped ones are "not configured"
		Expect(got.Registrars["inherited"].Config.Config).To(BeNil())
		Expect(got.Registrars["untyped"].Config.Config).To(Equal("left alone"))
	})

	t.Run("an unknown type in the map names the offending registrar", func(t *testing.T) {
		RegisterTestingT(t)
		p := &PerStackResourcesDescriptor{Registrars: map[string]RegistrarDescriptor{
			"yc": {Type: "no-such-registrar"},
		}}
		got, err := DetectRegistrarType(p)
		Expect(err).To(HaveOccurred())
		Expect(got).To(BeNil())
		Expect(err.Error()).To(ContainSubstring(`registrar "yc"`))
		Expect(err.Error()).To(ContainSubstring("no-such-registrar"))
	})

	t.Run("a config that fails to read is reported", func(t *testing.T) {
		RegisterTestingT(t)
		p := &PerStackResourcesDescriptor{Registrars: map[string]RegistrarDescriptor{
			"yc": {Type: "test-dns-broken"},
		}}
		_, err := DetectRegistrarType(p)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("cannot read config"))
	})
}

// A named registrar may inherit from a parent that still uses the single block, so a
// stack can adopt `registrars:` before its parent does.
func TestResolveInheritanceOfNamedRegistrars(t *testing.T) {
	RegisterTestingT(t)

	parentRegistrar := RegistrarDescriptor{Type: "test-dns", Config: Config{Config: "from-parent"}}
	namedRegistrar := RegistrarDescriptor{Type: "test-dns", Config: Config{Config: "named-in-parent"}}

	stacks := StacksMap{
		"common": {Name: "common", Server: ServerDescriptor{Resources: PerStackResourcesDescriptor{
			Registrar:  parentRegistrar,
			Registrars: map[string]RegistrarDescriptor{"yc": namedRegistrar},
		}}},
		"child": {Name: "child", Server: ServerDescriptor{Resources: PerStackResourcesDescriptor{
			Registrars: map[string]RegistrarDescriptor{
				"yc":       {Inherit: Inherit{Inherit: "common"}},
				"fallback": {Inherit: Inherit{Inherit: "common"}},
			},
		}}},
	}

	resolved := *stacks.ResolveInheritance()
	child := resolved["child"].Server.Resources.Registrars
	Expect(child["yc"]).To(Equal(namedRegistrar), "a matching name in the parent wins")
	Expect(child["fallback"]).To(Equal(parentRegistrar), "otherwise the parent's single block is inherited")
}

// TestRegistrarsYamlShape pins the YAML the operator actually writes, including that an
// absent `registrars:` stays absent rather than becoming an empty map.
func TestRegistrarsYamlShape(t *testing.T) {
	RegisterTestingT(t)

	var multi PerStackResourcesDescriptor
	Expect(yaml.Unmarshal([]byte(`
registrars:
  cloudflare:
    type: cloudflare
    zoneName: simple-forge.com
  yandex:
    type: yc-dns
    zoneName: simple-forge.ru
`), &multi)).To(Succeed())
	Expect(multi.Registrars).To(HaveLen(2))
	Expect(multi.Registrars["yandex"].Type).To(Equal("yc-dns"))
	Expect(multi.Registrar.IsConfigured()).To(BeFalse())

	var legacy PerStackResourcesDescriptor
	Expect(yaml.Unmarshal([]byte(`
registrar:
  type: cloudflare
  zoneName: simple-forge.com
`), &legacy)).To(Succeed())
	Expect(legacy.Registrar.Type).To(Equal("cloudflare"))
	Expect(legacy.Registrars).To(BeNil())

	out, err := yaml.Marshal(legacy)
	Expect(err).NotTo(HaveOccurred())
	Expect(string(out)).NotTo(ContainSubstring("registrars"))
}
