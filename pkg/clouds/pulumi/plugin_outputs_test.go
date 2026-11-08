// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"testing"

	. "github.com/onsi/gomega"

	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

func stringValue(s string, secret bool) *scpluginv1.InputValue {
	return &scpluginv1.InputValue{
		Value:  &scpluginv1.InputValue_StringValue{StringValue: s},
		Secret: secret,
	}
}

// The harvest feeds two consumers from one pass: the parent stack root's exports
// (where the secret flag decides sdk.ToSecret) and the in-memory sibling path on
// .Ref. Both read the flag, so losing it here loses it in both places.
func TestHarvestPluginOutputs_KeepsTheSecretFlagPerValue(t *testing.T) {
	RegisterTestingT(t)

	got := harvestPluginOutputs(map[string]*scpluginv1.InputValue{
		"acme-store-bucket-name":        stringValue("acme-store-staging", false),
		"acme-store-hmac-access-key-id": stringValue("GOOG1EEXAMPLE", false),
		"acme-store-hmac-secret-key":    stringValue("s3cr3t", true),
	})

	Expect(got).To(HaveLen(3))
	Expect(got["acme-store-bucket-name"].Value).To(Equal("acme-store-staging"))
	Expect(got["acme-store-bucket-name"].Secret).To(BeFalse())
	Expect(got["acme-store-hmac-access-key-id"].Secret).To(BeFalse())
	Expect(got["acme-store-hmac-secret-key"].Value).To(Equal("s3cr3t"))
	Expect(got["acme-store-hmac-secret-key"].Secret).To(BeTrue())
}

// A value core cannot flatten is left out rather than recorded as a zero, so a
// consumer asking for it is told it was not published instead of being handed an
// empty string that reads like it was.
func TestHarvestPluginOutputs_DropsUnrepresentableValues(t *testing.T) {
	RegisterTestingT(t)

	got := harvestPluginOutputs(map[string]*scpluginv1.InputValue{
		"good": stringValue("value", false),
		"bad":  {}, // no oneof set at all
	})

	Expect(got).To(HaveLen(1))
	Expect(got).To(HaveKey("good"))
	Expect(got).ToNot(HaveKey("bad"))
}

// Older plugins send no outputs at all; the harvest has to be a no-op for them
// rather than a nil map a consumer would then have to guard.
func TestHarvestPluginOutputs_EmptyIsAnEmptyMap(t *testing.T) {
	RegisterTestingT(t)

	Expect(harvestPluginOutputs(nil)).ToNot(BeNil())
	Expect(harvestPluginOutputs(nil)).To(BeEmpty())
}
