// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
)

// The refresh runs inside the preview and the update, with the program, rather
// than as a separate `pulumi refresh`. A separate refresh configures each
// provider from state, so after a credential change (a rotated or deleted
// service-account key, a move to ambient credentials) every refresh of a stack
// not deployed since fails to authenticate until a deploy without refresh has
// rewritten the state. Running the program gives the providers the
// configuration the stack uses now.
//
// Inside the operation the program runs once, with the operation's dry-run
// semantics: a preview stays a preview (resources not created yet are unknown,
// nothing is written), and an update applies what it refreshed. A standalone
// `refresh --run-program` would instead run the program as a non-dry run, with
// empty outputs for resources that do not exist yet, and execute side effects
// guarded by !ctx.DryRun() before the update.

func previewRefresh(skip bool) []optpreview.Option {
	if skip {
		return nil
	}
	return []optpreview.Option{optpreview.Refresh(), optpreview.RunProgram(true)}
}

func upRefresh(skip bool) []optup.Option {
	if skip {
		return nil
	}
	return []optup.Option{optup.Refresh(), optup.RunProgram(true)}
}
