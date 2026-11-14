// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"reflect"
	"testing"
)

// TestDeepMergeComponentRaw_NestedMapPerKeyUnion pins the core promise:
// plugin keys the operator did NOT write stay, operator keys overwrite.
// This is the shape sc-component-backup slice 1d.6 needs — a plugin
// default `cloudExtras: {lambdaRoutingType, awsRoles}` carries through
// when the operator doesn't write cloudExtras at all.
func TestDeepMergeComponentRaw_NestedMapPerKeyUnion(t *testing.T) {
	defaults := map[string]any{
		"cloudExtras": map[string]any{
			"lambdaRoutingType": "function-url",
			"awsRoles":          []any{"s3:PutObject", "iam:PassRole"},
		},
		"deployTo": "lambda",
	}
	operator := map[string]any{
		"deployTo":    "cloud-compose",
		"storage":     map[string]any{"type": "s3"},
		"cloudExtras": map[string]any{"memory": 512},
	}
	got := DeepMergeComponentRaw(defaults, operator)

	ce, ok := got["cloudExtras"].(map[string]any)
	if !ok {
		t.Fatalf("cloudExtras not a map: %T", got["cloudExtras"])
	}
	if ce["lambdaRoutingType"] != "function-url" {
		t.Errorf("plugin default lambdaRoutingType lost: got %v", ce["lambdaRoutingType"])
	}
	if ce["memory"] != 512 {
		t.Errorf("operator memory not applied: got %v", ce["memory"])
	}
	if got["deployTo"] != "cloud-compose" {
		t.Errorf("operator deployTo must win: got %v", got["deployTo"])
	}
	if _, ok := got["storage"]; !ok {
		t.Errorf("operator-only key storage dropped")
	}
}

// TestDeepMergeComponentRaw_OperatorListReplaces pins the list rule.
// When the operator writes `awsRoles: [custom]`, the result is
// `[custom]` not the appended `[s3:PutObject, iam:PassRole, custom]`.
// Rationale documented in SchemaResponse.component_defaults — operator
// should see the exact effective value.
func TestDeepMergeComponentRaw_OperatorListReplaces(t *testing.T) {
	defaults := map[string]any{
		"cloudExtras": map[string]any{
			"awsRoles": []any{"s3:PutObject", "iam:PassRole"},
		},
	}
	operator := map[string]any{
		"cloudExtras": map[string]any{
			"awsRoles": []any{"custom-role"},
		},
	}
	got := DeepMergeComponentRaw(defaults, operator)
	ce := got["cloudExtras"].(map[string]any)
	want := []any{"custom-role"}
	if !reflect.DeepEqual(ce["awsRoles"], want) {
		t.Errorf("list should REPLACE not append: got %v, want %v", ce["awsRoles"], want)
	}
}

// TestDeepMergeComponentRaw_NilDefaults_PassThrough — plugin that
// registered no defaults (pre-1d.6 plugin binaries). Merge must be a
// no-op; operator config crosses verbatim, matching today's behaviour
// before the slice landed.
func TestDeepMergeComponentRaw_NilDefaults_PassThrough(t *testing.T) {
	operator := map[string]any{"deployTo": "lambda", "signingKey": "x"}
	got := DeepMergeComponentRaw(nil, operator)
	if !reflect.DeepEqual(got, operator) {
		t.Errorf("nil-defaults: operator must pass through, got %v", got)
	}
	// Must be a copy, not an alias (mutation should not bleed back).
	got["deployTo"] = "mutated"
	if operator["deployTo"] != "lambda" {
		t.Errorf("merge return aliases operator map: mutation bled back")
	}
}

// TestDeepMergeComponentRaw_NilOperator — an operator stack that only
// has `my-backup: type: sc-backup; config: {}` (every field defaulted).
// Plugin defaults must come through fully.
func TestDeepMergeComponentRaw_NilOperator(t *testing.T) {
	defaults := map[string]any{"cloudExtras": map[string]any{"routing": "function-url"}}
	got := DeepMergeComponentRaw(defaults, nil)
	ce := got["cloudExtras"].(map[string]any)
	if ce["routing"] != "function-url" {
		t.Errorf("nil-operator: plugin defaults must apply, got %v", got)
	}
	// Mutation isolation: defaults must not be mutated by downstream edits.
	ce["routing"] = "api-gateway"
	src := defaults["cloudExtras"].(map[string]any)
	if src["routing"] != "function-url" {
		t.Errorf("merge return aliases defaults map: mutation bled back")
	}
}

// TestDeepMergeComponentRaw_DeepRecursion — nested maps two levels
// down. Pin that the operator-wins rule applies per-leaf, not
// per-top-level-key.
func TestDeepMergeComponentRaw_DeepRecursion(t *testing.T) {
	defaults := map[string]any{
		"worker": map[string]any{
			"resources": map[string]any{
				"cpu":    512,
				"memory": 2048,
			},
			"subnets": []any{"default-subnet"},
		},
	}
	operator := map[string]any{
		"worker": map[string]any{
			"resources": map[string]any{
				"memory": 4096,
			},
		},
	}
	got := DeepMergeComponentRaw(defaults, operator)
	w := got["worker"].(map[string]any)
	res := w["resources"].(map[string]any)
	if res["cpu"] != 512 {
		t.Errorf("deep default cpu lost: %v", res["cpu"])
	}
	if res["memory"] != 4096 {
		t.Errorf("deep operator memory not applied: %v", res["memory"])
	}
	subnets := w["subnets"].([]any)
	if !reflect.DeepEqual(subnets, []any{"default-subnet"}) {
		t.Errorf("deep default subnets lost: %v", subnets)
	}
}

// TestRegisterComponentDefaults_EmptyNoOps — registration is idempotent
// on empty inputs. A plugin that advertises a component type without a
// default registers only the type, not an empty-bytes entry.
func TestRegisterComponentDefaults_EmptyNoOps(t *testing.T) {
	// Snapshot + restore so tests don't leak state.
	before := ComponentDefaults["test-empty"]
	t.Cleanup(func() {
		if before == nil {
			delete(ComponentDefaults, "test-empty")
		} else {
			ComponentDefaults["test-empty"] = before
		}
	})

	RegisterComponentDefaults("test-empty", nil)
	if _, ok := ComponentDefaults["test-empty"]; ok {
		t.Errorf("nil bytes registered an entry")
	}
	RegisterComponentDefaults("test-empty", []byte{})
	if _, ok := ComponentDefaults["test-empty"]; ok {
		t.Errorf("empty-slice bytes registered an entry")
	}
	RegisterComponentDefaults("", []byte(`{"x":1}`))
	if _, ok := ComponentDefaults[""]; ok {
		t.Errorf("empty resource type registered an entry")
	}
}

// TestGetComponentDefaults_IsolatedCopy — a caller mutating the
// returned bytes must not corrupt the shared registry entry. The
// per-deploy sidecar unmarshals the returned bytes in a json.Unmarshal
// call that's bounds-safe but still, copy-on-read is the safe
// contract.
func TestGetComponentDefaults_IsolatedCopy(t *testing.T) {
	RegisterComponentDefaults("test-iso", []byte(`{"k":"v"}`))
	t.Cleanup(func() { delete(ComponentDefaults, "test-iso") })

	got := GetComponentDefaults("test-iso")
	if string(got) != `{"k":"v"}` {
		t.Fatalf("GetComponentDefaults: %s", got)
	}
	got[0] = 'X'
	again := GetComponentDefaults("test-iso")
	if string(again) != `{"k":"v"}` {
		t.Errorf("caller mutation bled into registry: %s", again)
	}
}
