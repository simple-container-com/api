// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"
)

type secretsTestResource struct {
	sdk.CustomResourceState
}

func TestSecretStringMarksValuesSecret(t *testing.T) {
	RegisterTestingT(t)

	const credential = "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBg\n-----END PRIVATE KEY-----\n"

	mocks := testutil.NewRecordingMocks()
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		var res secretsTestResource
		return ctx.RegisterResource("test:index:Resource", "creds", sdk.Map{
			"plain":            sdk.String(credential),
			"fromString":       pApi.SecretString(sdk.String(credential)),
			"fromOutput":       pApi.SecretString(sdk.String(credential).ToStringOutput()),
			"fromSecretOutput": pApi.SecretString(sdk.ToSecret(sdk.String(credential)).(sdk.StringOutput)),
			"empty":            pApi.SecretString(sdk.String("")),
		}, &res)
	}, sdk.WithMocks("acme", "staging", mocks))
	Expect(err).ToNot(HaveOccurred())

	inputs, ok := mocks.Inputs("test:index:Resource", "creds")
	Expect(ok).To(BeTrue(), "the resource should have been registered")
	Expect(inputs).To(HaveLen(5))

	// The baseline: without the helper the credential is handed over in clear,
	// which is what the engine then prints in a preview diff.
	Expect(inputs["plain"].IsSecret()).To(BeFalse())

	for _, key := range []resource.PropertyKey{"fromString", "fromOutput", "fromSecretOutput"} {
		value := inputs[key]
		Expect(value.IsSecret()).To(BeTrue(), "%s must be secret", key)
		Expect(value.SecretValue().Element.StringValue()).To(Equal(credential),
			"%s must keep the value it wraps", key)
	}

	// An empty credential is a real state: GCP auth can come from the ambient
	// environment instead of a key. It must stay empty rather than become
	// unknown, and stay secret rather than be skipped.
	Expect(inputs["empty"].IsSecret()).To(BeTrue())
	Expect(inputs["empty"].SecretValue().Element.StringValue()).To(BeEmpty())
}
