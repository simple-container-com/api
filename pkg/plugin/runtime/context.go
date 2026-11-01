// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package runtime

import (
	"context"

	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// Context wraps a Pulumi SDK context plus the deferred teardown that drains
// pending RegisterResource goroutines before closing it. Every plugin RPC
// handler constructs one with NewContext and defers Settle on the result.
type Context struct {
	Pulumi *sdk.Context
	done   func() error
}

// Done returns the deferred teardown Settle expects. Also exposed for the
// rare caller that wants to drain the context without the generic Settle
// response-folding behaviour.
func (c *Context) Done() func() error {
	if c == nil {
		return nil
	}
	return c.done
}

type contextOptions struct {
	localEngineProgress *scPlugin.Publisher
	dryRunOverride      *bool
	newPulumiContext    func(context.Context, *scpluginv1.RequestHeader) (*sdk.Context, error)
}

// ContextOption configures NewContext's behaviour.
type ContextOption func(*contextOptions)

// WithLocalEngineFallback opts the context into using LocalEngineAddress when
// the request header carries no engine address. The progress publisher is the
// plugin's log sink — pulumi log calls routed through the local engine land
// on it. Without this option a header with no engine address returns an error
// (the aws plugin's historical shape).
func WithLocalEngineFallback(progress *scPlugin.Publisher) ContextOption {
	return func(o *contextOptions) { o.localEngineProgress = progress }
}

// WithDryRun overrides the header's dry-run flag. The usual callers do not
// need this — NewContext reads header.GetDryRun() by default. Exposed for
// tests that construct a header-less context directly.
func WithDryRun(dryRun bool) ContextOption {
	return func(o *contextOptions) { o.dryRunOverride = &dryRun }
}

// WithContextFactory replaces sdk.NewContext with a test seam. Production
// callers do not need this.
func WithContextFactory(fn func(context.Context, *scpluginv1.RequestHeader) (*sdk.Context, error)) ContextOption {
	return func(o *contextOptions) { o.newPulumiContext = fn }
}

// NewContext builds a Pulumi SDK context from a plugin request header plus
// the deferred teardown. The returned Context's done function waits for all
// outstanding RegisterResource goroutines and then closes the SDK context —
// handlers must call it via Settle (or Done()) before returning, or
// in-flight registrations will be dropped and SC core will plan a destroy
// of every state entry on the next apply.
//
// See forge/docs/design/2026-10-24/sc-plugin-architecture/04-no-destroy-invariant.md
// Addendum U for the cascade this guard prevents.
func NewContext(ctx context.Context, h *scpluginv1.RequestHeader, opts ...ContextOption) (*Context, error) {
	o := &contextOptions{}
	for _, opt := range opts {
		opt(o)
	}

	if o.newPulumiContext != nil {
		pctx, err := o.newPulumiContext(ctx, h)
		if err != nil {
			return nil, err
		}
		return &Context{Pulumi: pctx, done: func() error { return finishPulumiContext(pctx) }}, nil
	}

	if h.GetMonitorAddress() == "" {
		return nil, errors.New("no monitor address in request header")
	}

	engineAddr := h.GetEngineAddress()
	if o.localEngineProgress != nil {
		addr, err := LocalEngineAddress(engineAddr, o.localEngineProgress)
		if err != nil {
			return nil, errors.Wrap(err, "start local pulumi engine endpoint")
		}
		engineAddr = addr
	}

	dryRun := h.GetDryRun()
	if o.dryRunOverride != nil {
		dryRun = *o.dryRunOverride
	}

	pctx, err := sdk.NewContext(ctx, sdk.RunInfo{
		MonitorAddr: h.GetMonitorAddress(),
		EngineAddr:  engineAddr,
		Project:     h.GetProjectName(),
		Stack:       h.GetStackName(),
		DryRun:      dryRun,
	})
	if err != nil {
		return nil, errors.Wrap(err, "open pulumi context from monitor address")
	}
	return &Context{Pulumi: pctx, done: func() error { return finishPulumiContext(pctx) }}, nil
}

// Settle runs a handler's deferred context teardown and folds its error into
// the handler's result: the RPC response is only sent once every registration
// the handler started has reached the engine, and a registration that failed
// fails the RPC instead of being lost. A handler error already set takes
// precedence.
//
// Idiomatic use at the top of every handler:
//
//	rctx, err := runtime.NewContext(stdctx, req.GetHeader(), runtime.WithLocalEngineFallback(p.progress))
//	if err != nil { return nil, err }
//	defer runtime.Settle(rctx, &resp, &err)
//
// The handler's named return values must be (resp *T, err error) for Settle
// to fold them.
func Settle[T any](rctx *Context, resp **T, err *error) {
	if rctx == nil || rctx.done == nil {
		return
	}
	if doneErr := rctx.done(); doneErr != nil && *err == nil {
		*resp = nil
		*err = doneErr
	}
}
