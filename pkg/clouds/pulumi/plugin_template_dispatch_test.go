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
	out, _, err := bundle.dispatchComponentTemplate(nil /*ctx — not used in fake template*/, stack, input, params)
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
	_, _, err := bundle.dispatchComponentTemplate(nil, stack, input, pApi.ProvisionParams{Log: logger.New()})
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
	_, _, err := bundle.dispatchComponentTemplate(nil, stack, input, pApi.ProvisionParams{Log: logger.New()})
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
	_, _, err := bundle.dispatchComponentTemplate(nil, stack, input, pApi.ProvisionParams{Log: logger.New()})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("vpc"))
	Expect(err.Error()).To(ContainSubstring("not in the allowlist"))
}

// TestComponentTemplateDispatch_ComponentDefaults_Applied — slice 1d.6:
// a plugin's declared defaults are merged UNDER the operator's .Raw at
// dispatch time. Operator fields win per-key. This is the shape the
// operator UX ask is targeting — writing `my-backup: config: {deployTo,
// signingKey, storage, metadata, atlasAdmin}` should produce an
// effective config that includes the plugin's
// cloudExtras.{lambdaRoutingType, awsRoles} even though the operator
// didn't write it.
func TestComponentTemplateDispatch_ComponentDefaults_Applied(t *testing.T) {
	RegisterTestingT(t)

	const (
		componentType = "test-sc-backup-defaults"
		templateType  = "test-fake-template-defaults"
		templateName  = "fake-lambda-defaults"
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

	// Plugin advertises defaults for this component type: lambda routing
	// + aws roles — the real sc-backup shape.
	pApi.RegisterComponentDefaults(componentType, []byte(`{
		"cloudExtras": {
			"lambdaRoutingType": "function-url",
			"awsRoles": ["s3:PutObject", "s3:GetObject", "iam:PassRole"]
		}
	}`))
	t.Cleanup(func() { delete(pApi.ComponentDefaults, componentType) })

	stack := api.Stack{
		Server: api.ServerDescriptor{
			Templates: map[string]api.StackDescriptor{
				templateName: {Type: templateType, Config: api.Config{Config: map[string]any{"memory": 128}}},
			},
		},
	}
	// Operator writes a MINIMAL config — only the component-level fields,
	// no cloudExtras. This is the UX win: operator doesn't spell out
	// routing or awsRoles; plugin defaults fill them in.
	input := api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type: componentType,
			Name: "my-backup",
			Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{
				"deployTo":   templateName,
				"signingKey": "${secret:k}",
			}}},
		},
		StackParams: &api.StackParams{StackName: "smoke", Environment: "staging"},
	}

	bundle := &preEnginePluginBundle{log: logger.New()}
	_, _, err := bundle.dispatchComponentTemplate(nil, stack, input, pApi.ProvisionParams{Log: logger.New()})
	Expect(err).ToNot(HaveOccurred())

	// ComponentRaw on the dispatched input carries the MERGED map:
	// plugin's cloudExtras is present even though the operator didn't
	// write it.
	Expect(capturedInput.ComponentRaw).ToNot(BeNil())
	ce, ok := capturedInput.ComponentRaw["cloudExtras"].(map[string]any)
	Expect(ok).To(BeTrue(), "cloudExtras must be a map after default merge")
	Expect(ce["lambdaRoutingType"]).To(Equal("function-url"), "plugin default routing must apply")
	roles, ok := ce["awsRoles"].([]any)
	Expect(ok).To(BeTrue(), "awsRoles must be a []any")
	Expect(roles).To(ConsistOf("s3:PutObject", "s3:GetObject", "iam:PassRole"))
	// Operator fields still present verbatim.
	Expect(capturedInput.ComponentRaw["deployTo"]).To(Equal(templateName))
	Expect(capturedInput.ComponentRaw["signingKey"]).To(Equal("${secret:k}"))
}

