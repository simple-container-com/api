// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package gcloud

import (
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

func TestTopologySpreadThreadedByToGkeAutopilotConfig(t *testing.T) {
	cloudExtras := any(map[string]any{
		"topologySpreadConstraints": []map[string]any{
			{"topologyKey": "kubernetes.io/hostname", "minDomains": 2},
		},
	})
	stackCfg := &api.StackConfigCompose{
		Runs:        []string{},
		CloudExtras: &cloudExtras,
	}

	res, err := ToGkeAutopilotConfig(&GkeAutopilotTemplate{}, compose.Config{Project: &types.Project{}}, stackCfg)
	require.NoError(t, err)
	input, ok := res.(*GkeAutopilotInput)
	require.True(t, ok)
	require.Len(t, input.Deployment.TopologySpreadConstraints, 1)
	assert.Equal(t, "kubernetes.io/hostname", input.Deployment.TopologySpreadConstraints[0].TopologyKey)
}
