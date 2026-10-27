// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// PluginRequirement names a plugin and the type tokens it owns. Each
// out-of-tree sc-plugin's `/api` module calls RegisterPluginRequirement
// at init() time so that SC core — which has already deleted the
// corresponding in-tree subtree — can detect missing `plugins:` blocks
// in server.yaml BEFORE any Pulumi state is touched.
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

	// DefaultSource is the `source:` block the operator-facing error
	// message suggests. Version is deliberately left empty — operators
	// pin a specific release.
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

// ValidatePluginRequirements walks every type token used in a
// ServerDescriptor and fails fast when a token is owned by a plugin
// that is not configured under `plugins:`. The error carries the exact
// YAML block the operator needs to copy-paste.
//
// Call this at the END of server.yaml parsing (ReadServerConfigs), BEFORE
// any Pulumi engine starts. The point is to catch missing plugin pins
// before SC core mutates state.
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

	// Missing = needed - configured.
	var missing []string
	for name := range needed {
		if !configured[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)

	// One actionable message per missing plugin.
	var b strings.Builder
	b.WriteString("server.yaml uses type tokens owned by out-of-tree SC plugins ")
	b.WriteString("but is missing the corresponding `plugins:` entries. ")
	b.WriteString("Add the block(s) below to this stack's `server.yaml` ")
	b.WriteString("(or inherit from the parent stack):\n\n")
	b.WriteString("plugins:\n")
	for _, name := range missing {
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
	b.WriteString("\n")
	b.WriteString("Pin a specific release tag (SC does not auto-resolve 'latest'). ")
	b.WriteString("Browse releases at ")
	for _, name := range missing {
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
