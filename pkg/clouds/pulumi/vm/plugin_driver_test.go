// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	. "github.com/onsi/gomega"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// --- wire-shape helpers, so the tests read as the plugin author would write them ---

func wireLiteral(v string) *scpluginv1.OutputValue {
	return &scpluginv1.OutputValue{From: &scpluginv1.OutputValue_Literal{Literal: v}}
}

func wireSecretLiteral(v string) *scpluginv1.OutputValue {
	return &scpluginv1.OutputValue{From: &scpluginv1.OutputValue_Literal{Literal: v}, Secret: true}
}

func wireRef(urn, property string) *scpluginv1.OutputValue {
	return &scpluginv1.OutputValue{From: &scpluginv1.OutputValue_Ref{
		Ref: &scpluginv1.ResourcePropertyRef{Urn: urn, Property: property},
	}}
}

// pluginVmConfig is a config naming a plugin-advertised driver, with the capability
// row the plugin would have advertised alongside it.
func pluginVmConfig() *vm.VmConfig {
	return &vm.VmConfig{
		Driver:   "fake-cloud-vm",
		Region:   "ru-central1",
		PublicIP: true,
	}
}

func pluginVmRow() vm.DriverInfo {
	return vm.DriverInfo{
		Implemented: true,
		RoadmapItem: "sc-plugin-architecture",
		Capabilities: vm.Capabilities{
			CloudInit:      true,
			PrivateNetwork: true,
			FloatingIP:     true,
			SecurityGroups: true,
		},
	}
}

// registerPluginVm puts a driver in both tiers, the way RegisterSidecars does.
func registerPluginVm(t *testing.T, call VmHostsCall) {
	t.Helper()
	if err := vm.RegisterPluginDriver("sc-plugin-fake", "fake-cloud-vm", pluginVmRow()); err != nil {
		t.Fatalf("register plugin driver: %v", err)
	}
	RegisterPluginClient("sc-plugin-fake", call)
	t.Cleanup(func() {
		vm.ResetPluginDrivers()
		ResetPluginClients()
	})
}

// registerFakeMachine creates a resource in the mocked context and returns its URN,
// so a test can hand the plugin driver a ref to something that genuinely exists.
func registerFakeMachine(ctx *sdk.Context, name string, props sdk.Map) (string, error) {
	var st sdk.CustomResourceState
	if err := ctx.RegisterResource("test:index:Machine", name, props, &st); err != nil {
		return "", err
	}
	urn, err := internals.UnsafeAwaitOutput(ctx.Context(), st.URN())
	if err != nil {
		return "", err
	}
	return string(urn.Value.(sdk.URN)), nil
}

func awaitString(t *testing.T, ctx context.Context, out sdk.StringOutput) internals.UnsafeAwaitOutputResult {
	t.Helper()
	res, err := internals.UnsafeAwaitOutput(ctx, out)
	if err != nil {
		t.Fatalf("await output: %v", err)
	}
	return res
}

