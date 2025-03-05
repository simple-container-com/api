package pulumi

import (
	"context"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger/color"
)

func (p *pulumi) CancelStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.StackParams) error {
	if params.Environment != "" && params.StackName != "" {
		stack = toChildStack(stack, params)
	}
	_, err := p.selectStack(ctx, cfg, stack)
	if err != nil {
		return err
	}
	return p.cancelStack(ctx, cfg, stack, params)
}

func (p *pulumi) cancelStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.StackParams) error {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return err
	}
	var program func(ctx *sdk.Context) error
	if params.Environment != "" && params.StackName != "" {
		program = p.deployStackProgram(stack, params, stack.Name, s.Ref().FullyQualifiedName().String())
	} else {
		program = p.provisionProgram(stack, cfg)
	}
	stackSource, err := p.prepareStackForOperations(ctx, s.Ref(), cfg, program)
	if err != nil {
		return err
	}

	p.logger.Info(ctx, color.RedFmt("Canceling stack %q...", s.Ref().String()))
	err = stackSource.Cancel(ctx)
	if err != nil {
		return err
	}

	if p.secretsStackRef != nil {
		defer p.withPulumiPassphrase(ctx)()
		ssSource, err := auto.UpsertStackInlineSource(ctx, p.secretsStackRef.FullyQualifiedName().String(), cfg.ProjectName, p.provisionProgram(stack, cfg))
		if err != nil {
			return err
		}
		p.logger.Info(ctx, color.RedFmt("Canceling stack %q...", ssSource.Name()))
		err = ssSource.Cancel(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}
