// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"testing"
)

// MergeDefaults is what makes a plugin-supplied default for a REQUIRED
// component key (deployTo, image) reachable by parse-time validation,
// which runs BEFORE the dispatcher's own merge. If validation and
// dispatch ever disagreed about the effective config, a descriptor
// could validate and then provision something else — hence both go
// through GetComponentDefaults + DeepMergeComponentRaw.

func TestMergeDefaults_PluginDefaultFillsOmittedKey(t *testing.T) {
	RegisterComponentDefaults("test-md-fill", []byte(`{"deployTo":"lambda","image":"repo/img:v1"}`))

	got := componentConfigValidator{}.MergeDefaults("test-md-fill", map[string]any{
		"ingress": map[string]any{"hostname": "x.example.com"},
	})

	if got["deployTo"] != "lambda" {
		t.Fatalf("expected the plugin's deployTo default, got %#v", got["deployTo"])
	}
	if got["image"] != "repo/img:v1" {
		t.Fatalf("expected the plugin's image default, got %#v", got["image"])
	}
	if _, ok := got["ingress"]; !ok {
		t.Fatal("operator-written keys must survive the merge")
	}
}

func TestMergeDefaults_OperatorWins(t *testing.T) {
	RegisterComponentDefaults("test-md-win", []byte(`{"deployTo":"lambda","timeout":30}`))

	got := componentConfigValidator{}.MergeDefaults("test-md-win", map[string]any{
		"deployTo": "compose-on-vm",
	})

	if got["deployTo"] != "compose-on-vm" {
		t.Fatalf("operator must win per key, got %#v", got["deployTo"])
	}
	if got["timeout"] == nil {
		t.Fatal("a default the operator did not set must stay")
	}
}

func TestMergeDefaults_NoRegistrationPassesThrough(t *testing.T) {
	raw := map[string]any{"deployTo": "lambda"}

	// A plugin that pre-dates ComponentDefaults must validate exactly as
	// it did before this change.
	got := componentConfigValidator{}.MergeDefaults("test-md-absent", raw)
	if len(got) != 1 || got["deployTo"] != "lambda" {
		t.Fatalf("expected pass-through, got %#v", got)
	}
}

func TestMergeDefaults_UnparseableDefaultsDegradeToRaw(t *testing.T) {
	RegisterComponentDefaults("test-md-bad", []byte(`{not json`))

	// A malformed plugin default must not make an otherwise-valid
	// descriptor unprovisionable at parse time — dispatch surfaces it.
	got := componentConfigValidator{}.MergeDefaults("test-md-bad", map[string]any{"image": "i"})
	if len(got) != 1 || got["image"] != "i" {
		t.Fatalf("expected degrade-to-raw, got %#v", got)
	}
}

func TestMergeDefaults_DoesNotMutateInputs(t *testing.T) {
	RegisterComponentDefaults("test-md-nomut", []byte(`{"deployTo":"lambda"}`))

	raw := map[string]any{"image": "i"}
	_ = componentConfigValidator{}.MergeDefaults("test-md-nomut", raw)

	if len(raw) != 1 {
		t.Fatalf("operator raw was mutated: %#v", raw)
	}
	// The registry copy must still be intact for the dispatcher.
	if string(GetComponentDefaults("test-md-nomut")) != `{"deployTo":"lambda"}` {
		t.Fatal("the registered defaults blob was mutated")
	}
}
