// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package testutil_test

import (
	"testing"

	. "github.com/onsi/gomega"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"
)

type recorderTestResource struct {
	sdk.CustomResourceState
}

// A stack registers several providers of the same type, so the recorder keys
// by type AND name. Keyed by type alone the last registration wins and every
// assertion silently targets the wrong resource, which is the failure mode
// that makes a security test pass while the thing it guards is broken.
func TestRecordingMocksKeepsRegistrationsApart(t *testing.T) {
	RegisterTestingT(t)

	mocks := testutil.NewRecordingMocks()
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		var first, second recorderTestResource
		if err := ctx.RegisterResource("test:index:Resource", "first",
			sdk.Map{"value": sdk.String("one")}, &first); err != nil {
			return err
		}
		return ctx.RegisterResource("test:index:Resource", "second",
			sdk.Map{"value": sdk.String("two")}, &second)
	}, sdk.WithMocks("acme", "staging", mocks))
	Expect(err).ToNot(HaveOccurred())

	first, ok := mocks.Inputs("test:index:Resource", "first")
	Expect(ok).To(BeTrue())
	Expect(first["value"].StringValue()).To(Equal("one"))

	second, ok := mocks.Inputs("test:index:Resource", "second")
	Expect(ok).To(BeTrue())
	Expect(second["value"].StringValue()).To(Equal("two"))

	Expect(mocks.InputsOfType("test:index:Resource")).To(HaveLen(2))

	_, ok = mocks.Inputs("test:index:Resource", "absent")
	Expect(ok).To(BeFalse())
	Expect(mocks.InputsOfType("test:index:Other")).To(BeEmpty())
}

// A data-source fixture that is never consumed means the test is exercising
// something other than what it claims. CalledTokens makes that assertable.
func TestRecordingMocksRecordsCallTokens(t *testing.T) {
	RegisterTestingT(t)

	mocks := testutil.NewRecordingMocks()
	mocks.CallResults["test:index:getThing"] = nil

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		var out struct {
			Name string `pulumi:"name"`
		}
		return ctx.Invoke("test:index:getThing", nil, &out)
	}, sdk.WithMocks("acme", "staging", mocks))
	Expect(err).ToNot(HaveOccurred())

	Expect(mocks.CalledTokens()).To(ContainElement("test:index:getThing"))
}
