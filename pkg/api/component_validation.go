// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"fmt"
	"sort"

	"github.com/pkg/errors"
)

// ComponentValidator is the signature for a callback that validates a single
// Component resource's config against SC core's set of known Component types.
// Implementations live in `pkg/clouds/pulumi/api` where the fleet-wide
// registration maps (ComponentResourceTypes, ComponentAllowedOverrides,
// ProvisionFuncByType) live; this package stays free of pulumi imports.
//
// The validator reports back the component-resource type, the location in
// the descriptor, and an error if invalid. Nil return means ok.
type ComponentValidator interface {
	IsComponentType(typeToken string) bool
	IsOverrideAllowed(typeToken, key string) bool
	TemplateTypeExists(templateType string) bool

	// HasInTreeMerge reports whether the given template type has an in-tree
	// ComponentTemplateMergeFunc registration (e.g. vm-compose). For types
	// that do, the merge func consumes the component's fields inside core
	// before the template's ProvisionFunc runs, so the plugin-side
	// SupportsComponent opt-in (below) is not required.
	HasInTreeMerge(templateType string) bool

	// AnyPluginSupportsComponent reports whether any loaded plugin has
	// advertised supports_component for the given template type via Schema
	// (slice 1a.1 of sc-components-platform; design doc §#19 path (b)). A
	// plugin that has not opted in cannot receive the ComponentRaw
	// side-channel on its Provision RPC; parse-time validation refuses any
	// dispatch targeting such a template, unless the type has an in-tree
	// merge func.
	AnyPluginSupportsComponent(templateType string) bool

	// MergeDefaults overlays operatorRaw on top of the plugin-declared
	// ComponentDefaults for typeToken (operator wins per key) and returns
	// the EFFECTIVE config the dispatcher will actually provision with.
	//
	// Validation must reason about the effective config, not the
	// operator's raw. ValidateComponentConfigs runs at
	// provisioner.go:RegisterSidecars time, which is BEFORE
	// dispatchComponentTemplate's own mergeComponentDefaults — so
	// validating the raw made every plugin-supplied default for a
	// REQUIRED key (`deployTo`, `image`) dead code: the operator had to
	// write the value anyway to get past validation, and the default
	// could never win. That is the whole reason an operator's
	// `server.yaml` carried `deployTo: lambda` and a hand-pinned
	// `image:` next to a plugin that already defaulted both.
	//
	// Implementations return a freshly allocated map and must not mutate
	// either input. A nil/absent defaults registration returns
	// operatorRaw's content unchanged, so a plugin that pre-dates
	// ComponentDefaults validates exactly as it did before.
	MergeDefaults(typeToken string, operatorRaw map[string]any) map[string]any
}

