// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"testing"

	. "github.com/onsi/gomega"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// TestComponentTemplateDispatch — the slice 1a pre-step contract:
// when a resource's type is in pApi.ComponentResourceTypes, the sidecar
// dispatches the operator's named template with a merged ResourceInput,
// captures its output, and returns it. The plugin's own Provision RPC
// then fires with `_deployed_service` on its dependency outputs.
//
// This test hits `dispatchComponentTemplate` directly rather than the
// full `makeResourceSidecar` closure so it doesn't need an active
// pluginSession (attached by RunFunc, not by a unit test). The two
// halves of slice 1a are tested where each is simplest: the dispatch
// pre-step here, the existing plugin RPC + output harvest in the
// sidecar's longstanding tests.
//
// Design: forge/docs/design/2026-11-10/sc-components-platform/README.md §P1.
func TestComponentTemplateDispatch_PassThrough(t *testing.T) {
	RegisterTestingT(t)

	const (
		componentType = "test-sc-dummy"
		templateType  = "test-fake-template"
		templateName  = "fake-lambda"
	)

	// Register the fake template's ProvisionFunc, capturing its input for
	// later assertion. Unregistered on cleanup so the fleet-wide map stays
	// clean across tests.
	var capturedInput api.ResourceInput
	var capturedParams pApi.ProvisionParams
	fakeTpl := func(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
		capturedInput = input
		capturedParams = params
		return &api.ResourceOutput{Ref: &api.PluginResourceOutput{
			PluginName: "test-fake-template",
			Urn:        "urn:pulumi:test::proj::test-fake-template::fake-lambda",
			Outputs: map[string]api.PluginOutputValue{
				"url": {Value: "https://fake.test", Secret: false},
			},
		}}, nil
	}
	pApi.ProvisionFuncByType[templateType] = fakeTpl
	t.Cleanup(func() { delete(pApi.ProvisionFuncByType, templateType) })

	// Register the component type + its allowlist.
	pApi.RegisterComponentResourceType(componentType, []string{"image", "env"})
	t.Cleanup(func() {
		delete(pApi.ComponentResourceTypes, componentType)
		delete(pApi.ComponentAllowedOverrides, componentType)
	})

	// Build the Stack with the named template in Server.Templates.
	stack := api.Stack{
		Name: "smoke--staging",
		Server: api.ServerDescriptor{
			Templates: map[string]api.StackDescriptor{
				templateName: {
					Type:   templateType,
					Config: api.Config{Config: map[string]any{"memory": 256, "timeout": 10}},
				},
			},
		},
	}
	input := api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type: componentType,
			Name: "my-dummy",
			Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{
				"deployTo":   templateName,
				"image":      "simplecontainer/sc-component-dummy:v1",
				"signingKey": "${secret:dummy-key}",
				"env":        map[string]any{"ENV": "smoke"},
			}}},
		},
		StackParams: &api.StackParams{StackName: "smoke", Environment: "staging"},
	}
	params := pApi.ProvisionParams{
		Log: logger.New(),
	}

	// dispatchComponentTemplate is the subject under test — it's the
	// pre-step `makeResourceSidecar` runs when the type is a Component.
	bundle := &preEnginePluginBundle{log: logger.New()}
	out, err := bundle.dispatchComponentTemplate(nil /*ctx — not used in fake template*/, stack, input, params)
	Expect(err).ToNot(HaveOccurred())
	Expect(out).ToNot(BeNil())

	// (a) Fake template was called with the component's resource name
	// (URN-parity invariant — the dispatched template takes the
	// component's own name, not the template's declared name).
	Expect(capturedInput.Descriptor).ToNot(BeNil())
	Expect(capturedInput.Descriptor.Name).To(Equal("my-dummy"))
	Expect(capturedInput.Descriptor.Type).To(Equal(templateType))

	// (b) Operator template config was passed through verbatim (slice 1a
	// default is pass-through merge — per-template-type merge funcs are
	// future components' concern).
	cfg, ok := capturedInput.Descriptor.Config.Config.(map[string]any)
	Expect(ok).To(BeTrue(), "expected pass-through of map[string]any template config")
	Expect(cfg["memory"]).To(Equal(256))
	Expect(cfg["timeout"]).To(Equal(10))

	// (c) ProvisionParams forwarded with Log intact.
	Expect(capturedParams.Log).ToNot(BeNil())

	// (d) The dispatched output's Ref carries the fake template's
	// *PluginResourceOutput verbatim — the full sidecar body would
	// serialize this into DependencyOutputsJson._deployed_service for
	// the plugin's Provision RPC.
	ref, ok := out.Ref.(*api.PluginResourceOutput)
	Expect(ok).To(BeTrue(), "expected *PluginResourceOutput as Ref")
	Expect(ref.Urn).To(Equal("urn:pulumi:test::proj::test-fake-template::fake-lambda"))
	Expect(ref.Outputs["url"].Value).To(Equal("https://fake.test"))
}

