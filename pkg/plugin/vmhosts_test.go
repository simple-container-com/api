// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// The shape test: what a VM-driver plugin writes, and what comes out on the wire.
// Every ref must carry the engine's own URN and the provider's own property name,
// because core resolves against those two strings and nothing else.
func TestVmHostBuilder_BuildsTheWireShape(t *testing.T) {
	RegisterTestingT(t)

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		var instance, fip, key sdk.CustomResourceState
		Expect(ctx.RegisterResource("sc:test:Instance", "web-1", nil, &instance)).To(Succeed())
		Expect(ctx.RegisterResource("sc:test:FloatingIp", "web-1-ip", nil, &fip)).To(Succeed())
		Expect(ctx.RegisterResource("sc:test:Key", "web-1-key", nil, &key)).To(Succeed())

		resp, err := BuildVmHosts(
			NewVmHost(ctx.Context(), "web-1").
				User("ubuntu").Port(2222).Role("compose").
				AddressOf(&fip, "externalIpv4Address").
				PrivateKeyOf(&key, "privateKeyOpenssh").
				HostKeyOf(&key, "publicKeyOpenssh").
				DependsOn(&instance, &fip),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.GetHosts()).To(HaveLen(1))

		h := resp.GetHosts()[0]
		Expect(h.GetName()).To(Equal("web-1"))
		Expect(h.GetUser()).To(Equal("ubuntu"))
		Expect(h.GetPort()).To(Equal(int32(2222)))
		Expect(h.GetRole()).To(Equal("compose"))

		Expect(h.GetAddress().GetRef().GetUrn()).To(Equal("urn:pulumi:e2e::proj::sc:test:FloatingIp::web-1-ip"))
		Expect(h.GetAddress().GetRef().GetProperty()).To(Equal("externalIpv4Address"))
		Expect(h.GetAddress().GetSecret()).To(BeFalse(), "an address is not a secret")

		Expect(h.GetPrivateKey().GetRef().GetUrn()).To(Equal("urn:pulumi:e2e::proj::sc:test:Key::web-1-key"))
		Expect(h.GetPrivateKey().GetRef().GetProperty()).To(Equal("privateKeyOpenssh"))
		Expect(h.GetPrivateKey().GetSecret()).To(BeTrue(), "a private key crosses the bus marked secret")

		Expect(h.GetHostKey().GetRef().GetProperty()).To(Equal("publicKeyOpenssh"))
		Expect(h.GetHostKeyPinned()).To(BeTrue(), "a driver that supplies a host key is pinning it")

		Expect(h.GetDependsOn()).To(Equal([]string{
			"urn:pulumi:e2e::proj::sc:test:Instance::web-1",
			"urn:pulumi:e2e::proj::sc:test:FloatingIp::web-1-ip",
		}))

		// Nothing was set for the key password, which core reads as the empty
		// passphrase every self-keying driver means.
		Expect(h.GetPrivateKeyPassword()).To(BeNil())

		return nil
	}, sdk.WithMocks("proj", "e2e", authoringMocks{}))
	Expect(err).ToNot(HaveOccurred())
}

// The byo-shaped host: every fact is already known, so nothing is a ref — and the
// secrecy of each field is still decided by which setter was used, not by the caller
// remembering a flag.
func TestVmHostBuilder_LiteralsCarryTheirOwnSecrecy(t *testing.T) {
	RegisterTestingT(t)

	h, err := NewVmHost(context.Background(), "byo-1").
		User("root").
		Address("198.51.100.4").
		PrivateKey("-----BEGIN OPENSSH PRIVATE KEY-----").
		PrivateKeyPassword("hunter2").
		HostKey("ssh-ed25519 AAAAC3Nz").
		Build()
	Expect(err).ToNot(HaveOccurred())

	Expect(h.GetAddress().GetLiteral()).To(Equal("198.51.100.4"))
	Expect(h.GetAddress().GetSecret()).To(BeFalse())
	Expect(h.GetPrivateKey().GetSecret()).To(BeTrue())
	Expect(h.GetPrivateKeyPassword().GetLiteral()).To(Equal("hunter2"))
	Expect(h.GetPrivateKeyPassword().GetSecret()).To(BeTrue())
	Expect(h.GetHostKey().GetLiteral()).To(Equal("ssh-ed25519 AAAAC3Nz"))
	Expect(h.GetHostKey().GetSecret()).To(BeFalse(), "a public host key is not a secret")
	Expect(h.GetHostKeyPinned()).To(BeTrue())
}

// Core's two structural refusals, raised locally so the plugin author sees them
// without a gRPC round-trip — and with the host named, which core's message cannot
// always do.
func TestVmHostBuilder_RefusesAHostCoreWouldReject(t *testing.T) {
	RegisterTestingT(t)

	_, err := NewVmHost(context.Background(), "web-1").TrustOnFirstUse().Build()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring(`vm host "web-1" has no address`))

	_, err = NewVmHost(context.Background(), "").Address("198.51.100.4").TrustOnFirstUse().Build()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("vm host with no name"))
}

// The host-key question has to be answered one way or the other. Defaulting to
// trust-on-first-use would make the weaker choice the silent one.
func TestVmHostBuilder_RefusesAnUnansweredHostKey(t *testing.T) {
	RegisterTestingT(t)

	_, err := NewVmHost(context.Background(), "web-1").Address("198.51.100.4").Build()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("answered neither HostKey nor TrustOnFirstUse"))
}

// …and answered once. A chain that pins a key and then asks for trust-on-first-use
// has two readings and no way to tell which the author meant.
func TestVmHostBuilder_RefusesContradictoryHostKeyAnswers(t *testing.T) {
	RegisterTestingT(t)

	_, err := NewVmHost(context.Background(), "web-1").
		Address("198.51.100.4").
		HostKey("ssh-ed25519 AAAAC3Nz").
		TrustOnFirstUse().
		Build()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("already answered"))
	Expect(err.Error()).To(ContainSubstring(`vm host "web-1"`))
}

// A setter that fails does not stop the chain — it records, and Build reports. This
// is what lets a host read as one expression; the test is that the error survives
// everything written after it.
func TestVmHostBuilder_FirstErrorSurvivesTheRestOfTheChain(t *testing.T) {
	RegisterTestingT(t)

	_, err := NewVmHost(context.Background(), "web-1").
		AddressOf(nil, "externalIpv4Address").
		User("ubuntu").
		PrivateKey("key").
		TrustOnFirstUse().
		Build()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring(`vm host "web-1"`))
	Expect(err.Error()).To(ContainSubstring("address"))
	Expect(err.Error()).To(ContainSubstring("nil resource"))
}

// A VmHosts response with no hosts is a success report for a `type: vm` resource
// that produced no machines; core refuses it, and so does this side.
func TestBuildVmHosts_RefusesAnEmptySet(t *testing.T) {
	RegisterTestingT(t)

	_, err := BuildVmHosts()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("at least one machine"))
}

// An empty property name would resolve core-side to a complaint about a nameless
// property, which says nothing about where it came from.
func TestOutputRefTo_RefusesAnEmptyProperty(t *testing.T) {
	RegisterTestingT(t)

	_, err := OutputRefTo(context.Background(), nil, "")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("empty property name"))
}
