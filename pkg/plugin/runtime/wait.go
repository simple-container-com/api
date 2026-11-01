// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package runtime is the shared RPC-handler scaffolding every SC plugin uses:
// context construction with a local-engine fallback, deferred drain-then-settle
// of the Pulumi SDK context, harvest of pctx exports into ProvisionResponse.Outputs,
// and the compute-process parent-env swap. The patterns here are the ones all
// five plugin extractions eventually converged on; packaging them here keeps the
// next extraction (gcp) from re-copying the same four helpers again.
//
// See forge/docs/design/2026-11-01/plugin-extraction-playbook/ for the trap
// catalogue this package exists to prevent, and forge/docs/design/2026-10-24/
// sc-plugin-architecture/04-no-destroy-invariant.md Addendum V for the
// methodology that produced it.
package runtime

import (
	_ "unsafe" // for go:linkname

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// pulumiContextWait is the Pulumi Go SDK's (*Context).wait: it blocks until
// every goroutine the context started has finished — the RegisterResource
// RPCs, which run asynchronously and only fire once their input Outputs
// resolve, and the Apply callbacks — then refuses further RPCs and returns
// the first RPC error. pulumi.RunWithContext calls it before a program exits.
//
// Plugins cannot use RunWithContext: it registers a pulumi:pulumi:Stack
// resource, and the plugin's context writes into SC core's stack which
// already has one. A context made with sdk.NewContext has no exported way
// to drain it, and without the drain a handler that returns and closes its
// context drops every registration still waiting on an Output.
//
// The pulumi SDK version is pinned in api's go.mod; a rename of this method
// in a future SDK release fails the link, not silently at runtime.
//
//go:linkname pulumiContextWait github.com/pulumi/pulumi/sdk/v3/go/pulumi.(*Context).wait
func pulumiContextWait(ctx *sdk.Context) error

// finishPulumiContext waits for all of pctx's outstanding work, then closes
// it, and returns the wait's error. A Close error stays ignored — by then
// every RPC has completed.
func finishPulumiContext(pctx *sdk.Context) error {
	err := pulumiContextWait(pctx)
	_ = pctx.Close()
	return err
}
