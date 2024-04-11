package pulumi

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"
	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger/color"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func (p *pulumi) deployStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.DeployParams) error {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, color.GreenFmt("Deploying stack %q...", s.Ref().FullyQualifiedName()))
	parentStack := stack.Client.Stacks[params.Environment].ParentStack
	fullStackName := s.Ref().FullyQualifiedName().String()

	program := p.deployStackProgram(stack, params.StackParams, parentStack, fullStackName)
	stackSource, err := auto.UpsertStackInlineSource(ctx, s.Ref().FullyQualifiedName().String(), cfg.ProjectName, program)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, color.GreenFmt("Refreshing stack %q...", stackSource.Name()))
	refreshResult, err := stackSource.Refresh(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, color.GreenFmt("Refresh summary: \n%s", p.toRefreshResult(refreshResult)))
	p.logger.Info(ctx, color.GreenFmt("Preview stack %q...", stackSource.Name()))
	previewResult, err := stackSource.Preview(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, color.GreenFmt("Preview summary: \n%s", p.toPreviewResult(stackSource.Name(), previewResult)))
	p.logger.Info(ctx, color.GreenFmt("Updating stack %q...", stackSource.Name()))
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

		stackDir := params.StackDir

		if stackDir == "" {
			// assuming stack's directory is related to stacks
			if params.StacksDir == "" {
				return errors.Errorf("either single stack's or all stacks directory must be specified")
			}
			stackDir = filepath.Join(params.StacksDir, params.StackName)
		}

		clientStackDesc, err := api.PrepareClientConfigForDeploy(ctx.Context(), stackDir, fullStackName, stackDesc, stackClientDesc)
		if err != nil {
			return errors.Wrapf(err, "failed to prepare client descriptor for deploy for stack %q in env %q", fullStackName, params.Environment)
		}

		uses := make(map[string]bool)
		if a, resAware := clientStackDesc.Config.Config.(api.ResourceAware); resAware {
			uses = lo.Associate(a.Uses(), func(resName string) (string, bool) {
				return resName, true
			})
		}

		p.logger.Debug(ctx.Context(), "converted compose to cloud compose input: %q", clientStackDesc)

		collector := pApi.NewComputeContextCollector(stack.Name, params.Environment)
		for resName, res := range stack.Server.Resources.Resources[params.Environment].Resources {
			if !uses[resName] {
				p.logger.Info(ctx.Context(), "stack %q does not use resource %q, skipping...", stack.Name, resName)
				continue
			}
			if fnc, ok := computeProcessorFuncByType[res.Type]; !ok {
				p.logger.Info(ctx.Context(), "could not find compute processor for resource %q of type %q, skipping...", resName, res.Type)
				continue
			} else if provisionParams, err := p.getProvisionParams(ctx, stack, res, params.Environment); err != nil {
				p.logger.Warn(ctx.Context(), "failed to get provision params for resource %q of type %q in stack %q: %q", resName, res.Type, stack.Name, err.Error())
				continue
			} else {
				if res.Name == "" {
					res.Name = resName
				}
				provisionParams.ParentStack = &pApi.ParentInfo{
					StackName: parentStack,
					RefString: parentRefString,
				}
				provisionParams.ComputeContext = collector
				_, err := fnc(ctx, stack, api.ResourceInput{
					Descriptor:  &res,
					StackParams: &params,
				}, collector, provisionParams)
				if err != nil {
					return errors.Wrapf(err, "failed to process compute context for resource %q of env %q", resName, params.Environment)
				}
			}
		}

		sdk.ToArrayOutput(collector.Outputs()).ApplyT(func(args []any) (any, error) {
			resDesc := api.ResourceDescriptor{
				Type:   clientStackDesc.Type,
				Name:   fullStackName,
				Config: clientStackDesc.Config,
			}
			p.logger.Debug(ctx.Context(), "getting provisioning params for %q in stack %q", clientStackDesc)
			provisionParams, err := p.getProvisionParams(ctx, stack, resDesc, params.Environment)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to init provision params for %q", resDesc.Type)
			}
			provisionParams.ComputeContext = collector

			if fnc, ok := provisionFuncByType[resDesc.Type]; !ok {
				return nil, errors.Errorf("unknown resource type %q", resDesc.Type)
			} else if _, err := fnc(ctx, stack, api.ResourceInput{
				Descriptor:  &resDesc,
				StackParams: &params,
			}, provisionParams); err != nil {
				return nil, errors.Wrapf(err, "failed to provision stack %q in env %q", fullStackName, params.Environment)
			}
			return nil, nil
		})
		return nil
	}
}

func getSecretValueFromStack(ctx *sdk.Context, refName, outName string, proc func(val string) error) error {
	// Create a StackReference to the parent stack
	ref, err := sdk.NewStackReference(ctx, fmt.Sprintf("%s-ref", outName), &sdk.StackReferenceArgs{
		Name: sdk.String(refName).ToStringOutput(),
	})
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
