// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	"context"
	"os/exec"

	hclog "github.com/hashicorp/go-hclog"
	hplugin "github.com/hashicorp/go-plugin"
	"github.com/pkg/errors"

	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// ClientConfig describes how SC core spawns a cloud plugin and makes RPCs
// against it. Discovery (lock file, user cache, auto-fetch) lives in a
// separate file; this type assumes the Path on disk is already resolved.
type ClientConfig struct {
	// Path to the plugin binary on disk. The caller is responsible for
	// having verified its SHA256 (see 06-decisions.md D10) and for having
	// made it executable.
	Path string

	// Managed makes the hashicorp/go-plugin runtime track the client so
	// plugin.CleanupClients() tears it down on SC core exit — important to
	// avoid zombie child processes if SC core itself is killed with SIGKILL.
	// Default: true. Only set to false in tests that want to retain a
	// crash-dump of the plugin's output.
	Managed bool

	// Logger passed to the plugin via stdout/stderr capture (hashicorp/go-plugin
	// prefixes every captured line with the plugin's binary path). Required.
	Logger hclog.Logger
}

// Client is a connected handle to a running cloud plugin. The caller SHOULD
// defer Close() on it; Close is idempotent.
type Client struct {
	gpClient *hplugin.Client
	stub     scpluginv1.SCCloudPluginClient
}

// NewClient spawns a plugin binary and dials its gRPC server. It validates
// the handshake (magic cookie + protocol version) and returns a Client that
// speaks the SCCloudPlugin service. The caller is expected to Close() when
// done.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.Path == "" {
		return nil, errors.New("plugin path is empty")
	}
	if cfg.Logger == nil {
		return nil, errors.New("plugin logger is nil (hashicorp/go-plugin requires it)")
	}

	managed := true
	if !cfg.Managed { //nolint:gosimple // explicit for the comment
		managed = false
	}

	gp := hplugin.NewClient(&hplugin.ClientConfig{
		HandshakeConfig:  Handshake,
		Plugins:          PluginSet(nil), // host side: Impl is nil
		Cmd:              exec.Command(cfg.Path),
		AllowedProtocols: []hplugin.Protocol{hplugin.ProtocolGRPC},
		Managed:          managed,
		Logger:           cfg.Logger,
		AutoMTLS:         true,
	})

	rpcClient, err := gp.Client()
	if err != nil {
		gp.Kill()
		return nil, errors.Wrap(err, "failed to establish plugin RPC client")
	}

	raw, err := rpcClient.Dispense(PluginName)
	if err != nil {
		gp.Kill()
		return nil, errors.Wrapf(err, "failed to dispense plugin %q", PluginName)
	}

	stub, ok := raw.(scpluginv1.SCCloudPluginClient)
	if !ok {
		gp.Kill()
		return nil, errors.Errorf("dispensed plugin is of type %T, not SCCloudPluginClient", raw)
	}

	return &Client{gpClient: gp, stub: stub}, nil
}

// Schema returns the plugin's advertised schema. Called once at SC core startup
// after a plugin is spawned, before any dispatch to that plugin.
func (c *Client) Schema(ctx context.Context, header *scpluginv1.RequestHeader) (*scpluginv1.SchemaResponse, error) {
	return c.stub.Schema(ctx, &scpluginv1.SchemaRequest{Header: header})
}

// Provider calls through to the plugin's Provider RPC — registers the explicit
// pulumi:providers:<pkg> resource against the shared Pulumi monitor.
func (c *Client) Provider(ctx context.Context, req *scpluginv1.ProviderRequest) (*scpluginv1.ProviderResponse, error) {
	return c.stub.Provider(ctx, req)
}

// Provision calls through to the plugin's Provision RPC — the main workhorse.
func (c *Client) Provision(ctx context.Context, req *scpluginv1.ProvisionRequest) (*scpluginv1.ProvisionResponse, error) {
	return c.stub.Provision(ctx, req)
}

// InitStateStore calls through to the plugin's InitStateStore RPC.
func (c *Client) InitStateStore(ctx context.Context, req *scpluginv1.InitStateStoreRequest) (*scpluginv1.InitStateStoreResponse, error) {
	return c.stub.InitStateStore(ctx, req)
}

// Close terminates the plugin subprocess. Safe to call more than once.
func (c *Client) Close() {
	if c.gpClient != nil {
		c.gpClient.Kill()
	}
}
