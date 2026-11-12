// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"strings"
	"testing"
)

// fakeValidator is a test double for ComponentValidator. Mimics the
// runtime shape (ComponentResourceTypes set + ComponentAllowedOverrides +
// ProvisionFuncByType) without importing pkg/clouds/pulumi.
type fakeValidator struct {
	componentTypes map[string]struct{}
	allowed        map[string]map[string]struct{}
	templates      map[string]struct{}
	inTreeMerge    map[string]struct{}
	pluginSupports map[string]struct{}
}

func (f fakeValidator) IsComponentType(t string) bool {
	_, ok := f.componentTypes[t]
	return ok
}

func (f fakeValidator) IsOverrideAllowed(t, key string) bool {
	byKey, ok := f.allowed[t]
	if !ok {
		return false
	}
	_, ok = byKey[key]
	return ok
}

func (f fakeValidator) TemplateTypeExists(templateType string) bool {
	_, ok := f.templates[templateType]
	return ok
}

func (f fakeValidator) HasInTreeMerge(templateType string) bool {
	_, ok := f.inTreeMerge[templateType]
	return ok
}

func (f fakeValidator) AnyPluginSupportsComponent(templateType string) bool {
	_, ok := f.pluginSupports[templateType]
	return ok
}

func newFakeValidator() fakeValidator {
	return fakeValidator{
		componentTypes: map[string]struct{}{"sc-dummy": {}},
		allowed:        map[string]map[string]struct{}{"sc-dummy": {"storage": {}}},
		templates:      map[string]struct{}{"vm-compose": {}},
		// vm-compose has an in-tree merge func → bypasses the
		// plugin-supports_component opt-in check.
		inTreeMerge:    map[string]struct{}{"vm-compose": {}},
		pluginSupports: map[string]struct{}{},
	}
}

func newValidDescriptor() *ServerDescriptor {
	return &ServerDescriptor{
		Templates: map[string]StackDescriptor{
			"compose-on-vm": {Type: "vm-compose"},
		},
		Resources: PerStackResourcesDescriptor{
			Resources: map[string]PerEnvResourcesDescriptor{
				"smoke": {
					Resources: map[string]ResourceDescriptor{
						"my-dummy": {
							Type: "sc-dummy",
							Name: "my-dummy",
							Config: Config{Config: &PluginFieldConfig{Raw: map[string]any{
								"deployTo":   "compose-on-vm",
								"image":      "simplecontainer/sc-component-dummy:v1",
								"signingKey": "${secret:dummy-signing-key}",
								"env":        map[string]any{"ENV": "smoke"},
							}}},
						},
					},
				},
			},
		},
	}
}

func TestValidateComponentConfigs_HappyPath(t *testing.T) {
	t.Parallel()
	if err := ValidateComponentConfigs(newValidDescriptor(), newFakeValidator()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateComponentConfigs_NilInputs(t *testing.T) {
	t.Parallel()
	if err := ValidateComponentConfigs(nil, newFakeValidator()); err != nil {
		t.Fatalf("nil descriptor: %v", err)
	}
	if err := ValidateComponentConfigs(newValidDescriptor(), nil); err != nil {
		t.Fatalf("nil validator: %v", err)
	}
}

func TestValidateComponentConfigs_TemplateMissing(t *testing.T) {
	t.Parallel()
	desc := newValidDescriptor()
	// Remove the template.
	delete(desc.Templates, "compose-on-vm")
	err := ValidateComponentConfigs(desc, newFakeValidator())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not declared in top-level `templates:` block") {
		t.Fatalf("missing template error: %v", err)
	}
}

func TestValidateComponentConfigs_UnregisteredTemplateType(t *testing.T) {
	t.Parallel()
	v := newFakeValidator()
	delete(v.templates, "vm-compose")
	err := ValidateComponentConfigs(newValidDescriptor(), v)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "is not registered in ProvisionFuncByType") {
		t.Fatalf("unregistered template error: %v", err)
	}
}

func TestValidateComponentConfigs_MissingImage(t *testing.T) {
	t.Parallel()
	desc := newValidDescriptor()
	raw := desc.Resources.Resources["smoke"].Resources["my-dummy"].Config.Config.(*PluginFieldConfig).Raw
	delete(raw, "image")
	err := ValidateComponentConfigs(desc, newFakeValidator())
	if err == nil || !strings.Contains(err.Error(), "image is required") {
		t.Fatalf("expected image-required, got: %v", err)
	}
}

func TestValidateComponentConfigs_BadSigningKey(t *testing.T) {
	t.Parallel()
	desc := newValidDescriptor()
	raw := desc.Resources.Resources["smoke"].Resources["my-dummy"].Config.Config.(*PluginFieldConfig).Raw
	// SigningKey validation post-secret-resolution only rejects shape
	// problems: non-string or empty string. The `${secret:...}` pattern
	// check was removed 2026-11-11 after the first slice-1a smoke caught
	// that SC resolves ${secret:...} → value BEFORE validation runs, so
	// the pattern check always saw a resolved value.
	raw["signingKey"] = ""
	err := ValidateComponentConfigs(desc, newFakeValidator())
	if err == nil || !strings.Contains(err.Error(), "empty string") {
		t.Fatalf("expected empty-signingKey error, got: %v", err)
	}
}

