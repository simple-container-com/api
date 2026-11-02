// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"encoding/json"
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

// TestPluginPreparedInput_MarshalJSONUnwraps locks in the wire-shape
// invariant that panicked forge-aigateway on isolated-stack run
// 41483738313 (2026-11-02): SC core forwards the deploying stack's
// Config.Config across the Provision bus via
// pkg/clouds/pulumi/plugin_sidecars.go's serializeConfigToInputs,
// which calls json.Marshal. The plugin's Provision handler decodes
// the InputValue tree into its typed Input shape. Without
// MarshalJSON unwrap, the plugin saw the wrapper shape (input: {},
// baseDnsZone_: ..., …) and every expected Input field was nil,
// panic'ing on first dereference.
//
// This test asserts the json.Marshal output is identical to a direct
// marshal of the typed Input — no wrapper envelope.
func TestPluginPreparedInput_MarshalJSONUnwraps(t *testing.T) {
	typedInput := map[string]any{
		"handler":     "bootstrap",
		"memory":      1024,
		"baseDnsZone": "example.com",
		"environment": map[string]any{"LOG_LEVEL": "info"},
	}
	prepared := NewPluginPreparedInput(&PluginStackConvertResult{
		Input:              typedInput,
		BaseDnsZone:        "example.com",
		Uses:               []string{"mongodb"},
		DependsOnResources: []StackConfigDependencyResource{{Name: "parent"}},
	})

	got, err := json.Marshal(prepared)
	require.NoError(t, err)

	want, err := json.Marshal(typedInput)
	require.NoError(t, err)

	require.JSONEq(t, string(want), string(got),
		"MarshalJSON must unwrap to the raw Input shape (no wrapper envelope) — the plugin's Provision handler decodes directly into its typed Input (*LambdaInput, *StaticSiteInput, …) with the fields at the top level, not nested under `input:`")

	// A nil / empty wrapper marshals to JSON null so the plugin can
	// treat the absence correctly rather than seeing an empty object.
	empty := NewPluginPreparedInput(nil)
	emptyJSON, err := json.Marshal(empty)
	require.NoError(t, err)
	require.Equal(t, "null", string(emptyJSON))
}
