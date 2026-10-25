// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/pkg/errors"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// The plugin end of the VM seam.
//
// A plugin advertises its `provider:` values in SchemaResponse.vm_drivers; SC core
// puts each one in the config-tier driver table (vm.RegisterPluginDriver, which is
// what makes Validate treat it exactly like a built-in) and registers the plugin's
// VmHosts callable here. From then on pluginDriver is just another Driver: it
// creates no cloud resources itself, it asks the plugin to, and it converts what
// comes back into HostFacts. Everything above the driver — the SSH connection,
// readiness, cloud-init, compose, k3s — neither knows nor can tell.

// VmHostsCall is one plugin's VmHosts RPC, as a function value.
//
// A function and not the *plugin.Client itself, for an import reason that is also a
// layering one: pkg/clouds/pulumi (which owns the plugin subprocesses, the
// MonitorProxy and the active session) imports THIS package, not the other way
// round — see pkg/clouds/pulumi/init.go. So the dependency has to be inverted, and
// inverting it through a one-method function keeps the whole plugin lifecycle on the
// side that already owns it.
//
// The caller fills RequestHeader.MonitorAddress and ProviderUrns — this package
// cannot see the active session. The driver fills the rest of the header (stack,
// environment) because it is the side that has the ResourceInput.
type VmHostsCall func(ctx context.Context, req *scpluginv1.VmHostsRequest) (*scpluginv1.VmHostsResponse, error)

var (
	pluginCallsMu sync.RWMutex
	pluginCalls   = map[string]VmHostsCall{}
)

// RegisterPluginClient records how to reach a loaded plugin's VmHosts RPC, keyed by
// plugin name.
//
// Keyed by PLUGIN and not by driver on purpose: vm.RegisterPluginDriver already owns
// the driver → plugin mapping, and it is where the "two plugins claim one driver"
// and "a plugin may not shadow a built-in" refusals live. A second driver-keyed map
// here would be a second place for those two rules to be enforced, which in practice
// means a second place for them to disagree.
func RegisterPluginClient(pluginName string, call VmHostsCall) {
	pluginCallsMu.Lock()
	defer pluginCallsMu.Unlock()
	pluginCalls[pluginName] = call
}

// ResetPluginClients drops every registered plugin. Called when the plugin bundle is
// torn down, so a second provision in the same process does not dispatch to a
// subprocess that has exited.
func ResetPluginClients() {
	pluginCallsMu.Lock()
	defer pluginCallsMu.Unlock()
	pluginCalls = map[string]VmHostsCall{}
}

func pluginCallFor(pluginName string) (VmHostsCall, bool) {
	pluginCallsMu.RLock()
	defer pluginCallsMu.RUnlock()
	call, ok := pluginCalls[pluginName]
	return call, ok
}

// pluginDriverFor builds the Driver for a plugin-advertised `provider:`, or returns
// false when the provider is not one.
func pluginDriverFor(cfg *vm.VmConfig) (Driver, bool, error) {
	pluginName := vm.PluginForDriver(cfg.Driver)
	if pluginName == "" {
		return nil, false, nil
	}
	call, ok := pluginCallFor(pluginName)
	if !ok {
		// The driver is in the config-tier table but its plugin has no callable. Two
		// ways to get here, and the message has to cover both: the plugin was loaded
		// for its schema and then the bundle was closed, or a `plugins:` block was
		// edited between the provision that registered the row and this call.
		return nil, false, errors.Errorf(
			"vm provider %q is provided by plugin %q, which is not loaded in this run; "+
				"add it to the parent stack's `plugins:` block", cfg.Driver, pluginName,
		)
	}
	return pluginDriver{driver: cfg.Driver, pluginName: pluginName, call: call}, true, nil
}

type pluginDriver struct {
	driver     string
	pluginName string
	call       VmHostsCall
}

func (d pluginDriver) Name() string { return d.driver }

// Capabilities reads the row the plugin's schema registered. Not an RPC: the layers
// above call this while building the graph, and a capability table that could fail
// or block would have to be handled at every one of those call sites.
func (d pluginDriver) Capabilities() vm.Capabilities {
	info, _ := vm.LookupDriver(d.driver)
	return info.Capabilities
}

