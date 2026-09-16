// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package docker

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type noopMocks struct{}

func (noopMocks) NewResource(args sdk.MockResourceArgs) (string, resource.PropertyMap, error) {
	return args.Name + "-id", args.Inputs, nil
}
func (noopMocks) Call(args sdk.MockCallArgs) (resource.PropertyMap, error) { return args.Args, nil }

// skipPushOutput casts the result of ApplyT to BoolPtrOutput. A wrong mapping
// is a runtime panic that only shows up in a real deploy, never at compile time
// and never in a preview of anything else, so resolve it under mocks here.
func TestSkipPushOutputResolves(t *testing.T) {
	RegisterTestingT(t)

	for _, tt := range []struct {
		name   string
		digest string
		want   bool
	}{
		{"reusable digest skips the push", "registry.example.com/repo@sha256:abc", true},
		{"no reusable digest pushes", "", false},
	} {
		var got *bool
		done := make(chan struct{})
		err := sdk.RunErr(func(ctx *sdk.Context) error {
			out := skipPushOutput(ctx, sdk.String(tt.digest).ToStringOutput())
			out.ApplyT(func(v *bool) *bool {
				got = v
				close(done)
				return v
			})
			return nil
		}, sdk.WithMocks("project", "stack", noopMocks{}))
		Expect(err).NotTo(HaveOccurred(), tt.name)
		<-done
		Expect(got).NotTo(BeNil(), tt.name)
		Expect(*got).To(Equal(tt.want), tt.name)
	}
}
