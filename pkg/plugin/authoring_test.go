// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	"context"
	"testing"

	"github.com/hashicorp/go-hclog"
	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// A header with no monitor address means this RPC was not dispatched from an active
// plugin session, and opening a context against "" would otherwise fail much later
// with something about dialling.
func TestPulumiContext_RefusesAHeaderWithNoMonitorAddress(t *testing.T) {
	RegisterTestingT(t)

	_, _, err := PulumiContext(context.Background(), &scpluginv1.RequestHeader{StackName: "s"})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("no monitor address"))

	// And a nil header, which is the same situation reached a different way.
	_, _, err = PulumiContext(context.Background(), nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("no monitor address"))
}

// AwaitURN is the call a plugin makes before returning a ResourcePropertyRef, so what
// it returns has to be the engine's own URN for the resource — byte-identical, or
// core resolves a different resource (or none).
func TestAwaitURN_ReturnsTheEngineAssignedURN(t *testing.T) {
	RegisterTestingT(t)

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		var fip sdk.CustomResourceState
		if err := ctx.RegisterResource("sc:test:FloatingIp", "web-ip", sdk.Map{
			"externalIpv4Address": sdk.String("203.0.113.9"),
		}, &fip); err != nil {
			return err
		}

		urn, err := AwaitURN(ctx.Context(), &fip)
		Expect(err).ToNot(HaveOccurred())
		Expect(urn).To(Equal("urn:pulumi:e2e::proj::sc:test:FloatingIp::web-ip"))

		urns, err := AwaitURNs(ctx.Context(), &fip, &fip)
		Expect(err).ToNot(HaveOccurred())
		Expect(urns).To(Equal([]string{urn, urn}))

		return nil
	}, sdk.WithMocks("proj", "e2e", authoringMocks{}))
	Expect(err).ToNot(HaveOccurred())
}

func TestAwaitURN_RefusesANilResource(t *testing.T) {
	RegisterTestingT(t)

	_, err := AwaitURN(context.Background(), nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("nil resource"))

	_, err = AwaitURNs(context.Background(), nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("resource #0"))
}

// AwaitAll must not refuse an unknown output. Everything is unknown during a
// preview, so a plugin that treated unknown as a failure would never preview.
func TestAwaitAll_AcceptsUnknownAndSkipsNil(t *testing.T) {
	RegisterTestingT(t)

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		Expect(AwaitAll(
			ctx.Context(),
			sdk.String("known").ToStringOutput(),
			sdk.UnsafeUnknownOutput(nil),
			nil,
		)).To(Succeed())
		return nil
	}, sdk.WithMocks("proj", "e2e", authoringMocks{}))
	Expect(err).ToNot(HaveOccurred())
}

// The happy path, against a real monitor rather than a stub: core stands up a
// MonitorProxy over its own pulumi.Context, the plugin opens a context against that
// address exactly as PulumiContext tells it to, registers a resource through it, and
// awaits the URN.
//
// That is the production arrangement in one process, and it is the only way to show
// that the address in the request header is usable at all — a plugin that gets this
// wrong registers nothing and reports no error, so a test that merely checks the
// refusal path proves very little.
func TestPulumiContext_RegistersThroughCoresMonitor(t *testing.T) {
	RegisterTestingT(t)

	err := sdk.RunErr(func(coreCtx *sdk.Context) error {
		proxy, err := NewMonitorProxy(coreCtx, hclog.NewNullLogger())
		if err != nil {
			return err
		}
		addr, stop, err := proxy.Serve()
		if err != nil {
			return err
		}
		defer stop()

		// --- everything below is what runs inside the plugin subprocess ---
		pctx, done, err := PulumiContext(context.Background(), &scpluginv1.RequestHeader{
			MonitorAddress: addr,
			ProjectName:    "proj",
			StackName:      "e2e",
		})
		if err != nil {
			return err
		}
		defer done()

		var fip sdk.CustomResourceState
		if err := pctx.RegisterResource("sc:test:FloatingIp", "web-ip", sdk.Map{
			"externalIpv4Address": sdk.String("203.0.113.9"),
		}, &fip); err != nil {
			return err
		}

		// The await that has to happen before the handler returns. Without it, the
		// deferred close above tears the connection down mid-registration.
		urn, err := AwaitURN(pctx.Context(), &fip)
		Expect(err).ToNot(HaveOccurred())
		Expect(urn).To(Equal("urn:pulumi:e2e::proj::sc:test:FloatingIp::web-ip"))

		// And the ref a VM driver would return for it is answerable, which is the
		// whole point of having awaited.
		ref, err := OutputRefTo(pctx.Context(), &fip, "externalIpv4Address")
		Expect(err).ToNot(HaveOccurred())
		Expect(ref.GetRef().GetUrn()).To(Equal(urn))

		return nil
	}, sdk.WithMocks("proj", "e2e", authoringMocks{}))
	Expect(err).ToNot(HaveOccurred())
}

type authoringMocks struct{}

func (authoringMocks) NewResource(args sdk.MockResourceArgs) (string, resource.PropertyMap, error) {
	return args.Name + "-id", args.Inputs, nil
}

func (authoringMocks) Call(args sdk.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}
