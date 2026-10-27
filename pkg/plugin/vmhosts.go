// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	"context"

	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"

	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// The plugin side of the VmHosts RPC.
//
// A VM driver's whole job on this bus is: create the machines, then describe them
// well enough that core can connect over SSH and run everything else (readiness,
// docker, compose, k3s) itself. "Describe" is the part with teeth, because three of
// the four facts per host — address, private key, host key — are not values the
// plugin has. They are properties of resources the engine has not finished creating,
// so they cross as a ResourcePropertyRef that core resolves against its own monitor.
//
// Building that by hand means awaiting a URN at the right moment and getting the
// provider's own property name right, and the failure mode for the first is a deploy
// that hangs with no error (see AwaitURN). Hence the builder: the awaits happen where
// they must, and the refusals core would return over gRPC are raised locally, in the
// plugin, with the host's name attached.
//
//	hosts, err := plugin.BuildVmHosts(
//	    plugin.NewVmHost(pctx.Context(), "web-1").
//	        User("ubuntu").Port(22).Role(string(vm.RoleCompose)).
//	        AddressOf(fip, "externalIpv4Address").
//	        PrivateKeyOf(key, "privateKeyOpenssh").
//	        TrustOnFirstUse().
//	        DependsOn(instance, fip),
//	)

// OutputLiteral is a fact the plugin already knows: a byo host's address, an empty
// key password, a host key pinned in the operator's config.
func OutputLiteral(value string) *scpluginv1.OutputValue {
	return &scpluginv1.OutputValue{From: &scpluginv1.OutputValue_Literal{Literal: value}}
}

// SecretLiteral is OutputLiteral for a value that must not reach a log or the state
// file in plain text. Core wraps it with pulumi.ToSecret.
func SecretLiteral(value string) *scpluginv1.OutputValue {
	v := OutputLiteral(value)
	v.Secret = true
	return v
}

// OutputRefTo names a property of a resource the plugin registered, for core to
// resolve against the engine once it exists.
//
// `property` is the PROVIDER's output property name as the Pulumi package spells it
// (`externalIpv4Address`, `privateKeyOpenssh`, `accessIpV4`) — not an SC-level name,
// and not the Terraform snake_case spelling. Getting it wrong surfaces core-side as
// a resolution error naming the properties the resource does have, which is the
// fastest way to find the right one.
//
// Blocks until the resource's URN is known; see AwaitURN for why that is required
// rather than merely tidy.
func OutputRefTo(ctx context.Context, res sdk.Resource, property string) (*scpluginv1.OutputValue, error) {
	if property == "" {
		return nil, errors.New("output ref with an empty property name")
	}
	urn, err := AwaitURN(ctx, res)
	if err != nil {
		return nil, errors.Wrapf(err, "output ref to property %q", property)
	}
	return &scpluginv1.OutputValue{
		From: &scpluginv1.OutputValue_Ref{
			Ref: &scpluginv1.ResourcePropertyRef{Urn: urn, Property: property},
		},
	}, nil
}

// SecretRefTo is OutputRefTo for a property that must stay secret once resolved.
func SecretRefTo(ctx context.Context, res sdk.Resource, property string) (*scpluginv1.OutputValue, error) {
	v, err := OutputRefTo(ctx, res, property)
	if err != nil {
		return nil, err
	}
	v.Secret = true
	return v, nil
}

// VmHostBuilder accumulates one host's facts, and the first error that occurred
// while doing so. Every setter returns the builder, so a whole host reads as one
// expression and the error is checked once, by Build.
type VmHostBuilder struct {
	ctx  context.Context
	host *scpluginv1.VmHostFacts
	err  error

	// hostKeyDecided records that the plugin answered the host-key question
	// either way, so Build can tell "pinned nothing" from "said trust-on-first-use"
	// and refuse the contradiction of doing both.
	hostKeyDecided bool
}

// NewVmHost starts a host. `name` is how the operator will see this machine and how
// core names the resources it hangs off it, so it must be stable across deploys —
// derive it from the resource name in the request, not from anything the cloud
// generated.
//
// ctx is the one to await URNs on: pass pctx.Context() from the PulumiContext opened
// for this RPC.
func NewVmHost(ctx context.Context, name string) *VmHostBuilder {
	return &VmHostBuilder{ctx: ctx, host: &scpluginv1.VmHostFacts{Name: name}}
}

