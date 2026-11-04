// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"encoding/json"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// Regression anchor for the forge-conductor isolated verify failure on
// api v2026.11.11 (run 41680853191): plugin "aws" InitStateStore("s3-bucket")
// received the raw string `${auth:aws}` instead of a resolved JSON
// credentials blob, because json.Marshal(*PluginFieldConfig) only emitted
// the four typed slots (type/url/credentials/projectId/provision) and
// one of them wasn't reached by the placeholder resolver's walk through
// an unexported-field path on the stack.
//
// MarshalJSON now merges Raw with the typed fields, so the plugin bus
// (serializeConfigToInputs → json.Marshal → gRPC) carries whichever
// representation the resolver actually touched. This test locks that in.

func TestPluginFieldConfig_MarshalJSON_MergesRawAndTypedFields(t *testing.T) {
	RegisterTestingT(t)

	cfg := &PluginFieldConfig{
		TypeName: "s3-bucket",
		Raw: map[string]any{
			// A key only in Raw — the plugin needs this (bucketName / keyName /
			// region / anything the plugin's own StateStorageConfig struct
			// decodes), and before the merge nothing in the typed slot would
			// have carried it to the plugin bus.
			"bucketName": "forge-conductor-state",
			// A placeholder-shaped value — the walker resolves this in-place
			// on the Raw map AND on the typed Credentials slot. The marshal
			// MUST emit the resolved form, not the ${...} string.
			"credentials": `{"accessKey":"AKIA","secretAccessKey":"S3CR3T"}`,
			"region":      "us-east-1",
		},
		Credentials: `{"accessKey":"AKIA","secretAccessKey":"S3CR3T"}`,
	}

	b, err := json.Marshal(cfg)
	Expect(err).ToNot(HaveOccurred())

	var out map[string]any
	Expect(json.Unmarshal(b, &out)).To(Succeed())

	// Raw-only key reaches the plugin.
	Expect(out).To(HaveKeyWithValue("bucketName", "forge-conductor-state"),
		"bucketName was only in Raw — before this merge, json.Marshal with `json:\"-\"` on Raw would drop it")
	Expect(out).To(HaveKeyWithValue("region", "us-east-1"))

	// Typed-slot override: both Raw[credentials] and Credentials hold the
	// resolved blob, output must carry one unresolved-free value.
	Expect(out).To(HaveKey("credentials"))
	Expect(out["credentials"].(string)).ToNot(ContainSubstring("${"))

	// TypeName from the typed slot.
	Expect(out).To(HaveKeyWithValue("type", "s3-bucket"))
}

func TestPluginFieldConfig_MarshalJSON_UnresolvedPlaceholder_FailsLoudly(t *testing.T) {
	RegisterTestingT(t)

	// Simulate the pre-fix pathology — the typed Credentials field was
	// resolved by the walker, but Raw[credentials] was somehow missed. The
	// merge order (Raw first, typed override) means the resolved typed
	// value still wins, so the plugin never sees `${...}`.
	cfg := &PluginFieldConfig{
		TypeName: "s3-bucket",
		Raw: map[string]any{
			"credentials": "${auth:aws}", // stale/unresolved
			"bucketName":  "forge-conductor-state",
		},
		Credentials: `{"accessKey":"AKIA"}`, // resolved
	}

	b, err := json.Marshal(cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(b)).ToNot(ContainSubstring("${"),
		"typed Credentials slot must override an unresolved Raw[credentials] on marshal")
}

func TestPluginFieldConfig_MarshalJSON_RawResolved_TypedZero(t *testing.T) {
	RegisterTestingT(t)

	// The inverse case — Raw was resolved by the walker but the typed
	// Credentials slot stayed empty (never touched for whatever reason).
	// Raw's value should still make it to the plugin.
	cfg := &PluginFieldConfig{
		TypeName: "s3-bucket",
		Raw: map[string]any{
			"credentials": `{"accessKey":"AKIA"}`,
			"bucketName":  "forge-conductor-state",
		},
		// Credentials deliberately empty.
	}

	b, err := json.Marshal(cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(strings.Contains(string(b), "AKIA")).To(BeTrue(),
		"Raw-resolved credentials must survive the merge when the typed slot is empty")
}

func TestPluginFieldConfig_MarshalJSON_OmitsEmptyTypedFields(t *testing.T) {
	RegisterTestingT(t)

	// Operators who use ambient credentials omit `credentials:` entirely.
	// json.Marshal must NOT emit an empty `"credentials":""` string —
	// that would clobber the plugin's own ambient-credentials fallback.
	cfg := &PluginFieldConfig{
		TypeName: "s3-bucket",
		Raw: map[string]any{
			"region": "us-east-1",
		},
		// All typed slots zero. Only `provision` is a bool (always emitted).
	}

	b, err := json.Marshal(cfg)
	Expect(err).ToNot(HaveOccurred())

	var out map[string]any
	Expect(json.Unmarshal(b, &out)).To(Succeed())

	Expect(out).ToNot(HaveKey("credentials"),
		"empty Credentials must NOT be emitted — would otherwise override the plugin's ambient-creds path")
	Expect(out).ToNot(HaveKey("url"))
	Expect(out).ToNot(HaveKey("projectId"))
	Expect(out).To(HaveKeyWithValue("type", "s3-bucket"))
	Expect(out).To(HaveKeyWithValue("region", "us-east-1"))
}
