package pulumi

import (
	"context"
	"fmt"
	"gopkg.in/yaml.v3"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
)

func (p *pulumi) deployStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.DeployParams) error {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Found stack %q", s.Ref().String())
	fullStackName := fmt.Sprintf("%s-%s-%s", cfg.ProjectName, params.Stack, params.Environment)

	stackSource, err := auto.UpsertStackInlineSource(ctx, fullStackName, cfg.ProjectName, func(ctx *sdk.Context) error {

		stackClientDesc := stack.Client.Stacks[params.Environment]
		templateName := stack.Server.Resources.Resources[params.Environment].Template
		if templateName == "" {
			return errors.Errorf("no template for stack %q in env %q", params.Stack, params.Environment)
		}

		// Create a StackReference to the parent stack
		parentRef, err := sdk.NewStackReference(ctx, params.Stack, nil)
		if err != nil {
			return err
		}

		templateRef := stackDescriptorTemplateName(params.Stack, templateName)
		parentOutput, err := parentRef.GetOutputDetails(templateRef)
		if err != nil {
			return errors.Wrapf(err, "failed to get template descriptpor for stack %q in %q", params.Stack, params.Environment)
		}
		if parentOutput.SecretValue == nil {
			return errors.Errorf("no secret value for template %q in stack %q for env %q", templateName, params.Stack, params.Environment)
		}
		var stackDesc api.StackDescriptor
		err = yaml.Unmarshal([]byte(parentOutput.SecretValue.(string)), &stackDesc)
		if err != nil {
			return errors.Wrapf(err, "failed to serialize template's %q descriptor", templateName)
		}
		_, err = api.ConvertTemplateToCloudCompose(ctx.Context(), params.RootDir, params.Stack, stackDesc, stackClientDesc)
		return err
	})
	if err != nil {
		return err
	}
	_, err = stackSource.Up(ctx)
	if err != nil {
		return err
	}
	return nil
}