// fail records the first error and keeps the chain going, so a handler does not have
// to interleave error checks with the description of a machine.
func (b *VmHostBuilder) fail(err error) *VmHostBuilder {
	if b.err == nil {
		b.err = err
	}
	return b
}

// User is the SSH user core connects as.
func (b *VmHostBuilder) User(user string) *VmHostBuilder {
	b.host.User = user
	return b
}

// Port is the SSH port. Leave it unset for 22 — core defaults it.
func (b *VmHostBuilder) Port(port int) *VmHostBuilder {
	b.host.Port = int32(port)
	return b
}

// Role is what the machine is being prepared for: the values of
// pkg/clouds/vm.HostRole ("compose", "k3s-server", "k3s-agent").
func (b *VmHostBuilder) Role(role string) *VmHostBuilder {
	b.host.Role = role
	return b
}

// Address is the address core connects to, when the plugin already knows it.
func (b *VmHostBuilder) Address(value string) *VmHostBuilder {
	b.host.Address = OutputLiteral(value)
	return b
}

// AddressOf is Address as a property of a resource — the usual case, since the
// address is a floating IP or an instance field that does not exist yet.
func (b *VmHostBuilder) AddressOf(res sdk.Resource, property string) *VmHostBuilder {
	v, err := OutputRefTo(b.ctx, res, property)
	if err != nil {
		return b.fail(errors.Wrap(err, "address"))
	}
	b.host.Address = v
	return b
}

// AddressOutput sets the address from an sdk.StringOutput — the pattern a plugin
// that shells out to a CLI needs, since the address is parsed out of a
// local.Command's stdout at apply time, not a property on a provider resource.
//
// Awaits the value. On apply the await blocks until the upstream command has
// run and the parsed address is known. On preview the value is unknown — the
// in-tree drivers' sdk.StringOutput was treated as a known empty string in the
// same case (see cloudru_evolution.go's parseEvolutionResult preview branch,
// which returns ""), and this mirrors that: emit OutputLiteral("") so Build()
// still has a non-nil address and core-side readers see the same shape they
// used to. Everything that would actually dial SSH in preview has unknown
// resource dependencies and never runs, so a known-empty address is correct.
//
// Prefer AddressOf when the address IS a property of a Pulumi provider
// resource; this one is for the no-provider / CLI-driven case.
func (b *VmHostBuilder) AddressOutput(out sdk.StringOutput) *VmHostBuilder {
	result, err := internals.UnsafeAwaitOutput(b.ctx, out)
	if err != nil {
		return b.fail(errors.Wrap(err, "address"))
	}
	if !result.Known {
		b.host.Address = OutputLiteral("")
		return b
	}
	s, ok := result.Value.(string)
	if !ok {
		return b.fail(errors.Errorf("address output resolved to %T, not a string", result.Value))
	}
	b.host.Address = OutputLiteral(s)
	return b
}

// PrivateKey is the SSH private key core authenticates with.
//
// Marked secret on the wire regardless: core applies pulumi.ToSecret to this field
// unconditionally, and a plugin cannot opt out of that by omission. Setting it here
// too means the value is also marked for anything that reads the wire message.
func (b *VmHostBuilder) PrivateKey(pem string) *VmHostBuilder {
	b.host.PrivateKey = SecretLiteral(pem)
	return b
}

// PrivateKeyOf is PrivateKey as a property of a resource — a key pair the plugin
// generated in this same deploy, typically `privateKeyOpenssh` on a tls.PrivateKey.
func (b *VmHostBuilder) PrivateKeyOf(res sdk.Resource, property string) *VmHostBuilder {
	v, err := SecretRefTo(b.ctx, res, property)
	if err != nil {
		return b.fail(errors.Wrap(err, "private key"))
	}
	b.host.PrivateKey = v
	return b
}

// PrivateKeyPassword is the passphrase protecting the private key. Leave it unset
// for a key the plugin generated itself — core reads an absent value as the empty
// passphrase, which is what every self-keying driver means.
func (b *VmHostBuilder) PrivateKeyPassword(password string) *VmHostBuilder {
	b.host.PrivateKeyPassword = SecretLiteral(password)
	return b
}

// PrivateKeyPasswordOf is PrivateKeyPassword as a property of a resource.
func (b *VmHostBuilder) PrivateKeyPasswordOf(res sdk.Resource, property string) *VmHostBuilder {
	v, err := SecretRefTo(b.ctx, res, property)
	if err != nil {
		return b.fail(errors.Wrap(err, "private key password"))
	}
	b.host.PrivateKeyPassword = v
	return b
}