func TestValidateComponentConfigs_DisallowedOverride(t *testing.T) {
	t.Parallel()
	desc := newValidDescriptor()
	raw := desc.Resources.Resources["smoke"].Resources["my-dummy"].Config.Config.(*PluginFieldConfig).Raw
	raw["vpc"] = "attacker-vpc-id"
	err := ValidateComponentConfigs(desc, newFakeValidator())
	if err == nil || !strings.Contains(err.Error(), "unknown or disallowed top-level key") {
		t.Fatalf("expected disallowed-override error, got: %v", err)
	}
}

func TestValidateComponentConfigs_AllowedOverride(t *testing.T) {
	t.Parallel()
	desc := newValidDescriptor()
	raw := desc.Resources.Resources["smoke"].Resources["my-dummy"].Config.Config.(*PluginFieldConfig).Raw
	// "storage" is in the fake validator's allowlist for sc-dummy.
	raw["storage"] = "some-bucket"
	if err := ValidateComponentConfigs(desc, newFakeValidator()); err != nil {
		t.Fatalf("allowed override rejected: %v", err)
	}
}

// TestValidateComponentConfigs_PluginOptedIn covers the slice-1a.1 path (b)
// happy case: the dispatched template's type has no in-tree merge func but
// SOME plugin has advertised supports_component for it. Validation passes.
func TestValidateComponentConfigs_PluginOptedIn(t *testing.T) {
	t.Parallel()
	desc := newValidDescriptor()
	// Repoint the template at a plugin-owned type.
	desc.Templates["compose-on-vm"] = StackDescriptor{Type: "aws-lambda"}
	v := newFakeValidator()
	v.templates = map[string]struct{}{"aws-lambda": {}}
	// Plugin opted in via SchemaResponse.supports_component.
	v.pluginSupports = map[string]struct{}{"aws-lambda": {}}
	// NOT in the in-tree merge set.
	v.inTreeMerge = map[string]struct{}{"vm-compose": {}}
	if err := ValidateComponentConfigs(desc, v); err != nil {
		t.Fatalf("plugin-opted-in template rejected: %v", err)
	}
}

// TestValidateComponentConfigs_PluginNotOptedIn covers the slice-1a.1 path (b)
// foot-gun guard: template type is registered but NO plugin has advertised
// supports_component for it, and no in-tree merge func exists. Validation
// refuses with a descriptive error.
func TestValidateComponentConfigs_PluginNotOptedIn(t *testing.T) {
	t.Parallel()
	desc := newValidDescriptor()
	desc.Templates["compose-on-vm"] = StackDescriptor{Type: "aws-lambda"}
	v := newFakeValidator()
	v.templates = map[string]struct{}{"aws-lambda": {}}
	// Neither in-tree merge nor plugin-opted-in.
	v.inTreeMerge = map[string]struct{}{}
	v.pluginSupports = map[string]struct{}{}
	err := ValidateComponentConfigs(desc, v)
	if err == nil {
		t.Fatal("expected error for plugin-not-opted-in template")
	}
	msg := err.Error()
	if !strings.Contains(msg, "not accepted by any loaded plugin") {
		t.Fatalf("error does not mention opt-in gap: %v", err)
	}
	if !strings.Contains(msg, "supports_component") {
		t.Fatalf("error does not point at Schema.supports_component: %v", err)
	}
	if !strings.Contains(msg, "aws-lambda") {
		t.Fatalf("error does not name the template type: %v", err)
	}
}

// TestValidateComponentConfigs_InTreeMergeBypasses covers the slice-1a.1
// bypass: vm-compose (in-tree merge) is accepted even though NO plugin
// advertises supports_component — the merge happens inside core before
// the template's ProvisionFunc runs, so the plugin-opt-in check is moot.
func TestValidateComponentConfigs_InTreeMergeBypasses(t *testing.T) {
	t.Parallel()
	v := newFakeValidator()
	// Explicitly ensure NO plugin advertises support for vm-compose.
	v.pluginSupports = map[string]struct{}{}
	// vm-compose stays in the in-tree merge set.
	if err := ValidateComponentConfigs(newValidDescriptor(), v); err != nil {
		t.Fatalf("in-tree merge template rejected: %v", err)
	}
}

func TestValidateComponentConfigs_NonComponentSkipped(t *testing.T) {
	t.Parallel()
	desc := newValidDescriptor()
	// Add a non-component resource that would otherwise fail every check.
	desc.Resources.Resources["smoke"].Resources["regular"] = ResourceDescriptor{
		Type: "s3-bucket",
		Name: "regular",
		Config: Config{Config: &PluginFieldConfig{Raw: map[string]any{
			"anything": "goes",
		}}},
	}
	if err := ValidateComponentConfigs(desc, newFakeValidator()); err != nil {
		t.Fatalf("non-component resource rejected: %v", err)
	}
}
