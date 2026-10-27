// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// PluginRequirement names a plugin and the type tokens it owns. Each
// out-of-tree sc-plugin's `/api` module calls RegisterPluginRequirement
// at init() time so that SC core — which has already deleted the
// corresponding in-tree subtree — can detect missing `plugins:` blocks
// in server.yaml and auto-inject the latest release BEFORE any Pulumi
// state is touched.
//
// See forge/docs/design/2026-10-24/sc-plugin-architecture/
// 04-no-destroy-invariant.md Addendum J.
type PluginRequirement struct {
	// PluginName is the key operators write under `plugins:` in server.yaml.
	// It MUST match the name the plugin itself uses — it's what SC core
	// matches against `desc.Plugins[PluginName]`.
	PluginName string

	// OwnedTypes are the type tokens (resource / registrar / provider /
	// state-storage / secrets-provider / compute-processor / template)
	// this plugin's /api module registers.
	OwnedTypes []string

	// DefaultSource is the `source:` block the auto-injector uses when
	// an operator's server.yaml references OwnedTypes without declaring
	// `plugins.<PluginName>`. Version is left empty — resolved against
	// the GitHub releases API at parse time.
	DefaultSource PluginSource
}

var (
	pluginRequirementsMu sync.Mutex
	pluginRequirements   []PluginRequirement
	// pluginByType resolves a used type token back to its owning plugin,
	// derived from pluginRequirements.
	pluginByType = map[string]PluginRequirement{}
)

// RegisterPluginRequirement declares that a plugin owns a set of type
// tokens. Called at init() time from each sc-plugin's /api module.
// Idempotent — a repeated registration with the same name updates the
// owned-types list and the default source.
func RegisterPluginRequirement(req PluginRequirement) {
	pluginRequirementsMu.Lock()
	defer pluginRequirementsMu.Unlock()
	for i, existing := range pluginRequirements {
		if existing.PluginName == req.PluginName {
			pluginRequirements[i] = req
			for _, t := range req.OwnedTypes {
				pluginByType[t] = req
			}
			return
		}
	}
	pluginRequirements = append(pluginRequirements, req)
	for _, t := range req.OwnedTypes {
		pluginByType[t] = req
	}
}

// LookupPluginRequirement returns the plugin requirement that owns a
// type token, if any. False when the type is in-tree (or unknown).
func LookupPluginRequirement(typeToken string) (PluginRequirement, bool) {
	pluginRequirementsMu.Lock()
	defer pluginRequirementsMu.Unlock()
	req, ok := pluginByType[typeToken]
	return req, ok
}

// resolveLatestVersion returns the latest-release tag_name for a GitHub
// repository. Package-level var so tests can stub without touching the
// network — see SetLatestVersionResolver for callers outside this package.
var (
	resolveLatestVersionMu sync.RWMutex
	resolveLatestVersion   = resolveLatestVersionFromGitHub
)

// SetLatestVersionResolver replaces the plugin-release resolver and returns a
// func that restores the previous one. Defer the result.
//
// This exists because auto-injection reaches api.github.com, and a unit test in
// ANY package that parses a server.yaml using a plugin-owned type without a
// `plugins:` pin will silently make that call. That is not hypothetical: it is
// how pkg/provisioner's own fool-protector test started hitting the network the
// moment auto-injection landed, passing or failing depending on whether the
// runner had egress. Tests that exercise either path must stub the resolver.
func SetLatestVersionResolver(fn func(repo string) (string, error)) func() {
	resolveLatestVersionMu.Lock()
	defer resolveLatestVersionMu.Unlock()
	prev := resolveLatestVersion
	resolveLatestVersion = fn
	return func() {
		resolveLatestVersionMu.Lock()
		defer resolveLatestVersionMu.Unlock()
		resolveLatestVersion = prev
	}
}

func currentResolver() func(repo string) (string, error) {
	resolveLatestVersionMu.RLock()
	defer resolveLatestVersionMu.RUnlock()
	return resolveLatestVersion
}

