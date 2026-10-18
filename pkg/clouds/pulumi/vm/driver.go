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
	// HostKey is the machine's public SSH host key as an authorized_keys line, or
	// empty when the operator did not pin one.
	//
	// A plain string, not an Output, unlike Address: a machine SC creates will not
	// learn its host key from the cloud API either (no cloud returns it), so the
	// future answer there is to GENERATE the host key in cloud-init and know it
	// before the machine boots — which is also a plain string. Making it an Output
	// would additionally make "is it set?" unknowable while the graph is being
	// built, and that question decides whether the connection verifies anything.
	HostKey string
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
// PublicIP/LoadBalancer/Zones), and their input and output shapes cannot honestly be
// designed against a single driver — the first byo driver creates none of them. They
// arrive with `cloudru-vm`, the first driver that actually has to create a machine,
// when the shapes can be read off two clouds instead of guessed from zero.
//
// What is stable today is the seam every consumer uses: give me reachable hosts.
type Driver interface {
	// Name is the `provider:` value this driver answers to.
	Name() string
	// Capabilities is what the cloud behind this driver physically does.
	Capabilities() vm.Capabilities
	// Hosts returns the machines of this resource, creating them first if the driver
	// is a cloud driver. It must not wait for readiness — that is shared code
	// (ready.go) and identical across clouds.
	Hosts(ctx *sdk.Context, input api.ResourceInput, cfg *vm.VmConfig, params pApi.ProvisionParams) ([]HostFacts, error)
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
		return nil, errors.Errorf("no vm driver for provider %q", cfg.Driver)
	}
}