// HostKey pins the machine's SSH host key, so core verifies the host it reaches is
// the host it created instead of trusting the first answer.
//
// Only a cloud whose first boot SC controls can do this — the key has to be seeded
// into the machine before it comes up. A driver that cannot (Cloud.ru Evolution
// refuses user-data outright) calls TrustOnFirstUse instead.
func (b *VmHostBuilder) HostKey(key string) *VmHostBuilder {
	if b.hostKeyDecided {
		return b.fail(errors.New("host key set after the host-key question was already answered"))
	}
	b.host.HostKey = OutputLiteral(key)
	b.host.HostKeyPinned = true
	b.hostKeyDecided = true
	return b
}

// HostKeyOf is HostKey as a property of a resource — the public half of a host key
// the plugin generated and seeded via user-data.
func (b *VmHostBuilder) HostKeyOf(res sdk.Resource, property string) *VmHostBuilder {
	if b.hostKeyDecided {
		return b.fail(errors.New("host key set after the host-key question was already answered"))
	}
	v, err := OutputRefTo(b.ctx, res, property)
	if err != nil {
		return b.fail(errors.Wrap(err, "host key"))
	}
	b.host.HostKey = v
	b.host.HostKeyPinned = true
	b.hostKeyDecided = true
	return b
}

// TrustOnFirstUse says this driver cannot pin a host key, and that core should
// accept whatever the machine presents on the first connection.
//
// It is a separate call rather than the default so that it is a decision in the
// plugin's source, reviewable as such: it is a real weakening of the connection's
// guarantees, and the operator's `hostKey:` escape hatch exists because of it.
func (b *VmHostBuilder) TrustOnFirstUse() *VmHostBuilder {
	if b.hostKeyDecided {
		return b.fail(errors.New("trust-on-first-use set after the host-key question was already answered"))
	}
	b.host.HostKey = nil
	b.host.HostKeyPinned = false
	b.hostKeyDecided = true
	return b
}

// DependsOn names resources everything core hangs off this host must wait for — the
// instance itself, the floating-IP association, a firewall rule that has to exist
// before the SSH connection is attempted.
//
// Core rehydrates these URNs into resource handles and passes them as pulumi.DependsOn,
// so the ordering is enforced by the engine rather than by luck.
func (b *VmHostBuilder) DependsOn(resources ...sdk.Resource) *VmHostBuilder {
	urns, err := AwaitURNs(b.ctx, resources...)
	if err != nil {
		return b.fail(errors.Wrap(err, "depends on"))
	}
	b.host.DependsOn = append(b.host.DependsOn, urns...)
	return b
}

// Build returns the host, or the first error the chain hit.
//
// The two structural refusals are core's, raised here instead: core rejects a host
// with no name (it has nothing to call the resources it creates) and a host with no
// address (it has nowhere to connect). Catching them in the plugin turns a gRPC
// round-trip into a local error that names the host.
func (b *VmHostBuilder) Build() (*scpluginv1.VmHostFacts, error) {
	if b.err != nil {
		return nil, errors.Wrapf(b.err, "vm host %q", b.host.GetName())
	}
	if b.host.GetName() == "" {
		return nil, errors.New("vm host with no name")
	}
	if b.host.GetAddress() == nil {
		return nil, errors.Errorf("vm host %q has no address — core has nowhere to connect", b.host.GetName())
	}
	if !b.hostKeyDecided {
		return nil, errors.Errorf("vm host %q answered neither HostKey nor TrustOnFirstUse", b.host.GetName())
	}
	return b.host, nil
}

// BuildVmHosts builds every host and assembles the response.
//
// It refuses an empty set for the same reason core does: a VmHosts call that returns
// no hosts has reported success for a `type: vm` resource that produced no machines,
// and the deploy would then fail much later on something that looks unrelated.
func BuildVmHosts(hosts ...*VmHostBuilder) (*scpluginv1.VmHostsResponse, error) {
	if len(hosts) == 0 {
		return nil, errors.New("no vm hosts built — a VmHosts response must describe at least one machine")
	}
	out := make([]*scpluginv1.VmHostFacts, 0, len(hosts))
	for i, h := range hosts {
		facts, err := h.Build()
		if err != nil {
			return nil, errors.Wrapf(err, "host #%d", i)
		}
		out = append(out, facts)
	}
	return &scpluginv1.VmHostsResponse{Hosts: out}, nil
}