func (d pluginDriver) Hosts(
	ctx *sdk.Context,
	input api.ResourceInput,
	cfg *vm.VmConfig,
	ports []vm.PortRange,
	_ pApi.ProvisionParams,
) ([]HostFacts, error) {
	configJSON, err := json.Marshal(cfg)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to serialize vm config for plugin %q", d.pluginName)
	}

	req := &scpluginv1.VmHostsRequest{
		Header:       &scpluginv1.RequestHeader{},
		Driver:       d.driver,
		ResourceName: input.Descriptor.Name,
		ConfigJson:   configJSON,
		Ports:        portRanges(ports),
	}
	if input.StackParams != nil {
		req.Header.StackName = input.StackParams.StackName
		req.Header.Environment = input.StackParams.Environment
	}

	resp, err := d.call(ctx.Context(), req)
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			// A plugin that advertises a vm_driver and does not serve VmHosts. The
			// generated UnimplementedSCCloudPluginServer the plugin embeds answers
			// this way, so without the translation an operator gets a bare gRPC
			// "unknown method VmHosts" naming neither the plugin nor the driver.
			return nil, errors.Errorf(
				"plugin %q advertises vm provider %q but does not implement the VmHosts RPC "+
					"(it is probably built against an SC plugin bus older than the VM seam)",
				d.pluginName, d.driver,
			)
		}
		return nil, errors.Wrapf(err, "plugin %q VmHosts(%q, %q)", d.pluginName, d.driver, input.Descriptor.Name)
	}
	if len(resp.GetHosts()) == 0 {
		// Returning no hosts is not a valid success: everything above the driver is
		// written against "give me reachable hosts", and an empty slice there turns
		// into a stack that provisions clean and deploys nothing.
		return nil, errors.Errorf("plugin %q returned no hosts for vm %q (provider %q)",
			d.pluginName, input.Descriptor.Name, d.driver)
	}

	hosts := make([]HostFacts, 0, len(resp.GetHosts()))
	for i, h := range resp.GetHosts() {
		facts, err := d.hostFacts(ctx, h)
		if err != nil {
			return nil, errors.Wrapf(err, "plugin %q host #%d of vm %q", d.pluginName, i, input.Descriptor.Name)
		}
		hosts = append(hosts, facts)
	}
	return hosts, nil
}

