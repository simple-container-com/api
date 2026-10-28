// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"testing"

	"google.golang.org/grpc"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// stubRecordClient answers only the two registrar RPCs these tests dispatch.
// The embedded interface is nil, so any other call panics loudly rather than
// quietly returning a zero value.
type stubRecordClient struct {
	scpluginv1.SCCloudPluginClient
	urn string
}

func (s *stubRecordClient) NewDnsRecord(context.Context, *scpluginv1.NewDnsRecordRequest, ...grpc.CallOption) (*scpluginv1.NewDnsRecordResponse, error) {
	return &scpluginv1.NewDnsRecordResponse{Urn: s.urn}, nil
}

func (s *stubRecordClient) NewOverrideHeaderRule(context.Context, *scpluginv1.NewOverrideHeaderRuleRequest, ...grpc.CallOption) (*scpluginv1.NewOverrideHeaderRuleResponse, error) {
	return &scpluginv1.NewOverrideHeaderRuleResponse{Urn: s.urn}, nil
}

type refMocks struct{}

func (*refMocks) NewResource(args sdk.MockResourceArgs) (string, resource.PropertyMap, error) {
	return args.Name, args.Inputs, nil
}

func (*refMocks) Call(args sdk.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func stubbedPluginRegistrar(urn string) *pluginRegistrar {
	return &pluginRegistrar{
		mainDomain: "example.com",
		log:        logger.New(),
		pluginName: "sc-plugin-cloudflare",
		typeToken:  "cloudflare",
		cli:        scPlugin.NewClientWithStub(&stubRecordClient{urn: urn}),
	}
}

// Every in-tree registrar returns an Output as its Ref — cloudru hands back
// cmd.ID(), the old in-tree cloudflare handed back the record resource's ID —
// and gcp/StaticWebsite has always exported it with a bare, unchecked
// `dnsRecord.Ref.(sdk.Output)`. pluginRegistrar.NewRecord's synchronous path
// (taken whenever record.Value is a plain string, which is ALWAYS the case for
// the dist--prod CNAME) returned resp.GetUrn() — a bare string — so that export
// panicked. The panic surfaces from inside deployStackProgram's ApplyT
// continuation, which means the operator sees Pulumi's opaque "waiting for
// RPCs" wrapper and not the type assertion, exactly as with the session-lifetime
// defect that preceded it.
//
// So: whichever path NewRecord takes, its Ref must satisfy sdk.Output.
func TestPluginRegistrar_RecordRefIsAlwaysAnOutput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		record api.DnsRecord
	}{
		{
			// The dist--prod shape: a literal CNAME target, dispatched
			// synchronously so the RPC fires even when the ref is discarded.
			name: "literal value (synchronous dispatch)",
			record: api.DnsRecord{
				Name: "dist.simple-container.com", Type: "CNAME",
				Value: "bucket.storage.googleapis.com", Proxied: true,
			},
		},
		{
			name: "output value (ApplyT dispatch)",
			record: api.DnsRecord{
				Name: "dist.simple-container.com", Type: "CNAME",
				ValueOut: sdk.String("bucket.storage.googleapis.com").ToStringOutput(),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Registered per-subtest: a gomega failure calls FailNow, and one bound
			// to the parent test aborts every remaining case.
			RegisterTestingT(t)
			resetSessionsForTest(t)

			var ref any
			err := sdk.RunErr(func(ctx *sdk.Context) error {
				attachedSession(ctx, "127.0.0.1:65010")
				out, err := stubbedPluginRegistrar("urn:pulumi:prod::dist::cloudflare:index/record:Record::rec").
					NewRecord(ctx, tc.record)
				if err != nil {
					return err
				}
				ref = out.Ref
				return nil
			}, sdk.WithMocks("project", "stack", &refMocks{}))
			Expect(err).NotTo(HaveOccurred())

			_, ok := ref.(sdk.Output)
			Expect(ok).To(BeTrue(),
				"gcp/StaticWebsite exports this Ref as an sdk.Output; got %T", ref)
		})
	}
}

// NewOverrideHeaderRule is the very next call on the dist--prod path, and it
// always returns an ApplyT chain — pinned so it stays that way.
func TestPluginRegistrar_OverrideHeaderRuleRefIsAnOutput(t *testing.T) {
	RegisterTestingT(t)
	resetSessionsForTest(t)

	var ref any
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		attachedSession(ctx, "127.0.0.1:65011")
		out, err := stubbedPluginRegistrar("urn:pulumi:prod::dist::cloudflare:index/ruleset:Ruleset::rule").
			NewOverrideHeaderRule(ctx, api.Stack{Name: "dist"}, pApi.OverrideHeaderRule{
				FromHost: "dist.simple-container.com",
				ToHost:   sdk.String("bucket.storage.googleapis.com"),
			})
		if err != nil {
			return err
		}
		ref = out.Ref
		return nil
	}, sdk.WithMocks("project", "stack", &refMocks{}))
	Expect(err).NotTo(HaveOccurred())

	_, ok := ref.(sdk.Output)
	Expect(ok).To(BeTrue(),
		"gcp/StaticWebsite exports this Ref as an sdk.Output; got %T", ref)
}
