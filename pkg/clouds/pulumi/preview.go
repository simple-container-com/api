package pulumi

import (
	"context"
	"fmt"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/simple-container-com/api/pkg/api"
)

func (p *pulumi) previewStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.DeployParams) (*api.PreviewResult, error) {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return nil, err
	}
	p.logger.Info(ctx, "Deploying stack %q...", s.Ref().String())
	parentStack := params.ParentStack
	fullStackName := fmt.Sprintf("%s--%s--%s", cfg.ProjectName, parentStack, params.Environment)

	program := p.deployStackProgram(stack, params.StackParams, parentStack, fullStackName)
	stackSource, err := auto.UpsertStackInlineSource(ctx, s.Ref().FullyQualifiedName().String(), cfg.ProjectName, program)
	if err != nil {
		return nil, err
	}
	p.logger.Info(ctx, "Refreshing stack %q...", stackSource.Name())
	refreshResult, err := stackSource.Refresh(ctx)
	if err != nil {
		return nil, err
	}
	p.logger.Info(ctx, "Refresh summary: %q", refreshResult.Summary)
	p.logger.Info(ctx, "Preview stack %q...", stackSource.Name())
	previewResult, err := stackSource.Preview(ctx)
	if err != nil {
		return nil, err
	}
	p.logger.Info(ctx, "Preview summary: %q", previewResult.ChangeSummary)
	return &api.PreviewResult{
		Operations: lo.MapKeys(previewResult.ChangeSummary, func(value int, key apitype.OpType) string {
			return string(key)
		}),
	}, nil
}
