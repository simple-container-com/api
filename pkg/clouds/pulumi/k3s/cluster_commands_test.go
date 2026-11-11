// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k3s

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	cfgK3s "github.com/simple-container-com/api/pkg/clouds/k3s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	pulumiVm "github.com/simple-container-com/api/pkg/clouds/pulumi/vm"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

const (
	tokenRemoteCommand = "command:remote:Command"

	testClusterName = "ru-cluster"
)

type clusterMocks struct {
	mu      sync.Mutex
	created []sdk.MockResourceArgs
}

func (m *clusterMocks) NewResource(args sdk.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.created = append(m.created, args)
	return args.Name + "-id", args.Inputs, nil
}

func (m *clusterMocks) Call(args sdk.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

// commands keys the cluster's remote commands by the suffix of their resource name
// ("install", "node-ready", "kubeconfig"), which is the only part of the name this
// package chooses.
func (m *clusterMocks) commands() map[string]resource.PropertyMap {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]resource.PropertyMap{}
	for _, a := range m.created {
		if a.TypeToken != tokenRemoteCommand {
			continue
		}
		name := a.Name
		if i := strings.Index(name, "--"); i >= 0 {
			name = name[:i]
		}
		// Only the cluster's own commands: the fixture's host readiness gate stands
		// in for the vm resource's and is not under test here.
		i := strings.Index(name, testClusterName+"-")
		if i < 0 {
			continue
		}
		out[name[i+len(testClusterName)+1:]] = a.Inputs
	}
	return out
}

// provisionCluster walks the real K3sCluster path against a vm resource output built
// the way pkg/clouds/pulumi/vm builds one — a host plus its readiness gate, handed
// over through params.ResourceOutputs, which is how provision.go wires a parent
// dependency (provision.go:145-172).
func provisionCluster(cfg *cfgK3s.K3sClusterConfig, mocks *clusterMocks) error {
	return sdk.RunErr(func(ctx *sdk.Context) error {
		input := api.ResourceInput{
			Descriptor: &api.ResourceDescriptor{
				Type:   cfgK3s.ResourceTypeK3sCluster,
				Name:   testClusterName,
				Config: api.Config{Config: cfg},
			},
			StackParams: &api.StackParams{Environment: "test"},
		}

		host := pulumiVm.HostFacts{
			Name:       "node",
			Address:    sdk.String("203.0.113.10").ToStringOutput(),
			User:       "root",
			Port:       22,
			PrivateKey: sdk.ToSecret(sdk.String("KEY")).(sdk.StringOutput),
			// Set, though empty: ConnectionArgs calls ApplyT on it, and a zero-value
			// StringOutput has no state to apply to. Every driver fills every Output
			// field of HostFacts for this reason.
			PrivateKeyPassword: sdk.ToSecret(sdk.String("")).(sdk.StringOutput),
		}
		gate, err := remote.NewCommand(ctx, "host-ready", &remote.CommandArgs{
			Connection: pulumiVm.ConnectionArgs(host),
			Create:     sdk.String("true"),
		})
		if err != nil {
			return err
		}
		vmOut := &pulumiVm.VmOutput{
			Hosts: []pulumiVm.HostFacts{host},
			Ready: []*remote.Command{gate},
		}

		params := pApi.ProvisionParams{
			Log:             logger.New(),
			ResourceOutputs: map[string]*api.ResourceOutput{cfg.VmResource: {Ref: vmOut}},
		}
		_, err = K3sCluster(ctx, api.Stack{Name: "test-stack"}, input, params)
		return err
	}, sdk.WithMocks("project", "stack", mocks))
}

func clusterConfig() *cfgK3s.K3sClusterConfig {
	return &cfgK3s.K3sClusterConfig{VmResource: "cluster-vm"}
}

