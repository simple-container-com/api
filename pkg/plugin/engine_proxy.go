// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	"context"
	"net"
	"reflect"
	"unsafe"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

// engineAccessor is the subset of Engine RPCs this proxy forwards.
// Plugins mostly need Log (pulumi/sdk Context.Log → engine.Log), without
// which a background ApplyT that calls ctx.Log.Warn nil-derefs with
// `logState._log` on the stack trace. The rest of the surface
// (SetRootResource, GetRootResource, GetRequiredPlugins, …) is kept
// so the proxy stays wire-compatible when future plugin code wants it.
type engineAccessor = pulumirpc.EngineClient

// EngineProxy is a loopback gRPC Engine server SC core stands up alongside
// its MonitorProxy so a plugin-side pulumi.NewContext(RunInfo{EngineAddr:…})
// gets a real engine conn. Without it, logState.engine is nil, and any
// pulumi-aws driver that calls ctx.Log.Warn in a background goroutine
// (resolveDeployImageRef, provider conflict paths) panics with
// `invalid memory address or nil pointer dereference` and exits the
// plugin process — which SC core's gRPC client sees as `error reading
// from server: EOF`.
//
// The engine addr is NOT in os.Environ for an inline-RunFunc program, so
// `os.Getenv("PULUMI_ENGINE")` returns empty inside SC core. The engine
// client is held in pctx.state.engine; this proxy extracts it via the
// same reflect+unsafe path monitor_proxy.go uses and republishes it as
// loopback TCP for the plugin's own gRPC dial.
type EngineProxy struct {
	pulumirpc.UnimplementedEngineServer

	engine engineAccessor
	logger hclog.Logger
}

// NewEngineProxy extracts the live EngineClient from a running
// pulumi.Context and wraps it in a proxy server dialable over gRPC. The
// returned Addr() is what SC core puts in RequestHeader.EngineAddress
// on each plugin.Provision call.
func NewEngineProxy(ctx *pulumi.Context, logger hclog.Logger) (*EngineProxy, error) {
	e, err := engineFromContext(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "cannot extract Pulumi engine from context — "+
			"Pulumi SDK may have changed its internals; refresh engineFromContext")
	}
	return &EngineProxy{engine: e, logger: logger}, nil
}

// Serve binds a loopback TCP listener, starts the gRPC server, and returns
// its address. Caller MUST call Stop() when done. Same transport/threat
// model as monitor_proxy.go — plaintext on 127.0.0.1 by Pulumi SDK's
// requirement.
func (p *EngineProxy) Serve() (string, func(), error) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, errors.Wrap(err, "listen engine proxy")
	}
	// Plaintext on loopback, forced by the Pulumi SDK's engine transport.
	// nosemgrep: grpc-server-insecure-connection
	srv := grpc.NewServer()
	pulumirpc.RegisterEngineServer(srv, p)
	go func() {
		if err := srv.Serve(lis); err != nil && p.logger != nil {
			p.logger.Debug("engine proxy stopped", "err", err)
		}
	}()
	stop := func() { srv.Stop() }
	return lis.Addr().String(), stop, nil
}

// Log forwards to the real Pulumi engine — the hot path plugins reach via
// ctx.Log.{Debug,Info,Warn,Error}.
func (p *EngineProxy) Log(ctx context.Context, req *pulumirpc.LogRequest) (*emptypb.Empty, error) {
	if p.logger != nil {
		p.logger.Debug("proxy Log",
			"severity", req.GetSeverity(), "urn", req.GetUrn())
	}
	return p.engine.Log(ctx, req)
}

// GetRootResource forwards — some SDK paths probe it.
func (p *EngineProxy) GetRootResource(ctx context.Context, req *pulumirpc.GetRootResourceRequest) (*pulumirpc.GetRootResourceResponse, error) {
	return p.engine.GetRootResource(ctx, req)
}

// SetRootResource forwards — not currently used by plugins, kept for
// completeness.
func (p *EngineProxy) SetRootResource(ctx context.Context, req *pulumirpc.SetRootResourceRequest) (*pulumirpc.SetRootResourceResponse, error) {
	return p.engine.SetRootResource(ctx, req)
}

// engineFromContext reads the unexported `engine` field from
// pulumi.Context.state. If the field name or path changes in a Pulumi SDK
// upgrade, this returns an error so callers get a clear message instead
// of a nil deref.
func engineFromContext(ctx *pulumi.Context) (engineAccessor, error) {
	if ctx == nil {
		return nil, errors.New("nil pulumi.Context")
	}
	cv := reflect.ValueOf(ctx).Elem()
	stateField := cv.FieldByName("state")
	if !stateField.IsValid() || stateField.Kind() != reflect.Ptr {
		return nil, errors.New("pulumi.Context has no `state` field (SDK renamed?)")
	}
	statePtr := reflect.NewAt(stateField.Type(), unsafe.Pointer(stateField.UnsafeAddr())).Elem()
	if statePtr.IsNil() {
		return nil, errors.New("pulumi.Context.state is nil")
	}
	stateVal := statePtr.Elem()
	engField := stateVal.FieldByName("engine")
	if !engField.IsValid() {
		return nil, errors.New("pulumi.Context.state has no `engine` field (SDK renamed?)")
	}
	engVal := reflect.NewAt(engField.Type(), unsafe.Pointer(engField.UnsafeAddr())).Elem()
	iface := engVal.Interface()
	e, ok := iface.(pulumirpc.EngineClient)
	if !ok {
		return nil, errors.Errorf("pulumi.Context.state.engine is %T, not EngineClient", iface)
	}
	return e, nil
}
