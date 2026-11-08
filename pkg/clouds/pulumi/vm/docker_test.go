// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"testing"

	. "github.com/onsi/gomega"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// cloudInitDriver stands in for any cloud vm plugin: a machine SC created, whose
// user-data already installed docker before SSH was ever opened.
type cloudInitDriver struct{ byoDriver }

func (cloudInitDriver) Capabilities() vm.Capabilities {
	return vm.Capabilities{CloudInit: true}
}

func (cloudInitDriver) Hosts(
	*sdk.Context, api.ResourceInput, *vm.VmConfig, []vm.PortRange, pApi.ProvisionParams,
) ([]HostFacts, error) {
	return nil, nil
}

func ingressed() *vm.VmConfig {
	return &vm.VmConfig{Ingress: &vm.VmIngress{Enable: true, Email: "ops@example.com"}}
}

// The gating matrix, which is the whole decision: putting docker on a machine SC
// does not own is not free, and neither is leaving it off one that needs it.
func TestHostNeedsDocker(t *testing.T) {
	RegisterTestingT(t)

	for _, tc := range []struct {
		name    string
		cfg     *vm.VmConfig
		role    vm.HostRole
		drv     Driver
		want    bool
		because string
	}{
		{
			name:    "byo compose host",
			cfg:     &vm.VmConfig{},
			role:    vm.RoleCompose,
			drv:     byoDriver{},
			want:    true,
			because: "this host exists to run `docker compose`",
		},
		{
			name:    "byo compose host with an ingress",
			cfg:     ingressed(),
			role:    vm.RoleCompose,
			drv:     byoDriver{},
			want:    true,
			because: "twice over: the stacks and the Caddy container",
		},
		{
			name: "byo k3s server, no ingress",
			cfg:  &vm.VmConfig{},
			role: vm.RoleK3sServer,
			drv:  byoDriver{},
			want: false,
			because: "k3s brings its own containerd, and docker's `iptables -P FORWARD DROP` " +
				"is a live hazard next to flannel",
		},
		{
			name:    "byo k3s agent, no ingress",
			cfg:     &vm.VmConfig{},
			role:    vm.RoleK3sAgent,
			drv:     byoDriver{},
			want:    false,
			because: "same as a server",
		},
		{
			name: "byo k3s server WITH an ingress",
			cfg:  ingressed(),
			role: vm.RoleK3sServer,
			drv:  byoDriver{},
			want: true,
			because: "the vm resource's Caddy is a docker container whatever else the machine " +
				"is for, and this combination is permitted (the port-80 race is refused " +
				"separately, in pkg/clouds/pulumi/k3s)",
		},
		{
			name:    "cloud-init machine, compose",
			cfg:     ingressed(),
			role:    vm.RoleCompose,
			drv:     cloudInitDriver{},
			want:    false,
			because: "user-data installed it on first boot; opening an SSH session to find that out is waste",
		},
		{
			name:    "cloud-init machine, k3s",
			cfg:     &vm.VmConfig{},
			role:    vm.RoleK3sServer,
			drv:     cloudInitDriver{},
			want:    false,
			because: "neither reason applies",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(hostNeedsDocker(tc.cfg, HostFacts{Name: "h", Role: tc.role}, tc.drv)).
				To(Equal(tc.want), tc.because)
		})
	}
}

// A host whose `role:` was left empty is every pre-role config in existence, and
// EffectiveRole resolves it to `compose` before it ever reaches HostFacts — so the
// empty role must never read as "not compose" and skip the install. Pinned because
// the test above can only exercise concrete roles.
func TestHostNeedsDockerTreatsTheDefaultRoleAsCompose(t *testing.T) {
	RegisterTestingT(t)

	// What byo.go actually writes into HostFacts.Role, for a host that declared none.
	Expect(vm.VmHost{Address: "203.0.113.10"}.EffectiveRole()).To(Equal(vm.RoleCompose))

	Expect(hostNeedsDocker(&vm.VmConfig{}, HostFacts{Role: vm.RoleCompose}, byoDriver{})).To(BeTrue())
	// And the bare zero value, which is what a driver written against an older
	// HostFacts would leave behind, is not accidentally read as k3s.
	Expect(hostNeedsDocker(&vm.VmConfig{}, HostFacts{}, byoDriver{})).To(BeTrue())
}
