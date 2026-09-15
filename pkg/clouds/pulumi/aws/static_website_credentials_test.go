// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package aws

import (
	"testing"

	. "github.com/onsi/gomega"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/clouds/aws"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"
)

type staticWebsiteTestResource struct {
	sdk.CustomResourceState
}

// The bundle upload runs through a command:local:Command, and pulumi-command
// wraps nothing of its own: an unwrapped key in that command's environment is
// a plaintext resource input, rendered in the diff like any other.
func TestStaticWebsiteSyncEnvironmentIsSecret(t *testing.T) {
	RegisterTestingT(t)

	mocks := testutil.NewRecordingMocks()
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		var res staticWebsiteTestResource
		return ctx.RegisterResource("command:local:Command", "acme-sync", sdk.Map{
			"environment": syncEnvironment(aws.AccountConfig{
				Region:          "us-east-1",
				AccessKey:       "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMIexampleKEY",
			}),
		}, &res)
	}, sdk.WithMocks("acme", "staging", mocks))
	Expect(err).ToNot(HaveOccurred())

	inputs, ok := mocks.Inputs("command:local:Command", "acme-sync")
	Expect(ok).To(BeTrue())
	env := inputs["environment"].ObjectValue()
	Expect(env).To(HaveLen(3))

	Expect(env["AWS_SECRET_ACCESS_KEY"].IsSecret()).To(BeTrue(),
		"the secret access key must reach the engine as a secret")
	Expect(env["AWS_SECRET_ACCESS_KEY"].SecretValue().Element.StringValue()).To(Equal("wJalrXUtnFEMIexampleKEY"))
	Expect(env["AWS_ACCESS_KEY_ID"].IsSecret()).To(BeTrue())

	// The region is not a credential, and keeping it legible is what makes the
	// diff worth reading.
	Expect(env["AWS_DEFAULT_REGION"].IsSecret()).To(BeFalse())
	Expect(env["AWS_DEFAULT_REGION"].StringValue()).To(Equal("us-east-1"))
}

// Ambient credentials are a supported mode: nothing is set, so the AWS default
// chain resolves them in the command's own process.
func TestStaticWebsiteSyncEnvironmentOmitsAmbientCredentials(t *testing.T) {
	RegisterTestingT(t)

	env := syncEnvironment(aws.AccountConfig{Region: "us-east-1"})

	Expect(env).To(HaveLen(1))
	Expect(env).To(HaveKey("AWS_DEFAULT_REGION"))
	Expect(env).ToNot(HaveKey("AWS_ACCESS_KEY_ID"))
}