// The slice end to end: a plugin returns wire facts, and what comes out the other
// side is a HostFacts indistinguishable from an in-tree driver's — the address
// resolved off a real resource the plugin registered, the key secret, the host key
// trimmed.
func TestPluginDriver_HostsConvertsWireFacts(t *testing.T) {
	RegisterTestingT(t)

	var captured *scpluginv1.VmHostsRequest
	var machineURN string

	registerPluginVm(t, func(_ context.Context, req *scpluginv1.VmHostsRequest) (*scpluginv1.VmHostsResponse, error) {
		captured = req
		return &scpluginv1.VmHostsResponse{Hosts: []*scpluginv1.VmHostFacts{{
			Name:    "node-0",
			User:    "ubuntu",
			Port:    22,
			Role:    string(vm.RoleCompose),
			Address: wireRef(machineURN, "publicIp"),
			// Deliberately NOT flagged secret, to prove core marks it anyway.
			PrivateKey: wireLiteral("-----BEGIN OPENSSH PRIVATE KEY-----"),
			// Trailing newline, as every provider's "public key, OpenSSH format"
			// output carries.
			HostKey:       wireLiteral("ssh-ed25519 AAAAC3Nz fake\n"),
			HostKeyPinned: true,
			DependsOn:     []string{machineURN},
		}}}, nil
	})

	cfg := pluginVmConfig()
	ports := []vm.PortRange{
		{Label: "ssh", Protocol: "tcp", From: 22, To: 22},
		{Label: "kubelet", Protocol: "tcp", From: 10250, To: 10250, Internal: true},
	}

	var facts HostFacts
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		var err error
		machineURN, err = registerFakeMachine(ctx, "machine",
			sdk.Map{"publicIp": sdk.String("203.0.113.9")})
		if err != nil {
			return err
		}

		driver, err := driverFor(cfg)
		if err != nil {
			return err
		}
		Expect(driver.Name()).To(Equal("fake-cloud-vm"))
		// The capability row came from the plugin's schema, not from an in-tree table.
		Expect(driver.Capabilities().CloudInit).To(BeTrue())
		Expect(driver.Capabilities().SecurityGroups).To(BeTrue())

		hosts, err := driver.Hosts(ctx, resourceInput(cfg), cfg, ports,
			pApi.ProvisionParams{Log: logger.New()})
		if err != nil {
			return err
		}
		Expect(hosts).To(HaveLen(1))
		facts = hosts[0]

		address := awaitString(t, ctx.Context(), facts.Address)
		Expect(address.Value).To(Equal("203.0.113.9"))
		Expect(address.Known).To(BeTrue())
		Expect(address.Secret).To(BeFalse())

		// Secret even though the plugin did not say so. HostFacts requires it, and a
		// plugin forgetting the flag must not be able to put a deploy key in a CI log.
		key := awaitString(t, ctx.Context(), facts.PrivateKey)
		Expect(key.Value).To(Equal("-----BEGIN OPENSSH PRIVATE KEY-----"))
		Expect(key.Secret).To(BeTrue())

		// Empty and still secret-wrapped — the consumer cannot tell which driver
		// produced the host.
		pass := awaitString(t, ctx.Context(), facts.PrivateKeyPassword)
		Expect(pass.Value).To(Equal(""))
		Expect(pass.Secret).To(BeTrue())

		// Trimmed, and NOT secret: it is the machine's public key, and keeping it
		// readable is what lets an operator compare it against the cloud console.
		hostKey := awaitString(t, ctx.Context(), facts.HostKey)
		Expect(hostKey.Value).To(Equal("ssh-ed25519 AAAAC3Nz fake"))
		Expect(hostKey.Secret).To(BeFalse())
		return nil
	}, sdk.WithMocks("project", "stack", newVmMocks()))
	Expect(err).ToNot(HaveOccurred())

	Expect(facts.Name).To(Equal("node-0"))
	Expect(facts.User).To(Equal("ubuntu"))
	Expect(facts.Port).To(Equal(22))
	Expect(facts.Role).To(Equal(vm.RoleCompose))
	Expect(facts.HostKeyPinned).To(BeTrue())
	// Rehydrated from the URN, so readiness and everything downstream wait on the
	// resource the PLUGIN created.
	Expect(facts.DependsOn).To(HaveLen(1))

	// The request the plugin saw. The firewall holes are core's — resolved from the
	// whole parent stack, which a plugin is deliberately not given.
	Expect(captured.GetDriver()).To(Equal("fake-cloud-vm"))
	Expect(captured.GetResourceName()).To(Equal("portal-vm"))
	Expect(captured.GetHeader().GetEnvironment()).To(Equal("test"))
	Expect(captured.GetPorts()).To(HaveLen(2))
	Expect(captured.GetPorts()[1].GetLabel()).To(Equal("kubelet"))
	Expect(captured.GetPorts()[1].GetFrom()).To(Equal(int32(10250)))
	Expect(captured.GetPorts()[1].GetInternal()).To(BeTrue())

	// config_json is the VmConfig itself, so a plugin unmarshals it into the same
	// struct rather than agreeing on a second encoding of it.
	var roundTripped vm.VmConfig
	Expect(json.Unmarshal(captured.GetConfigJson(), &roundTripped)).To(Succeed())
	Expect(roundTripped.Driver).To(Equal("fake-cloud-vm"))
	Expect(roundTripped.Region).To(Equal("ru-central1"))
	Expect(roundTripped.PublicIP).To(BeTrue())
}

// A plugin that DOES flag a ref secret gets it honoured — the other direction from
// the unconditional wrap above, and the only way a plugin can protect a value core
// has no field-specific rule for.
func TestPluginDriver_HonoursTheSecretFlag(t *testing.T) {
	RegisterTestingT(t)

	var machineURN string
	registerPluginVm(t, func(_ context.Context, _ *scpluginv1.VmHostsRequest) (*scpluginv1.VmHostsResponse, error) {
		addr := wireRef(machineURN, "publicIp")
		addr.Secret = true
		return &scpluginv1.VmHostsResponse{Hosts: []*scpluginv1.VmHostFacts{{
			Name: "node-0", User: "ubuntu", Port: 22, Role: string(vm.RoleCompose),
			Address: addr,
			HostKey: wireSecretLiteral("ssh-ed25519 AAAA"),
		}}}, nil
	})

	cfg := pluginVmConfig()
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		var err error
		machineURN, err = registerFakeMachine(ctx, "machine",
			sdk.Map{"publicIp": sdk.String("203.0.113.9")})
		if err != nil {
			return err
		}
		driver, err := driverFor(cfg)
		if err != nil {
			return err
		}
		hosts, err := driver.Hosts(ctx, resourceInput(cfg), cfg, nil, pApi.ProvisionParams{Log: logger.New()})
		if err != nil {
			return err
		}
		Expect(awaitString(t, ctx.Context(), hosts[0].Address).Secret).To(BeTrue())
		Expect(awaitString(t, ctx.Context(), hosts[0].HostKey).Secret).To(BeTrue())
		return nil
	}, sdk.WithMocks("project", "stack", newVmMocks()))
	Expect(err).ToNot(HaveOccurred())
}

