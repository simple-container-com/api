// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcloud

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/compose-spec/compose-go/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

const (
	spotKey         = "cloud.google.com/gke-spot"
	computeClassKey = "cloud.google.com/compute-class"
)

func convertWithTemplate(t *testing.T, tpl *GkeAutopilotTemplate, cloudExtras map[string]any) *GkeAutopilotInput {
	t.Helper()
	stackCfg := &api.StackConfigCompose{Runs: []string{}}
	if cloudExtras != nil {
		extras := any(cloudExtras)
		stackCfg.CloudExtras = &extras
	}
	res, err := ToGkeAutopilotConfig(tpl, compose.Config{Project: &types.Project{}}, stackCfg)
	require.NoError(t, err)
	input, ok := res.(*GkeAutopilotInput)
	require.True(t, ok)
	return input
}

func TestTemplateNodeSelectorMergedIntoDeployment(t *testing.T) {
	tests := []struct {
		name        string
		template    map[string]string
		cloudExtras map[string]any
		want        map[string]string
	}{
		{
			name:        "no template default keeps client selector",
			cloudExtras: map[string]any{"nodeSelector": map[string]any{"team": "payments"}},
			want:        map[string]string{"team": "payments"},
		},
		{
			name:     "template default applies without cloudExtras",
			template: map[string]string{spotKey: "true"},
			want:     map[string]string{spotKey: "true"},
		},
		{
			name:        "template default applies when cloudExtras has no nodeSelector",
			template:    map[string]string{spotKey: "true"},
			cloudExtras: map[string]any{"vpa": map[string]any{"enabled": true}},
			want:        map[string]string{spotKey: "true"},
		},
		{
			name:        "template default merges with client keys",
			template:    map[string]string{spotKey: "true"},
			cloudExtras: map[string]any{"nodeSelector": map[string]any{"team": "payments"}},
			want:        map[string]string{spotKey: "true", "team": "payments"},
		},
		{
			name:        "client value overrides template value",
			template:    map[string]string{"tier": "batch"},
			cloudExtras: map[string]any{"nodeSelector": map[string]any{"tier": "web"}},
			want:        map[string]string{"tier": "web"},
		},
		{
			name:        "empty client value drops template key",
			template:    map[string]string{spotKey: "true", "tier": "batch"},
			cloudExtras: map[string]any{"nodeSelector": map[string]any{spotKey: ""}},
			want:        map[string]string{"tier": "batch"},
		},
		{
			name:        "null client value drops template key",
			template:    map[string]string{spotKey: "true", "tier": "batch"},
			cloudExtras: map[string]any{"nodeSelector": map[string]any{spotKey: nil}},
			want:        map[string]string{"tier": "batch"},
		},
		{
			name:        "dropping the only template key leaves no selector",
			template:    map[string]string{spotKey: "true"},
			cloudExtras: map[string]any{"nodeSelector": map[string]any{spotKey: ""}},
			want:        nil,
		},
		{
			name:        "empty client value never reaches the pod spec without a template",
			cloudExtras: map[string]any{"nodeSelector": map[string]any{spotKey: "", "team": "payments"}},
			want:        map[string]string{"team": "payments"},
		},
		{
			name:        "empty client value for a key the template lacks is dropped",
			template:    map[string]string{"tier": "batch"},
			cloudExtras: map[string]any{"nodeSelector": map[string]any{spotKey: ""}},
			want:        map[string]string{"tier": "batch"},
		},
		{
			name:     "empty template value is ignored",
			template: map[string]string{spotKey: ""},
			want:     nil,
		},
		{
			name:        "affinity compute class merges with template default",
			template:    map[string]string{spotKey: "true"},
			cloudExtras: map[string]any{"affinity": map[string]any{"computeClass": "Balanced"}},
			want:        map[string]string{spotKey: "true", computeClassKey: "Balanced"},
		},
		{
			name:        "affinity compute class overrides template compute class",
			template:    map[string]string{computeClassKey: "Scale-Out"},
			cloudExtras: map[string]any{"affinity": map[string]any{"computeClass": "Balanced"}},
			want:        map[string]string{computeClassKey: "Balanced"},
		},
		{
			name:     "affinity compute class wins over an empty client value",
			template: map[string]string{computeClassKey: "Scale-Out"},
			cloudExtras: map[string]any{
				"nodeSelector": map[string]any{computeClassKey: ""},
				"affinity":     map[string]any{"computeClass": "Balanced"},
			},
			want: map[string]string{computeClassKey: "Balanced"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := convertWithTemplate(t, &GkeAutopilotTemplate{NodeSelector: tt.template}, tt.cloudExtras)
			assert.Equal(t, tt.want, input.Deployment.NodeSelector)
		})
	}
}

func TestTemplateNodeSelectorWithAffinityNodePool(t *testing.T) {
	tpl := &GkeAutopilotTemplate{NodeSelector: map[string]string{spotKey: "true", "workload-group": "a"}}
	input := convertWithTemplate(t, tpl, map[string]any{"affinity": map[string]any{"nodePool": "b"}})

	assert.Equal(t, map[string]string{spotKey: "true", "workload-group": "b"}, input.Deployment.NodeSelector)
	assert.Equal(t, []k8s.Toleration{{Key: "workload-group", Operator: "Equal", Value: "b", Effect: "NoSchedule"}}, input.Deployment.Tolerations)
}

func TestTemplateNodeSelectorNotAliased(t *testing.T) {
	tpl := &GkeAutopilotTemplate{NodeSelector: map[string]string{spotKey: "true"}}

	convertWithTemplate(t, tpl, map[string]any{"nodeSelector": map[string]any{spotKey: "", "team": "a"}})
	assert.Equal(t, map[string]string{spotKey: "true"}, tpl.NodeSelector)

	input := convertWithTemplate(t, tpl, nil)
	input.Deployment.NodeSelector["extra"] = "x"
	assert.Equal(t, map[string]string{spotKey: "true"}, tpl.NodeSelector)
	assert.Nil(t, input.GkeAutopilotTemplate.NodeSelector)
}

func TestTemplateNodeSelectorSurvivesParentStackOutput(t *testing.T) {
	read, err := ReadGkeAutopilotTemplateConfig(&api.Config{Config: map[string]any{
		"gkeClusterResource": "cluster",
		"nodeSelector":       map[string]any{spotKey: true},
	}})
	require.NoError(t, err)

	exported, err := yaml.Marshal(api.StackDescriptor{Type: TemplateTypeGkeAutopilot, Config: read})
	require.NoError(t, err)

	var imported api.StackDescriptor
	require.NoError(t, yaml.Unmarshal(exported, &imported))
	reread, err := ReadGkeAutopilotTemplateConfig(&imported.Config)
	require.NoError(t, err)

	tpl, ok := reread.Config.(*GkeAutopilotTemplate)
	require.True(t, ok)
	assert.Equal(t, map[string]string{spotKey: "true"}, tpl.NodeSelector)
	assert.Equal(t, map[string]string{spotKey: "true"}, convertWithTemplate(t, tpl, nil).Deployment.NodeSelector)
}
