// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"strings"
	"testing"

	"github.com/simple-container-com/api/pkg/api"
)

// fakeDeps is a resource config that declares parent dependencies, standing in
// for k8s.KubernetesConfig (whose dependency comes from a kubeconfig reference)
// and k3s.K3sClusterConfig (whose comes from its vm resource).
type fakeDeps struct {
	deps []string
}

func (f fakeDeps) DependsOnResources() []api.ParentResourceDependency {
	out := make([]api.ParentResourceDependency, 0, len(f.deps))
	for _, d := range f.deps {
		out = append(out, api.ParentResourceDependency{Name: d})
	}
	return out
}

// fakePlain declares no dependencies at all — it does not even satisfy
// api.WithParentDependencies, which is the shape of most resource types.
type fakePlain struct{}

func resources(spec map[string][]string) map[string]api.ResourceDescriptor {
	out := make(map[string]api.ResourceDescriptor, len(spec))
	for name, deps := range spec {
		var cfg any = fakePlain{}
		if deps != nil {
			cfg = fakeDeps{deps: deps}
		}
		out[name] = api.ResourceDescriptor{Config: api.Config{Config: cfg}}
	}
	return out
}

func indexOf(order []string, name string) int {
	for i, n := range order {
		if n == name {
			return i
		}
	}
	return -1
}

// The chain this whole slice exists for: a vm resource, a k3s cluster on it, and
// a Caddy edge whose kubeconfig references the cluster. The old two-bucket split
// put the cluster and the edge in the same bucket, in map order, with the
// cluster's output thrown away.
func TestResourceProvisionOrder_TransitiveChain(t *testing.T) {
	order, err := resourceProvisionOrder(resources(map[string][]string{
		"smoke-vm":    nil,
		"smoke-k3s":   {"smoke-vm"},
		"smoke-caddy": {"smoke-k3s"},
	}))
	if err != nil {
		t.Fatalf("resourceProvisionOrder: %v", err)
	}
	want := []string{"smoke-vm", "smoke-k3s", "smoke-caddy"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

// Determinism is not cosmetic. Pulumi resource names are derived from
// provisioning order in places, so a map-order pass makes an unchanged
// server.yaml produce a diff that appears and disappears between runs. Repeated
// because Go randomises map iteration per range, so a single call proves nothing.
func TestResourceProvisionOrder_IsDeterministic(t *testing.T) {
	spec := map[string][]string{
		"zeta":  {"alpha"},
		"alpha": nil,
		"beta":  nil,
		"gamma": {"beta"},
		"delta": nil,
	}
	first, err := resourceProvisionOrder(resources(spec))
	if err != nil {
		t.Fatalf("resourceProvisionOrder: %v", err)
	}
	for i := 0; i < 50; i++ {
		got, err := resourceProvisionOrder(resources(spec))
		if err != nil {
			t.Fatalf("resourceProvisionOrder: %v", err)
		}
		if strings.Join(got, ",") != strings.Join(first, ",") {
			t.Fatalf("order is not deterministic: %v then %v", first, got)
		}
	}
	// And it still respects the edges, not just itself.
	if indexOf(first, "alpha") > indexOf(first, "zeta") {
		t.Errorf("alpha must precede zeta: %v", first)
	}
	if indexOf(first, "beta") > indexOf(first, "gamma") {
		t.Errorf("beta must precede gamma: %v", first)
	}
}

// A dependency on a resource that is not in this environment is NOT an error: it
// may name another environment's block of the same server.yaml, or be satisfied
// through a parent stack. The resource that needs the value reports the miss
// itself, with the list of what was available.
func TestResourceProvisionOrder_UnknownDependencyIsNotAnError(t *testing.T) {
	order, err := resourceProvisionOrder(resources(map[string][]string{
		"caddy": {"a-cluster-in-another-env"},
	}))
	if err != nil {
		t.Fatalf("an unresolvable dependency name must not fail the ordering: %v", err)
	}
	if len(order) != 1 || order[0] != "caddy" {
		t.Errorf("order = %v, want [caddy]", order)
	}
}

// Every resource must appear exactly once, including ones nothing points at.
func TestResourceProvisionOrder_PlacesEveryResourceOnce(t *testing.T) {
	order, err := resourceProvisionOrder(resources(map[string][]string{
		"vm":        nil,
		"k3s":       {"vm"},
		"caddy":     {"k3s"},
		"unrelated": nil,
		"bucket":    {},
	}))
	if err != nil {
		t.Fatalf("resourceProvisionOrder: %v", err)
	}
	seen := make(map[string]int)
	for _, n := range order {
		seen[n]++
	}
	for _, n := range []string{"vm", "k3s", "caddy", "unrelated", "bucket"} {
		if seen[n] != 1 {
			t.Errorf("resource %q appears %d times in %v, want once", n, seen[n], order)
		}
	}
}

// A self-reference is a config quirk, not a cycle — refusing it would turn
// something harmless into a failed provision.
func TestResourceProvisionOrder_SelfDependencyIsIgnored(t *testing.T) {
	order, err := resourceProvisionOrder(resources(map[string][]string{
		"caddy": {"caddy", ""},
	}))
	if err != nil {
		t.Fatalf("a self-dependency must not fail the ordering: %v", err)
	}
	if len(order) != 1 {
		t.Errorf("order = %v, want one entry", order)
	}
}

// A real cycle has to be refused, and the message has to name the members —
// the operator's job is to break it, and they cannot do that from "circular
// dependency detected".
func TestResourceProvisionOrder_RefusesCycle(t *testing.T) {
	_, err := resourceProvisionOrder(resources(map[string][]string{
		"independent": nil,
		"a":           {"b"},
		"b":           {"c"},
		"c":           {"a"},
	}))
	if err == nil {
		t.Fatal("a dependency cycle was not refused")
	}
	for _, name := range []string{`"a"`, `"b"`, `"c"`} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("cycle error does not name %s: %v", name, err)
		}
	}
	if strings.Contains(err.Error(), `"independent"`) {
		t.Errorf("cycle error blames a resource outside the cycle: %v", err)
	}
}

