// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"strings"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// assignMeshAddresses fills in HostFacts.MeshAddress for every machine of a resource
// that declares a mesh.
//
// Called before anything reads the hosts, so that every consumer — the exports, the
// certificate store, k3s — sees the same addresses. It is a pure computation (the
// CIDR and the index; see vm.VmConfig.MeshAddress), which is what lets it run here
// rather than after the machines exist, and what lets the store's endpoint be a
// constant in a rendered config instead of an Output threaded between hosts.
//
// Deliberately leaves MeshAddress as the zero Output when no mesh is declared, which
// is what EffectiveMeshAddress keys off — "no mesh" has to be distinguishable from
// "a mesh whose address is empty".
func assignMeshAddresses(cfg *vm.VmConfig, hosts []HostFacts) error {
	if !cfg.MeshEnabled() {
		return nil
	}
	for i := range hosts {
		addr, err := cfg.MeshAddress(i)
		if err != nil {
			return errors.Wrapf(err, "failed to assign a mesh address to host %q", hosts[i].Name)
		}
		hosts[i].MeshAddress = sdk.String(addr).ToStringOutput()
	}
	return nil
}

// ensureMesh builds the WireGuard overlay across the resource's machines and returns
// one gate per host for everything that needs node-to-node reachability.
//
// TWO PASSES WITH A HARD BARRIER BETWEEN THEM, and the barrier is the whole shape of
// this function. A machine's peer list names its siblings' PUBLIC KEYS, so no
// machine can be configured until every machine has generated one. Pass 1 installs
// `wireguard-tools` and generates the keypair on each host, printing only the public
// half; pass 2 writes each host's config naming the other N-1 and brings the
// interface up. Every pass-2 command DependsOn every pass-1 command — expressed in
// the Pulumi graph, not by ordering the loop, because Pulumi runs resources
// concurrently and a loop's order is not an ordering.
//
// The shape it replaces, worth naming so it is not re-proposed: a single loop that
// generates and configures host by host. That cannot work — host 0 would be
// configured before host 1 has a key — and it fails in the way this package keeps
// having to guard against: the interface comes up, the provision is green, and the
// mesh carries nothing until the next provision happens to run in a lucky order.
func ensureMesh(
	ctx *sdk.Context,
	input api.ResourceInput,
	cfg *vm.VmConfig,
	hosts []HostFacts,
	ready []*remote.Command,
	params pApi.ProvisionParams,
) ([]*remote.Command, error) {
	if !cfg.MeshEnabled() {
		return nil, nil
	}
	if len(hosts) < 2 {
		// The config tier refuses this (validateMesh), so reaching it means a driver
		// returned a different number of machines than the config described.
		return nil, errors.Errorf(
			"vm %q declares a mesh but has %d host(s); a mesh needs at least two",
			input.Descriptor.Name, len(hosts),
		)
	}
	prefixLen, err := cfg.MeshPrefixLen()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to resolve the mesh range of vm %q", input.Descriptor.Name)
	}

	keygen, err := meshKeygen(ctx, input, hosts, ready)
	if err != nil {
		return nil, err
	}
	// The barrier. Every host's configuration waits on EVERY host's key, including
	// its own.
	barrier := make([]sdk.Resource, 0, len(keygen))
	for _, cmd := range keygen {
		barrier = append(barrier, cmd)
	}

	gates := make([]*remote.Command, len(hosts))
	for i, h := range hosts {
		gate, err := meshConfigure(ctx, input, cfg, hosts, keygen, i, prefixLen, barrier)
		if err != nil {
			return nil, err
		}
		gates[i] = gate
		if err := meshTeardown(ctx, input, h); err != nil {
			return nil, err
		}
	}

	addrs := make([]string, 0, len(hosts))
	for i := range hosts {
		addr, err := cfg.MeshAddress(i)
		if err != nil {
			return nil, err
		}
		addrs = append(addrs, addr)
	}
	params.Log.Info(ctx.Context(),
		"vm %q builds a WireGuard mesh %q across %d host(s) on %s (%s), listening on udp/%d; node-to-node "+
			"traffic uses these addresses and not the machines' public ones",
		input.Descriptor.Name, vm.MeshInterface, len(hosts), cfg.MeshCidr(),
		strings.Join(addrs, ", "), cfg.MeshPort())
	return gates, nil
}

