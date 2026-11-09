// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/stackread"
)

// The two plugin versions this smoke parses with. Pinned rather than resolved so
// the test never reaches api.github.com, and cache-gated below so it never
// downloads: a plugin release is ~112 MiB and CI has neither the cache nor
// reliable egress to GitHub release assets (see
// forge/docs/roadmap/items/fleet-cannot-fetch-go-module-zips.md).
const (
	e2eCloudflarePin = "v2026.11.04-7d66106"
	e2eYandexPin     = "v2026.11.05-aa8cb19"
)

// cachedPluginDir returns the local plugin cache path for a pinned release.
func cachedPluginDir(repo, version string) (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	dir := path.Join(home, ".sc", "plugins", "simple-container-com", repo, version)
	if _, err := os.Stat(dir); err != nil {
		return "", false
	}
	return dir, true
}

// End-to-end through the REAL parse, with the REAL plugin binaries: YAML ->
// plugin Schema discovery -> opaque PluginFieldConfig decode -> declaredZones ->
// multiRegistrar.forDomain. No cloud contact; nothing is provisioned and no
// credential is needed, because routing is decided before any registrar is
// constructed (which is the whole point of RegistrarZonesAware).
//
// Why this exists alongside the unit tests, and what it already caught. The unit
// tests build a RegistrarDescriptor in Go, so they prove forDomain given a
// correct PluginFieldConfig; they cannot prove what the PARSE produces. Run
// against a real two-registrar server.yaml it immediately showed that the two
// registrars decode to DIFFERENT Go types, only one of which was ever broken:
//
//	cloudflare              type=cloudflare   *api.PluginFieldConfig    zones=[]  <- opaque, could not report
//	yandex-simple-forge-ru  type=yc-dns       *yandex.RegistrarConfig   zones=[simple-forge.ru]
//
// The yc-dns config is a TYPED struct from the plugin's own Go module (the
// pre-A3 `/api/init.go` link-time path), and it implements RegistrarZonesAware
// perfectly well. So "no registrar can report its zones" was false, and routing
// for simple-forge.ru already worked. That killed a root-cause theory before it
// could be merged on top of two earlier fixes that had also been verified
// against hand-built shapes.
//
// SKIPPED unless both plugin releases are already in ~/.sc/plugins, so a CI run
// never downloads 224 MiB to assert a routing decision.
func TestMultiRegistrar_EndToEndFromRealServerYaml(t *testing.T) {
	for _, p := range []struct{ repo, version string }{
		{"sc-plugin-cloudflare", e2eCloudflarePin},
		{"sc-plugin-yandex", e2eYandexPin},
	} {
		if _, ok := cachedPluginDir(p.repo, p.version); !ok {
			t.Skipf("%s %s is not in the local plugin cache — run this smoke on a machine that has it", p.repo, p.version)
		}
	}

	// The forge/infra registrar arrangement, reduced to the two that matter:
	// Cloudflare is `default: true` for simple-container.com, and a yc-dns
	// registrar serves simple-forge.ru.
	root := t.TempDir()
	stackDir := path.Join(root, "infra")
	if err := os.MkdirAll(stackDir, 0o755); err != nil {
		t.Fatal(err)
	}
	serverYaml := `schemaVersion: 1.0
provisioner:
  type: pulumi
  config:
    organization: acme
    state-storage:
      type: pulumi-cloud
      config:
        credentials: "token"
    secrets-provider:
      type: pulumi-cloud
      config:
        credentials: "token"
plugins:
  cloudflare:
    source:
      type: github-release
      repository: simple-container-com/sc-plugin-cloudflare
      version: ` + e2eCloudflarePin + `
  yandex:
    source:
      type: github-release
      repository: simple-container-com/sc-plugin-yandex
      version: ` + e2eYandexPin + `
resources:
  registrars:
    cloudflare:
      type: cloudflare
      default: true
      config:
        zoneName: simple-container.com
        credentials: "unused-routing-is-decided-before-construction"
        accountId: "unused"
    yandex-simple-forge-ru:
      type: yc-dns
      config:
        zoneName: simple-forge.ru
        credentials: "unused-routing-is-decided-before-construction"
`
	if err := os.WriteFile(path.Join(stackDir, api.ServerDescriptorFileName), []byte(serverYaml), 0o600); err != nil {
		t.Fatal(err)
	}

	desc, err := stackread.ReadServerDescriptor(context.Background(), path.Join(stackDir, api.ServerDescriptorFileName))
	if err != nil {
		t.Fatalf("parsing the server.yaml failed: %v", err)
	}
	registrars, err := desc.Resources.AllRegistrars()
	if err != nil {
		t.Fatalf("AllRegistrars: %v", err)
	}
	if len(registrars) != 2 {
		t.Fatalf("expected 2 registrars off the parse, got %d: %v", len(registrars), registrars)
	}

	// Prove the parse took the PLUGIN path: a plugin-owned registrar type decodes
	// to the opaque api.PluginFieldConfig, which is the exact value that could not
	// report its zones. If this were a typed in-tree config the test would be
	// exercising a path that no longer exists in production.
	for name, reg := range registrars {
		zones := []string(nil)
		if aware, ok := reg.Config.Config.(api.RegistrarZonesAware); ok {
			zones = aware.Zones()
		}
		t.Logf("registrar %-24s type=%-12s goType=%-28T zones=%v", name, reg.Type, reg.Config.Config, zones)
	}

	m := newMultiRegistrar("infra", registrars, nil, logger.New())

	for _, tt := range []struct{ domain, want string }{
		// Routing to a TYPED registrar config worked before api#592 too — this
		// row is a guard, not a regression test.
		{"simple-forge.ru", "yc-dns"},
		{"landing.simple-forge.ru", "yc-dns"},
		// These two are what api#592 actually changes. Before it, an OPAQUE
		// plugin-owned registrar reported no zones, so a domain in its zone
		// reached it only because it happens to be `default: true`. Flip the
		// default to the other registrar and the same domain would have gone to
		// yc-dns. Correct routing must not depend on which registrar is default.
		{"simple-container.com", "cloudflare"},
		{"dist.simple-container.com", "cloudflare"},
		// Unclaimed by either zone: the `default: true` marker still catches it,
		// which is what it is for.
		{"something.example.com", "cloudflare"},
	} {
		got, err := m.RegistrarTypeFor(tt.domain)
		if err != nil {
			t.Fatalf("RegistrarTypeFor(%q): %v", tt.domain, err)
		}
		if got != tt.want {
			t.Errorf("RegistrarTypeFor(%q) = %q, want %q", tt.domain, got, tt.want)
		}
	}
}

