package pulumi

import (
	"context"

	"github.com/samber/lo"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/simple-container-com/api/pkg/api"
)

func (p *pulumi) destroyChildStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.DestroyParams) error {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Destroying child stack %q...", s.Ref().String())
	parentStack := stack.Client.Stacks[params.Environment].ParentStack
	fullStackName := s.Ref().FullyQualifiedName().String()
	program := p.deployStackProgram(stack, params.StackParams, parentStack, fullStackName)
	stackSource, err := auto.UpsertStackInlineSource(ctx, s.Ref().FullyQualifiedName().String(), cfg.ProjectName, program)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Refreshing child stack %q...", stackSource.Name())
	refreshResult, err := stackSource.Refresh(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Refresh child summary: %q", p.toRefreshResult(refreshResult))
	p.logger.Info(ctx, "Destroying child stack %q...", stackSource.Name())
	destroyResult, err := stackSource.Destroy(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Destroy child summary: %q", p.toDestroyResult(destroyResult))
	s, err = p.validateStateAndGetStack(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Removing child stack: %q...", stackSource.Name())
	res, err := p.backend.RemoveStack(ctx, s, false)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Removed child stack: %s", lo.If(res, "WARN: some resources have remained!").Else("all resources have been destroyed"))
	return nil
}

func (p *pulumi) destroyParentStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack) error {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return err
	}

	p.logger.Info(ctx, "Found parent stack %q", s.Ref().String())

	stackSource, err := auto.UpsertStackInlineSource(ctx, s.Ref().FullyQualifiedName().String(), cfg.ProjectName, p.provisionProgram(stack))
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Refreshing parent stack %q...", stackSource.Name())
	refreshResult, err := stackSource.Refresh(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Refresh parent summary: %q", p.toRefreshResult(refreshResult))
	p.logger.Info(ctx, "Destroying parent stack %q...", stackSource.Name())
	destroyResult, err := stackSource.Destroy(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Destroy parent summary: %q", p.toDestroyResult(destroyResult))
	s, err = p.validateStateAndGetStack(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Removing parent stack: %q...", stackSource.Name())
	res, err := p.backend.RemoveStack(ctx, s, false)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Removed parent stack: %s", lo.If(res, "WARN: some resources have remained!").Else("all resources have been destroyed"))

	return nil
}