// TestComponentTemplateDispatch_ComponentDefaults_OperatorWins — when
// the operator writes a key that collides with a plugin default, the
// operator wins. Prevents a plugin version bump from silently
// rewriting operator-declared config.
func TestComponentTemplateDispatch_ComponentDefaults_OperatorWins(t *testing.T) {
	RegisterTestingT(t)

	const (
		componentType = "test-sc-backup-opwin"
		templateType  = "test-fake-template-opwin"
		templateName  = "fake-lambda-opwin"
	)
	var capturedInput api.ResourceInput
	pApi.ProvisionFuncByType[templateType] = func(_ *sdk.Context, _ api.Stack, input api.ResourceInput, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
		capturedInput = input
		return &api.ResourceOutput{}, nil
	}
	t.Cleanup(func() { delete(pApi.ProvisionFuncByType, templateType) })
	// Operator wants to override cloudExtras, so cloudExtras must be in
	// the allowlist — sc-component-backup opens it (plugin_main.go:145).
	pApi.RegisterComponentResourceType(componentType, []string{"cloudExtras"})
	t.Cleanup(func() {
		delete(pApi.ComponentResourceTypes, componentType)
		delete(pApi.ComponentAllowedOverrides, componentType)
	})
	pApi.RegisterComponentDefaults(componentType, []byte(`{
		"cloudExtras": {
			"lambdaRoutingType": "function-url",
			"awsRoles": ["s3:PutObject", "iam:PassRole"]
		}
	}`))
	t.Cleanup(func() { delete(pApi.ComponentDefaults, componentType) })

	stack := api.Stack{
		Server: api.ServerDescriptor{
			Templates: map[string]api.StackDescriptor{
				templateName: {Type: templateType},
			},
		},
	}
	// Operator overrides routing, provides a custom awsRoles list.
	input := api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type: componentType,
			Name: "my-backup",
			Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{
				"deployTo": templateName,
				"cloudExtras": map[string]any{
					"lambdaRoutingType": "api-gateway",
					"awsRoles":          []any{"custom-role"},
				},
			}}},
		},
		StackParams: &api.StackParams{},
	}
	bundle := &preEnginePluginBundle{log: logger.New()}
	_, _, err := bundle.dispatchComponentTemplate(nil, stack, input, pApi.ProvisionParams{Log: logger.New()})
	Expect(err).ToNot(HaveOccurred())

	ce := capturedInput.ComponentRaw["cloudExtras"].(map[string]any)
	Expect(ce["lambdaRoutingType"]).To(Equal("api-gateway"), "operator scalar must win")
	Expect(ce["awsRoles"]).To(Equal([]any{"custom-role"}), "operator list REPLACES, no append")
}

// TestComponentTemplateDispatch_ComponentDefaults_NoneRegistered — the
// older-plugin / unadvertised case: no defaults for this type means
// dispatch behaves exactly as pre-1d.6 (operator config crosses verbatim).
// Regression guard against the merge path mutating operator config when
// there's no default to overlay.
func TestComponentTemplateDispatch_ComponentDefaults_NoneRegistered(t *testing.T) {
	RegisterTestingT(t)

	const (
		componentType = "test-sc-no-defaults"
		templateType  = "test-fake-template-nodef"
		templateName  = "fake-lambda-nodef"
	)
	var capturedInput api.ResourceInput
	pApi.ProvisionFuncByType[templateType] = func(_ *sdk.Context, _ api.Stack, input api.ResourceInput, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
		capturedInput = input
		return &api.ResourceOutput{}, nil
	}
	t.Cleanup(func() { delete(pApi.ProvisionFuncByType, templateType) })
	pApi.RegisterComponentResourceType(componentType, nil)
	t.Cleanup(func() { delete(pApi.ComponentResourceTypes, componentType) })
	// Deliberately NOT registering defaults.

	stack := api.Stack{
		Server: api.ServerDescriptor{
			Templates: map[string]api.StackDescriptor{templateName: {Type: templateType}},
		},
	}
	input := api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type: componentType,
			Name: "my-dummy",
			Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{
				"deployTo":   templateName,
				"signingKey": "${secret:k}",
			}}},
		},
		StackParams: &api.StackParams{},
	}
	bundle := &preEnginePluginBundle{log: logger.New()}
	_, _, err := bundle.dispatchComponentTemplate(nil, stack, input, pApi.ProvisionParams{Log: logger.New()})
	Expect(err).ToNot(HaveOccurred())

	Expect(capturedInput.ComponentRaw).ToNot(BeNil())
	Expect(capturedInput.ComponentRaw["deployTo"]).To(Equal(templateName))
	Expect(capturedInput.ComponentRaw["signingKey"]).To(Equal("${secret:k}"))
	// No default-only keys appeared.
	Expect(capturedInput.ComponentRaw).ToNot(HaveKey("cloudExtras"))
}

