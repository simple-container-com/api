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

func gkeInput(t *testing.T, cloudExtras map[string]any) (*GkeAutopilotInput, error) {
	t.Helper()
	extras := any(cloudExtras)
	stackCfg := &api.StackConfigCompose{Runs: []string{}, CloudExtras: &extras}
	res, err := ToGkeAutopilotConfig(&GkeAutopilotTemplate{}, compose.Config{Project: &types.Project{}}, stackCfg)
	if err != nil {
		return nil, err
	}
	input, ok := res.(*GkeAutopilotInput)
	require.True(t, ok)
	return input, nil
}

func TestSecurityContextThreadedByToGkeAutopilotConfig(t *testing.T) {
	input, err := gkeInput(t, map[string]any{
		"securityContext": map[string]any{
			"fsGroup":             10001,
			"fsGroupChangePolicy": "OnRootMismatch",
			"runAsNonRoot":        true,
		},
	})
	require.NoError(t, err)
	sc := input.Deployment.SecurityContext
	require.NotNil(t, sc)
	assert.Equal(t, 10001, *sc.FSGroup)
	assert.Equal(t, "OnRootMismatch", *sc.FSGroupChangePolicy)
	assert.True(t, *sc.RunAsNonRoot)
	assert.Nil(t, sc.RunAsUser)
}

func TestSecurityContextAbsentStaysNil(t *testing.T) {
	input, err := gkeInput(t, map[string]any{"priorityClassName": "p"})
	require.NoError(t, err)
	assert.Nil(t, input.Deployment.SecurityContext)
}

func TestSecurityContextInvalidRejectedByToGkeAutopilotConfig(t *testing.T) {
	_, err := gkeInput(t, map[string]any{
		"securityContext": map[string]any{"fsGroupChangePolicy": "Sometimes"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cloudExtras.securityContext")
}
