// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package runtime

import (
	"context"
	"net"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"

	scPlugin "github.com/simple-container-com/api/pkg/plugin"
)

// localEngine is the Pulumi Engine endpoint a plugin's pulumi.Context logs to
// when SC core sends no engine_address — which today it never does.
//
// A Context built without an engine has a nil EngineClient, and the Go SDK
// calls it unguarded (log.go `log.engine.Log`), so the first ctx.Log.* in the
// plugin is a nil-pointer panic that takes the whole plugin process down.
// Core's own docker helper does exactly that: resolveDeployImageRef warns
// when an image has no digest, which is every preview of a new image, so
// every preview of a yc-serverless-container died with "plugin process
// exited … exit status 2" followed by `error reading from server: EOF`.
//
// Log lines are relayed to core through the progress publisher (LogStream),
// so they still reach the operator. Ported verbatim from sc-plugin-yandex's
// engine.go — the pattern is cloud-agnostic.
type localEngine struct {
	pulumirpc.UnimplementedEngineServer
	progress *scPlugin.Publisher
}

func (e *localEngine) Log(_ context.Context, req *pulumirpc.LogRequest) (*emptypb.Empty, error) {
	switch req.GetSeverity() {
	case pulumirpc.LogSeverity_ERROR:
		e.progress.Error("pulumi", req.GetMessage())
	case pulumirpc.LogSeverity_WARNING:
		e.progress.Warn("pulumi", req.GetMessage())
	case pulumirpc.LogSeverity_DEBUG:
		e.progress.Debug("pulumi", req.GetMessage())
	default:
		e.progress.Info("pulumi", req.GetMessage())
	}
	return &emptypb.Empty{}, nil
}

func (e *localEngine) GetRootResource(context.Context, *pulumirpc.GetRootResourceRequest) (*pulumirpc.GetRootResourceResponse, error) {
	return &pulumirpc.GetRootResourceResponse{}, nil
}

func (e *localEngine) SetRootResource(context.Context, *pulumirpc.SetRootResourceRequest) (*pulumirpc.SetRootResourceResponse, error) {
	return &pulumirpc.SetRootResourceResponse{}, nil
}

var (
	localEngineOnce sync.Once
	localEngineAddr string
	localEngineErr  error
)

// LocalEngineAddress returns an engine address a plugin Context should use:
// the one core named in the request header (fromHeader), or a process-wide
// local fallback if the header is empty.
//
// The fallback is lazily initialised on first call and shared across every
// Context the plugin ever opens — a plugin process has one localEngine, not
// one per RPC. The progress publisher is the plugin's own log sink; log
// lines routed through the local engine land on it.
func LocalEngineAddress(fromHeader string, progress *scPlugin.Publisher) (string, error) {
	if fromHeader != "" {
		return fromHeader, nil
	}
	localEngineOnce.Do(func() {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			localEngineErr = err
			return
		}
		srv := grpc.NewServer()
		pulumirpc.RegisterEngineServer(srv, &localEngine{progress: progress})
		go func() { _ = srv.Serve(lis) }()
		localEngineAddr = lis.Addr().String()
	})
	return localEngineAddr, localEngineErr
}
