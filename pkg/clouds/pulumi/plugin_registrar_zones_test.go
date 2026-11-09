// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"testing"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
)

// pluginRegistrar builds the descriptor shape api.pluginFieldConfig produces for a
// plugin-owned registrar type — an opaque PluginFieldConfig whose Raw holds the
// operator's keys verbatim. Both registrar types in the fleet are plugin-owned
// since the extraction, so this is the ONLY shape that reaches newMultiRegistrar
// in production.
func pluginOwnedRegistrar(typeName, zoneName string, isDefault bool) api.RegistrarDescriptor {
	return api.RegistrarDescriptor{
		Type:    typeName,
		Default: isDefault,
		Config: api.Config{Config: &api.PluginFieldConfig{
			TypeName: typeName,
			Raw:      map[string]any{"zoneName": zoneName, "credentials": "redacted"},
		}},
	}
}

// The forge/infra parent, reduced to the two registrars that matter: Cloudflare
// is `default: true` for simple-container.com, and a yc-dns registrar serves
// simple-forge.ru.
//
// Before PluginFieldConfig implemented RegistrarZonesAware, declaredZones
// returned nil for BOTH of these, forDomain could not match any zone, and every
// domain fell through to the default. That one fault produced both halves of the
// 2026-11-09 landing outage: the cloudflare plugin was asked to serve
// simple-forge.ru and answered "0 found", and provisionDomainRegistrar computed
// regType=cloudflare, saw that the yandex plugin does not serve it, returned
// early, and the yandex plugin refused with "no yc-dns registrar initialised
// that serves simple-forge.ru".
func TestMultiRegistrar_RoutesAPluginOwnedZoneToItsOwnRegistrar(t *testing.T) {
	m := newMultiRegistrar("infra", map[string]api.RegistrarDescriptor{
		"cloudflare":             pluginOwnedRegistrar("cloudflare", "simple-container.com", true),
		"yandex-simple-forge-ru": pluginOwnedRegistrar("yc-dns", "simple-forge.ru", false),
	}, nil, logger.New())

	for _, tt := range []struct {
		domain string
		want   string
	}{
		// The regression. Must NOT be "cloudflare".
		{"simple-forge.ru", "yc-dns"},
		{"www.simple-forge.ru", "yc-dns"},
		// The default registrar's own zone still resolves to it.
		{"simple-container.com", "cloudflare"},
		{"dist.simple-container.com", "cloudflare"},
		// A zone nobody declares still falls back to the default, which is what
		// `default: true` is for. Unchanged behaviour, asserted so the fix is
		// not mistaken for removing the fallback.
		{"unclaimed.example.com", "cloudflare"},
	} {
		t.Run(tt.domain, func(t *testing.T) {
			got, err := m.RegistrarTypeFor(tt.domain)
			if err != nil {
				t.Fatalf("RegistrarTypeFor(%q): %v", tt.domain, err)
			}
			if got != tt.want {
				t.Fatalf("RegistrarTypeFor(%q) = %q, want %q", tt.domain, got, tt.want)
			}
		})
	}
}

// Longest-zone-wins still holds across plugin-owned registrars, so a delegated
// subzone is not swallowed by its parent zone's registrar.
func TestMultiRegistrar_LongestZoneWinsForPluginOwnedRegistrars(t *testing.T) {
	m := newMultiRegistrar("infra", map[string]api.RegistrarDescriptor{
		"apex":    pluginOwnedRegistrar("cloudflare", "example.com", true),
		"subzone": pluginOwnedRegistrar("yc-dns", "ru.example.com", false),
	}, nil, logger.New())

	for domain, want := range map[string]string{
		"ru.example.com":   "yc-dns",
		"a.ru.example.com": "yc-dns",
		"a.example.com":    "cloudflare",
	} {
		got, err := m.RegistrarTypeFor(domain)
		if err != nil {
			t.Fatalf("RegistrarTypeFor(%q): %v", domain, err)
		}
		if got != want {
			t.Fatalf("RegistrarTypeFor(%q) = %q, want %q", domain, got, want)
		}
	}
}

// A `zones:` list is the other spelling in use, and arrives as []any from the
// YAML decoder.
func TestPluginFieldConfig_ZonesAcceptsBothSpellings(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  map[string]any
		want int
	}{
		{"zoneName only", map[string]any{"zoneName": "a.com"}, 1},
		{"zones list from yaml", map[string]any{"zones": []any{"a.com", "b.com"}}, 2},
		{"zones list built in go", map[string]any{"zones": []string{"a.com", "b.com"}}, 2},
		{"both", map[string]any{"zoneName": "a.com", "zones": []any{"b.com"}}, 2},
		// Neither declared: nil, and the registrar keeps working as a sole or
		// default registrar exactly as before.
		{"neither", map[string]any{"credentials": "x"}, 0},
		{"empty strings are not zones", map[string]any{"zoneName": "", "zones": []any{""}}, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &api.PluginFieldConfig{TypeName: "cloudflare", Raw: tt.raw}
			if got := len(cfg.Zones()); got != tt.want {
				t.Fatalf("len(Zones()) = %d, want %d (zones=%v)", got, tt.want, cfg.Zones())
			}
		})
	}
	// And the interface is actually satisfied — the thing whose absence caused
	// all of this.
	var _ api.RegistrarZonesAware = (*api.PluginFieldConfig)(nil)
}
