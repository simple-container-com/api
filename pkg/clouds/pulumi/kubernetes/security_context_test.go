// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"testing"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// Stacks that do not set cloudExtras.securityContext must keep rendering a pod spec without one.
func TestToPodSecurityContextArgsNil(t *testing.T) {
	assert.Nil(t, toPodSecurityContextArgs(nil))
}

func TestToPodSecurityContextArgsMapsFields(t *testing.T) {
	res := toPodSecurityContextArgs(&k8s.PodSecurityContext{
		RunAsUser:           lo.ToPtr(10001),
		RunAsGroup:          lo.ToPtr(10001),
		RunAsNonRoot:        lo.ToPtr(true),
		FSGroup:             lo.ToPtr(10001),
		FSGroupChangePolicy: lo.ToPtr("OnRootMismatch"),
		SupplementalGroups:  []int{20, 30},
	})
	require.NotNil(t, res)
	assert.Equal(t, sdk.IntPtr(10001), res.RunAsUser)
	assert.Equal(t, sdk.IntPtr(10001), res.RunAsGroup)
	assert.Equal(t, sdk.BoolPtr(true), res.RunAsNonRoot)
	assert.Equal(t, sdk.IntPtr(10001), res.FsGroup)
	assert.Equal(t, sdk.StringPtr("OnRootMismatch"), res.FsGroupChangePolicy)
	assert.Equal(t, sdk.ToIntArray([]int{20, 30}), res.SupplementalGroups)
}

func TestToPodSecurityContextArgsLeavesUnsetFieldsUnset(t *testing.T) {
	res := toPodSecurityContextArgs(&k8s.PodSecurityContext{FSGroup: lo.ToPtr(10001)})
	require.NotNil(t, res)
	assert.Equal(t, sdk.IntPtr(10001), res.FsGroup)
	assert.Nil(t, res.SupplementalGroups)
	assert.Nil(t, res.FsGroupChangePolicy)
}
