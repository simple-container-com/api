package pulumi

import (
	"context"

	"github.com/pulumi/pulumi/pkg/v3/backend"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optrefresh"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger/color"
)

func (p *pulumi) destroyStack(ctx context.Context, cfg *api.ConfigFile, s backend.Stack, skipRefresh bool) error {
	stackSource, err := p.prepareStackForOperations(ctx, s.Ref(), cfg, nil)
	if err != nil {
		return err
	}

	if !skipRefresh {
		p.logger.Info(ctx, color.YellowFmt("Refreshing stack %q...", stackSource.Name()))
		refreshResult, err := stackSource.Refresh(ctx, optrefresh.EventStreams(p.watchEvents(ctx)))
		if err != nil {
			return err
		}
		p.logger.Info(ctx, color.YellowFmt("Refresh summary: \n%s", p.toRefreshResult(refreshResult)))
	}
	p.logger.Info(ctx, color.RedFmt("Destroying stack %q...", stackSource.Name()))
	destroyResult, err := stackSource.Destroy(ctx, optdestroy.EventStreams(p.watchEvents(ctx)))
	if err != nil {
		return err
	}
	p.logger.Info(ctx, color.RedFmt("Destroy summary: \n%s", p.toDestroyResult(destroyResult)))
	s, err = p.validateStateAndGetStack(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, color.RedFmt("Removing stack: %q...", stackSource.Name()))
	res, err := p.backend.RemoveStack(ctx, s, false)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, color.RedFmt("Removed stack: %s",
		lo.If(res, "WARN: some resources have remained!").Else("all resources have been destroyed")))

	if p.secretsStackRef != nil {
		defer p.withPulumiPassphrase(ctx)()
		sStack, err := p.backend.GetStack(ctx, p.secretsStackRef)
		if err != nil {
			return err
		}
		ssSource, err := auto.UpsertStackInlineSource(ctx, p.secretsStackRef.FullyQualifiedName().String(), cfg.ProjectName, nil)
		if err != nil {
			return err
		}
		p.logger.Info(ctx, color.RedFmt("Destroying stack %q...", ssSource.Name()))
		destroyResult, err = ssSource.Destroy(ctx, optdestroy.EventStreams(p.watchEvents(ctx)))
		if err != nil {
			return err
		}
		p.logger.Info(ctx, color.RedFmt("Destroy summary: \n%s", p.toDestroyResult(destroyResult)))
		_, err = p.backend.RemoveStack(ctx, sStack, false)
		if err != nil {
			return err
		}
	}
	return nil
}
