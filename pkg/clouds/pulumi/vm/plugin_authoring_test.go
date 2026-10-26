// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// Both ends of the VM seam in one test: the plugin describes its machines with the
// authoring helpers in pkg/plugin, and core converts the result with the production
// path in plugin_driver.go.
//
// The other tests in this package hand-build wire messages, which is the right way
// to test core's tolerance of a plugin that gets something wrong. This one is the
// opposite check, and the one that keeps the two ends honest: whatever the helper a
// plugin author is told to use produces, core must accept. The sibling test in
// pkg/plugin can never assert this — it cannot import this package without a cycle.
func TestPluginDriver_AcceptsWhatTheAuthoringHelperBuilds(t *testing.T) {
	RegisterTestingT(t)

	var (
		machine sdk.CustomResourceState
		pctx    *sdk.Context
	)

	registerPluginVm(t, func(_ context.Context, req *scpluginv1.VmHostsRequest) (*scpluginv1.VmHostsResponse, error) {
		// Verbatim what a VM-driver plugin's VmHosts implementation writes. The
		// resource it refers to is one it registered against core's monitor; here the
		// test registered it in the same context, which is what MonitorProxy makes
		// true in production.
		return scPlugin.BuildVmHosts(
			scPlugin.NewVmHost(pctx.Context(), req.GetResourceName()+"-0").
				User("ubuntu").
				Port(22).
				Role(string(vm.RoleCompose)).
				AddressOf(&machine, "publicIp").
				PrivateKeyOf(&machine, "privateKeyOpenssh").
				TrustOnFirstUse().
				DependsOn(&machine),
		)
	})

	cfg := pluginVmConfig()

	var facts HostFacts
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		pctx = ctx
		if err := ctx.RegisterResource("test:index:Machine", "machine", sdk.Map{
			"publicIp":          sdk.String("203.0.113.9"),
			"privateKeyOpenssh": sdk.String("-----BEGIN OPENSSH PRIVATE KEY-----"),
		}, &machine); err != nil {
			return err
		}

		driver, err := driverFor(cfg)
		if err != nil {
			return err
		}
		hosts, err := driver.Hosts(ctx, resourceInput(cfg), cfg, nil,
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

		// The helper marked it secret on the wire and core marks it secret again.
		// Both, deliberately — see PrivateKey's doc comment.
		key := awaitString(t, ctx.Context(), facts.PrivateKey)
		Expect(key.Value).To(Equal("-----BEGIN OPENSSH PRIVATE KEY-----"))
		Expect(key.Secret).To(BeTrue())

		// TrustOnFirstUse sent no host key at all, which core reads as the empty
		// string rather than refusing the host.
		hostKey := awaitString(t, ctx.Context(), facts.HostKey)
		Expect(hostKey.Value).To(Equal(""))

		// Nothing was said about the key password, and it still arrives as an
		// empty secret — the consumer cannot tell this host from a byo one.
		pass := awaitString(t, ctx.Context(), facts.PrivateKeyPassword)
		Expect(pass.Value).To(Equal(""))
		Expect(pass.Secret).To(BeTrue())

		return nil
	}, sdk.WithMocks("project", "stack", newVmMocks()))
	Expect(err).ToNot(HaveOccurred())

	// The name the plugin derived from the request's resource name, so it is stable
	// across deploys rather than taken from anything the cloud generated.
	Expect(facts.Name).To(Equal("portal-vm-0"))
	Expect(facts.User).To(Equal("ubuntu"))
	Expect(facts.Port).To(Equal(22))
	Expect(facts.Role).To(Equal(vm.RoleCompose))
	Expect(facts.HostKeyPinned).To(BeFalse(), "TrustOnFirstUse is not a pinned key")
	Expect(facts.DependsOn).To(HaveLen(1))
}
