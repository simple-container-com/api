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

func newFakeValidator() fakeValidator {
	return fakeValidator{
		componentTypes: map[string]struct{}{"sc-dummy": {}},
		allowed:        map[string]map[string]struct{}{"sc-dummy": {"storage": {}}},
		templates:      map[string]struct{}{"vm-compose": {}},
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
	raw["signingKey"] = "plaintext-should-fail"
	err := ValidateComponentConfigs(desc, newFakeValidator())
	if err == nil || !strings.Contains(err.Error(), "is not a ${secret:...} reference") {
		t.Fatalf("expected signingKey shape error, got: %v", err)
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
