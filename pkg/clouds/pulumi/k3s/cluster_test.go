// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k3s

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"

	"github.com/simple-container-com/api/pkg/api"
	cfgK3s "github.com/simple-container-com/api/pkg/clouds/k3s"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pulumiVm "github.com/simple-container-com/api/pkg/clouds/pulumi/vm"
	"github.com/simple-container-com/api/pkg/clouds/vm"
	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
)

func TestDisableSet(t *testing.T) {
	RegisterTestingT(t)

	for _, tc := range []struct {
		name string
		cfg  cfgK3s.K3sClusterConfig
		vm   pulumiVm.VmOutput
		want []string
	}{
		{
			name: "unset disables traefik",
			cfg:  cfgK3s.K3sClusterConfig{VmResource: "cluster-vm"},
			want: []string{"traefik"},
		},
		{
			name: "unset with compose ingress also disables servicelb",
			cfg:  cfgK3s.K3sClusterConfig{VmResource: "cluster-vm"},
			vm:   pulumiVm.VmOutput{IngressEnabled: true},
			want: []string{"traefik", "servicelb"},
		},
		{
			name: "explicit empty list keeps everything",
			cfg:  cfgK3s.K3sClusterConfig{VmResource: "cluster-vm", Disable: []string{}},
			want: []string{},
		},
		{
			name: "explicit list wins",
			cfg:  cfgK3s.K3sClusterConfig{VmResource: "cluster-vm", Disable: []string{"metrics-server"}},
			vm:   pulumiVm.VmOutput{IngressEnabled: true},
			want: []string{"metrics-server"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			Expect(disableSet(&tc.cfg, &tc.vm)).To(Equal(tc.want))
		})
	}
}

// TestDisableSetSurvivesPlaceholderWalk is an end-to-end guard on the nil-vs-empty
// distinction disableSet is built on.
//
// Every parsed config is walked by the placeholder resolver before a provisioner sees
// it, and that walk used to turn every nil slice into an empty-but-present one
// (reflect.MakeSlice never returns nil). The unit test above therefore passed while
// the live behaviour was the opposite: `disable` arrived non-nil, the traefik default
// never applied, and the first `type: LoadBalancer` Service on the cluster sat
// `<pending>` behind a klipper-lb pod that could not bind the ports traefik held.
// Caught on the first live k3s smoke, 2026-10-18; fixed in
// provisioner/placeholders/placeholders.go.
func TestDisableSetSurvivesPlaceholderWalk(t *testing.T) {
	RegisterTestingT(t)

	cfg := cfgK3s.K3sClusterConfig{VmResource: "cluster-vm"}
	Expect(placeholders.New().Apply(&cfg)).To(BeNil())

	Expect(cfg.Disable).To(BeNil(), "an unset `disable` must stay nil through the placeholder walk")
	Expect(disableSet(&cfg, &pulumiVm.VmOutput{})).To(Equal([]string{"traefik"}))
}

