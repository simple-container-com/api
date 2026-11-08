// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"

	"github.com/simple-container-com/api/pkg/api"
)

// fakeSibling stands in for k3s.K3sOutput: an in-tree resource output that
// publishes values a sibling may read.
type fakeSibling struct {
	values map[string]sdk.StringOutput
}

func (f *fakeSibling) OutputValue(key string) (sdk.StringOutput, bool) {
	v, ok := f.values[key]
	return v, ok
}

func (f *fakeSibling) PublishedOutputKeys() []string {
	return []string{"endpoint", "kubeconfig"}
}

func ref(name, key string) api.ResourceRef {
	return api.ResourceRef{ResourceName: name, Key: key}
}

func await(t *testing.T, out sdk.StringOutput) internals.UnsafeAwaitOutputResult {
	t.Helper()
	res, err := internals.UnsafeAwaitOutput(context.Background(), out)
	if err != nil {
		t.Fatalf("awaiting the resolved value: %v", err)
	}
	return res
}

// The motivating case end to end: a Caddy edge resolving
// `${resource:my-k3s.kubeconfig}` against the cluster provisioned a moment
// earlier in the same pass.
func TestResolveResourceRef_TypedSibling(t *testing.T) {
	const kubeconfig = "apiVersion: v1\nkind: Config\n"
	outs := ResourcesOutputs{
		"my-k3s": {Ref: &fakeSibling{values: map[string]sdk.StringOutput{
			"kubeconfig": sdk.ToSecret(sdk.String(kubeconfig)).(sdk.StringOutput),
			"endpoint":   sdk.String("10.0.0.1").ToStringOutput(),
		}}},
	}

	got, err := ResolveResourceRef(ref("my-k3s", "kubeconfig"), outs)
	if err != nil {
		t.Fatalf("ResolveResourceRef: %v", err)
	}
	res := await(t, got)
	if res.Value != kubeconfig {
		t.Errorf("value = %q, want %q", res.Value, kubeconfig)
	}
	// The producer's secret marking has to survive the resolution — this value is
	// a cluster-admin credential, and an unmarked one is printed in the update
	// summary and stored in the checkpoint in cleartext.
	if !res.Secret {
		t.Error("a secret sibling output came back unmarked")
	}

	plain, err := ResolveResourceRef(ref("my-k3s", "endpoint"), outs)
	if err != nil {
		t.Fatalf("ResolveResourceRef(endpoint): %v", err)
	}
	if v := await(t, plain); v.Value != "10.0.0.1" {
		t.Errorf("endpoint = %q, want 10.0.0.1", v.Value)
	}
}

// A key the resource type does not expose is the realistic miss — not a typo —
// so the message has to list what it does expose.
func TestResolveResourceRef_TypedSiblingUnknownKey(t *testing.T) {
	outs := ResourcesOutputs{"my-k3s": {Ref: &fakeSibling{}}}

	_, err := ResolveResourceRef(ref("my-k3s", "ca-cert"), outs)
	if err == nil {
		t.Fatal("an unpublished key was not refused")
	}
	for _, want := range []string{"ca-cert", "kubeconfig", "endpoint"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q: %v", want, err)
		}
	}
}

// Plugin-owned resources are most of the fleet post-extraction, so a managed
// cluster from a plugin has to be referenceable with no extra plumbing.
func TestResolveResourceRef_PluginOutput(t *testing.T) {
	outs := ResourcesOutputs{
		"managed-k8s": {Ref: &api.PluginResourceOutput{
			PluginName: "gcp",
			Urn:        "urn:pulumi:test::p::gcp:container/cluster:Cluster::c",
			Outputs: map[string]api.PluginOutputValue{
				"kubeconfig": {Value: "apiVersion: v1\n", Secret: true},
				"endpoint":   {Value: "34.1.2.3"},
				"nodeCount":  {Value: 3},
			},
		}},
	}

	secret, err := ResolveResourceRef(ref("managed-k8s", "kubeconfig"), outs)
	if err != nil {
		t.Fatalf("ResolveResourceRef: %v", err)
	}
	res := await(t, secret)
	if res.Value != "apiVersion: v1\n" {
		t.Errorf("value = %q", res.Value)
	}
	// The plugin's own `secret` flag is the only thing that knows this; dropping
	// it here is how a credential reaches a log in cleartext.
	if !res.Secret {
		t.Error("a plugin value flagged secret came back unmarked")
	}

	plain, err := ResolveResourceRef(ref("managed-k8s", "endpoint"), outs)
	if err != nil {
		t.Fatalf("ResolveResourceRef(endpoint): %v", err)
	}
	res = await(t, plain)
	if res.Value != "34.1.2.3" {
		t.Errorf("endpoint = %q, want 34.1.2.3", res.Value)
	}
	if res.Secret {
		t.Error("a value the plugin did not flag was marked secret")
	}
}

