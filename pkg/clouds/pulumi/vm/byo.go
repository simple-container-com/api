// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"fmt"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// byoDriver is the zeroth cloud: the operator brought the machine.
//
// It creates nothing, which is exactly why it is first. Everything the VM path is
// made of above the machine — cloud-init's output, readiness, image delivery, the
// compose render, Caddy, blue-green, k3s — is proven against a hand-made host with
// no provider bridged, in any cloud on earth or on a laptop
// (compose-on-vm.md §14.1).
type byoDriver struct{}

func (byoDriver) Name() string { return vm.DriverByo }

func (byoDriver) Capabilities() vm.Capabilities {
	return vm.Drivers[vm.DriverByo].Capabilities
}

func (byoDriver) Hosts(ctx *sdk.Context, input api.ResourceInput, cfg *vm.VmConfig, params pApi.ProvisionParams) ([]HostFacts, error) {
	hosts := make([]HostFacts, 0, len(cfg.Hosts))
	for i, h := range cfg.Hosts {
		params.Log.Info(ctx.Context(), "vm %q uses byo host %q (user %q)",
			input.Descriptor.Name, h.Address, h.EffectiveUser(cfg.SshUser))

		hosts = append(hosts, HostFacts{
			Name: fmt.Sprintf("%s-host-%d", input.Descriptor.Name, i),
			// Known statically — there is nothing to create and therefore nothing
			// to wait on — but still an Output, so a cloud driver that genuinely
			// has to resolve an address later is not a signature change here.
			Address: sdk.String(h.Address).ToStringOutput(),
			User:    h.EffectiveUser(cfg.SshUser),
			Port:    h.EffectivePort(),
			// Both halves marked secret here, at the single point where they enter
			// Pulumi. See the HostFacts.PrivateKey comment for why that is not
			// optional.
			PrivateKey:         secretString(h.PrivateKey),
			PrivateKeyPassword: secretString(h.PrivateKeyPassword),
			// A byo host is always a compose host for now; k3s on a byo host is the
			// next slice (roadmap item sc-k3s-cluster-resource, K.0) and will pass
			// its own role in.
			Role: vm.RoleCompose,
		})
	}
	return hosts, nil
}

// secretString wraps a credential so Pulumi treats it as a secret, which also
// encrypts it in the checkpoint rather than storing it in plaintext.
//
// The same helper exists in pkg/clouds/pulumi/yandex/credentials.go for the same
// reason; it is duplicated rather than shared because a secret-marking helper is
// three lines and the alternative is a utility package that every provider imports
// for one function.
func secretString(value string) sdk.StringOutput {
	return sdk.ToSecret(sdk.String(value)).(sdk.StringOutput)
}