// A plugin that advertises a vm driver and does not serve the RPC. The generated
// UnimplementedSCCloudPluginServer it embeds answers with codes.Unimplemented, so
// without the translation the operator gets a bare gRPC error naming neither the
// plugin nor the driver.
func TestPluginDriver_UnimplementedRpcIsLegible(t *testing.T) {
	RegisterTestingT(t)

	registerPluginVm(t, func(_ context.Context, _ *scpluginv1.VmHostsRequest) (*scpluginv1.VmHostsResponse, error) {
		return nil, status.Error(codes.Unimplemented, "unknown method VmHosts for service scplugin.v1.SCCloudPlugin")
	})

	err := hostsErr(t, pluginVmConfig())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("sc-plugin-fake"))
	Expect(err.Error()).To(ContainSubstring("fake-cloud-vm"))
	Expect(err.Error()).To(ContainSubstring("does not implement the VmHosts RPC"))
	Expect(err.Error()).To(ContainSubstring("older than the VM seam"))
}

// Zero hosts is not a valid success: everything above the driver is written against
// "give me reachable hosts", and an empty slice there provisions clean and deploys
// nothing.
func TestPluginDriver_NoHostsIsAnError(t *testing.T) {
	RegisterTestingT(t)

	registerPluginVm(t, func(_ context.Context, _ *scpluginv1.VmHostsRequest) (*scpluginv1.VmHostsResponse, error) {
		return &scpluginv1.VmHostsResponse{}, nil
	})

	err := hostsErr(t, pluginVmConfig())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("returned no hosts"))
	Expect(err.Error()).To(ContainSubstring("portal-vm"))
}

// A host with no address: there is nothing for SSH to connect to, and the layers
// above would otherwise build a connection to "".
func TestPluginDriver_MissingAddressIsAnError(t *testing.T) {
	RegisterTestingT(t)

	registerPluginVm(t, func(_ context.Context, _ *scpluginv1.VmHostsRequest) (*scpluginv1.VmHostsResponse, error) {
		return &scpluginv1.VmHostsResponse{Hosts: []*scpluginv1.VmHostFacts{{
			Name: "node-0", User: "ubuntu", Port: 22,
		}}}, nil
	})

	err := hostsErr(t, pluginVmConfig())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("no `address`"))
	// Named, so an operator with a multi-node resource knows which host.
	Expect(err.Error()).To(ContainSubstring("host #0"))
}

// A driver whose row is registered but whose plugin is gone. Reachable for real: the
// row is registered pre-engine and the bundle is closed when the provision returns.
func TestPluginDriver_UnloadedPluginIsLegible(t *testing.T) {
	RegisterTestingT(t)
	defer vm.ResetPluginDrivers()

	Expect(vm.RegisterPluginDriver("sc-plugin-gone", "gone-vm", pluginVmRow())).ToNot(HaveOccurred())

	_, err := driverFor(&vm.VmConfig{Driver: "gone-vm", Region: "r", PublicIP: true})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("sc-plugin-gone"))
	Expect(err.Error()).To(ContainSubstring("not loaded in this run"))
	Expect(err.Error()).To(ContainSubstring("`plugins:` block"))
}

// And a provider that is neither in-tree nor from any plugin still gets the original
// message — the fall-through must not swallow it.
func TestPluginDriver_UnknownProviderStillSaysSo(t *testing.T) {
	RegisterTestingT(t)

	_, err := driverFor(&vm.VmConfig{Driver: "no-such-vm"})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(Equal(`no vm driver for provider "no-such-vm"`))
}

// Provider() hands a plugin driver pulumi-command's provider: the plugin builds its
// own cloud provider inside its own pulumi.Context, and what core still needs is a
// provider for the work core still does on the machine (readiness, docker, compose).
func TestPluginDriver_ProviderIsTheCommandProvider(t *testing.T) {
	RegisterTestingT(t)

	registerPluginVm(t, func(_ context.Context, _ *scpluginv1.VmHostsRequest) (*scpluginv1.VmHostsResponse, error) {
		return nil, nil
	})

	cfg := pluginVmConfig()
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		out, err := Provider(ctx, api.Stack{Name: "test-stack"}, resourceInput(cfg),
			pApi.ProvisionParams{Log: logger.New()})
		if err != nil {
			return err
		}
		_, ok := out.Ref.(sdk.ProviderResource)
		Expect(ok).To(BeTrue(), "a vm provider must satisfy sdk.ProviderResource for initProvider")
		return nil
	}, sdk.WithMocks("project", "stack", newVmMocks()))
	Expect(err).ToNot(HaveOccurred())
}

// hostsErr runs driverFor + Hosts inside a mocked context and returns whatever error
// came out, so the failure-path tests above stay one assertion each.
func hostsErr(t *testing.T, cfg *vm.VmConfig) error {
	t.Helper()
	return sdk.RunErr(func(ctx *sdk.Context) error {
		driver, err := driverFor(cfg)
		if err != nil {
			return err
		}
		_, err = driver.Hosts(ctx, resourceInput(cfg), cfg, nil, pApi.ProvisionParams{Log: logger.New()})
		return err
	}, sdk.WithMocks("project", "stack", newVmMocks()))
}
