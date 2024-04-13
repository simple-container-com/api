package pulumi

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
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

	stackSource, err := p.prepareStackForOperations(ctx, s.Ref(), cfg, p.deployStackProgram(stack, params.StackParams, parentStack, fullStackName))
	if err != nil {
		return err
	}

	if !params.SkipRefresh {
		p.logger.Info(ctx, color.GreenFmt("Refreshing stack %q...", stackSource.Name()))
		refreshResult, err := stackSource.Refresh(ctx)
		if err != nil {
			return err
		}
		p.logger.Info(ctx, color.GreenFmt("Refresh summary: \n%s", p.toRefreshResult(refreshResult)))
	}
	if !params.SkipPreview {
		p.logger.Info(ctx, color.GreenFmt("Preview stack %q...", stackSource.Name()))
		previewResult, err := stackSource.Preview(ctx)
		if err != nil {
			return err
		}
		p.logger.Info(ctx, color.GreenFmt("Preview summary: \n%s", p.toPreviewResult(stackSource.Name(), previewResult)))
	}
	p.logger.Info(ctx, color.GreenFmt("Updating stack %q...", stackSource.Name()))
	upRes, err := stackSource.Up(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, color.GreenFmt("Update summary: \n%s", p.toUpdateResult(stackSource.Name(), upRes)))
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

		parentRefString := expandStackReference(parentStack, p.provisionerCfg.Organization, p.configFile.ProjectName)

		// get template from parent
		templateRef := stackDescriptorTemplateName(parentRefString, templateName)
		var stackDesc api.StackDescriptor
		err := getSecretValueFromStack(ctx, parentRefString, templateRef, func(val string) error {
			if val == "" {
				return errors.Errorf("no template descriptor for stack %q in %q, consider re-provisioning of parent stack", parentStack, params.Environment)
			}
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
			if res.Name == "" {
				res.Name = resName
			}
			if !uses[resName] {
				p.logger.Info(ctx.Context(), "stack %q does not use resource %q, skipping...", stack.Name, resName)
				continue
			}
			if fnc, ok := pApi.ComputeProcessorFuncByType[res.Type]; !ok {
				p.logger.Info(ctx.Context(), "could not find compute processor for resource %q of type %q, skipping...", resName, res.Type)
				continue
			} else if provisionParams, err := p.getProvisionParams(ctx, stack, res, params.Environment); err != nil {
				p.logger.Warn(ctx.Context(), "failed to get provision params for resource %q of type %q in stack %q: %q", resName, res.Type, stack.Name, err.Error())
				continue
			} else {
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
			// resolve resource-dependent client placeholders that have remained after initial resolve of basic values
			if err := collector.ResolvePlaceholders(&clientStackDesc.Config.Config); err != nil {
				return nil, errors.Wrapf(err, "failed to resolve placeholders for secrets in stack %q in %q", stack.Name, params.Environment)
			}

			resDesc := api.ResourceDescriptor{
				Type:   clientStackDesc.Type,
				Name:   fullStackName,
				Config: clientStackDesc.Config,
			}

			provisionParams, err := p.getProvisionParams(ctx, stack, resDesc, params.Environment)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to init provision params for %q", resDesc.Type)
			}
			provisionParams.ComputeContext = collector

			if fnc, ok := pApi.ProvisionFuncByType[resDesc.Type]; !ok {
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

func expandStackReference(parentStack string, organization string, projectName string) string {
	parentStackParts := strings.SplitN(parentStack, "/", 3)
	if len(parentStackParts) == 3 {
		return parentStack
	} else if len(parentStackParts) == 2 {
		return fmt.Sprintf("%s/%s", organization, parentStack)
	} else {
		return fmt.Sprintf("%s/%s/%s", organization, projectName, parentStack)
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