// TestComponentTemplateDispatch_DefaultsInterpolated — slice
// sc-components-defaults-template-interpolation: plugin-declared
// $$sc:-prefixed defaults get ${name}/${env}/${stack}/${template.*}
// substituted at dispatch time. End-to-end: register a component with
// a default carrying `$$sc:sc-backup-${name}-archives--${env}`, dispatch
// it, assert the dispatched input's ComponentRaw carries the resolved
// literal.
func TestComponentTemplateDispatch_DefaultsInterpolated(t *testing.T) {
	RegisterTestingT(t)

	const (
		componentType = "test-sc-defaults-interp"
		templateType  = "test-fake-template-interp"
		templateName  = "fake-lambda-interp"
	)
	var capturedInput api.ResourceInput
	pApi.ProvisionFuncByType[templateType] = func(_ *sdk.Context, _ api.Stack, input api.ResourceInput, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
		capturedInput = input
		return &api.ResourceOutput{}, nil
	}
	t.Cleanup(func() { delete(pApi.ProvisionFuncByType, templateType) })

	pApi.RegisterComponentResourceType(componentType, nil)
	t.Cleanup(func() { delete(pApi.ComponentResourceTypes, componentType) })

	// Plugin's defaults use the sentinel + tokens. Mix a self-derived
	// one (${name}, ${env}) with a template-config one
	// (${template.region}) with a parent-token one (${parent:…}) to
	// pin all three paths at once.
	pApi.RegisterComponentDefaults(componentType, []byte(`{
		"storage": {
			"type":   "s3",
			"bucket": "$$sc:sc-backup-${name}-archives--${env}",
			"region": "$$sc:${template.region}"
		},
		"worker": {
			"cluster":   "$$sc:sc-backup-${name}-cluster",
			"subnetIds": "$$sc:${parent:default-vpc.subnets}"
		}
	}`))
	t.Cleanup(func() { delete(pApi.ComponentDefaults, componentType) })

	// Template carries a region in its config — the interpolation
	// reads it for ${template.region}.
	stack := api.Stack{
		Name: "smoke--prod",
		Server: api.ServerDescriptor{
			Templates: map[string]api.StackDescriptor{
				templateName: {
					Type:   templateType,
					Config: api.Config{Config: map[string]any{"region": "eu-central-1"}},
				},
			},
		},
	}
	input := api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type: componentType,
			Name: "my-backup",
			Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{
				"deployTo": templateName,
			}}},
		},
		StackParams: &api.StackParams{StackName: "smoke", Environment: "prod"},
	}
	bundle := &preEnginePluginBundle{log: logger.New()}
	_, _, err := bundle.dispatchComponentTemplate(nil, stack, input, pApi.ProvisionParams{Log: logger.New()})
	Expect(err).ToNot(HaveOccurred())

	s := capturedInput.ComponentRaw["storage"].(map[string]any)
	Expect(s["bucket"]).To(Equal("sc-backup-my-backup-archives--prod"), "${name} + ${env} resolved")
	Expect(s["region"]).To(Equal("eu-central-1"), "${template.region} resolved")

	w := capturedInput.ComponentRaw["worker"].(map[string]any)
	Expect(w["cluster"]).To(Equal("sc-backup-my-backup-cluster"), "${name}-only resolved")
	// ${parent:…} is NOT resolved in this pass — sentinel + token kept
	// so a future parent-resolver slice picks it up.
	Expect(w["subnetIds"]).To(Equal("$$sc:${parent:default-vpc.subnets}"), "parent token retains sentinel for a later pass")
}

// TestComponentTemplateDispatch_OperatorSentinelRefused — a hostile or
// confused operator writing a $$sc:-prefixed string is refused at
// dispatch time. Parse-time validation is the first guard; this is
// defense-in-depth for test fixtures + future direct callers.
func TestComponentTemplateDispatch_OperatorSentinelRefused(t *testing.T) {
	RegisterTestingT(t)

	const (
		componentType = "test-sc-op-sentinel"
		templateType  = "test-fake-template-op-sentinel"
		templateName  = "fake-lambda-op-sentinel"
	)
	pApi.ProvisionFuncByType[templateType] = func(_ *sdk.Context, _ api.Stack, _ api.ResourceInput, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
		return &api.ResourceOutput{}, nil
	}
	t.Cleanup(func() { delete(pApi.ProvisionFuncByType, templateType) })
	pApi.RegisterComponentResourceType(componentType, []string{"cloudExtras"})
	t.Cleanup(func() {
		delete(pApi.ComponentResourceTypes, componentType)
		delete(pApi.ComponentAllowedOverrides, componentType)
	})

	stack := api.Stack{
		Server: api.ServerDescriptor{
			Templates: map[string]api.StackDescriptor{templateName: {Type: templateType}},
		},
	}
	// Operator writes a $$sc: string — must be refused.
	input := api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type: componentType,
			Name: "my-dummy",
			Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{
				"deployTo": templateName,
				"cloudExtras": map[string]any{
					"lambdaRoutingType": "$$sc:${name}-smuggled",
				},
			}}},
		},
		StackParams: &api.StackParams{},
	}
	bundle := &preEnginePluginBundle{log: logger.New()}
	_, _, err := bundle.dispatchComponentTemplate(nil, stack, input, pApi.ProvisionParams{Log: logger.New()})
	if err == nil {
		t.Fatal("expected refusal, got nil")
	}
	Expect(err.Error()).To(ContainSubstring("$$sc:"))
	Expect(err.Error()).To(ContainSubstring("reserved prefix"))
}

