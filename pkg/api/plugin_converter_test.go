// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPluginPreparedInput_ReflectWalkerCanSetEveryField locks in the
// invariant that panicked forge-aigateway on isolated-stack run
// 41469779418 (2026-11-02): pkg/provisioner/placeholders'
// applyTemplatesRecursive walks the Config.Config struct via
// reflection and calls reflect.Value.Set on every field. An
// unexported field triggers "reflect.Value.Set using value obtained
// using unexported field" and the deploy panics before any
// resource is provisioned.
//
// If any field on PluginPreparedInput reverts to unexported, this
// test fails on the exact same reflect.Set path the placeholder
// walker uses.
func TestPluginPreparedInput_ReflectWalkerCanSetEveryField(t *testing.T) {
	sample := NewPluginPreparedInput(&PluginStackConvertResult{
		Input:              map[string]any{"stackName": "demo"},
		BaseDnsZone:        "example.com",
		Uses:               []string{"mongodb"},
		DependsOnResources: []StackConfigDependencyResource{{Name: "parent"}},
	})

	// Mirror the walker: for each field on the struct, allocate a
	// fresh value and reflect.Value.Set it onto an addressable copy.
	// A single unexported field is enough to panic.
	original := reflect.ValueOf(sample).Elem()
	copyPtr := reflect.New(original.Type())
	copyValue := copyPtr.Elem()
	for i := 0; i < original.NumField(); i++ {
		require.True(t, copyValue.Field(i).CanSet(),
			"PluginPreparedInput field %q is unexported — placeholders.applyTemplatesRecursive would panic (regression of forge-aigateway isolated run 41469779418)",
			original.Type().Field(i).Name)
		copyValue.Field(i).Set(original.Field(i))
	}

	// Spot-check the three interface methods thunk to the exported
	// fields (not nil).
	require.Equal(t, "example.com", sample.OverriddenBaseZone())
	require.Equal(t, []string{"mongodb"}, sample.Uses())
	require.Len(t, sample.DependsOnResources(), 1)
	require.Equal(t, "parent", sample.DependsOnResources()[0].Name)

	// Nil receiver — the interface methods must not panic.
	var nilPrepared *PluginPreparedInput
	require.Equal(t, "", nilPrepared.OverriddenBaseZone())
	require.Nil(t, nilPrepared.Uses())
	require.Nil(t, nilPrepared.DependsOnResources())
}
