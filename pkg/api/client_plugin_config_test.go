// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestPluginClientConfig_RoundTrip(t *testing.T) {
	RegisterTestingT(t)

	in := ClientDescriptor{
		SchemaVersion: "1.0",
		Stacks:        map[string]StackClientDescriptor{"production": {}},
		Security:      &SecurityDescriptor{Enabled: true, Scan: &ScanDescriptor{Enabled: true}},
		ImageBuild:    &ImageBuildDescriptor{ReuseExistingCommitTag: true},
	}
	raw, err := in.PluginClientConfigJSON()
	Expect(err).ToNot(HaveOccurred())

	// Only what the image build reads crosses the bus: not the stacks, not
	// the defaults (which can carry arbitrary operator configuration).
	Expect(string(raw)).ToNot(ContainSubstring("stacks"))
	Expect(string(raw)).ToNot(ContainSubstring("schemaVersion"))

	out, err := ClientDescriptorFromPluginConfig(raw)
	Expect(err).ToNot(HaveOccurred())
	Expect(out.Security).To(Equal(in.Security))
	Expect(out.ReuseExistingCommitTagEnabled()).To(BeTrue())
	Expect(out.Stacks).To(BeEmpty())
}

func TestPluginClientConfig_AbsentIsTheZeroDescriptor(t *testing.T) {
	RegisterTestingT(t)

	raw, err := (&ClientDescriptor{SchemaVersion: "1.0"}).PluginClientConfigJSON()
	Expect(err).ToNot(HaveOccurred())
	Expect(raw).To(BeNil())

	var nilClient *ClientDescriptor
	raw, err = nilClient.PluginClientConfigJSON()
	Expect(err).ToNot(HaveOccurred())
	Expect(raw).To(BeNil())

	// An older core sends no field; the plugin must run as it did before it.
	out, err := ClientDescriptorFromPluginConfig(nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(out.Security).To(BeNil())
	Expect(out.ReuseExistingCommitTagEnabled()).To(BeFalse())

	_, err = ClientDescriptorFromPluginConfig([]byte("{not json"))
	Expect(err).To(HaveOccurred())
}

// The passphrase is json:"-" on the descriptor, so a plain marshal would hand a
// plugin key-based signing with an empty password.
func TestPluginClientConfig_CarriesTheSigningPassphrase(t *testing.T) {
	RegisterTestingT(t)

	in := ClientDescriptor{Security: &SecurityDescriptor{
		Enabled: true,
		Signing: &SigningDescriptor{Enabled: true, PrivateKey: "key", Password: "s3cret"},
	}}
	raw, err := in.PluginClientConfigJSON()
	Expect(err).ToNot(HaveOccurred())

	out, err := ClientDescriptorFromPluginConfig(raw)
	Expect(err).ToNot(HaveOccurred())
	Expect(out.Security.Signing.PrivateKey).To(Equal("key"))
	Expect(out.Security.Signing.Password).To(Equal("s3cret"))
}
