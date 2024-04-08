package pulumi

import (
	"context"

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
	fullStackName := s.Ref().FullyQualifiedName().String()

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
	p.logger.Info(ctx, "Refresh summary: %q", p.toRefreshResult(refreshResult))
	p.logger.Info(ctx, "Preview stack %q...", stackSource.Name())
	previewResult, err := stackSource.Preview(ctx)
	if err != nil {
		return nil, err
	}
	p.logger.Info(ctx, "Preview summary: %q", p.toPreviewResult(previewResult))
	return p.toPreviewResult(previewResult), nil
}

func (p *pulumi) toPreviewResult(result auto.PreviewResult) *api.PreviewResult {
	return &api.PreviewResult{
		Operations: lo.MapKeys(result.ChangeSummary, func(value int, key apitype.OpType) string {
			return string(key)
		}),
	}
}

func (p *pulumi) toDestroyResult(result auto.DestroyResult) *api.DestroyResult {
	return &api.DestroyResult{
		Operations: lo.MapValues(*result.Summary.ResourceChanges, func(value int, key string) int {
			return int(value)
		}),
	}
}

func (p *pulumi) toRefreshResult(result auto.RefreshResult) *api.RefreshResult {
	return &api.RefreshResult{
		Operations: lo.MapValues(*result.Summary.ResourceChanges, func(value int, key string) int {
			return int(value)
		}),
	}
}
