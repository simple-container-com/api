package pulumi

import (
	"context"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/simple-container-com/api/pkg/api"
)

func (p *pulumi) cancelStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.DeployParams) error {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return err
	}
	parentStack := stack.Client.Stacks[params.Environment].ParentStack
	fullStackName := s.Ref().FullyQualifiedName().String()

	program := p.deployStackProgram(stack, params.StackParams, parentStack, fullStackName)
	stackSource, err := auto.UpsertStackInlineSource(ctx, s.Ref().FullyQualifiedName().String(), cfg.ProjectName, program)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Canceling stack %q...", s.Ref().String())
	err = stackSource.Cancel(ctx)
	if err != nil {
		return err
	}
	return nil
}