func TestResourceProvisionOrder_Empty(t *testing.T) {
	order, err := resourceProvisionOrder(nil)
	if err != nil {
		t.Fatalf("resourceProvisionOrder(nil): %v", err)
	}
	if len(order) != 0 {
		t.Errorf("order = %v, want empty", order)
	}
}

// TestResourceProvisionOrder_ComponentResourceRef is the end-to-end assertion
// for the ordering half of component→component references.
//
// Before *api.PluginFieldConfig implemented WithParentDependencies, no
// plugin-owned resource could report a dependency, so two of them were ordered
// ALPHABETICALLY. These are the names from the sc-component-notifier design doc
// and they sort the wrong way round: "my-backup" < "my-notifier", so the
// consumer provisioned first, the notifier's `self-url` template extension was
// not on the collector yet, and ResolvePlaceholders — which fails OPEN — handed
// the dispatched template the literal string `${resource:my-notifier.self-url}`.
//
// Renaming the notifier "alerts" would have made the same config work, which is
// what makes this a landmine rather than a constraint. The assertion is
// therefore positional, not just "no error".
func TestResourceProvisionOrder_ComponentResourceRef(t *testing.T) {
	component := func(raw map[string]any) api.ResourceDescriptor {
		return api.ResourceDescriptor{Config: api.Config{
			Config: &api.PluginFieldConfig{TypeName: "sc-component", Raw: raw},
		}}
	}

	order, err := resourceProvisionOrder(map[string]api.ResourceDescriptor{
		"my-notifier": component(map[string]any{"deployTo": "lambda"}),
		"my-backup": component(map[string]any{
			"notify": map[string]any{"url": "${resource:my-notifier.self-url}"},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := indexOf(order, "my-notifier"), indexOf(order, "my-backup"); got > want {
		t.Fatalf("referenced resource must be provisioned FIRST, got order %v", order)
	}
}

// A reference pointing at a resource that is not in this env is ignored rather
// than treated as an unsatisfiable dependency — the resource still provisions.
// resourceProvisionOrder already drops out-of-env names; this pins it against a
// future change to DependsOnResources that assumed every name resolves.
func TestResourceProvisionOrder_ComponentResourceRefOutOfEnv(t *testing.T) {
	order, err := resourceProvisionOrder(map[string]api.ResourceDescriptor{
		"my-backup": {Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{
			"notify": map[string]any{"url": "${resource:not-in-this-env.self-url}"},
		}}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 1 || order[0] != "my-backup" {
		t.Fatalf("expected the resource to provision anyway, got %v", order)
	}
}

// Two components referencing each other are now REFUSED, where before they both
// silently fail-opened to literals. The refusal is the intended trade, so the
// message has to name both resources and say how to break the cycle.
func TestResourceProvisionOrder_ComponentResourceRefCycle(t *testing.T) {
	_, err := resourceProvisionOrder(map[string]api.ResourceDescriptor{
		"my-backup": {Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{
			"notify": map[string]any{"url": "${resource:my-notifier.self-url}"},
		}}}},
		"my-notifier": {Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{
			"probe": map[string]any{"url": "${resource:my-backup.self-url}"},
		}}}},
	})
	if err == nil {
		t.Fatal("expected a circular-dependency refusal")
	}
	for _, want := range []string{"my-backup", "my-notifier", "circular", "${resource:"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal must mention %q, got: %v", want, err)
		}
	}
}

// A self-reference is not a cycle. DependsOnResources reports it (it cannot know
// its own name) and resourceProvisionOrder drops it — if that drop were removed,
// every component that referenced its own output would become unprovisionable.
func TestResourceProvisionOrder_ComponentSelfRef(t *testing.T) {
	order, err := resourceProvisionOrder(map[string]api.ResourceDescriptor{
		"my-backup": {Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{
			"url": "${resource:my-backup.self-url}",
		}}}},
	})
	if err != nil {
		t.Fatalf("a self-reference must not be a cycle: %v", err)
	}
	if len(order) != 1 || order[0] != "my-backup" {
		t.Fatalf("expected the resource to provision, got %v", order)
	}
}
