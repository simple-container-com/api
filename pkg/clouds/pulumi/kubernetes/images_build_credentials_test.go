// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"testing"

	. "github.com/onsi/gomega"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"
)

type imagesBuildTestResource struct {
	sdk.CustomResourceState
}

// pulumi-docker does not wrap `registry.password`, and the value Simple
// Container puts there is a live credential: a GCP OAuth token on the GKE
// path, an ECR token on the AWS one, or a configured registry password.
func TestBuildAndPushImagesRegistryPasswordIsSecret(t *testing.T) {
	RegisterTestingT(t)

	mocks := testutil.NewRecordingMocks()
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		var res imagesBuildTestResource
		return ctx.RegisterResource("docker:index/image:Image", "acme-image", sdk.Map{
			"registry": sdk.Map{
				"server":   sdk.String("europe-docker.pkg.dev"),
				"username": sdk.String("oauth2accesstoken"),
				"password": registryPassword(lo.ToPtr("ya29.exampleAccessToken")),
			},
		}, &res)
	}, sdk.WithMocks("acme", "staging", mocks))
	Expect(err).ToNot(HaveOccurred())

	inputs, ok := mocks.Inputs("docker:index/image:Image", "acme-image")
	Expect(ok).To(BeTrue())
	registry := inputs["registry"].ObjectValue()

	Expect(registry["password"].IsSecret()).To(BeTrue(),
		"the registry password must reach the engine as a secret")
	Expect(registry["password"].SecretValue().Element.StringValue()).To(Equal("ya29.exampleAccessToken"))

	// The registry and the account name are what make the diff readable.
	Expect(registry["server"].IsSecret()).To(BeFalse())
	Expect(registry["username"].IsSecret()).To(BeFalse())
}

// No configured password means the registry needs no auth, and the field has
// to stay absent rather than become an empty secret.
func TestBuildAndPushImagesOmitsAbsentRegistryPassword(t *testing.T) {
	RegisterTestingT(t)

	Expect(registryPassword(nil)).To(BeNil())
}