func resolveLatestVersionFromGitHub(repo string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	// Opportunistic auth — unauth 60/h is enough for typical operator usage,
	// but if a token is around we may as well use its 5000/h budget.
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.TagName == "" {
		return "", fmt.Errorf("empty tag_name from %s", url)
	}
	return body.TagName, nil
}

// ValidatePluginRequirements walks every type token used in a
// ServerDescriptor and, for any token owned by an un-declared plugin,
// resolves the latest release from GitHub and injects a synthetic
// `plugins:` entry into the descriptor. Only when resolution itself
// fails (offline, rate-limited, repo gone) does it return an error —
// with the exact YAML block the operator needs to paste by hand.
//
// Call this at the END of server.yaml parsing (ReadServerConfigs), BEFORE
// any Pulumi engine starts. The point is to make an existing server.yaml
// that predates the plugin split keep working across the upgrade: before
// this, parsing succeeded (the plugin /api module's blank import registers
// the type reader) and the deploy then died mid-provision with
// `unsupported registrar type "cloudflare"`, possibly after state had
// already been mutated.
//
// Auto-injection — rather than a hard error — is deliberate: backwards
// compatibility means the upgrade must WORK, not merely diagnose. The
// resolver is skipped entirely for any plugin the operator did declare,
// so a pinned (reproducible) steady-state deploy makes no network call.
func ValidatePluginRequirements(descriptor *ServerDescriptor) error {
	if descriptor == nil {
		return nil
	}
	// Everything the operator declared under plugins: by name.
	configured := map[string]bool{}
	for name := range descriptor.Plugins {
		configured[name] = true
	}

	// Collect every type token used across the descriptor.
	used := map[string][]string{} // typeToken → where (for the error message)
	add := func(typeToken, location string) {
		if typeToken == "" {
			return
		}
		used[typeToken] = append(used[typeToken], location)
	}

	// Provisioner — state storage + secrets provider only. The provisioner
	// type itself (pulumi) is in-tree.
	if descriptor.Provisioner.Config.Config != nil {
		if pc, ok := descriptor.Provisioner.Config.Config.(ProvisionerConfigAware); ok {
			add(pc.StateStorageType(), "provisioner.state-storage.type")
			add(pc.SecretsProviderType(), "provisioner.secrets-provider.type")
		}
	}

	// Templates.
	for name, tpl := range descriptor.Templates {
		add(tpl.Type, fmt.Sprintf("templates.%s.type", name))
	}

	// Registrars — both legacy single and plural map.
	if descriptor.Resources.Registrar.IsConfigured() {
		add(descriptor.Resources.Registrar.Type, "resources.registrar.type")
	}
	for name, reg := range descriptor.Resources.Registrars {
		add(reg.Type, fmt.Sprintf("resources.registrars.%s.type", name))
	}

	// Per-env resources.
	for env, perEnv := range descriptor.Resources.Resources {
		for name, res := range perEnv.Resources {
			add(res.Type, fmt.Sprintf("resources.resources.%s.resources.%s.type", env, name))
			// Some resources pick a cloud-specific driver via a sub-type —
			// `type: vm` resources carry `provider: <driver>`, where the driver
			// itself can be a plugin-owned token (e.g. `cloudru-evolution-vm`).
			// Walking only `res.Type` would miss those, so a server.yaml whose
			// only Cloud.ru surface is the VM would not trigger fool-protection
			// despite the plugin being mandatory for the deploy.
			if vc, ok := res.Config.Config.(VmConfigAware); ok {
				add(vc.DriverType(), fmt.Sprintf("resources.resources.%s.resources.%s.provider", env, name))
			}
		}
	}

	// Build the mapping of plugins actually needed + evidence of their uses.
	needed := map[string]PluginRequirement{}
	locationsByPlugin := map[string][]string{}
	pluginRequirementsMu.Lock()
	for typeToken, locations := range used {
		req, ok := pluginByType[typeToken]
		if !ok {
			continue
		}
		needed[req.PluginName] = req
		locationsByPlugin[req.PluginName] = append(locationsByPlugin[req.PluginName], locations...)
	}
	pluginRequirementsMu.Unlock()

	// Everything that needs a plugin and doesn't have one — try to resolve
	// it against the GitHub Releases API.
	var missingNames []string
	for name := range needed {
		if !configured[name] {
			missingNames = append(missingNames, name)
		}
	}
	if len(missingNames) == 0 {
		return nil
	}
	sort.Strings(missingNames)

	unresolved := map[string]error{}
	for _, name := range missingNames {
		req := needed[name]
		tag, err := currentResolver()(req.DefaultSource.Repository)
		if err != nil {
			unresolved[name] = err
			continue
		}
		if descriptor.Plugins == nil {
			descriptor.Plugins = map[string]PluginSpec{}
		}
		src := req.DefaultSource
		src.Version = tag
		descriptor.Plugins[name] = PluginSpec{Source: &src}
	}

	if len(unresolved) == 0 {
		return nil
	}
	var unresolvedNames []string
	for name := range unresolved {
		unresolvedNames = append(unresolvedNames, name)
	}
	sort.Strings(unresolvedNames)

	var b strings.Builder
	b.WriteString("server.yaml uses type tokens owned by out-of-tree SC plugins ")
	b.WriteString("that are not declared under `plugins:` and whose latest release ")
	b.WriteString("could not be resolved from GitHub. Reasons:\n")
	for _, name := range unresolvedNames {
		b.WriteString(fmt.Sprintf("  - %s: %s\n", name, unresolved[name]))
	}
	b.WriteString("\nPaste the block(s) below into this stack's `server.yaml` ")
	b.WriteString("(or inherit from the parent stack):\n\n")
	b.WriteString("plugins:\n")
	for _, name := range unresolvedNames {
		req := needed[name]
		sort.Strings(locationsByPlugin[name])
		b.WriteString(fmt.Sprintf("  %s:\n", name))
		b.WriteString("    source:\n")
		b.WriteString(fmt.Sprintf("      type: %s\n", req.DefaultSource.Type))
		b.WriteString(fmt.Sprintf("      repository: %s\n", req.DefaultSource.Repository))
		b.WriteString("      version: v<pick-a-release-tag>\n")
		b.WriteString(fmt.Sprintf("    # Owns these types used in this stack: %s\n",
			strings.Join(dedup(locationsByPlugin[name]), ", ")))
	}
	b.WriteString("\nBrowse releases at ")
	for _, name := range unresolvedNames {
		req := needed[name]
		b.WriteString(fmt.Sprintf("https://github.com/%s/releases ", req.DefaultSource.Repository))
	}
	return fmt.Errorf("%s", b.String())
}

func dedup(ss []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// ProvisionerConfigAware is implemented by the pulumi provisioner's config.
// Used by ValidatePluginRequirements to reach state-storage and
// secrets-provider type tokens without importing pkg/clouds/pulumi.
// Method names use the `Type` suffix to avoid colliding with same-named
// struct fields on the concrete config.
type ProvisionerConfigAware interface {
	StateStorageType() string
	SecretsProviderType() string
}

// VmConfigAware is implemented by `pkg/clouds/vm.VmConfig`. Used by
// ValidatePluginRequirements to walk a `type: vm` resource's `provider:`
// field, which selects a driver that itself can be plugin-owned
// (`cloudru-evolution-vm`, future `mws-vm`, `selectel-vm`, …). Without this
// hook the walker only sees `res.Type == "vm"` and misses the plugin that
// owns the actual driver.
//
// Method is named `DriverType` rather than `Driver` because VmConfig already
// has a `Driver string` field — same reason StateStorageType is not named
// StateStorage.
type VmConfigAware interface {
	DriverType() string
}