// rolesOf is the one place the server/agent split is decided, and the table below is
// the whole decision. Two sources, and which wins is not a preference: a byo pool
// that names its own roles has an operator who knows which box has the disk, the
// memory and the stable address, and SC cannot learn that.
func TestRolesOf(t *testing.T) {
	pool := func(roles ...vm.HostRole) *pulumiVm.VmOutput {
		return &pulumiVm.VmOutput{
			Hosts: lo.Map(roles, func(r vm.HostRole, i int) pulumiVm.HostFacts {
				return pulumiVm.HostFacts{Name: fmt.Sprintf("host-%d", i), Role: r}
			}),
		}
	}
	input := api.ResourceInput{
		Descriptor:  &api.ResourceDescriptor{Name: "cluster"},
		StackParams: &api.StackParams{Environment: "staging"},
	}

	for _, tc := range []struct {
		name    string
		servers int
		out     *pulumiVm.VmOutput
		want    []vm.HostRole
		wantErr string
	}{
		{
			// The default, and the topology worth reaching for first: it schedules
			// pods across the whole fleet and survives the loss of any agent.
			name: "undeclared pool defaults to one server and the rest agents",
			out:  pool(vm.RoleCompose, vm.RoleCompose, vm.RoleCompose),
			want: []vm.HostRole{vm.RoleK3sServer, vm.RoleK3sAgent, vm.RoleK3sAgent},
		},
		{
			name: "a single undeclared host is the lone server",
			out:  pool(vm.RoleCompose),
			want: []vm.HostRole{vm.RoleK3sServer},
		},
		{
			// By INDEX, and that is load-bearing: raising the agent count must never
			// convert an existing agent into an etcd member, which is not undone by
			// putting the number back.
			name:    "servers 3 takes the first three hosts",
			servers: 3,
			out:     pool(vm.RoleCompose, vm.RoleCompose, vm.RoleCompose, vm.RoleCompose),
			want: []vm.HostRole{
				vm.RoleK3sServer, vm.RoleK3sServer, vm.RoleK3sServer, vm.RoleK3sAgent,
			},
		},
		{
			name:    "more servers than hosts is refused",
			servers: 3,
			out:     pool(vm.RoleCompose, vm.RoleCompose),
			wantErr: "every server needs a machine of its own",
		},
		{
			name: "a declared pool is taken as written",
			out:  pool(vm.RoleK3sServer, vm.RoleK3sAgent, vm.RoleK3sAgent),
			want: []vm.HostRole{vm.RoleK3sServer, vm.RoleK3sAgent, vm.RoleK3sAgent},
		},
		{
			name: "a declared three-server pool needs no servers field",
			out:  pool(vm.RoleK3sServer, vm.RoleK3sServer, vm.RoleK3sServer),
			want: []vm.HostRole{vm.RoleK3sServer, vm.RoleK3sServer, vm.RoleK3sServer},
		},
		{
			name:    "a servers field agreeing with the declaration is allowed",
			servers: 1,
			out:     pool(vm.RoleK3sServer, vm.RoleK3sAgent),
			want:    []vm.HostRole{vm.RoleK3sServer, vm.RoleK3sAgent},
		},
		{
			// Refused rather than silently resolved, because either outcome of a
			// silent resolution puts a machine in a role its owner did not pick.
			name:    "a servers field contradicting the declaration is refused",
			servers: 3,
			out:     pool(vm.RoleK3sServer, vm.RoleK3sAgent, vm.RoleK3sAgent),
			wantErr: "can only contradict it",
		},
		{
			// Not an arbitrary convention: SC installs on host 0 first, reads the
			// join token there and exports the kubeconfig from there, so an agent in
			// that slot would have to join a cluster that does not exist yet.
			name:    "an agent in the first slot is refused",
			out:     pool(vm.RoleK3sAgent, vm.RoleK3sServer),
			wantErr: "the FIRST host must be a",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			roles, err := rolesOf(
				&cfgK3s.K3sClusterConfig{VmResource: "vm", Servers: tc.servers}, input, tc.out,
			)
			if tc.wantErr != "" {
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring(tc.wantErr))
				return
			}
			Expect(err).To(BeNil())
			Expect(roles).To(Equal(tc.want))
		})
	}
}

// Roadmap item sc-vm-k3s-port80-race-not-validated: the vm's Caddy ingress and k3s's
// own ingress/LB controllers both want :80, and until this refusal SC provisioned the
// combination and let them race. The default disable set already avoids it, so the
// only way here is an explicit `disable:` that puts one of them back.
func TestValidateIngressPortsRefusesTheRace(t *testing.T) {
	input := api.ResourceInput{
		Descriptor:  &api.ResourceDescriptor{Name: "cluster"},
		StackParams: &api.StackParams{Environment: "staging"},
	}

	for _, tc := range []struct {
		name    string
		ingress bool
		disable []string
		wantErr string
	}{
		{
			name:    "no vm ingress: the cluster owns the edge and nothing conflicts",
			disable: nil,
		},
		{
			name:    "the default disable set on an ingressed host is fine",
			ingress: true,
			disable: []string{componentTraefik, componentServiceLB},
		},
		{
			// `disable: []` is the documented escape hatch for an operator who wants
			// traefik after all — and on an ingressed host it is the exact fault.
			name:    "an empty explicit disable on an ingressed host is refused",
			ingress: true,
			disable: []string{},
			wantErr: "traefik and servicelb",
		},
		{
			name:    "re-enabling only servicelb is refused and names only it",
			ingress: true,
			disable: []string{componentTraefik},
			wantErr: "leaves servicelb enabled",
		},
		{
			name:    "re-enabling only traefik is refused and names only it",
			ingress: true,
			disable: []string{componentServiceLB},
			wantErr: "leaves traefik enabled",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			err := validateIngressPorts(
				&cfgK3s.K3sClusterConfig{VmResource: "vm"}, input,
				&pulumiVm.VmOutput{IngressEnabled: tc.ingress}, tc.disable,
			)
			if tc.wantErr == "" {
				Expect(err).To(BeNil())
				return
			}
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring(tc.wantErr))
			// The remedy has to be in the message: an operator who set `disable`
			// explicitly did so for a reason, and "pick one of these two" is the
			// only actionable thing to say.
			Expect(err.Error()).To(ContainSubstring("`ingress.enable: false`"))
		})
	}
}