// TestComponentTemplateDispatch_MergedRawForOwnProvision — the merged
// raw dispatch returns for the component plugin's OWN Provision RPC.
//
// Two Provision calls happen in one component dispatch: the dispatched
// template's (aws-lambda & co) and the component plugin's own. Until
// 2026-11-25 only the first saw the merged config; the second got
// `serializeConfigToInputs(input.Descriptor.Config.Config)` — the
// operator's raw, with none of the plugin's own ComponentDefaults
// applied. A plugin could therefore ship a default, watch it reach the
// deployed Lambda, and still hard-fail its own Provision on the key
// being absent.
//
// That is not hypothetical: forge#2032 deleted the hand-written
// `worker:` block from `my-backup` after sc-component-backup started
// defaulting it, and provision-preview failed with
// `EcsProvisionInputFromRaw: deployTo: lambda requires `worker` block`
// while the very same preview showed the Lambda picking the defaults up.
//
// The `ingress` half is the subtler assertion. Dispatch strips
// `ingress.hostname` before handing `merged` to the template plugin
// (whose `params.Registrar` is nil across the bus — SC core wires DNS
// itself afterwards). The component plugin published that hostname and
// must still see it, so the snapshot has to be taken BEFORE the strip.
// Move the `mergedForPlugin` line below the strip and this test fails
// on ingress while every other assertion still passes.
func TestComponentTemplateDispatch_MergedRawForOwnProvision(t *testing.T) {
	RegisterTestingT(t)

	const (
		componentType = "test-sc-own-provision"
		templateType  = "test-fake-template-own-provision"
		templateName  = "fake-lambda-own-provision"
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

	// `worker` is the real sc-component-backup default: a Fargate task
	// sizing block its own Provision reads to build the one-shot
	// mongodump task, and nothing the operator should have to repeat.
	pApi.RegisterComponentDefaults(componentType, []byte(`{
		"worker": {"cpu": 512, "memoryMB": 2048, "cluster": "$$sc:sc-backup-${name}-cluster"},
		"timeout": 300
	}`))
	t.Cleanup(func() { delete(pApi.ComponentDefaults, componentType) })

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
			Name: "my-backup",
			Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{
				"deployTo": templateName,
				"ingress":  map[string]any{"hostname": "backup.example.com"},
			}}},
		},
		StackParams: &api.StackParams{StackName: "smoke", Environment: "staging"},
	}

	bundle := &preEnginePluginBundle{log: logger.New()}
	// A non-nil registrar: `ingress.hostname` is set, so dispatch runs
	// its post-dispatch DNS wire. The fake template's output carries no
	// *api.PluginResourceOutput Ref, so the wire takes its in-tree
	// early-return and never reaches the registrar.
	_, mergedRaw, err := bundle.dispatchComponentTemplate(nil, stack, input, mkParams(&cdnsFakeRegistrar{}))
	Expect(err).ToNot(HaveOccurred())
	Expect(mergedRaw).ToNot(BeNil(), "the component plugin's own Provision gets this map; nil means it sees the operator's raw again")

	// The defaults-only keys the operator no longer writes.
	worker, ok := mergedRaw["worker"].(map[string]any)
	Expect(ok).To(BeTrue(), "worker must be a map on the merged raw: %#v", mergedRaw["worker"])
	Expect(worker["cpu"]).To(BeEquivalentTo(512))
	Expect(worker["memoryMB"]).To(BeEquivalentTo(2048))
	Expect(worker["cluster"]).To(Equal("sc-backup-my-backup-cluster"), "self-token interpolation must have run before the snapshot")
	Expect(mergedRaw["timeout"]).To(BeEquivalentTo(300))
	// Operator fields cross through verbatim.
	Expect(mergedRaw["deployTo"]).To(Equal(templateName))

	// ingress: the component plugin sees the hostname, the dispatched
	// template does not.
	ingress, ok := mergedRaw["ingress"].(map[string]any)
	Expect(ok).To(BeTrue(), "ingress must survive on the component plugin's view: %#v", mergedRaw["ingress"])
	Expect(ingress["hostname"]).To(Equal("backup.example.com"))
	if tplIngress, present := capturedInput.ComponentRaw["ingress"]; present {
		m, isMap := tplIngress.(map[string]any)
		Expect(isMap).To(BeTrue())
		Expect(m).ToNot(HaveKey("hostname"), "the template plugin must never see ingress.hostname — its params.Registrar is nil across the bus")
	}

	// The two views must not alias: a template plugin mutating its
	// overlay cannot rewrite what the component plugin is handed.
	capturedInput.ComponentRaw["timeout"] = 1
	Expect(mergedRaw["timeout"]).To(BeEquivalentTo(300), "the two Provision views must be independent copies")
}
