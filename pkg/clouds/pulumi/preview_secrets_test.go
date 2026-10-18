// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"

	"github.com/simple-container-com/api/pkg/api"
)

// printed renders the result the way cmd_stack.NewOutputsCmd does — json.Marshal of
// the struct, which is the exact byte sequence that reaches a terminal or a CI log.
// Asserting against this, not against a single field, is what catches a secret that
// is masked in one branch and embedded in a larger blob by another.
func printed(t *testing.T, res *api.OutputsResult) string {
	t.Helper()
	j, err := json.Marshal(*res)
	if err != nil {
		t.Fatalf("marshal outputs: %v", err)
	}
	return string(j)
}

// The kubeconfig k3s exports is a cluster-admin credential and the join token lets a
// machine join the cluster. Both were printed in the clear by `sc stack outputs`
// during the 2026-10-18 k3s smoke, which is what this masking exists to stop. The
// assertions below are deliberately "the secret's own bytes do not appear anywhere
// in the result", not "the field equals the placeholder" — the original bug was a
// branch that embedded the value inside a larger JSON blob, so an equality-only test
// would have passed against it.
const (
	fakeKubeconfig = "apiVersion: v1\nclusters:\n- cluster:\n    server: https://10.0.0.1:6443\n    client-key-data: c3VwZXItc2VjcmV0LWtleQ==\n"
	fakeJoinToken  = "K10abcdef0123456789::server:f1e2d3c4b5a6"
)

func TestOutputsMaskSecretsByDefault(t *testing.T) {
	RegisterTestingT(t)

	outputs := auto.OutputMap{
		"cluster-kubeconfig": {Value: fakeKubeconfig, Secret: true},
		"cluster-ingress-ip": {Value: "10.0.0.1", Secret: false},
	}
	p := &pulumi{}

	masked := p.toOutputsResult("mycompany", outputs, false)

	// AC1: the secret is gone, and the placeholder is what took its place.
	Expect(masked.Outputs["cluster-kubeconfig"]).To(Equal(api.SecretOutputPlaceholder))
	Expect(printed(t, masked)).NotTo(ContainSubstring("c3VwZXItc2VjcmV0LWtleQ=="))
	// A non-secret output is untouched — masking must not cost visibility of the
	// endpoint an operator actually came to read.
	Expect(masked.Outputs["cluster-ingress-ip"]).To(Equal("10.0.0.1"))
}

func TestOutputsRevealSecretsOnOptIn(t *testing.T) {
	RegisterTestingT(t)

	outputs := auto.OutputMap{
		"cluster-kubeconfig": {Value: fakeKubeconfig, Secret: true},
	}
	p := &pulumi{}

	// AC2: --show-secrets is the only way the plaintext is reachable.
	revealed := p.toOutputsResult("mycompany", outputs, true)

	Expect(revealed.Outputs["cluster-kubeconfig"]).To(Equal(fakeKubeconfig))
	Expect(revealed.Outputs["cluster-kubeconfig"]).NotTo(Equal(api.SecretOutputPlaceholder))
}

// The map-valued and the fall-through branches both reached the secret's bytes too,
// and a structured output (a join token alongside its endpoint) is the shape that
// hits them. Covered separately because the string branch above cannot reach them.
func TestOutputsMaskNonStringSecrets(t *testing.T) {
	RegisterTestingT(t)

	outputs := auto.OutputMap{
		"cluster-join": {
			Value:  map[string]any{"token": fakeJoinToken, "url": "https://10.0.0.1:6443"},
			Secret: true,
		},
		"cluster-replicas": {Value: 3, Secret: true},
	}
	p := &pulumi{}

	masked := p.toOutputsResult("mycompany", outputs, false)

	Expect(masked.Outputs["cluster-join"]).To(Equal(api.SecretOutputPlaceholder))
	Expect(masked.Outputs["cluster-replicas"]).To(Equal(api.SecretOutputPlaceholder))
	// The whole serialized result is the thing that reaches a terminal or a CI log,
	// so assert against that rather than the single field.
	Expect(printed(t, masked)).NotTo(ContainSubstring(fakeJoinToken))

	revealed := p.toOutputsResult("mycompany", outputs, true)
	Expect(printed(t, revealed)).To(ContainSubstring(fakeJoinToken))
}
