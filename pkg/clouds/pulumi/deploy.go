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
	p.logger.Info(ctx, "Deploying stack %q...", s.Ref().String())
	parentStack := params.ParentStack
	fullStackName := fmt.Sprintf("%s--%s--%s", cfg.ProjectName, parentStack, params.Environment)

	stackSource, err := auto.UpsertStackInlineSource(ctx, fullStackName, cfg.ProjectName, func(ctx *sdk.Context) error {
		stackClientDesc := stack.Client.Stacks[params.Environment]
		templateName := stack.Server.Resources.Resources[params.Environment].Template
		if templateName == "" {
			return errors.Errorf("no template configured for stack %q in env %q", parentStack, params.Environment)
		}

		if err := p.initRegistrar(ctx, stack); err != nil {
			return errors.Errorf("failed to init registrar for stack %q in env %q", fullStackName, params.Environment)
		}

		// Create a StackReference to the parent stack
		parentRef, err := sdk.NewStackReference(ctx, parentStack, nil)
		if err != nil {
			return err
		}

		templateRef := stackDescriptorTemplateName(parentStack, templateName)
		parentOutput, err := parentRef.GetOutputDetails(templateRef)
		if err != nil {
			return errors.Wrapf(err, "failed to get template descriptpor for stack %q in %q", parentStack, params.Environment)
		}
		if parentOutput.SecretValue == nil {
			return errors.Errorf("no secret value for template %q in stack %q for env %q", templateName, parentStack, params.Environment)
		}
		var stackDesc api.StackDescriptor
		err = yaml.Unmarshal([]byte(parentOutput.SecretValue.(string)), &stackDesc)
		if err != nil {
			return errors.Wrapf(err, "failed to serialize template's %q descriptor", templateName)
		}
		deployInput, err := api.PrepareClientConfigForDeploy(ctx.Context(), params.RootDir, fullStackName, stackDesc, stackClientDesc)
		if err != nil {
			return errors.Wrapf(err, "failed to prepare client descriptor for deploy for stack %q in env %q", fullStackName, params.Environment)
		}

		p.logger.Debug(ctx.Context(), "converted compose to cloud compose input: %q", deployInput)

		resDesc := api.ResourceDescriptor{
			Type:   deployInput.Type,
			Name:   fullStackName,
			Config: deployInput.Config,
		}
		provisionParams, err := p.getProvisionParams(ctx, stack, resDesc)
		if err != nil {
			return errors.Wrapf(err, "failed to init provision params for %q", resDesc.Type)
		}

		if fnc, ok := provisionFuncByType[resDesc.Type]; !ok {
			return errors.Errorf("unknown resource type %q", resDesc.Type)
		} else if _, err := fnc(ctx, stack, api.ResourceInput{
			Descriptor: &resDesc,
		}, provisionParams); err != nil {
			return errors.Wrapf(err, "failed to provision stack %q in env %q", fullStackName, params.Environment)
		}
		return nil
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
