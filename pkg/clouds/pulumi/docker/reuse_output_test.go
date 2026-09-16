// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package docker

import (
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

var (
	reusedDigestRef = "registry.example.com/repo@sha256:" + strings.Repeat("a", 64)
	builtDigestRef  = "registry.example.com/repo@sha256:" + strings.Repeat("b", 64)
)

// resolveUnderMocks runs fn inside a Pulumi program and returns what the output
// resolved to, failing rather than hanging when the apply never runs.
func resolveUnderMocks[T any](t *testing.T, name string, dryRun bool, fn func(ctx *sdk.Context) sdk.Output) T {
	t.Helper()
	var got T
	done := make(chan struct{})
	opts := []sdk.RunOption{sdk.WithMocks("project", "stack", &deployImageRefMocks{})}
	if dryRun {
		opts = append(opts, func(ri *sdk.RunInfo) { ri.DryRun = true })
	}
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		fn(ctx).ApplyT(func(v T) T {
			got = v
			close(done)
			return v
		})
		return nil
	}, opts...)
	Expect(err).NotTo(HaveOccurred(), name)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s: the output never resolved", name)
	}
	return got
}

// skipPushOutput casts the result of ApplyT to BoolPtrOutput. A wrong mapping
// is a runtime panic that only shows up in a real deploy, never at compile time
// and never in a preview of anything else, so resolve it under mocks here.
//
// The dry-run rows carry the pre-existing preview behaviour: without them,
// deleting the dryRun term leaves the suite green while every preview pushes.
func TestSkipPushOutputResolves(t *testing.T) {
	RegisterTestingT(t)

	for _, tt := range []struct {
		name   string
		dryRun bool
		digest string
		want   bool
	}{
		{"reusable digest skips the push", false, reusedDigestRef, true},
		{"no reusable digest pushes", false, "", false},
		{"preview skips the push even with no reusable digest", true, "", true},
		{"preview skips the push with a reusable digest", true, reusedDigestRef, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			got := resolveUnderMocks[*bool](t, tt.name, tt.dryRun, func(ctx *sdk.Context) sdk.Output {
				return skipPushOutput(ctx, sdk.String(tt.digest).ToStringOutput())
			})
			Expect(got).NotTo(BeNil())
			Expect(*got).To(Equal(tt.want))
		})
	}
}

// pickEffectiveDigest is what keeps a locally built, never-pushed digest out of
// the task definition and out of the security pipeline. Deleting the reuse arm
// otherwise leaves the suite green.
func TestPickEffectiveDigest(t *testing.T) {
	RegisterTestingT(t)

	for _, tt := range []struct {
		name          string
		reused, built string
		want          string
	}{
		{"reused wins over the locally built digest", reusedDigestRef, builtDigestRef, reusedDigestRef},
		{"no reuse falls back to what was pushed", "", builtDigestRef, builtDigestRef},
		{"neither yields empty", "", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			got := resolveUnderMocks[string](t, tt.name, false, func(ctx *sdk.Context) sdk.Output {
				return pickEffectiveDigest(
					sdk.String(tt.reused).ToStringOutput(),
					sdk.String(tt.built).ToStringOutput(),
				)
			})
			Expect(got).To(Equal(tt.want))
		})
	}
}
