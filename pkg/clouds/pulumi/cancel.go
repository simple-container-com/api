package pulumi

import (
	"context"

	"github.com/simple-container-com/api/pkg/api/logger/color"

	"github.com/simple-container-com/api/pkg/api"
)

func (p *pulumi) CancelStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.StackParams) error {
	if params.Environment != "" && params.StackName != "" {
		stack = toChildStack(stack, params)
	}
	_, err := p.selectStack(ctx, cfg, stack)
	if err != nil {
		return err
	}
	return p.cancelStack(ctx, cfg)
}

func (p *pulumi) cancelStack(ctx context.Context, cfg *api.ConfigFile) error {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return err
	}
	stackSource, err := p.prepareStackForOperations(ctx, s.Ref(), cfg, nil)
	if err != nil {
		return err
	}

	p.logger.Info(ctx, color.RedFmt("Canceling stack %q...", s.Ref().String()))
	err = stackSource.Cancel(ctx)
	if err != nil {
		return err
	}
	return nil
}