// TestComponentTemplateDispatch_ComponentRawForwarded — slice 1a.1
// (design doc §#19 path (b)): the dispatched template's ResourceInput
// MUST carry a ComponentRaw map holding the operator's declared
// component config. This is what SC core then JSON-marshals into
// ProvisionRequest.component_raw for the plugin's Provision RPC, so a
// plugin that opted in via SchemaResponse.supports_component can overlay
// component fields onto its typed input struct.
//
// Regression guard: an empty/nil ComponentRaw here would collapse the
// aws-lambda dispatch path back to silent pass-through (the §#19
// foot-gun the operator picked path (b) to close).
func TestComponentTemplateDispatch_ComponentRawForwarded(t *testing.T) {
	RegisterTestingT(t)

	const (
		componentType = "test-sc-dummy-raw"
		templateType  = "test-fake-template-raw"
		templateName  = "fake-lambda-raw"
	)
	var capturedInput api.ResourceInput
	pApi.ProvisionFuncByType[templateType] = func(_ *sdk.Context, _ api.Stack, input api.ResourceInput, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
		capturedInput = input
		return &api.ResourceOutput{}, nil
	}
	t.Cleanup(func() { delete(pApi.ProvisionFuncByType, templateType) })
	pApi.RegisterComponentResourceType(componentType, []string{"image", "env"})
	t.Cleanup(func() {
		delete(pApi.ComponentResourceTypes, componentType)
		delete(pApi.ComponentAllowedOverrides, componentType)
	})

	stack := api.Stack{
		Server: api.ServerDescriptor{
			Templates: map[string]api.StackDescriptor{
				templateName: {Type: templateType, Config: api.Config{Config: map[string]any{"memory": 128}}},
			},
		},
	}
	input := api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type: componentType,
			Name: "my-dummy",
			Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{
				"deployTo":   templateName,
				"image":      "simplecontainer/sc-component-dummy:v2",
				"signingKey": "${secret:k}",
				"env":        map[string]any{"ENV": "smoke", "REGION": "eu-west-1"},
			}}},
		},
		StackParams: &api.StackParams{StackName: "smoke", Environment: "staging"},
	}

	bundle := &preEnginePluginBundle{log: logger.New()}
	_, err := bundle.dispatchComponentTemplate(nil, stack, input, pApi.ProvisionParams{Log: logger.New()})
	Expect(err).ToNot(HaveOccurred())

	// ComponentRaw is populated on the merged input the template saw.
	Expect(capturedInput.ComponentRaw).ToNot(BeNil(), "ComponentRaw must be forwarded to the dispatched template")
	Expect(capturedInput.ComponentRaw["image"]).To(Equal("simplecontainer/sc-component-dummy:v2"))
	Expect(capturedInput.ComponentRaw["deployTo"]).To(Equal(templateName))
	Expect(capturedInput.ComponentRaw["signingKey"]).To(Equal("${secret:k}"))
	envMap, ok := capturedInput.ComponentRaw["env"].(map[string]any)
	Expect(ok).To(BeTrue(), "env must be a map[string]any")
	Expect(envMap["ENV"]).To(Equal("smoke"))
	Expect(envMap["REGION"]).To(Equal("eu-west-1"))
}

// TestComponentTemplateDispatch_DeployToTemplateMissing — a config
// pointing at an undeclared template produces a diagnosable error
// naming the missing template, before any dispatch happens.
func TestComponentTemplateDispatch_DeployToTemplateMissing(t *testing.T) {
	RegisterTestingT(t)

	const componentType = "test-sc-dummy-missing"
	pApi.RegisterComponentResourceType(componentType, nil)
	t.Cleanup(func() { delete(pApi.ComponentResourceTypes, componentType) })

	stack := api.Stack{
		Server: api.ServerDescriptor{
			Templates: map[string]api.StackDescriptor{}, // none declared
		},
	}
	input := api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type: componentType,
			Name: "my-dummy",
			Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{
				"deployTo": "ghost-template",
				"image":    "x",
			}}},
		},
		StackParams: &api.StackParams{},
	}

	bundle := &preEnginePluginBundle{log: logger.New()}
	_, err := bundle.dispatchComponentTemplate(nil, stack, input, pApi.ProvisionParams{Log: logger.New()})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("ghost-template"))
	Expect(err.Error()).To(ContainSubstring("not declared"))
}

// TestComponentTemplateDispatch_DisallowedOverride — defense-in-depth
// check: a config carrying a key outside Component well-known keys +
// the plugin's allowlist is refused at dispatch time (parse-time
// validation catches it first in production).
func TestComponentTemplateDispatch_DisallowedOverride(t *testing.T) {
	RegisterTestingT(t)

	const (
		componentType = "test-sc-dummy-restricted"
		templateType  = "test-fake-template-restricted"
		templateName  = "restricted-tpl"
	)
	pApi.ProvisionFuncByType[templateType] = func(_ *sdk.Context, _ api.Stack, _ api.ResourceInput, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
		return nil, nil
	}
	t.Cleanup(func() { delete(pApi.ProvisionFuncByType, templateType) })
	pApi.RegisterComponentResourceType(componentType, []string{"image"}) // "env" not in allowlist, not well-known either
	t.Cleanup(func() {
		delete(pApi.ComponentResourceTypes, componentType)
		delete(pApi.ComponentAllowedOverrides, componentType)
	})

	stack := api.Stack{
		Server: api.ServerDescriptor{
			Templates: map[string]api.StackDescriptor{
				templateName: {Type: templateType},
			},
		},
	}
	input := api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type: componentType,
			Name: "my-dummy",
			Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{
				"deployTo": templateName,
				"image":    "x",
				"vpc":      "attacker-vpc", // neither well-known nor allowed
			}}},
		},
		StackParams: &api.StackParams{},
	}

	bundle := &preEnginePluginBundle{log: logger.New()}
	_, err := bundle.dispatchComponentTemplate(nil, stack, input, pApi.ProvisionParams{Log: logger.New()})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("vpc"))
	Expect(err.Error()).To(ContainSubstring("not in the allowlist"))
}
