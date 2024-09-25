package pulumi

import (
	"context"

	"github.com/samber/lo"

	"github.com/pulumi/pulumi/pkg/v3/backend"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optrefresh"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger/color"
)

func (p *pulumi) destroyStack(ctx context.Context, cfg *api.ConfigFile, s backend.Stack, params api.DestroyParams, program func(ctx *sdk.Context) error, preview bool) error {
	stackSource, err := p.prepareStackForOperations(ctx, s.Ref(), cfg, program)
	if err != nil {
		return err
	}

	if !params.SkipRefresh {
		p.logger.Info(ctx, color.YellowFmt("Refreshing stack %q...", s.Ref().FullyQualifiedName()))
		refreshResult, err := stackSource.Refresh(ctx, optrefresh.EventStreams(p.watchEvents(WithContextAction(ctx, ActionContextRefresh))))
		if err != nil {
			return err
		}
		p.logger.Info(ctx, color.YellowFmt("Refresh summary: \n%s", p.toRefreshResult(refreshResult)))
	}

	if preview {
		p.logger.Info(ctx, color.RedFmt("Previewing destroy stack %q...", s.Ref().FullyQualifiedName()))
		previewResult, err := stackSource.PreviewDestroy(ctx, optdestroy.EventStreams(p.watchEvents(WithContextAction(ctx, ActionContextDestroy))))
		if err != nil {
			return err
		}
		p.logger.Info(ctx, color.RedFmt("Preview destroy summary: \n%s", p.toPreviewResult(params.StackName, previewResult)))
		return nil
	}
	p.logger.Info(ctx, color.RedFmt("Destroying stack %q...", s.Ref().FullyQualifiedName()))
	destroyResult, err := stackSource.Destroy(ctx, optdestroy.EventStreams(p.watchEvents(WithContextAction(ctx, ActionContextDestroy))))
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
	if params.DestroySecretsStack && p.secretsStackRef != nil {
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
		destroyResult, err = ssSource.Destroy(ctx, optdestroy.EventStreams(p.watchEvents(WithContextAction(ctx, ActionContextDestroy))))
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
