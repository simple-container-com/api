package pulumi

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger/color"
)

func (p *pulumi) previewStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.ProvisionParams) (*api.PreviewResult, error) {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return nil, err
	}
	p.logger.Info(ctx, color.GreenFmt("Previewing parent stack %q...", s.Ref().FullyQualifiedName().String()))
	stackSource, err := p.prepareStackForOperations(ctx, s.Ref(), cfg, p.provisionProgram(stack, cfg))
	if err != nil {
		return nil, err
	}
	if !params.SkipRefresh {
		p.logger.Info(ctx, color.GreenFmt("Refreshing parent stack %q...", stackSource.Name()))
		refreshResult, err := stackSource.Refresh(ctx)
		if err != nil {
			return nil, err
		}
		p.logger.Info(ctx, color.GreenFmt("Refresh parent summary: %q", p.toRefreshResult(refreshResult)))
	}

	p.logger.Info(ctx, "Preview parent stack %q...", stackSource.Name())
	previewResult, err := stackSource.Preview(ctx, optpreview.EventStreams(p.watchEvents(WithContextAction(ctx, ActionContextPreview))))
	if err != nil {
		return nil, err
	}
	p.logger.Info(ctx, color.GreenFmt("Preview parent summary: %q", p.toPreviewResult(stackSource.Name(), previewResult)))
	return p.toPreviewResult(stackSource.Name(), previewResult), nil
}

func (p *pulumi) previewChildStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.DeployParams) (*api.PreviewResult, error) {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return nil, err
	}
	p.logger.Info(ctx, color.GreenFmt("Previewing child stack %q...", s.Ref().FullyQualifiedName().String()))

	parentStack := stack.Client.Stacks[params.Environment].ParentStack
	fullStackName := s.Ref().FullyQualifiedName().String()

	program := p.deployStackProgram(stack, params.StackParams, parentStack, fullStackName)
	stackSource, err := p.prepareStackForOperations(ctx, s.Ref(), cfg, program)
	if err != nil {
		return nil, err
	}
	p.logger.Info(ctx, color.GreenFmt("Preview child stack %q...", stackSource.Name()))
	previewResult, err := stackSource.Preview(ctx, optpreview.EventStreams(p.watchEvents(WithContextAction(ctx, ActionContextPreview))))
	if err != nil {
		return nil, err
	}
	p.logger.Info(ctx, color.GreenFmt("Preview child summary: %q", p.toPreviewResult(stackSource.Name(), previewResult)))
	return p.toPreviewResult(stackSource.Name(), previewResult), nil
}

func (p *pulumi) OutputsStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.StackParams) (*api.OutputsResult, error) {
	if params.Environment != "" && params.StackName != "" {
		stack = toChildStack(stack, params)
	}
	s, err := p.selectStack(ctx, cfg, stack)
	if err != nil {
		return nil, err
	}

	stackSource, err := p.prepareStackForOperations(ctx, s.Ref(), cfg, nil)
	if err != nil {
		return nil, err
	}

	res, err := stackSource.Outputs(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get outputs")
	}
	return p.toOutputsResult(stackSource.Name(), res), nil
}

func (p *pulumi) toOutputsResult(stackName string, result auto.OutputMap) *api.OutputsResult {
	return &api.OutputsResult{
		StackName: stackName,
		Outputs: lo.MapValues(result, func(value auto.OutputValue, key string) any {
			var res string
			if s, ok := value.Value.(string); ok {
				res = s
			} else if m, ok := value.Value.(map[string]any); ok {
				j, _ := json.Marshal(m)
				res = string(j)
			} else if value.Secret {
				j, _ := json.Marshal(value)
				res = string(j)
			}
			return res
		}),
	}
}

func (p *pulumi) toUpdateResult(stackName string, result auto.UpResult) *api.UpdateResult {
	changes := map[string]int{}
	if result.Summary.ResourceChanges != nil {
		changes = *result.Summary.ResourceChanges
	}
	return &api.UpdateResult{
		StackName:  stackName,
		Summary:    result.StdOut,
		Operations: changes,
	}
}

func (p *pulumi) toPreviewResult(stackName string, result auto.PreviewResult) *api.PreviewResult {
	return &api.PreviewResult{
		StackName: stackName,
		Summary:   result.StdOut,
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