// Two distinct plugin misses: a key that was never published, and one published
// as something other than a string. Both have to name the reference, because the
// operator is looking for it in their server.yaml.
func TestResolveResourceRef_PluginOutputMisses(t *testing.T) {
	outs := ResourcesOutputs{
		"managed-k8s": {Ref: &api.PluginResourceOutput{
			PluginName: "gcp",
			Outputs: map[string]api.PluginOutputValue{
				"endpoint":  {Value: "34.1.2.3"},
				"nodeCount": {Value: 3},
			},
		}},
	}

	_, err := ResolveResourceRef(ref("managed-k8s", "kubeconfig"), outs)
	if err == nil {
		t.Fatal("an unpublished plugin key was not refused")
	}
	if !strings.Contains(err.Error(), "${resource:managed-k8s.kubeconfig}") {
		t.Errorf("error does not quote the reference back: %v", err)
	}
	if !strings.Contains(err.Error(), "endpoint") {
		t.Errorf("error does not list the published keys: %v", err)
	}

	_, err = ResolveResourceRef(ref("managed-k8s", "nodeCount"), outs)
	if err == nil {
		t.Fatal("a non-string plugin value was not refused")
	}
	if !strings.Contains(err.Error(), "${resource:managed-k8s.nodeCount}") {
		t.Errorf("error does not quote the reference back: %v", err)
	}
}

// The likeliest real-world failure: the reference is correct but names a resource
// declared under a DIFFERENT environment's block of the same server.yaml, which
// reads as adjacent in the file and is invisible from here. The list of what WAS
// provisioned is the only clue the operator gets.
func TestResolveResourceRef_MissingResourceListsWhatWasAvailable(t *testing.T) {
	outs := ResourcesOutputs{
		"smoke-vm":     {Ref: &fakeSibling{}},
		"smoke-bucket": {Ref: &api.PluginResourceOutput{PluginName: "yandex"}},
	}

	_, err := ResolveResourceRef(ref("smoke-k3s", "kubeconfig"), outs)
	if err == nil {
		t.Fatal("a reference to an absent resource was not refused")
	}
	for _, want := range []string{`"smoke-k3s"`, `"smoke-vm"`, `"smoke-bucket"`, "SAME parent stack and environment"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s: %v", want, err)
		}
	}

	// And with nothing provisioned yet, the hint says so rather than reading as
	// an empty list of candidates.
	_, err = ResolveResourceRef(ref("smoke-k3s", "kubeconfig"), ResourcesOutputs{})
	if err == nil {
		t.Fatal("a reference against no outputs at all was not refused")
	}
	if !strings.Contains(err.Error(), "no sibling resource had been provisioned") {
		t.Errorf("empty-outs error is not specific: %v", err)
	}
}

// A nil entry is reachable: configureResource can return a nil output alongside a
// nil error for a resource type that produces nothing.
func TestResolveResourceRef_NilOutput(t *testing.T) {
	_, err := ResolveResourceRef(ref("smoke-k3s", "kubeconfig"), ResourcesOutputs{"smoke-k3s": nil})
	if err == nil {
		t.Fatal("a nil output was not refused")
	}
}

// Most in-tree resource outputs publish nothing a sibling can read, and a
// reference to one is a config error — never a silent empty string, which would
// be handed to a Kubernetes provider as the kubeconfig.
func TestResolveResourceRef_ProducerPublishesNothing(t *testing.T) {
	type bareOutput struct{ Name string }

	_, err := ResolveResourceRef(ref("smoke-bucket", "kubeconfig"),
		ResourcesOutputs{"smoke-bucket": {Ref: &bareOutput{Name: "b"}}})
	if err == nil {
		t.Fatal("a producer with no sibling outputs was not refused")
	}
	if !strings.Contains(err.Error(), "bareOutput") {
		t.Errorf("error does not name the output type it found: %v", err)
	}
}