// All three of the cluster's commands run on every provision, and each one is a
// different case of the rule in pkg/clouds/pulumi/vm/nonce.go. The two reads were
// caught first; the install was caught a day later and is the interesting one,
// because it CHANGES the machine and still has to be per-run:
//
// InstallScript's first act is to ask the node whether it is already running this
// version with these arguments, and exit 0 if it is. So the install is a guarded
// change — cheap and observable-no-op against a node that matches — and that is what
// makes a per-run trigger correct rather than wasteful. What it buys is the only
// deploy that matters: a node whose k3s died, was stopped, or was reinstalled by hand
// has drifted without a single input here moving, so a change-only trigger set skips
// the install and the node-ready read below fails the provision with nothing an
// operator can do about it but edit the config to force a change.
//
// Live-caught 2026-10-18 against a node torn down out of band with k3s-killall.sh:
// install reported up-to-date, then "no k3s node became Ready within 300s".
func TestK3sCluster_EveryCommandRunsEveryProvision(t *testing.T) {
	RegisterTestingT(t)

	mocks := &clusterMocks{}
	Expect(provisionCluster(clusterConfig(), mocks)).To(BeNil())

	cmds := mocks.commands()
	for name, why := range map[string]string{
		"install":    "guarded change: the installer checks version+args on the node and exits 0 if they match, so a stopped or reinstalled k3s is only repairable here",
		"node-ready": "reads only: whether the cluster's own API reports a Ready node",
		"kubeconfig": "reads only, and its stdout IS the exported credential",
	} {
		Expect(cmds).To(HaveKey(name))
		triggers, _ := unwrapSecret(cmds[name]["triggers"])
		Expect(triggers.String()).To(ContainSubstring(pulumiVm.RunNonce()),
			"%q is not triggered per run, and it has to be — %s", name, why)

		// A per-run trigger with no Update is a replacement on every provision, and
		// pulumi-command performs a replacement as delete-then-create. On the
		// install that would be an uninstall-reinstall of the control plane on every
		// deploy of the parent stack.
		update, _ := unwrapSecret(cmds[name]["update"])
		create, _ := unwrapSecret(cmds[name]["create"])
		Expect(update.StringValue()).To(Equal(create.StringValue()),
			"%q is triggered per run and has no Update equal to its Create", name)
		Expect(cmds[name]).ToNot(HaveKey(resource.PropertyKey("delete")),
			"%q is triggered per run and carries a Delete: a replacement would run it moments "+
				"after the Create. Put the Delete on a resource with no triggers, as the vm "+
				"ingress and the vm-compose teardown do", name)
	}
}

// The cluster's commands all carry the node's admin credentials one way or another,
// and two of them have a `--token`-shaped argument in the script itself. Logging off
// and no previous output in the environment, on every one of them: `Environment` is
// inlined into the command string by pulumi-command, and stdout/stderr of a remote
// command is printed by the engine.
func TestK3sCluster_NoCommandLogsOrForwardsOutput(t *testing.T) {
	RegisterTestingT(t)

	mocks := &clusterMocks{}
	Expect(provisionCluster(clusterConfig(), mocks)).To(BeNil())

	cmds := mocks.commands()
	Expect(cmds).To(HaveLen(3))
	for name, inputs := range cmds {
		logging, _ := unwrapSecret(inputs["logging"])
		Expect(logging.StringValue()).To(Equal("none"), "command %q logs its output", name)
		prev, _ := unwrapSecret(inputs["addPreviousOutputInEnv"])
		Expect(prev.BoolValue()).To(BeFalse(),
			"command %q forwards the previous command's output into its environment, which "+
				"pulumi-command inlines into the command string", name)
	}
}

// provisionFleet is provisionCluster for more than one machine: n servers, each with
// the public address 203.0.113.1<i> and, when withMesh, the overlay address SC would
// assign it out of the default CIDR (10.88.0.<i+1> — a pure function of the CIDR and
// the index, which is why a fixture can know it).
func provisionFleet(cfg *cfgK3s.K3sClusterConfig, mocks *clusterMocks, n int, withMesh bool) error {
	return sdk.RunErr(func(ctx *sdk.Context) error {
		input := api.ResourceInput{
			Descriptor: &api.ResourceDescriptor{
				Type:   cfgK3s.ResourceTypeK3sCluster,
				Name:   testClusterName,
				Config: api.Config{Config: cfg},
			},
			StackParams: &api.StackParams{Environment: "test"},
		}

		vmOut := &pulumiVm.VmOutput{}
		for i := 0; i < n; i++ {
			host := pulumiVm.HostFacts{
				Name:               fmt.Sprintf("node-%d", i),
				Address:            sdk.String(fmt.Sprintf("203.0.113.1%d", i)).ToStringOutput(),
				User:               "root",
				Port:               22,
				Role:               vm.RoleK3sServer,
				PrivateKey:         sdk.ToSecret(sdk.String("KEY")).(sdk.StringOutput),
				PrivateKeyPassword: sdk.ToSecret(sdk.String("")).(sdk.StringOutput),
			}
			if withMesh {
				host.MeshAddress = sdk.String(fmt.Sprintf("10.88.0.%d", i+1)).ToStringOutput()
			}
			gate, err := remote.NewCommand(ctx, fmt.Sprintf("host-%d-ready", i), &remote.CommandArgs{
				Connection: pulumiVm.ConnectionArgs(host),
				Create:     sdk.String("true"),
			})
			if err != nil {
				return err
			}
			vmOut.Hosts = append(vmOut.Hosts, host)
			vmOut.Ready = append(vmOut.Ready, gate)
		}

		params := pApi.ProvisionParams{
			Log:             logger.New(),
			ResourceOutputs: map[string]*api.ResourceOutput{cfg.VmResource: {Ref: vmOut}},
		}
		_, err := K3sCluster(ctx, api.Stack{Name: "test-stack"}, input, params)
		return err
	}, sdk.WithMocks("project", "stack", mocks))
}

