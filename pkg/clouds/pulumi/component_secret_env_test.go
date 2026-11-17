// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"testing"

	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// Cleanup helper — test registry mutations don't leak across tests.
func withRegisteredSecretEnv(t *testing.T, resourceType string, mapping map[string]string) {
	t.Helper()
	t.Cleanup(func() { delete(pApi.ComponentSecretEnvMap, resourceType) })
	pApi.RegisterComponentSecretEnvMap(resourceType, mapping)
}

// TestApplyComponentSecretEnvMap_TranslatesField — a declared field
// present on the merged map is copied into `merged["secrets"]` under the
// mapped env name. The original top-level field is NOT deleted (plugins
// may still read the typed value off `.Raw` elsewhere).
func TestApplyComponentSecretEnvMap_TranslatesField(t *testing.T) {
	withRegisteredSecretEnv(t, "sc-test-secret", map[string]string{
		"signingKey":       "SERVICE_JWT_SECRET",
		"operatorPassword": "OPERATOR_PASSWORD",
	})
	merged := map[string]any{
		"signingKey":       "${secret:jwt}",
		"operatorPassword": "${secret:op}",
	}
	applyComponentSecretEnvMap("sc-test-secret", merged)

	secrets, ok := merged["secrets"].(map[string]any)
	if !ok {
		t.Fatalf("secrets not written, merged=%v", merged)
	}
	if secrets["SERVICE_JWT_SECRET"] != "${secret:jwt}" {
		t.Errorf("SERVICE_JWT_SECRET: want ${secret:jwt}, got %v", secrets["SERVICE_JWT_SECRET"])
	}
	if secrets["OPERATOR_PASSWORD"] != "${secret:op}" {
		t.Errorf("OPERATOR_PASSWORD: want ${secret:op}, got %v", secrets["OPERATOR_PASSWORD"])
	}
	// Top-level fields preserved so plugins that still read the typed
	// raw keep working (e.g. sc-plugin-aws's `signingKey` block before
	// it's refactored).
	if merged["signingKey"] != "${secret:jwt}" {
		t.Errorf("signingKey stripped from merged: %v", merged)
	}
}

// TestApplyComponentSecretEnvMap_OperatorSecretWins — an operator-written
// `secrets[ENV]` entry is preserved against the plugin's declared mapping.
func TestApplyComponentSecretEnvMap_OperatorSecretWins(t *testing.T) {
	withRegisteredSecretEnv(t, "sc-test-opwins", map[string]string{
		"signingKey": "SERVICE_JWT_SECRET",
	})
	merged := map[string]any{
		"signingKey": "${secret:from-plugin-field}",
		"secrets": map[string]any{
			"SERVICE_JWT_SECRET": "${secret:from-operator}",
		},
	}
	applyComponentSecretEnvMap("sc-test-opwins", merged)

	secrets := merged["secrets"].(map[string]any)
	if secrets["SERVICE_JWT_SECRET"] != "${secret:from-operator}" {
		t.Errorf("operator value overwritten by plugin field: %v", secrets["SERVICE_JWT_SECRET"])
	}
}

// TestApplyComponentSecretEnvMap_NoopPaths — unknown resource type,
// empty mapping, missing field, and nil merged all return cleanly
// without creating a phantom secrets map.
func TestApplyComponentSecretEnvMap_NoopPaths(t *testing.T) {
	// Unknown resource type: registry empty.
	merged := map[string]any{"signingKey": "x"}
	applyComponentSecretEnvMap("sc-unknown-type", merged)
	if _, present := merged["secrets"]; present {
		t.Error("phantom secrets map created for unknown resource type")
	}

	// Empty mapping (registered but no fields): early-return.
	t.Cleanup(func() { delete(pApi.ComponentSecretEnvMap, "sc-noop-empty") })
	pApi.ComponentSecretEnvMap["sc-noop-empty"] = map[string]string{}
	merged = map[string]any{"signingKey": "x"}
	applyComponentSecretEnvMap("sc-noop-empty", merged)
	if _, present := merged["secrets"]; present {
		t.Error("phantom secrets map created for empty mapping")
	}

	// Field declared but absent from merged.
	withRegisteredSecretEnv(t, "sc-missing-field", map[string]string{
		"signingKey": "SERVICE_JWT_SECRET",
	})
	merged = map[string]any{}
	applyComponentSecretEnvMap("sc-missing-field", merged)
	if _, present := merged["secrets"]; present {
		t.Error("phantom secrets map created when declared field absent")
	}

	// Nil merged — panic-safe.
	applyComponentSecretEnvMap("sc-missing-field", nil)

	// Non-string field: skipped silently (plugin + operator should both
	// write strings; defensive against a future numeric field).
	merged = map[string]any{"signingKey": 42}
	applyComponentSecretEnvMap("sc-missing-field", merged)
	if _, present := merged["secrets"]; present {
		t.Error("non-string field written into secrets")
	}
}
