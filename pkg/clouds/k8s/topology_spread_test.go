package k8s

import (
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

func TestTopologySpreadConstraintsExtraction(t *testing.T) {
	cloudExtras := map[string]any{
		"topologySpreadConstraints": []map[string]any{
			{
				"maxSkew":           1,
				"topologyKey":       "kubernetes.io/hostname",
				"whenUnsatisfiable": "DoNotSchedule",
				"minDomains":        2,
				"labelSelector": map[string]any{
					"matchLabels": map[string]any{"tier": "web"},
				},
			},
		},
	}

	result, err := api.ConvertDescriptor(cloudExtras, &CloudExtras{})

	require.NoError(t, err)
	require.Len(t, result.TopologySpreadConstraints, 1)
	c := result.TopologySpreadConstraints[0]
	assert.Equal(t, "kubernetes.io/hostname", c.TopologyKey)
	assert.Equal(t, "DoNotSchedule", c.WhenUnsatisfiable)
	require.NotNil(t, c.MaxSkew)
	assert.Equal(t, 1, *c.MaxSkew)
	require.NotNil(t, c.MinDomains)
	assert.Equal(t, 2, *c.MinDomains)
	require.NotNil(t, c.LabelSelector)
	assert.Equal(t, map[string]string{"tier": "web"}, c.LabelSelector.MatchLabels)
}

func TestTopologySpreadConstraintsAbsent(t *testing.T) {
	result, err := api.ConvertDescriptor(map[string]any{}, &CloudExtras{})
	require.NoError(t, err)
	assert.Nil(t, result.TopologySpreadConstraints)
}

func TestTopologySpreadThreadedByToKubernetesRunConfig(t *testing.T) {
	cloudExtras := any(map[string]any{
		"topologySpreadConstraints": []map[string]any{
			{"topologyKey": "kubernetes.io/hostname"},
		},
	})
	stackCfg := &api.StackConfigCompose{
		Runs:        []string{},
		CloudExtras: &cloudExtras,
	}

	res, err := ToKubernetesRunConfig(&CloudrunTemplate{}, compose.Config{Project: &types.Project{}}, stackCfg)
	require.NoError(t, err)
	input, ok := res.(*KubeRunInput)
	require.True(t, ok)
	require.Len(t, input.Deployment.TopologySpreadConstraints, 1)
	assert.Equal(t, "kubernetes.io/hostname", input.Deployment.TopologySpreadConstraints[0].TopologyKey)
}
