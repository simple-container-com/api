// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package aws

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBuildBundlePayload_MergesBothSources pins the contract that the
// bundle gathers from BOTH `contextSecrets` (compute-context secret
// envs) and `stackSecrets` (the `.sc/stacks/<stack>/secrets.yaml`
// `values:` registry), so bundle mode is a drop-in for the legacy
// per-key path in `aws_lambda.go:189-204`.
func TestBuildBundlePayload_MergesBothSources(t *testing.T) {
	ctxVars := []secretValue{
		{Name: "TAVILY_API_KEY", Value: "tvly-xxx"},
		{Name: "RESEND_API_KEY", Value: "re_yyy"},
	}
	stackSecrets := map[string]string{
		"STRIPE_WEBHOOK_SECRET": "whsec_zzz",
	}

	got := buildBundlePayload(ctxVars, stackSecrets)

	assert.Equal(t, "tvly-xxx", got["TAVILY_API_KEY"])
	assert.Equal(t, "re_yyy", got["RESEND_API_KEY"])
	assert.Equal(t, "whsec_zzz", got["STRIPE_WEBHOOK_SECRET"])
	assert.Len(t, got, 3)
}

// TestBuildBundlePayload_StackSecretsWinOnCollision — context secrets
// come first, stack secrets apply second. If both source the same key,
// the stack secret value wins. (Mirrors the legacy per-key order:
// context secrets are built first, then stackConfig.Secrets, so the
// second write would overwrite in a map build.)
func TestBuildBundlePayload_StackSecretsWinOnCollision(t *testing.T) {
	ctxVars := []secretValue{
		{Name: "KEY", Value: "from-context"},
	}
	stackSecrets := map[string]string{
		"KEY": "from-stack",
	}
	got := buildBundlePayload(ctxVars, stackSecrets)
	assert.Equal(t, "from-stack", got["KEY"])
}

// TestBuildBundlePayload_EmptyInputs — nil + nil = empty map. Guard
// against a surprise nil-dereference when a service has no secrets at
// all but still opts into bundle mode (which is a valid-if-pointless
// configuration — the Pulumi side still builds a Secret, just an
// empty JSON object).
func TestBuildBundlePayload_EmptyInputs(t *testing.T) {
	got := buildBundlePayload(nil, nil)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

// TestBuildBundlePayload_SerialisesStably — the JSON key order is
// deterministic across deploys (json.Marshal sorts map keys), which
// keeps the SecretVersion diff tight when nothing changed. Protects
// against a future developer swapping json.Marshal for something that
// doesn't sort.
func TestBuildBundlePayload_SerialisesStably(t *testing.T) {
	payload := buildBundlePayload(
		[]secretValue{{Name: "B", Value: "2"}, {Name: "A", Value: "1"}},
		map[string]string{"C": "3"},
	)
	raw, err := json.Marshal(payload)
	assert.NoError(t, err)
	assert.Equal(t, `{"A":"1","B":"2","C":"3"}`, string(raw))
}