// Node-to-node traffic rides the mesh when the resource has one — and this is a
// REFUSAL made good, not a preference.
//
// `validateHostRoles` refuses a multi-host k3s pool whose hosts name no
// `privateAddress`, because k3s would then advertise whichever address the default
// route uses — the public one on a single-NIC VPS — and put etcd peer traffic and
// flannel's unencrypted VXLAN on the public internet. `mesh.enable: true` LIFTS that
// refusal, in those words, on the grounds that "SC builds one across them and
// addresses the nodes on it". If the provisioning tier then addresses them on their
// public interfaces anyway, the refusal has been bypassed and the exact failure it
// exists to prevent happens — as a green deploy, which is why the validator refuses
// rather than warns.
//
// Live-caught 2026-11-11 in the preview of the first `servers: 3` run (Run E): the
// mesh came up on 10.88.0.{1,2,3} and every k3s argument named 203.0.113.x, because
// all four call sites here were written before HostFacts.MeshAddress existed and
// Caddy was the only consumer that had been switched over.
func TestK3sNodeToNodeTrafficRidesTheMeshWhenThereIsOne(t *testing.T) {
	RegisterTestingT(t)

	cfg := clusterConfig()
	cfg.Servers = 3
	mocks := &clusterMocks{}
	Expect(provisionFleet(cfg, mocks, 3, true)).To(BeNil())

	cmds := mocks.commands()
	for _, name := range []string{"install-1", "install-2"} {
		Expect(cmds).To(HaveKey(name))
		// The arg vector, which is trigger[0] on a joining node and deliberately
		// token-free (see installNode).
		triggers, _ := unwrapSecret(cmds[name]["triggers"])
		args := triggers.ArrayValue()[0].StringValue()

		// Where this node registers: host 0 ON THE OVERLAY.
		Expect(args).To(ContainSubstring("--server https://10.88.0.1:6443"),
			"%q joins over the public internet while an overlay exists", name)
		// And what it advertises itself as, which is what peers — including etcd —
		// will dial it on.
		Expect(args).To(MatchRegexp(`--node-ip 10\.88\.0\.[23]\b`))
		Expect(args).To(MatchRegexp(`--advertise-address 10\.88\.0\.[23]\b`))
		Expect(args).NotTo(ContainSubstring("--node-ip 203.0.113."))
		Expect(args).NotTo(ContainSubstring("--advertise-address 203.0.113."))

		// Every server's overlay address has to be a SAN, or the join above is an
		// x509 failure: a cluster that forms and then cannot elect a leader.
		for _, san := range []string{"10.88.0.1", "10.88.0.2", "10.88.0.3"} {
			Expect(args).To(ContainSubstring("--tls-san " + san))
		}
		// The public ones stay too. An operator and the kubeconfig dial those, and
		// a SAN dropped later costs a certificate re-issue and an API server restart.
		for _, san := range []string{"203.0.113.10", "203.0.113.11", "203.0.113.12"} {
			Expect(args).To(ContainSubstring("--tls-san " + san))
		}
	}
}

// The other half of the same contract: a fleet with no mesh is addressed exactly as
// it was before the mesh existed. EffectiveMeshAddress falls back through
// EffectivePrivateAddress to Address, so this fixture — public addresses only — is
// the shape every k3s stack that predates the mesh has, and it must not move.
func TestK3sWithoutAMeshIsAddressedExactlyAsBefore(t *testing.T) {
	RegisterTestingT(t)

	cfg := clusterConfig()
	cfg.Servers = 3
	mocks := &clusterMocks{}
	Expect(provisionFleet(cfg, mocks, 3, false)).To(BeNil())

	cmds := mocks.commands()
	triggers, _ := unwrapSecret(cmds["install-1"]["triggers"])
	args := triggers.ArrayValue()[0].StringValue()
	Expect(args).To(ContainSubstring("--server https://203.0.113.10:6443"))
	Expect(args).To(ContainSubstring("--node-ip 203.0.113.11"))
	Expect(args).To(ContainSubstring("--advertise-address 203.0.113.11"))
	Expect(args).NotTo(ContainSubstring("10.88.0."))
}

// unwrapSecret peels secret and output wrappers off a property. Same helper as the vm
// package's, repeated rather than exported: it is three lines of test scaffolding, and
// exporting test helpers across packages makes them look like API.
func unwrapSecret(pv resource.PropertyValue) (resource.PropertyValue, bool) {
	secret := false
	for {
		switch {
		case pv.IsSecret():
			secret = true
			pv = pv.SecretValue().Element
		case pv.IsOutput():
			o := pv.OutputValue()
			secret = secret || o.Secret
			pv = o.Element
		default:
			return pv, secret
		}
	}
}
