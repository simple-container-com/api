// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k8s

import (
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

func TestPodSecurityContextValidate(t *testing.T) {
	cases := []struct {
		name    string
		sc      *PodSecurityContext
		wantErr string
	}{
		{name: "nil is valid", sc: nil},
		{name: "empty is valid", sc: &PodSecurityContext{}},
		{name: "fsGroup with OnRootMismatch", sc: &PodSecurityContext{FSGroup: lo.ToPtr(10001), FSGroupChangePolicy: lo.ToPtr("OnRootMismatch")}},
		{name: "Always policy", sc: &PodSecurityContext{FSGroupChangePolicy: lo.ToPtr("Always")}},
		{name: "root user without runAsNonRoot", sc: &PodSecurityContext{RunAsUser: lo.ToPtr(0)}},
		{name: "unknown policy", sc: &PodSecurityContext{FSGroupChangePolicy: lo.ToPtr("Sometimes")}, wantErr: "fsGroupChangePolicy"},
		{name: "negative fsGroup", sc: &PodSecurityContext{FSGroup: lo.ToPtr(-1)}, wantErr: "fsGroup must not be negative"},
		{name: "negative runAsUser", sc: &PodSecurityContext{RunAsUser: lo.ToPtr(-5)}, wantErr: "runAsUser must not be negative"},
		{name: "negative supplemental group", sc: &PodSecurityContext{SupplementalGroups: []int{1000, -1}}, wantErr: "supplementalGroups"},
		{name: "runAsNonRoot with uid 0", sc: &PodSecurityContext{RunAsNonRoot: lo.ToPtr(true), RunAsUser: lo.ToPtr(0)}, wantErr: "runAsNonRoot"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.sc.Validate()
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestSecurityContextThreadedByToKubernetesRunConfig(t *testing.T) {
	extras := any(map[string]any{
		"securityContext": map[string]any{"fsGroup": 2000, "supplementalGroups": []int{3000}},
	})
	stackCfg := &api.StackConfigCompose{Runs: []string{}, CloudExtras: &extras}
	res, err := ToKubernetesRunConfig(&CloudrunTemplate{}, compose.Config{Project: &types.Project{}}, stackCfg)
	require.NoError(t, err)
	input, ok := res.(*KubeRunInput)
	require.True(t, ok)
	require.NotNil(t, input.Deployment.SecurityContext)
	assert.Equal(t, 2000, *input.Deployment.SecurityContext.FSGroup)
	assert.Equal(t, []int{3000}, input.Deployment.SecurityContext.SupplementalGroups)
}

func TestSecurityContextInvalidRejectedByToKubernetesRunConfig(t *testing.T) {
	extras := any(map[string]any{"securityContext": map[string]any{"fsGroup": -1}})
	stackCfg := &api.StackConfigCompose{Runs: []string{}, CloudExtras: &extras}
	_, err := ToKubernetesRunConfig(&CloudrunTemplate{}, compose.Config{Project: &types.Project{}}, stackCfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cloudExtras.securityContext")
}
