// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	hplugin "github.com/hashicorp/go-plugin"
	"github.com/pkg/errors"

	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// The plugin side of the PeerInvoke D8 pattern.
//
// A composite plugin — one whose provisioning path depends on resources
// another plugin owns (sc-plugin-mongodb's PrivateLink needs an AWS
// SecurityGroup; sc-plugin-k3s wants both AWS and GCP) — asks SC core
// to broker the call. The peer plugin need not know about this plugin's
// existence, and this plugin never takes a Go module dependency on the
// peer's SDK — D8 in 06-decisions.md.
//
// Usage shape:
//
//	peer, err := plugin.NewPeerClient(ctx, req.GetHeader())
//	if err != nil { return nil, err }
//	defer peer.Close()
//
//	resp, err := peer.Provision(ctx, "sc-plugin-aws", &scpluginv1.ProvisionRequest{
//	    Header:       req.GetHeader(),
//	    ResourceType: "aws-security-group",
//	    ...
//	})
//
// The peer plugin's response flows back; the SC-core broker dispatches
// the sub-RPC against the peer's own stub, so byte-for-byte it's the
// same as if the SCCore RPC had been direct.

// PeerClient is a connected handle to SC core's SCCore callback endpoint,
// scoped to one SC-core-spawned plugin. Open per RPC that needs to invoke
// a peer; Close when done to release the broker stream.
type PeerClient struct {
	broker   *hplugin.GRPCBroker
	conn     *grpc.ClientConn
	stub     scpluginv1.SCCoreClient
	runToken string
}

// NewPeerClient dials SC core's SCCore endpoint via the broker stream
// SC core served for this plugin. The stream id and run_token come
// from the SCCloudPlugin RPC's RequestHeader.
//
// The broker is the one hashicorp/go-plugin handed the plugin at
// GRPCServer time; a plugin that wants PeerInvoke MUST store it off
// its Impl so NewPeerClient can dial. See the sc-plugin-echo
// composite demo for the wiring (follow-up).
func NewPeerClient(ctx context.Context, broker *hplugin.GRPCBroker, header *scpluginv1.RequestHeader) (*PeerClient, error) {
	if broker == nil {
		return nil, errors.New("peer client: nil broker — the plugin's GRPCServer must store the broker argument before any handler calls NewPeerClient")
	}
	streamID := header.GetSccoreBrokerStream()
	if streamID == 0 {
		return nil, errors.New("peer client: sccore_broker_stream is zero — SC core did not expose a callback endpoint (older core, test harness, or non-composite plugin spawn)")
	}
	conn, err := broker.Dial(streamID)
	if err != nil {
		return nil, errors.Wrapf(err, "peer client: dial SCCore broker stream %d", streamID)
	}
	return &PeerClient{
		broker:   broker,
		conn:     conn,
		stub:     scpluginv1.NewSCCoreClient(conn),
		runToken: header.GetRunToken(),
	}, nil
}

// Close releases the broker stream connection. Safe to call more than once.
func (c *PeerClient) Close() error {
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

// invoke marshals a request, sends it over PeerInvoke, and unmarshals
// the response into `into`. Returns a plugin-side error if the peer
// RPC itself errored (populated in PeerInvokeResponse.error).
func (c *PeerClient) invoke(ctx context.Context, pluginName, rpcName string, req proto.Message, into proto.Message) error {
	payload, err := proto.Marshal(req)
	if err != nil {
		return errors.Wrapf(err, "peer invoke %s.%s: marshal request", pluginName, rpcName)
	}
	resp, err := c.stub.PeerInvoke(ctx, &scpluginv1.PeerInvokeRequest{
		RunToken:   c.runToken,
		PluginName: pluginName,
		RpcName:    rpcName,
		Payload:    payload,
	})
	if err != nil {
		return errors.Wrapf(err, "peer invoke %s.%s: SCCore transport", pluginName, rpcName)
	}
	if resp.GetError() != "" {
		return errors.Errorf("peer invoke %s.%s: peer error: %s", pluginName, rpcName, resp.GetError())
	}
	if into == nil {
		return nil
	}
	if err := proto.Unmarshal(resp.GetPayload(), into); err != nil {
		return errors.Wrapf(err, "peer invoke %s.%s: unmarshal response", pluginName, rpcName)
	}
	return nil
}

// Provision calls the Provision RPC on another plugin.
func (c *PeerClient) Provision(ctx context.Context, pluginName string, req *scpluginv1.ProvisionRequest) (*scpluginv1.ProvisionResponse, error) {
	resp := &scpluginv1.ProvisionResponse{}
	if err := c.invoke(ctx, pluginName, "Provision", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// Provider calls the Provider RPC on another plugin.
func (c *PeerClient) Provider(ctx context.Context, pluginName string, req *scpluginv1.ProviderRequest) (*scpluginv1.ProviderResponse, error) {
	resp := &scpluginv1.ProviderResponse{}
	if err := c.invoke(ctx, pluginName, "Provider", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// NewDnsRecord calls the NewDnsRecord RPC on another plugin.
func (c *PeerClient) NewDnsRecord(ctx context.Context, pluginName string, req *scpluginv1.NewDnsRecordRequest) (*scpluginv1.NewDnsRecordResponse, error) {
	resp := &scpluginv1.NewDnsRecordResponse{}
	if err := c.invoke(ctx, pluginName, "NewDnsRecord", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ProvisionRegistrar calls the ProvisionRegistrar RPC on another plugin.
func (c *PeerClient) ProvisionRegistrar(ctx context.Context, pluginName string, req *scpluginv1.ProvisionRegistrarRequest) (*scpluginv1.ProvisionRegistrarResponse, error) {
	resp := &scpluginv1.ProvisionRegistrarResponse{}
	if err := c.invoke(ctx, pluginName, "ProvisionRegistrar", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// VmHosts calls the VmHosts RPC on another plugin.
func (c *PeerClient) VmHosts(ctx context.Context, pluginName string, req *scpluginv1.VmHostsRequest) (*scpluginv1.VmHostsResponse, error) {
	resp := &scpluginv1.VmHostsResponse{}
	if err := c.invoke(ctx, pluginName, "VmHosts", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}