// The front door of a k3s cluster is every node, and this is the value that says so.
//
// Asserted on the rendered string rather than through a provision, because the single
// thing that can go wrong here is silent: a list the child stack cannot split back
// apart ends up in an A record's value, and a record whose value is not an address is
// still a successful deploy.
func TestIngressAddressesJoinsEveryNodeInOrder(t *testing.T) {
	RegisterTestingT(t)

	for name, tc := range map[string]struct {
		addresses []string
		want      string
	}{
		// The default topology, and the one that must keep behaving exactly as it did
		// before this export existed: one node, one address, one record.
		"single node": {[]string{"203.0.113.10"}, "203.0.113.10"},
		"three nodes": {[]string{"203.0.113.10", "203.0.113.11", "203.0.113.12"}, "203.0.113.10,203.0.113.11,203.0.113.12"},
	} {
		t.Run(name, func(t *testing.T) {
			RegisterTestingT(t)

			hosts := lo.Map(tc.addresses, func(a string, _ int) pulumiVm.HostFacts {
				return pulumiVm.HostFacts{Address: sdk.String(a).ToStringOutput()}
			})
			Expect(sdk.RunErr(func(ctx *sdk.Context) error {
				res, err := internals.UnsafeAwaitOutput(ctx.Context(), ingressAddresses(hosts))
				if err != nil {
					return err
				}
				Expect(res.Value).To(Equal(tc.want))
				return nil
			}, sdk.WithMocks("project", "stack", &clusterMocks{}))).To(BeNil())
		})
	}
}

// The producer half of the `${resource:<cluster>.kubeconfig}` chain. Pinned
// because the key is a string shared across three packages — k8s declares it,
// k3s publishes it, pApi looks it up — and a rename on any one side fails at
// provision time as "publishes no output", pointing at the operator's server.yaml
// rather than at the rename.
func TestK3sOutput_OutputValue(t *testing.T) {
	RegisterTestingT(t)

	out := &K3sOutput{
		Kubeconfig: sdk.String("apiVersion: v1\n").ToStringOutput(),
		Endpoint:   sdk.String("203.0.113.10").ToStringOutput(),
	}

	Expect(out.PublishedOutputKeys()).To(ConsistOf("endpoint", k8s.KubeconfigRefKey))

	Expect(sdk.RunErr(func(ctx *sdk.Context) error {
		for key, want := range map[string]string{
			k8s.KubeconfigRefKey: "apiVersion: v1\n",
			"endpoint":           "203.0.113.10",
		} {
			value, ok := out.OutputValue(key)
			Expect(ok).To(BeTrue(), "k3s publishes no %q", key)
			res, err := internals.UnsafeAwaitOutput(ctx.Context(), value)
			if err != nil {
				return err
			}
			Expect(res.Value).To(Equal(want))
		}
		return nil
	}, sdk.WithMocks("project", "stack", &clusterMocks{}))).To(BeNil())

	// Ready is a resource, not a string: it cannot cross a reference, and
	// answering with a zero output would hand a consumer an UNKNOWN that never
	// resolves.
	_, ok := out.OutputValue("ready")
	Expect(ok).To(BeFalse())

	// Reached through the provisioner's ordering pass before the cluster exists.
	var absent *K3sOutput
	_, ok = absent.OutputValue(k8s.KubeconfigRefKey)
	Expect(ok).To(BeFalse())
}
