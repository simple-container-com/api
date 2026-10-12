// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"

	"github.com/Masterminds/semver/v3"
	"github.com/pkg/errors"

	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
)

// minRefreshCLI is the first pulumi CLI without known engine bugs in
// `up --refresh --run-program`.
var minRefreshCLI = semver.MustParse("3.231.0")

// previewOptions never refresh: a refresh inside a preview reads resources
// whose provider configuration is still unknown as deleted, and reports them
// as created.
func (p *pulumi) previewOptions(ctx context.Context, action contextActionValue, diff bool) []optpreview.Option {
	opts := []optpreview.Option{optpreview.EventStreams(p.watchEvents(WithContextAction(ctx, action)))}
	if diff {
		opts = append(opts, optpreview.Diff())
	}
	return opts
}

// upOptions refresh inside the update with the program, so the providers get
// the configuration the stack uses now rather than the one stored in state.
func (p *pulumi) upOptions(ctx context.Context, action contextActionValue, skipRefresh, diff bool) []optup.Option {
	opts := []optup.Option{optup.EventStreams(p.watchEvents(WithContextAction(ctx, action)))}
	if !skipRefresh {
		opts = append(opts, optup.Refresh(), optup.RunProgram(true))
	}
	if diff {
		opts = append(opts, optup.Diff())
	}
	return opts
}

func checkRefreshCLI(version string, skipRefresh bool) error {
	if skipRefresh {
		return nil
	}
	v, err := semver.NewVersion(version)
	if err != nil {
		return errors.Wrapf(err, "failed to parse pulumi CLI version %q", version)
	}
	if v.LessThan(minRefreshCLI) {
		return errors.Errorf("pulumi CLI %s is too old to refresh with the program: upgrade to %s or later, or skip the refresh", v, minRefreshCLI)
	}
	return nil
}
