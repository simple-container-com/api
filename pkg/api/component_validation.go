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
	// deployTo exists, points at a declared template of a known type.
	tplName, err := cfg.DeployTo()
	if err != nil {
		errs = append(errs, fmt.Sprintf("%s.config: %s", location, err))
	} else {
		tpl, ok := descriptor.Templates[tplName]
		if !ok {
			errs = append(errs, fmt.Sprintf("%s.config.deployTo: template %q not declared in top-level `templates:` block", location, tplName))
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
	if _, err := cfg.Image(); err != nil {
		errs = append(errs, fmt.Sprintf("%s.config: %s", location, err))
	}
	// signingKey is optional; if set must be ${secret:...}.
	if _, err := cfg.SigningKeyRef(); err != nil {
		errs = append(errs, fmt.Sprintf("%s.config: %s", location, err))
	}
	// Top-level keys: each one must be either well-known or allowed by
	// the plugin's override allowlist.
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
