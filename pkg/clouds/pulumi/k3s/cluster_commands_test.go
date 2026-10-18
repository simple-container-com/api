// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k3s

import (
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