// meshKeygen is pass 1: one command per host, each printing that host's PUBLIC key.
func meshKeygen(
	ctx *sdk.Context,
	input api.ResourceInput,
	hosts []HostFacts,
	ready []*remote.Command,
) ([]*remote.Command, error) {
	script := vm.MeshKeygenScript()
	out := make([]*remote.Command, len(hosts))
	for i, h := range hosts {
		cmd, err := remote.NewCommand(ctx, input.ToResName(h.Name+"-mesh-key"), &remote.CommandArgs{
			Connection: ConnectionArgs(h),
			Create:     sdk.String(script),
			Update:     sdk.String(script),
			// Guarded change AND a read in one command, which is why it gets the
			// nonce on both counts. Guarded: the script installs nothing that is
			// already installed and generates no key that already exists, so a
			// no-op provision costs one SSH round trip. A read: the stdout is the
			// machine's public key, and nothing in the trigger set moves when a host
			// is re-imaged — so without the nonce the key answered forever out of the
			// checkpoint would be the OLD machine's, and every sibling's peer list
			// would name a key that no longer exists. That failure is silent: the
			// interface comes up and simply never handshakes.
			Triggers: TriggersForGuardedChange(h.Address, sdk.String(hashOf(script))),
			// Not silenced, unlike most commands in this package: this script's
			// stdout is the value the caller consumes, and its stderr is where the
			// install narrates itself. Deliberately NOT AdditionalSecretOutputs —
			// the private key never leaves the machine (see vm.MeshKeygenScript), so
			// what is here is a public key, and redacting it would hide the one value
			// an operator needs to compare against `wg show` on the host.
			AddPreviousOutputInEnv: sdk.Bool(false),
		}, sdk.DependsOn([]sdk.Resource{ready[i]}))
		if err != nil {
			return nil, errors.Wrapf(err,
				"failed to generate the mesh key of host %q of vm %q", h.Name, input.Descriptor.Name)
		}
		out[i] = cmd
	}
	return out, nil
}

// meshConfigure is pass 2 for host i: render its peer list and apply it.
func meshConfigure(
	ctx *sdk.Context,
	input api.ResourceInput,
	cfg *vm.VmConfig,
	hosts []HostFacts,
	keygen []*remote.Command,
	i, prefixLen int,
	barrier []sdk.Resource,
) (*remote.Command, error) {
	h := hosts[i]
	selfAddr, err := cfg.MeshAddress(i)
	if err != nil {
		return nil, err
	}

	// Every peer's public key and public endpoint, in one All so the apply sees a
	// complete fleet or nothing. Index arithmetic: peer j contributes keys[2j] and
	// addresses[2j+1].
	inputs := make([]any, 0, 2*len(hosts))
	for j := range hosts {
		inputs = append(inputs, keygen[j].Stdout, hosts[j].Address)
	}
	conf := sdk.All(inputs...).ApplyT(func(vals []any) (string, error) {
		spec := vm.MeshSpec{
			Name:        h.Name,
			MeshAddress: selfAddr,
			PrefixLen:   prefixLen,
			ListenPort:  cfg.MeshPort(),
		}
		for j := range hosts {
			if j == i {
				continue
			}
			peerAddr, err := cfg.MeshAddress(j)
			if err != nil {
				return "", err
			}
			pubKey, _ := vals[2*j].(string)
			endpoint, _ := vals[2*j+1].(string)
			spec.Peers = append(spec.Peers, vm.MeshPeer{
				Name:        hosts[j].Name,
				PublicKey:   strings.TrimSpace(pubKey),
				MeshAddress: peerAddr,
				Endpoint:    strings.TrimSpace(endpoint),
			})
		}
		return vm.RenderMeshConfig(spec)
	}).(sdk.StringOutput)

	script := conf.ApplyT(vm.MeshUpScript).(sdk.StringOutput)

	cmd, err := remote.NewCommand(ctx, input.ToResName(h.Name+"-mesh-up"), &remote.CommandArgs{
		Connection: ConnectionArgs(h),
		Create:     script,
		Update:     script,
		// Guarded change: the script rewrites a file with identical bytes and then
		// either syncs the peer set in place (no session dropped) or does nothing
		// observable. Cheap, and it carries no Delete — the teardown is its own
		// resource for the reason the ingress learned on 2026-10-18.
		//
		// Triggered on the rendered config itself rather than a hash of it: it is not
		// a secret (the private key is referenced by path, never inlined), so the
		// preview diff showing a changed peer list is a feature. The nonce is what
		// repairs a host whose interface was taken down out of band.
		Triggers:               TriggersForGuardedChange(h.Address, conf),
		Logging:                remote.LoggingNone.ToLoggingPtrOutput(),
		AddPreviousOutputInEnv: sdk.Bool(false),
	}, sdk.DependsOn(append([]sdk.Resource{}, barrier...)))
	if err != nil {
		return nil, errors.Wrapf(err,
			"failed to configure the mesh on host %q of vm %q", h.Name, input.Descriptor.Name)
	}
	return cmd, nil
}

// meshTeardown registers the one thing that removes SC's interface from a machine
// when the vm resource is destroyed.
//
// Its own resource with NO triggers, for the reason nonce.go's third precondition
// states: the pass-2 command is replaced on every provision by its nonce, and
// pulumi-command performs a replacement as create-then-delete — so a Delete there
// would tear the mesh down moments after bringing it up, green, with the fleet's
// certificate store unreachable until the next provision.
func meshTeardown(ctx *sdk.Context, input api.ResourceInput, h HostFacts) error {
	if _, err := remote.NewCommand(ctx, input.ToResName(h.Name+"-mesh-teardown"), &remote.CommandArgs{
		Connection:             ConnectionArgs(h),
		Create:                 sdk.String("true"),
		Delete:                 sdk.String(vm.MeshTeardownScript()),
		AddPreviousOutputInEnv: sdk.Bool(false),
	}); err != nil {
		return errors.Wrapf(err,
			"failed to register the mesh teardown of host %q of vm %q", h.Name, input.Descriptor.Name)
	}
	return nil
}
