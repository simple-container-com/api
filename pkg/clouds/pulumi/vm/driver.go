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
	// DependsOn are the resources that must exist before anything connects. Empty
	// for byo: the machine predates the stack.
	DependsOn []sdk.Resource
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