// ValidateComponentConfigs walks a server descriptor and validates every
// resource whose type is a declared Component (per the supplied validator)
// against the §P1 config shape — top-level keys, deployTo-template
// existence, image shape, signingKey as a secret reference, and the
// plugin-override allowlist.
//
// Intended to run AFTER the pre-engine plugin bundle has registered sidecars
// (so the validator's IsComponentType returns true for live plugin types).
// Called from pkg/clouds/pulumi/provisioner.go right after RegisterSidecars.
//
// Pure: no mutation of the descriptor. Multiple validation errors in a
// descriptor are collected and surfaced in one error message rather than
// one-by-one.
//
// Design: forge/docs/design/2026-11-10/sc-components-platform/README.md §P1.
func ValidateComponentConfigs(descriptor *ServerDescriptor, v ComponentValidator) error {
	if descriptor == nil || v == nil {
		return nil
	}
	var violations []string
	for env, perEnv := range descriptor.Resources.Resources {
		for name, res := range perEnv.Resources {
			if !v.IsComponentType(res.Type) {
				continue
			}
			location := fmt.Sprintf("resources.resources.%s.resources.%s", env, name)
			if errs := validateOneComponent(descriptor, res, location, v); len(errs) > 0 {
				violations = append(violations, errs...)
			}
		}
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return errors.Errorf("%d invalid SC Component configuration(s):\n  - %s",
		len(violations), joinLines(violations))
}

func validateOneComponent(
	descriptor *ServerDescriptor,
	res ResourceDescriptor,
	location string,
	v ComponentValidator,
) []string {
	var errs []string
	cfg, err := NewComponentConfig(res.Config.Config)
	if err != nil {
		return []string{fmt.Sprintf("%s: %s", location, err)}
	}
	// Validate the EFFECTIVE config — the plugin's ComponentDefaults with
	// the operator's raw overlaid, which is what the dispatcher will
	// provision. Required keys the operator legitimately omitted because
	// the plugin defaults them must not fail here. See
	// ComponentValidator.MergeDefaults.
	eff := cfg
	if merged := v.MergeDefaults(res.Type, cfg.Raw()); merged != nil {
		if mc, mErr := NewComponentConfig(merged); mErr == nil {
			eff = mc
		}
	}
	// deployTo exists, points at a declared template of a known type.
	tplName, err := eff.DeployTo()
	if err != nil {
		errs = append(errs, fmt.Sprintf("%s.config: %s", location, err))
	} else {
		tpl, ok := descriptor.Templates[tplName]
		if !ok {
			// Name the provenance. With `deployTo` now defaultable, an
			// operator who omitted it sees a template name they never
			// typed — saying where it came from is the difference
			// between a one-line fix and a hunt.
			origin := "declared"
			if _, written := cfg.Raw()["deployTo"]; !written {
				origin = fmt.Sprintf("defaulted by the %q plugin (not written in your config)", res.Type)
			}
			errs = append(errs, fmt.Sprintf(
				"%s.config.deployTo: template %q (%s) not declared in top-level `templates:` block",
				location, tplName, origin,
			))
		} else if !v.TemplateTypeExists(tpl.Type) {
			errs = append(errs, fmt.Sprintf("%s.config.deployTo: template %q has type %q which is not registered in ProvisionFuncByType", location, tplName, tpl.Type))
		} else if !v.HasInTreeMerge(tpl.Type) && !v.AnyPluginSupportsComponent(tpl.Type) {
			// Slice 1a.1 path (b): the dispatched template's owning
			// plugin must opt into the ComponentRaw side-channel via
			// SchemaResponse.supports_component — otherwise the plugin's
			// ProvisionFunc won't read the component's fields and the
			// dispatched deploy silently behaves as pass-through (the
			// §#19 foot-gun). In-tree templates with a registered merge
			// func (vm-compose) are exempt: the merge happens inside
			// core before the template's ProvisionFunc runs.
			errs = append(errs, fmt.Sprintf(
				"%s.config.deployTo: template %q (type %q) is not accepted by any loaded plugin — "+
					"the plugin's Schema must advertise `supports_component: [%q]` "+
					"(design doc §#19 path (b), slice 1a.1 of sc-components-platform)",
				location, tplName, tpl.Type, tpl.Type,
			))
		}
	}
	// image is required.
	if _, err := eff.Image(); err != nil {
		errs = append(errs, fmt.Sprintf("%s.config: %s", location, err))
	}
	// signingKey is optional; if set must be ${secret:...}.
	if _, err := eff.SigningKeyRef(); err != nil {
		errs = append(errs, fmt.Sprintf("%s.config: %s", location, err))
	}
	// Top-level keys: each one must be either well-known or allowed by
	// the plugin's override allowlist. Deliberately checked against the
	// OPERATOR's raw, not the effective config: the allowlist exists to
	// constrain what an operator may write, and a plugin's own defaults
	// are plugin-authored by construction. Checking the merged map would
	// force every plugin to allowlist each key it defaults.
	raw := cfg.Raw()
	for k := range raw {
		if IsComponentConfigTopLevelKey(k) {
			continue
		}
		if v.IsOverrideAllowed(res.Type, k) {
			continue
		}
		errs = append(errs, fmt.Sprintf("%s.config: unknown or disallowed top-level key %q for component type %q", location, k, res.Type))
	}
	return errs
}

func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n  - "
		}
		out += l
	}
	return out
}
