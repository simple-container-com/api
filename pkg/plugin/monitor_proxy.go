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

// monitorAccessor is the subset of ResourceMonitor RPCs this proxy forwards.
// Everything not in the subset returns UNIMPLEMENTED to the plugin, which is
// fine for v1 — plugins only need RegisterResource for the single-session
// mechanism from 03-architecture.md §3 option (b). Richer surface (Invoke,
// ReadResource, SupportsFeature, ...) is added when a plugin needs it.
type monitorAccessor = pulumirpc.ResourceMonitorClient

// MonitorProxy is a loopback gRPC ResourceMonitor server that SC core stands up
// inside its Pulumi inline RunFunc. Each incoming RegisterResource from a plugin
// is forwarded verbatim to the REAL Pulumi monitor the engine handed SC core.
// This solves the gap between SC core's pulumi.Context (which holds the live
// monitor client as an unexported field) and the plugin subprocess (which can
// only reach the engine over gRPC).
//
// Why a proxy and not a public API: pulumi/sdk/v3 v3.245.0 does not expose
// Context.MonitorAddr — NewContext dials with info.MonitorAddr and discards it.
// Until Pulumi exposes an accessor, SC core either (a) runs this proxy, or
// (b) switches to the "plugin returns resource descriptions; SC core
// registers" model (option (d) in D1a). The proxy keeps the plugin side
// architecture clean — plugins talk real Pulumi gRPC — and keeps the hack
// localised to one file in pkg/plugin.
//
// The reflect+unsafe path to the monitor client is confined to monitorFromContext
// below. If a future Pulumi SDK version renames contextState.monitor, this
// will panic at startup, which is caught by the roundtrip test — not a prod
// hazard.
type MonitorProxy struct {
	pulumirpc.UnimplementedResourceMonitorServer

	monitor monitorAccessor
	logger  hclog.Logger
}

// NewMonitorProxy extracts the live ResourceMonitorClient from a running
// pulumi.Context and wraps it in a proxy server that can be dialed over
// gRPC. The returned Addr() is what SC core puts in
// RequestHeader.MonitorAddress on each plugin.Provision call.
//
// IMPORTANT: the proxy's lifetime must match the inline RunFunc's lifetime.
// Serve() runs in a goroutine until Stop() is called; call Stop() in the
// RunFunc's defer.
func NewMonitorProxy(ctx *pulumi.Context, logger hclog.Logger) (*MonitorProxy, error) {
	m, err := monitorFromContext(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "cannot extract Pulumi monitor from context — "+
			"Pulumi SDK may have changed its internals; refresh monitorFromContext")
	}
	return &MonitorProxy{monitor: m, logger: logger}, nil
}

// Serve binds a loopback TCP listener, starts the gRPC server, and returns
// its address. Caller MUST call Stop() when done.
func (p *MonitorProxy) Serve() (string, func(), error) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, errors.Wrap(err, "listen monitor proxy")
	}
	srv := grpc.NewServer()
	pulumirpc.RegisterResourceMonitorServer(srv, p)
	go func() {
		if err := srv.Serve(lis); err != nil && p.logger != nil {
			p.logger.Debug("monitor proxy stopped", "err", err)
		}
	}()
	stop := func() { srv.Stop() }
	return lis.Addr().String(), stop, nil
}

// RegisterResource forwards to the real Pulumi monitor. This is the one method
// plugins currently exercise.
func (p *MonitorProxy) RegisterResource(ctx context.Context, req *pulumirpc.RegisterResourceRequest) (*pulumirpc.RegisterResourceResponse, error) {
	if p.logger != nil {
		p.logger.Debug("proxy RegisterResource",
			"type", req.GetType(), "name", req.GetName(), "provider", req.GetProvider())
	}
	return p.monitor.RegisterResource(ctx, req)
}

// SupportsFeature forwards too — plugins often probe this before calling
// RegisterResource with options the engine may or may not understand.
func (p *MonitorProxy) SupportsFeature(ctx context.Context, req *pulumirpc.SupportsFeatureRequest) (*pulumirpc.SupportsFeatureResponse, error) {
	return p.monitor.SupportsFeature(ctx, req)
}

// RegisterResourceOutputs forwards for completeness — components use it.
func (p *MonitorProxy) RegisterResourceOutputs(ctx context.Context, req *pulumirpc.RegisterResourceOutputsRequest) (*emptypb.Empty, error) {
	return p.monitor.RegisterResourceOutputs(ctx, req)
}

// monitorFromContext reads the unexported `monitor` field from
// pulumi.Context.state. If the field name or path changes in a Pulumi SDK
// upgrade, this returns an error (callers get a clear message instead of a
// nil deref).
func monitorFromContext(ctx *pulumi.Context) (monitorAccessor, error) {
	if ctx == nil {
		return nil, errors.New("nil pulumi.Context")
	}
	cv := reflect.ValueOf(ctx).Elem()
	stateField := cv.FieldByName("state")
	if !stateField.IsValid() || stateField.Kind() != reflect.Ptr {
		return nil, errors.New("pulumi.Context has no `state` field (SDK renamed?)")
	}
	// Access the unexported `state` pointer value.
	statePtr := reflect.NewAt(stateField.Type(), unsafe.Pointer(stateField.UnsafeAddr())).Elem()
	if statePtr.IsNil() {
		return nil, errors.New("pulumi.Context.state is nil")
	}
	stateVal := statePtr.Elem()
	monField := stateVal.FieldByName("monitor")
	if !monField.IsValid() {
		return nil, errors.New("pulumi.Context.state has no `monitor` field (SDK renamed?)")
	}
	// Materialise an interface value from the unexported field.
	monVal := reflect.NewAt(monField.Type(), unsafe.Pointer(monField.UnsafeAddr())).Elem()
	iface := monVal.Interface()
	m, ok := iface.(pulumirpc.ResourceMonitorClient)
	if !ok {
		return nil, errors.Errorf("pulumi.Context.state.monitor is %T, not ResourceMonitorClient", iface)
	}
	return m, nil
}
