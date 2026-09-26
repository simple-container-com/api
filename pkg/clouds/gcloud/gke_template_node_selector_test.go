// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcloud

import (
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

func TestTemplateNodeSelectorMergedIntoDeployment(t *testing.T) {
	const spot = "cloud.google.com/gke-spot"

	tests := []struct {
		name        string
		template    map[string]string
		cloudExtras map[string]any
		want        map[string]string
	}{
		{
			name: "no template default keeps client selector",
			cloudExtras: map[string]any{
				"nodeSelector": map[string]any{"team": "payments"},
			},
			want: map[string]string{"team": "payments"},
		},
		{
			name:     "template default applies without cloudExtras",
			template: map[string]string{spot: "true"},
			want:     map[string]string{spot: "true"},
		},
		{
			name:     "template default merges with client keys",
			template: map[string]string{spot: "true"},
			cloudExtras: map[string]any{
				"nodeSelector": map[string]any{"team": "payments"},
			},
			want: map[string]string{spot: "true", "team": "payments"},
		},
		{
			name:     "client value overrides template value",
			template: map[string]string{"tier": "batch"},
			cloudExtras: map[string]any{
				"nodeSelector": map[string]any{"tier": "web"},
			},
			want: map[string]string{"tier": "web"},
		},
		{
			name:     "empty client value drops template key",
			template: map[string]string{spot: "true", "tier": "batch"},
			cloudExtras: map[string]any{
				"nodeSelector": map[string]any{spot: ""},
			},
			want: map[string]string{"tier": "batch"},
		},
		{
			name:     "dropping the only template key leaves no selector",
			template: map[string]string{spot: "true"},
			cloudExtras: map[string]any{
				"nodeSelector": map[string]any{spot: ""},
			},
			want: nil,
		},
		{
			name:     "empty client value for a non-template key is kept",
			template: map[string]string{spot: "true"},
			cloudExtras: map[string]any{
				"nodeSelector": map[string]any{"team": ""},
			},
			want: map[string]string{spot: "true", "team": ""},
		},
		{
			name:     "affinity compute class merges with template default",
			template: map[string]string{spot: "true"},
			cloudExtras: map[string]any{
				"affinity": map[string]any{"computeClass": "Balanced"},
			},
			want: map[string]string{spot: "true", "cloud.google.com/compute-class": "Balanced"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stackCfg := &api.StackConfigCompose{Runs: []string{}}
			if tt.cloudExtras != nil {
				extras := any(tt.cloudExtras)
				stackCfg.CloudExtras = &extras
			}
			tpl := &GkeAutopilotTemplate{NodeSelector: tt.template}

			res, err := ToGkeAutopilotConfig(tpl, compose.Config{Project: &types.Project{}}, stackCfg)
			require.NoError(t, err)
			input, ok := res.(*GkeAutopilotInput)
			require.True(t, ok)
			assert.Equal(t, tt.want, input.Deployment.NodeSelector)
		})
	}
}

func TestTemplateNodeSelectorNotMutated(t *testing.T) {
	tpl := &GkeAutopilotTemplate{NodeSelector: map[string]string{"cloud.google.com/gke-spot": "true"}}
	extras := any(map[string]any{"nodeSelector": map[string]any{"cloud.google.com/gke-spot": "", "team": "a"}})
	stackCfg := &api.StackConfigCompose{Runs: []string{}, CloudExtras: &extras}

	_, err := ToGkeAutopilotConfig(tpl, compose.Config{Project: &types.Project{}}, stackCfg)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"cloud.google.com/gke-spot": "true"}, tpl.NodeSelector)
}
