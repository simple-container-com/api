// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

func TestNormalizeTopologySpreadConstraintsEmpty(t *testing.T) {
	out, err := normalizeTopologySpreadConstraints(nil, map[string]string{"app": "x"})
	require.NoError(t, err)
	assert.Nil(t, out)
}

func TestNormalizeTopologySpreadConstraintsDefaults(t *testing.T) {
	appLabels := map[string]string{"app-name": "web", "sc-env": "prod"}
	out, err := normalizeTopologySpreadConstraints([]k8s.TopologySpreadConstraint{
		{TopologyKey: "kubernetes.io/hostname"},
	}, appLabels)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, 1, lo.FromPtr(out[0].MaxSkew))
	assert.Equal(t, "DoNotSchedule", out[0].WhenUnsatisfiable)
	require.NotNil(t, out[0].LabelSelector)
	assert.Equal(t, appLabels, out[0].LabelSelector.MatchLabels)
}

func TestNormalizeTopologySpreadConstraintsExplicitValuesPreserved(t *testing.T) {
	sel := &k8s.LabelSelector{MatchLabels: map[string]string{"tier": "web"}}
	out, err := normalizeTopologySpreadConstraints([]k8s.TopologySpreadConstraint{
		{TopologyKey: "topology.kubernetes.io/zone", MaxSkew: lo.ToPtr(3), WhenUnsatisfiable: "ScheduleAnyway", LabelSelector: sel},
	}, map[string]string{"app": "x"})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, 3, lo.FromPtr(out[0].MaxSkew))
	assert.Equal(t, "ScheduleAnyway", out[0].WhenUnsatisfiable)
	assert.Equal(t, map[string]string{"tier": "web"}, out[0].LabelSelector.MatchLabels)
}

func TestNormalizeTopologySpreadConstraintsMinDomains(t *testing.T) {
	out, err := normalizeTopologySpreadConstraints([]k8s.TopologySpreadConstraint{
		{TopologyKey: "kubernetes.io/hostname", MinDomains: lo.ToPtr(2)},
	}, map[string]string{"app": "x"})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, 2, lo.FromPtr(out[0].MinDomains))
}

func TestNormalizeTopologySpreadConstraintsErrors(t *testing.T) {
	cases := map[string]k8s.TopologySpreadConstraint{
		"empty topologyKey":             {TopologyKey: ""},
		"invalid whenUnsatisfiable":     {TopologyKey: "kubernetes.io/hostname", WhenUnsatisfiable: "Maybe"},
		"maxSkew zero":                  {TopologyKey: "kubernetes.io/hostname", MaxSkew: lo.ToPtr(0)},
		"maxSkew negative":              {TopologyKey: "kubernetes.io/hostname", MaxSkew: lo.ToPtr(-2)},
		"minDomains without DoNotSched": {TopologyKey: "kubernetes.io/hostname", WhenUnsatisfiable: "ScheduleAnyway", MinDomains: lo.ToPtr(2)},
		"minDomains zero":               {TopologyKey: "kubernetes.io/hostname", MinDomains: lo.ToPtr(0)},
		"minDomains negative":           {TopologyKey: "kubernetes.io/hostname", MinDomains: lo.ToPtr(-1)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := normalizeTopologySpreadConstraints([]k8s.TopologySpreadConstraint{c}, map[string]string{"app": "x"})
			assert.Error(t, err)
		})
	}
}

func TestNormalizeTopologySpreadConstraintsDoesNotMutateInput(t *testing.T) {
	in := []k8s.TopologySpreadConstraint{{TopologyKey: "kubernetes.io/hostname"}}
	_, err := normalizeTopologySpreadConstraints(in, map[string]string{"app": "x"})
	require.NoError(t, err)
	assert.Nil(t, in[0].MaxSkew)
	assert.Equal(t, "", in[0].WhenUnsatisfiable)
	assert.Nil(t, in[0].LabelSelector)
}

func TestNormalizeTopologySpreadConstraintsMultiple(t *testing.T) {
	out, err := normalizeTopologySpreadConstraints([]k8s.TopologySpreadConstraint{
		{TopologyKey: "kubernetes.io/hostname"},
		{TopologyKey: "topology.kubernetes.io/zone", WhenUnsatisfiable: "ScheduleAnyway"},
	}, map[string]string{"app": "x"})
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.Equal(t, "kubernetes.io/hostname", out[0].TopologyKey)
	assert.Equal(t, "topology.kubernetes.io/zone", out[1].TopologyKey)
}

func TestConvertTopologySpreadConstraints(t *testing.T) {
	assert.Nil(t, convertTopologySpreadConstraints(nil))

	normalized := []k8s.TopologySpreadConstraint{
		{TopologyKey: "kubernetes.io/hostname", MaxSkew: lo.ToPtr(1), WhenUnsatisfiable: "DoNotSchedule", MinDomains: lo.ToPtr(2), LabelSelector: &k8s.LabelSelector{MatchLabels: map[string]string{"app": "x"}}},
	}
	out := convertTopologySpreadConstraints(normalized)
	require.Len(t, out, 1)
}