// What api#592 actually fixes, isolated: with the Cloudflare registrar NOT the
// default, a domain in its own declared zone must still reach it. Before the fix
// the opaque config reported no zones, so this domain fell through to whichever
// registrar was default — the yc-dns one — and would have been published through
// the wrong provider.
//
// This is the case the fleet's own parent never hit, because there Cloudflare IS
// the default and the mis-route was therefore invisible.
func TestMultiRegistrar_OpaqueRegistrarZoneWinsWithoutBeingDefault(t *testing.T) {
	m := newMultiRegistrar("infra", map[string]api.RegistrarDescriptor{
		// Opaque, plugin-owned, and NOT the default.
		"cloudflare": {
			Type: "cloudflare",
			Config: api.Config{Config: &api.PluginFieldConfig{
				TypeName: "cloudflare",
				Raw:      map[string]any{"zoneName": "simple-container.com"},
			}},
		},
		// The default, serving a different zone.
		"other": {
			Type:    "other-dns",
			Default: true,
			Config: api.Config{Config: &api.PluginFieldConfig{
				TypeName: "other-dns",
				Raw:      map[string]any{"zoneName": "elsewhere.example"},
			}},
		},
	}, nil, logger.New())

	for domain, want := range map[string]string{
		"dist.simple-container.com": "cloudflare",
		"simple-container.com":      "cloudflare",
		"a.elsewhere.example":       "other-dns",
		// Claimed by neither: the default, as designed.
		"unclaimed.example.org": "other-dns",
	} {
		got, err := m.RegistrarTypeFor(domain)
		if err != nil {
			t.Fatalf("RegistrarTypeFor(%q): %v", domain, err)
		}
		if got != want {
			t.Errorf("RegistrarTypeFor(%q) = %q, want %q", domain, got, want)
		}
	}
}
