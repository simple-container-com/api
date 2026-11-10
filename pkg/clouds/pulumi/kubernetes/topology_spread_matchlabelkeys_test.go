// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"testing"

	"github.com/samber/lo"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"

	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// A spread constraint must count only the INCOMING pod's own revision.
//
// Live-caught on the 3-VPS fleet stand (2026-11-09): three Caddy replicas on
// three nodes, maxSkew 1, DoNotSchedule, correct hostname topologyKey, no
// taints — and a settled spread of 2/1/0 after a rollout, leaving one node with
// no edge while still holding a live A record. The constraint was right; the
// selector counted both revisions' pods, so a legal placement at scheduling
// time became skewed once the old pods terminated, and nothing rebalances after
// the fact.
func TestNormalizeTopologySpreadConstraints_DefaultsToPerRevisionCounting(t *testing.T) {
	appLabels := map[string]string{"app": "caddy"}

	out, err := normalizeTopologySpreadConstraints([]k8s.TopologySpreadConstraint{
		{TopologyKey: "kubernetes.io/hostname"},
	}, appLabels)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 constraint, got %d", len(out))
	}
	if got := out[0].MatchLabelKeys; len(got) != 1 || got[0] != "pod-template-hash" {
		t.Fatalf("matchLabelKeys = %v, want [pod-template-hash] — without it a rollout can settle skewed", got)
	}
	// The existing defaults must be untouched by this change.
	if lo.FromPtr(out[0].MaxSkew) != 1 || out[0].WhenUnsatisfiable != "DoNotSchedule" {
		t.Errorf("existing defaults changed: maxSkew=%v whenUnsatisfiable=%q", out[0].MaxSkew, out[0].WhenUnsatisfiable)
	}
	if out[0].LabelSelector == nil || out[0].LabelSelector.MatchLabels["app"] != "caddy" {
		t.Errorf("app labels no longer default into the selector: %+v", out[0].LabelSelector)
	}
}

// An operator's own value wins, and an explicitly EMPTY slice is an opt-out
// back to Kubernetes' own behaviour. nil means "unset"; those are different.
func TestNormalizeTopologySpreadConstraints_RespectsOperatorIntent(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   []string
		want []string
	}{
		{"unset gets the default", nil, []string{"pod-template-hash"}},
		{"explicit value wins", []string{"my-revision"}, []string{"my-revision"}},
		{"empty slice is an opt-out", []string{}, []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out, err := normalizeTopologySpreadConstraints([]k8s.TopologySpreadConstraint{
				{TopologyKey: "kubernetes.io/hostname", MatchLabelKeys: tt.in},
			}, map[string]string{"app": "x"})
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			got := out[0].MatchLabelKeys
			if len(got) != len(tt.want) {
				t.Fatalf("matchLabelKeys = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("matchLabelKeys = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// The opt-out must not reach the API server as an empty array: the API server
// rejects matchLabelKeys without a labelSelector, and an empty array is not the
// same as absent.
func TestConvertTopologySpreadConstraints_OmitsEmptyMatchLabelKeys(t *testing.T) {
	out := convertTopologySpreadConstraints([]k8s.TopologySpreadConstraint{
		{TopologyKey: "kubernetes.io/hostname", MaxSkew: lo.ToPtr(1), WhenUnsatisfiable: "DoNotSchedule", MatchLabelKeys: []string{}},
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 constraint, got %d", len(out))
	}
	// The array element is an Input interface, so read the concrete args back.
	args, ok := out[0].(corev1.TopologySpreadConstraintArgs)
	if !ok {
		t.Fatalf("unexpected element type %T", out[0])
	}
	if args.MatchLabelKeys != nil {
		t.Fatalf("an empty opt-out must not be sent as an array, got %v", args.MatchLabelKeys)
	}
}

// And the default DOES reach the API server, which is the whole point.
func TestConvertTopologySpreadConstraints_SendsTheDefault(t *testing.T) {
	normalized, err := normalizeTopologySpreadConstraints([]k8s.TopologySpreadConstraint{
		{TopologyKey: "kubernetes.io/hostname"},
	}, map[string]string{"app": "caddy"})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	out := convertTopologySpreadConstraints(normalized)
	args, ok := out[0].(corev1.TopologySpreadConstraintArgs)
	if !ok {
		t.Fatalf("unexpected element type %T", out[0])
	}
	if args.MatchLabelKeys == nil {
		t.Fatal("the per-revision default did not reach the SDK args")
	}
}