// hostFacts converts one wire host into the in-tree HostFacts, resolving each
// OutputValue against the live monitor (see pkg/plugin/outputref.go §3.5).
func (d pluginDriver) hostFacts(ctx *sdk.Context, h *scpluginv1.VmHostFacts) (HostFacts, error) {
	if h.GetName() == "" {
		return HostFacts{}, errors.New("host has no name; HostFacts.Name is what every machine-scoped resource name is built from")
	}

	// DependsOn first: the resolved Outputs need these as their dependency set, so
	// that an unknown value during preview still carries the right edges.
	deps := make([]sdk.Resource, 0, len(h.GetDependsOn()))
	for _, urn := range h.GetDependsOn() {
		res, err := scPlugin.RehydrateResource(ctx, urn)
		if err != nil {
			return HostFacts{}, err
		}
		deps = append(deps, res)
	}

	if h.GetAddress() == nil {
		return HostFacts{}, errors.New("host has no `address`; there is nothing for SSH to connect to")
	}
	address, err := resolvePluginOutput(ctx, h.GetAddress(), deps)
	if err != nil {
		return HostFacts{}, errors.Wrap(err, "`address`")
	}
	privateKey, err := resolvePluginOutput(ctx, h.GetPrivateKey(), deps)
	if err != nil {
		return HostFacts{}, errors.Wrap(err, "`private_key`")
	}
	privateKeyPassword, err := resolvePluginOutput(ctx, h.GetPrivateKeyPassword(), deps)
	if err != nil {
		return HostFacts{}, errors.Wrap(err, "`private_key_password`")
	}
	hostKey, err := resolvePluginOutput(ctx, h.GetHostKey(), deps)
	if err != nil {
		return HostFacts{}, errors.Wrap(err, "`host_key`")
	}

	return HostFacts{
		Name: h.GetName(),
		User: h.GetUser(),
		Port: int(h.GetPort()),
		Role: vm.HostRole(h.GetRole()),

		Address: address,
		// ToSecret unconditionally, and NOT only when the plugin set
		// OutputValue.secret. HostFacts requires these two to arrive already marked,
		// because remote.Connection.PrivateKey is a plain StringInput and an unmarked
		// key is printed verbatim in Pulumi's update summary and archived in the CI
		// log — the exact mechanism by which a Yandex service-account private key
		// reached a world-readable Actions log on 2026-09-30. A plugin forgetting the
		// flag must not be able to cause that, and ToSecret is idempotent, so the
		// guarantee is made here rather than trusted to every plugin author.
		PrivateKey:         sdk.ToSecret(privateKey).(sdk.StringOutput),
		PrivateKeyPassword: sdk.ToSecret(privateKeyPassword).(sdk.StringOutput),
		// Trailing whitespace trimmed, for the same reason cloudru.go trims it: this
		// value is compared against an authorized_keys-style line by
		// ssh.ParseAuthorizedKey, and every cloud/TLS provider's "public key, OpenSSH
		// format" output comes with a trailing newline. Done here so a plugin author
		// handing back `tls.PrivateKey.PublicKeyOpenssh` does not have to know.
		HostKey: hostKey.ApplyT(strings.TrimSpace).(sdk.StringOutput),
		// The plugin's own claim, not inferred from whether host_key is empty: during
		// a preview the key is unknown, so "is it empty" is unanswerable exactly when
		// the graph needs the answer. See HostFacts.HostKeyPinned.
		HostKeyPinned: h.GetHostKeyPinned(),
		DependsOn:     deps,
	}, nil
}

// resolvePluginOutput turns one wire OutputValue into a StringOutput.
//
// A nil value is the empty string and not an error: `private_key_password` is a
// literal "" on every driver that generates its own key, and a plugin author writing
// `&VmHostFacts{...}` without the field is making that claim correctly. The fields
// where absent is genuinely wrong (address, name) are checked by the caller, which
// is the side that knows which field it is holding.
func resolvePluginOutput(ctx *sdk.Context, v *scpluginv1.OutputValue, deps []sdk.Resource) (sdk.StringOutput, error) {
	if v == nil {
		return sdk.String("").ToStringOutput(), nil
	}
	if ref := v.GetRef(); ref != nil {
		out, err := scPlugin.ResolveStringOutput(ctx, scPlugin.OutputRef{
			URN:      ref.GetUrn(),
			Property: ref.GetProperty(),
		}, deps)
		if err != nil {
			return sdk.StringOutput{}, err
		}
		if v.GetSecret() {
			// Union, not override: ResolveStringOutput already marks the value secret
			// when the resolved property is itself secret. This covers the other
			// direction — a plugin reading a non-secret property it knows to be
			// sensitive.
			return sdk.ToSecret(out).(sdk.StringOutput), nil
		}
		return out, nil
	}
	if v.GetSecret() {
		return sdk.ToSecret(sdk.String(v.GetLiteral())).(sdk.StringOutput), nil
	}
	return sdk.String(v.GetLiteral()).ToStringOutput(), nil
}

// portRanges converts core's resolved firewall holes to the wire shape. One-to-one
// with vm.PortRange — see VmPortRange in plugin.proto for why `label` and `internal`
// are part of the contract rather than decoration.
func portRanges(ports []vm.PortRange) []*scpluginv1.VmPortRange {
	out := make([]*scpluginv1.VmPortRange, 0, len(ports))
	for _, p := range ports {
		out = append(out, &scpluginv1.VmPortRange{
			Label:    p.Label,
			Protocol: p.Protocol,
			From:     int32(p.From),
			To:       int32(p.To),
			Internal: p.Internal,
		})
	}
	return out
}
