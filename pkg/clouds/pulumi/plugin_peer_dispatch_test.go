// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api/logger"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// stubPeerTargetClient simulates a peer plugin's SCCloudPlugin stub.
// Only Provision + NewDnsRecord are populated; the dispatcher test
// routes through both to prove the by-name case switch works.
type stubPeerTargetClient struct {
	scpluginv1.SCCloudPluginClient

	gotProvision    *scpluginv1.ProvisionRequest
	gotNewDnsRecord *scpluginv1.NewDnsRecordRequest
}

func (s *stubPeerTargetClient) Provision(_ context.Context, req *scpluginv1.ProvisionRequest, _ ...grpc.CallOption) (*scpluginv1.ProvisionResponse, error) {
	s.gotProvision = req
	return &scpluginv1.ProvisionResponse{Urn: "urn:pulumi:test::proj::aws:s3/bucket:Bucket::b"}, nil
}

func (s *stubPeerTargetClient) NewDnsRecord(_ context.Context, req *scpluginv1.NewDnsRecordRequest, _ ...grpc.CallOption) (*scpluginv1.NewDnsRecordResponse, error) {
	s.gotNewDnsRecord = req
	return &scpluginv1.NewDnsRecordResponse{Urn: "urn:pulumi:test::proj::cloudflare:index/record:Record::r"}, nil
}

// TestBundlePeerDispatcher_RoutesByPluginNameAndRpcName — the broker
// contract: a composite plugin's PeerInvoke payload reaches the right
// peer plugin's stub, unmarshals cleanly, and the response flows back.
// Exercises two different RPCs to prove the switch dispatch, not just
// the plumbing.
func TestBundlePeerDispatcher_RoutesByPluginNameAndRpcName(t *testing.T) {
	RegisterTestingT(t)

	awsStub := &stubPeerTargetClient{}
	cfStub := &stubPeerTargetClient{}

	bundle := &preEnginePluginBundle{
		clients: map[string]*scPlugin.Client{
			"sc-plugin-aws":        scPlugin.NewClientWithStub(awsStub),
			"sc-plugin-cloudflare": scPlugin.NewClientWithStub(cfStub),
		},
		log: logger.New(),
	}
	disp := &bundlePeerDispatcher{bundle: bundle}

	// A composite plugin asking SC core to run Provision on sc-plugin-aws.
	req := &scpluginv1.ProvisionRequest{
		Header:       &scpluginv1.RequestHeader{StackName: "backend", Environment: "prod"},
		ResourceType: "aws-security-group",
		ResourceName: "mongodb-privatelink-sg",
	}
	payload, err := proto.Marshal(req)
	Expect(err).ToNot(HaveOccurred())

	out, err := disp.DispatchPeer(context.Background(), "sc-plugin-aws", "Provision", payload)
	Expect(err).ToNot(HaveOccurred())
	Expect(awsStub.gotProvision).ToNot(BeNil(), "the AWS peer's Provision stub must have been called")
	Expect(awsStub.gotProvision.GetResourceName()).To(Equal("mongodb-privatelink-sg"))
	Expect(cfStub.gotProvision).To(BeNil(), "the cloudflare peer must NOT have been called")

	var resp scpluginv1.ProvisionResponse
	Expect(proto.Unmarshal(out, &resp)).To(Succeed())
	Expect(resp.GetUrn()).To(Equal("urn:pulumi:test::proj::aws:s3/bucket:Bucket::b"))

	// Second RPC on a different peer to prove the switch doesn't lock
	// to one case.
	dnsReq := &scpluginv1.NewDnsRecordRequest{
		Header:     &scpluginv1.RequestHeader{StackName: "backend", Environment: "prod"},
		RecordName: "api",
		RecordType: "A",
	}
	dnsPayload, err := proto.Marshal(dnsReq)
	Expect(err).ToNot(HaveOccurred())

	dnsOut, err := disp.DispatchPeer(context.Background(), "sc-plugin-cloudflare", "NewDnsRecord", dnsPayload)
	Expect(err).ToNot(HaveOccurred())
	Expect(cfStub.gotNewDnsRecord).ToNot(BeNil())
	Expect(cfStub.gotNewDnsRecord.GetRecordName()).To(Equal("api"))

	var dnsResp scpluginv1.NewDnsRecordResponse
	Expect(proto.Unmarshal(dnsOut, &dnsResp)).To(Succeed())
	Expect(dnsResp.GetUrn()).To(Equal("urn:pulumi:test::proj::cloudflare:index/record:Record::r"))
}

// TestBundlePeerDispatcher_UnknownPluginError — a composite plugin
// asking for a peer that is NOT in the loaded registry gets a clear
// error, not a hang or panic. This is what prevents a plugin spawned
// into a reduced SC-core run (missing the peer it expected) from
// taking the whole deploy down with it.
func TestBundlePeerDispatcher_UnknownPluginError(t *testing.T) {
	RegisterTestingT(t)

	bundle := &preEnginePluginBundle{
		clients: map[string]*scPlugin.Client{
			"sc-plugin-cloudflare": scPlugin.NewClientWithStub(&stubPeerTargetClient{}),
		},
		log: logger.New(),
	}
	disp := &bundlePeerDispatcher{bundle: bundle}

	_, err := disp.DispatchPeer(context.Background(), "sc-plugin-aws", "Provision", nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("sc-plugin-aws"))
	Expect(err.Error()).To(ContainSubstring("not loaded"))
}

// TestBundlePeerDispatcher_UnsupportedRpcError — a composite plugin
// asking for an RPC not in the dispatch switch gets a clear error that
// enumerates the supported RPCs. Prevents the composite plugin from
// hanging on a transport-level error for a typo in rpc_name.
func TestBundlePeerDispatcher_UnsupportedRpcError(t *testing.T) {
	RegisterTestingT(t)

	bundle := &preEnginePluginBundle{
		clients: map[string]*scPlugin.Client{
			"sc-plugin-aws": scPlugin.NewClientWithStub(&stubPeerTargetClient{}),
		},
		log: logger.New(),
	}
	disp := &bundlePeerDispatcher{bundle: bundle}

	_, err := disp.DispatchPeer(context.Background(), "sc-plugin-aws", "DoTheHokeyPokey", nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("DoTheHokeyPokey"))
	Expect(err.Error()).To(ContainSubstring("supported"))
}
