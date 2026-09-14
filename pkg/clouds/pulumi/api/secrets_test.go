// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type secretsTestResource struct {
	sdk.CustomResourceState
}

// secretsTestMocks records the inputs a resource was registered with, which is
// the only place the secret marking is observable.
type secretsTestMocks struct {
	inputs resource.PropertyMap
}

func (m *secretsTestMocks) NewResource(args sdk.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.inputs = args.Inputs
	return args.Name + "-id", args.Inputs, nil
}

func (m *secretsTestMocks) Call(sdk.MockCallArgs) (resource.PropertyMap, error) {
	return resource.PropertyMap{}, nil
}

func TestSecretStringMarksValuesSecret(t *testing.T) {
	RegisterTestingT(t)

	const credential = "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBg\n-----END PRIVATE KEY-----\n"

	mocks := &secretsTestMocks{}
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		var res secretsTestResource
		return ctx.RegisterResource("test:index:Resource", "creds", sdk.Map{
			"plain":            sdk.String(credential),
			"fromString":       SecretString(credential),
			"fromOutput":       SecretStringOutput(sdk.String(credential).ToStringOutput()),
			"fromSecretOutput": SecretStringOutput(sdk.ToSecret(sdk.String(credential)).(sdk.StringOutput)),
		}, &res)
	}, sdk.WithMocks("acme", "staging", mocks))
	Expect(err).ToNot(HaveOccurred())

	// The baseline: without the helper the credential is handed over in clear,
	// which is what the engine then prints in a preview diff.
	Expect(mocks.inputs["plain"].IsSecret()).To(BeFalse())

	for _, key := range []resource.PropertyKey{"fromString", "fromOutput", "fromSecretOutput"} {
		value := mocks.inputs[key]
		Expect(value.IsSecret()).To(BeTrue(), "%s must be secret", key)
		Expect(value.SecretValue().Element.StringValue()).To(Equal(credential),
			"%s must keep the value it wraps", key)
	}
}
