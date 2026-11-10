// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package vm provisions the machines behind SC's VM-based delivery paths: compose on
// one VM (P5) and, next, k3s that SC stands up itself (P3). One config tier
// (pkg/clouds/vm), one driver per cloud, and everything above the driver — the SSH
// connection, readiness, cloud-init, compose, k3s — written exactly once.
//
// Design: forge/docs/design/2026-10-13/sc-russia-market-readiness/vm-matrix.md (D9).
package vm

import (
	"github.com/pkg/errors"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// HostFacts is one reachable machine: everything the layers above the driver need,
// and nothing about how it came to exist.
type HostFacts struct {
	// Name is a stable, resource-name-safe identifier within the vm resource.
	Name string
	// Address is what SSH connects to. An sdk.StringOutput because a cloud driver
	// only learns it once the instance (or its floating IP) has been created.
	Address sdk.StringOutput
	User    string
	Port    int
	// PrivateKey is the deploy key, ALREADY marked secret. Driver implementations
	// must wrap it: remote.Connection.PrivateKey is a plain StringInput, so an
	// unwrapped key is printed verbatim in Pulumi's preview and update summaries and
	// archived in the CI log — the exact mechanism by which a Yandex service-account
	// private key reached a world-readable Actions log on 2026-09-30.
	PrivateKey sdk.StringOutput
	// PrivateKeyPassword is empty unless the key is an encrypted PEM; also secret.
	PrivateKeyPassword sdk.StringOutput
	// HostKey is the machine's public SSH host key as an authorized_keys line.
	//
	// An Output, and the reason is worth recording because the first version of this
	// field was a plain string on the theory that a generated host key would also be
	// known up front. It is not: for SC to know a host key before the machine boots,
	// SC has to GENERATE it, and key material that must survive to the next deploy is
	// a Pulumi resource — otherwise every deploy mints a new key, user-data changes,
	// and the machine is replaced. A resource's outputs are Outputs.
	//
	// Not secret: it is the machine's PUBLIC key, and keeping it readable is what lets
	// an operator compare it against the cloud console by eye.
	HostKey sdk.StringOutput
	// HostKeyPinned says whether HostKey carries a key at all.
	//
	// Separate from HostKey, and a plain bool, because the question it answers —
	// "will this connection verify the machine's identity?" — has to be answerable
	// while the graph is being built: it decides whether to set
	// remote.Connection.HostKey at all (passing an empty one fails every connection
	// rather than falling back) and whether to warn. The driver always knows the
	// answer, even when it does not yet know the key.
	HostKeyPinned bool
	// Role is what the machine is being prepared for.
	Role vm.HostRole
	// PrivateAddress is what the machine's SIBLINGS dial it on, where Address is
	// what SC dials from outside. Equal to Address unless the driver knows better.
	//
	// It exists because k3s does not: SC sets `--node-ip` nowhere, so k3s advertises
	// whichever address the default-route interface carries — correct on a cloud VM
	// behind a floating/NAT IP, the PUBLIC address on a single-NIC VPS, which puts
	// etcd peer traffic and flannel's unencrypted VXLAN overlay on the public
	// internet. A multi-node k3s pool is refused unless this is known (see
	// vm.VmHost.PrivateAddress); a compose fleet never needs it, because those
	// machines do not talk to each other.
	//
	// An Output for the same reason Address is: a cloud driver learns the instance's
	// internal IP only after creating it. Not secret — a private address is not a
	// credential, and redacting it would hide the one value a reader needs to tell a
	// correctly-addressed cluster from an exposed one.
	//
	// Read it through EffectivePrivateAddress, never directly: a driver written
	// before this field existed — every out-of-tree plugin, today — leaves it as the
	// zero Output, which has no OutputState and panics inside pulumi's own
	// gatherJoinSet the moment it reaches sdk.All.
	PrivateAddress sdk.StringOutput
	// MeshAddress is this machine's address on the SC-managed WireGuard overlay,
	// when the resource declares one (`mesh.enable: true`; see vm.VmMesh).
	//
	// A field of its own rather than a value written over PrivateAddress, and the
	// distinction is load-bearing in both directions. `privateAddress` is an
	// operator's assertion about their own network, which SC did not make and cannot
	// check; this is a fact SC owns outright, assigned by index out of the mesh CIDR.
	// Overwriting the operator's field with SC's value would make the config file and
	// the running fleet disagree with nothing to show for it, and would make the
	// precedence between the two unexpressible — which is exactly the question every
	// consumer has to answer.
	//
	// Unlike every other address here it is NOT an Output in substance: it is a pure
	// function of the CIDR and the host index (vm.VmConfig.MeshAddress), so nothing
	// has to be created before it is known. It is typed as one anyway, so that the
	// three addresses are interchangeable at every call site and a consumer picking
	// between them does not also have to lift one into an Output.
	//
	// Read it through EffectiveMeshAddress, never directly — same zero-Output panic
	// as PrivateAddress, and here it is the COMMON case rather than the legacy one,
	// since most resources declare no mesh at all.
	MeshAddress sdk.StringOutput
	// DependsOn are the resources that must exist before anything connects. Empty
	// for byo: the machine predates the stack.
	DependsOn []sdk.Resource
}

// MeshAddressKnown reports whether this machine is on an SC-managed overlay.
//
// Separate from the address itself, and a plain bool for the same reason
// HostKeyPinned is one: the question is asked while the graph is being built, to
// decide WHICH address to use, and that decision cannot be made inside an ApplyT
// after the value has resolved.
func (h HostFacts) MeshAddressKnown() bool {
	return h.MeshAddress != (sdk.StringOutput{})
}

// EffectiveMeshAddress is the address this machine's siblings should dial, with the
// mesh preferred over the operator's private network and the public address last.
//
// The precedence is the policy, stated once: a resource that declares a mesh
// declared it in order to be used, and a machine whose private address the operator
// also supplied is not a reason to send node-to-node traffic outside the tunnel SC
// just built. Where no mesh exists this is exactly EffectivePrivateAddress, so a
// caller that switches to it changes nothing for every resource that predates the
// mesh.
//
// Callers that specifically want the operator's own value — a diagnostic, or a
// refusal that quotes `hosts[].privateAddress` — should keep calling
// EffectivePrivateAddress. The two exist because the fleet has two different
// questions, not as a fallback chain with one right answer.
func (h HostFacts) EffectiveMeshAddress() sdk.StringOutput {
	if h.MeshAddressKnown() {
		return h.MeshAddress
	}
	return h.EffectivePrivateAddress()
}

// EffectivePrivateAddress is the address this machine's siblings should dial, with
// Address as the fallback.
//
// The fallback is a compatibility guarantee, not a convenience. Every cloud VM
// driver is an out-of-tree plugin built against an older HostFacts, so
// PrivateAddress arrives as the ZERO Output — and a zero Output carries no
// OutputState, so passing it to sdk.All does not resolve to "" but panics with a nil
// dereference inside pulumi's gatherJoinSet. An accessor is the only place that can
// be fixed once for every consumer; comparing against sdk.StringOutput{} works
// because the type is a comparable struct wrapping that state pointer.
//
// Reading Address when a plugin supplied nothing is the pre-existing behaviour, and
// it is safe where it is reached: the config tier refuses a multi-node k3s pool
// whose private addresses are unknown (vm.VmHost.PrivateAddress), so a fleet that
// needs the distinction cannot get here by this path.
func (h HostFacts) EffectivePrivateAddress() sdk.StringOutput {
	if h.PrivateAddress == (sdk.StringOutput{}) {
		return h.Address
	}
	return h.PrivateAddress
}

// Driver is the per-cloud half of the VM path.
//
// Narrower than the nine-method interface sketched in vm-matrix.md §1.1 on purpose.
// That sketch enumerates the cloud primitives (Network/Firewall/Key/Instance/
// PublicIP/LoadBalancer/Zones) as separate methods. Two drivers in, the seam has
// stayed exactly this wide and the primitives have stayed private to the driver —
// `cloudru-vm` creates all seven of them inside its own Hosts() and the layers above
// never named one. Splitting them out would publish an abstraction over one cloud's
// vocabulary (Cloud.ru's EIP is not GCP's, and its "subnet" carries the gateway and
// the DNS servers) without a second cloud to check it against. Resist until there is.
//
// What is stable is the seam every consumer uses: give me reachable hosts.
type Driver interface {
	// Name is the `provider:` value this driver answers to.
	Name() string
	// Capabilities is what the cloud behind this driver physically does.
	Capabilities() vm.Capabilities
	// Hosts returns the machines of this resource, creating them first if the driver
	// is a cloud driver. It must not wait for readiness — that is shared code
	// (ready.go) and identical across clouds.
	//
	// `ports` is every hole to open in the machine's firewall, already resolved:
	// the base set, the resource's `ingressPorts`, and the presets inferred from the
	// consumers that name this resource in the parent stack (firewall.go). Passed in
	// rather than derived from cfg because that inference needs the whole parent
	// stack, which a driver is deliberately not given. It is empty — and must then
	// be ignored — for a driver whose cloud SC cannot write firewall rules in.
	Hosts(
		ctx *sdk.Context,
		input api.ResourceInput,
		cfg *vm.VmConfig,
		ports []vm.PortRange,
		params pApi.ProvisionParams,
	) ([]HostFacts, error)
}

// driverFor resolves the driver named by the resource's `provider:` field.
//
// It assumes cfg.Validate() has already run: that is where an unknown or
// not-yet-implemented provider gets its explanatory refusal, and duplicating those
// messages here would let the two drift.
func driverFor(cfg *vm.VmConfig) (Driver, error) {
	switch cfg.Driver {
	case vm.DriverByo:
		return byoDriver{}, nil
	default:
		// Not in-tree — it may still be a driver a loaded plugin advertised. The
		// in-tree switch comes first for the same reason vm.LookupDriver checks the
		// built-in table first: a plugin may not shadow a driver core ships.
		if driver, ok, err := pluginDriverFor(cfg); err != nil {
			return nil, err
		} else if ok {
			return driver, nil
		}
		return nil, errors.Errorf("no vm driver for provider %q", cfg.Driver)
	}
}
