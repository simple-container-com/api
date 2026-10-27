// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"reflect"
	"testing"

	. "github.com/onsi/gomega"
)

// TestServerDescriptorCopyCoversEveryField is the ServerDescriptor twin of
// TestCopyPreservesEveryScalarField, and it exists because the same class of bug
// landed twice. ServerDescriptor.Copy is a hand-written field-by-field
// constructor; a field added to the struct and not added to the constructor
// compiles, passes every existing test, and is then silently absent from every
// code path that copies — which is every child-stack deploy, because
// StacksMap.ReconcileForDeploy does `stack.Server = parentStack.Server.Copy()`.
//
// Plugins was the second victim (the first was RegistrarDescriptor.Default).
// `sc provision` kept working because it hands the freshly-read descriptor to
// the provisioner without copying, so the gap was invisible until a deploy.
//
// Rather than name fields, walk the type: populate every field with a non-zero
// value and require the copy to carry something non-zero for each. A newly
// added field defaults to its zero value in the copy and fails here.
func TestServerDescriptorCopyCoversEveryField(t *testing.T) {
	RegisterTestingT(t)

	original := ServerDescriptor{
		SchemaVersion: "1.0",
		Provisioner:   ProvisionerDescriptor{Type: "pulumi"},
		Secrets:       SecretsConfigDescriptor{Type: "pulumi-cloud"},
		CiCd:          CiCdDescriptor{Type: "github-actions"},
		Templates:     map[string]StackDescriptor{"t": {Type: "gcp-static-website"}},
		Resources: PerStackResourcesDescriptor{
			Registrar: RegistrarDescriptor{Type: "cloudflare"},
		},
		Variables: map[string]VariableDescriptor{"v": {Type: "string", Value: "x"}},
		Plugins: map[string]PluginSpec{
			"cloudflare": {Source: &PluginSource{
				Type:       "github-release",
				Repository: "simple-container-com/sc-plugin-cloudflare",
				Version:    "v2026.10.26-5eddde3",
			}},
		},
	}

	v := reflect.ValueOf(original)
	typ := v.Type()
	for i := 0; i < typ.NumField(); i++ {
		Expect(v.Field(i).IsZero()).To(
			BeFalse(),
			"test fixture is incomplete: ServerDescriptor.%s is zero, so the assertion below would pass vacuously — populate it",
			typ.Field(i).Name,
		)
	}

	copied := original.Copy()
	cv := reflect.ValueOf(copied)
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		Expect(cv.Field(i).IsZero()).To(
			BeFalse(),
			"ServerDescriptor.Copy() dropped field %q — add it to the constructor in copy.go", name,
		)
	}
}

// TestCopyKeepsPluginsThroughReconcileForDeploy is the end-to-end shape of the
// bug that run 40554605682 hit: the plugin loader reads Plugins off the
// descriptor it is handed, and on a deploy that descriptor has been through
// ReconcileForDeploy.
func TestCopyKeepsPluginsThroughReconcileForDeploy(t *testing.T) {
	RegisterTestingT(t)

	parent := Stack{
		Name: "dist",
		Server: ServerDescriptor{
			Resources: PerStackResourcesDescriptor{
				Registrar: RegistrarDescriptor{Type: "cloudflare"},
			},
			Plugins: map[string]PluginSpec{
				"cloudflare": {
					SHA256: "deadbeef",
					Source: &PluginSource{
						Type:       "github-release",
						Repository: "simple-container-com/sc-plugin-cloudflare",
						Version:    "v2026.10.26-5eddde3",
					},
				},
			},
		},
		Client: ClientDescriptor{
			Stacks: map[string]StackClientDescriptor{
				"prod": {ParentStack: "dist", Template: "static-website"},
			},
		},
	}
	stacks := StacksMap{"dist": parent}

	out, err := stacks.ReconcileForDeploy(StackParams{StackName: "dist", Environment: "prod"})
	Expect(err).To(BeNil())

	reconciled := (*out)["dist"]
	Expect(reconciled.Server.Plugins).To(HaveKey("cloudflare"),
		"the parent's plugins: block must survive reconciliation, or loadPreEnginePlugins no-ops on every deploy")
	spec := reconciled.Server.Plugins["cloudflare"]
	Expect(spec.Source).ToNot(BeNil())
	Expect(spec.Source.Version).To(Equal("v2026.10.26-5eddde3"))
	Expect(spec.SHA256).To(Equal("deadbeef"))

	// Deep copy, not a shared pointer: mutating the copy must not reach the original.
	spec.Source.Version = "mutated"
	Expect(parent.Server.Plugins["cloudflare"].Source.Version).To(Equal("v2026.10.26-5eddde3"))
}

// TestChildStackKeepsPlugins covers the other copying path — ChildStack, which
// pulumi's toChildStack uses to build the `<stack>--<env>` descriptor.
func TestChildStackKeepsPlugins(t *testing.T) {
	RegisterTestingT(t)

	parent := Stack{
		Name: "dist",
		Server: ServerDescriptor{
			Plugins: map[string]PluginSpec{"cloudflare": {Path: "/opt/sc-plugin-cloudflare"}},
		},
	}
	child := parent.ChildStack("dist--prod")
	Expect(child.Server.Plugins).To(HaveKeyWithValue("cloudflare", PluginSpec{Path: "/opt/sc-plugin-cloudflare"}))
}

// TestCopyKeepsPluginsNilWhenAbsent preserves the nil-vs-empty distinction the
// mapValues helper exists for: a stack with no plugins must not come back with
// an allocated empty map, so a copy still compares equal to its original.
func TestCopyKeepsPluginsNilWhenAbsent(t *testing.T) {
	RegisterTestingT(t)

	sd := ServerDescriptor{SchemaVersion: "1.0"}
	Expect(sd.Copy().Plugins).To(BeNil())
}
