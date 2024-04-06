package pulumi

import (
	"context"
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/simple-container-com/api/pkg/api"
)

func (p *pulumi) destroyChildStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.DestroyParams) error {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Destroying stack %q...", s.Ref().String())
	parentStack := params.ParentStack
	fullStackName := fmt.Sprintf("%s--%s--%s", cfg.ProjectName, parentStack, params.Environment)

	program := p.deployStackProgram(stack, params.StackParams, parentStack, fullStackName)
	stackSource, err := auto.UpsertStackInlineSource(ctx, fullStackName, cfg.ProjectName, program)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Refreshing stack %q...", stackSource.Name())
	refreshResult, err := stackSource.Refresh(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Refresh summary: %q", refreshResult.Summary)
	p.logger.Info(ctx, "Destroying stack %q...", stackSource.Name())
	destroyResult, err := stackSource.Destroy(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Destroy summary: %q", destroyResult.Summary)
	return nil
}

func (p *pulumi) destroyParentStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack) error {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return err
	}

	p.logger.Info(ctx, "Found stack %q", s.Ref().String())

	stackSource, err := auto.UpsertStackInlineSource(ctx, stack.Name, cfg.ProjectName, p.provisionProgram(stack))
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Refreshing stack %q...", stackSource.Name())
	refreshResult, err := stackSource.Refresh(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Refresh summary: %q", refreshResult.Summary)
	p.logger.Info(ctx, "Destroying stack %q...", stackSource.Name())
	destroyResult, err := stackSource.Destroy(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Destroy summary: %q", destroyResult.Summary)
	return nil
}
