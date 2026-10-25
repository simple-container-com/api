// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	hclog "github.com/hashicorp/go-hclog"
	hplugin "github.com/hashicorp/go-plugin"

	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// ServerConfig describes the plugin-side registration: who implements
// SCCloudPluginServer and what logger to use.
type ServerConfig struct {
	Impl   scpluginv1.SCCloudPluginServer
	Logger hclog.Logger
}

// Serve is the entry point every sc-plugin-<cloud> binary's main() calls.
// It blocks until the host disconnects.
//
// Example (plugin side):
//
//	func main() {
//	    plugin.Serve(plugin.ServerConfig{
//	        Impl:   &awsPlugin{},
//	        Logger: hclog.New(&hclog.LoggerOptions{Name: "sc-plugin-aws"}),
//	    })
//	}
//
// The host side (SC core) calls plugin.NewClient to spawn and dial this.
func Serve(cfg ServerConfig) {
	hplugin.Serve(&hplugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins:         PluginSet(cfg.Impl),
		GRPCServer:      hplugin.DefaultGRPCServer,
		Logger:          cfg.Logger,
	})
}
