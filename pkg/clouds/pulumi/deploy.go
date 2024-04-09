package pulumi

import (
	"context"
	"fmt"
	"path/filepath"

	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"

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
	p.logger.Info(ctx, "Deploying stack %q...", s.Ref().FullyQualifiedName())
	parentStack := stack.Client.Stacks[params.Environment].ParentStack
	fullStackName := s.Ref().FullyQualifiedName().String()

	program := p.deployStackProgram(stack, params.StackParams, parentStack, fullStackName)
	stackSource, err := auto.UpsertStackInlineSource(ctx, s.Ref().FullyQualifiedName().String(), cfg.ProjectName, program)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Refreshing stack %q...", stackSource.Name())
	refreshResult, err := stackSource.Refresh(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Refresh summary: %q", p.toRefreshResult(refreshResult))
	p.logger.Info(ctx, "Preview stack %q...", stackSource.Name())
	previewResult, err := stackSource.Preview(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Preview summary: %q", p.toPreviewResult(stackSource.Name(), previewResult))
	p.logger.Info(ctx, "Updating stack %q...", stackSource.Name())
	_, err = stackSource.Up(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (p *pulumi) deployStackProgram(stack api.Stack, params api.StackParams, parentStack string, fullStackName string) func(ctx *sdk.Context) error {
	return func(ctx *sdk.Context) error {
		stackClientDesc := stack.Client.Stacks[params.Environment]
		templateName := stack.Server.Resources.Resources[params.Environment].Template
		if templateName == "" {
			return errors.Errorf("no template configured for stack %q in env %q", parentStack, params.Environment)
		}

		if err := p.initRegistrar(ctx, stack, params.Environment); err != nil {
			return errors.Errorf("failed to init registrar for stack %q in env %q", fullStackName, params.Environment)
		}

		parentRefString := fmt.Sprintf("%s/%s/%s", p.provisionerCfg.Organization, p.configFile.ProjectName, parentStack)

		// get template from parent
		templateRef := stackDescriptorTemplateName(parentStack, templateName)
		var stackDesc api.StackDescriptor
		err := getSecretValueFromStack(ctx, parentRefString, templateRef, func(val string) error {
			err := yaml.Unmarshal([]byte(val), &stackDesc)
			if err != nil {
				return errors.Wrapf(err, "failed to serialize template's %q descriptor", templateName)
			}
			return nil
		})
		if err != nil {
			return errors.Wrapf(err, "failed to get template descriptpor for stack %q in %q", parentStack, params.Environment)
		}
		// get outputs from parent
		outputsRef := stackOutputValuesName(parentStack, params.Environment)
		var outputs pApi.OutputsCollector
		err = getSecretValueFromStack(ctx, parentRefString, outputsRef, func(val string) error {
			outputs = pApi.CollectorFromJson(val)
			return nil
		})
		if err != nil {
			return errors.Wrapf(err, "failed to get outputs for stack %q in %q", parentStack, params.Environment)
		}
		p.logger.Info(ctx.Context(), "got outputs from parent stack", outputs)

		if params.StacksDir == "" {
			return errors.Errorf("stacks directory must be specified")
		}
		stackDir := filepath.Join(params.StacksDir, params.StackName)

		deployInput, err := api.PrepareClientConfigForDeploy(ctx.Context(), stackDir, fullStackName, stackDesc, stackClientDesc)
		if err != nil {
			return errors.Wrapf(err, "failed to prepare client descriptor for deploy for stack %q in env %q", fullStackName, params.Environment)
		}

		p.logger.Debug(ctx.Context(), "converted compose to cloud compose input: %q", deployInput)

		resDesc := api.ResourceDescriptor{
			Type:   deployInput.Type,
			Name:   fullStackName,
			Config: deployInput.Config,
		}
		p.logger.Debug(ctx.Context(), "getting provisioning params for %q in stack %q", deployInput)
		provisionParams, err := p.getProvisionParams(ctx, stack, resDesc, params.Environment)
		if err != nil {
			return errors.Wrapf(err, "failed to init provision params for %q", resDesc.Type)
		}

		if fnc, ok := provisionFuncByType[resDesc.Type]; !ok {
			return errors.Errorf("unknown resource type %q", resDesc.Type)
		} else if _, err := fnc(ctx, stack, api.ResourceInput{
			Descriptor:   &resDesc,
			DeployParams: &params,
		}, provisionParams); err != nil {
			return errors.Wrapf(err, "failed to provision stack %q in env %q", fullStackName, params.Environment)
		}
		return nil
	}
}

func getSecretValueFromStack(ctx *sdk.Context, refName, outName string, proc func(val string) error) error {
	// Create a StackReference to the parent stack
	ref, err := sdk.NewStackReference(ctx, refName, nil)
	if err != nil {
		return err
	}
	parentOutput, err := ref.GetOutputDetails(outName)
	if err != nil {
		return errors.Wrapf(err, "failed to get output %q from %q", outName, refName)
	}
	if parentOutput.SecretValue == nil {
		return errors.Wrapf(err, "no secret value for output %q from %q", outName, refName)
	}
	if proc != nil {
		return proc(parentOutput.SecretValue.(string))
	}
	return nil
}
