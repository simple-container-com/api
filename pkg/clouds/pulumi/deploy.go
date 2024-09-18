package pulumi

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optrefresh"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

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
		refreshResult, err := stackSource.Refresh(ctx, optrefresh.EventStreams(p.watchEvents(WithContextAction(ctx, ActionContextRefresh))))
		if err != nil {
			return err
		}
		p.logger.Info(ctx, color.GreenFmt("Refresh summary: \n%s", p.toRefreshResult(refreshResult)))
	}
	if !params.SkipPreview {
		p.logger.Info(ctx, color.GreenFmt("Preview stack %q...", stackSource.Name()))
		previewResult, err := stackSource.Preview(ctx, optpreview.EventStreams(p.watchEvents(WithContextAction(ctx, ActionContextPreview))))
		if err != nil {
			return err
		}
		p.logger.Info(ctx, color.GreenFmt("Preview summary: \n%s", p.toPreviewResult(stackSource.Name(), previewResult)))
	}
	p.logger.Info(ctx, color.GreenFmt("Updating stack %q...", stackSource.Name()))
	if timeoutDuration, err := time.ParseDuration(params.Timeout); err == nil {
		p.logger.Info(ctx, color.YellowFmt("Setting timeout on deployment %q...", timeoutDuration.String()))
		ctxWithTimeout, cancel := context.WithTimeout(ctx, timeoutDuration)
		ctx = ctxWithTimeout
		defer cancel()
	}

	upRes, err := stackSource.Up(ctx, optup.EventStreams(p.watchEvents(WithContextAction(ctx, ActionContextDeploy))))
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
		if stackClientDesc.Template != "" {
			templateName = stackClientDesc.Template
		}
		if templateName == "" {
			return errors.Errorf("no template configured for stack %q in env %q", parentStack, params.Environment)
		}

		parentFullReference := pApi.ExpandStackReference(parentStack, p.provisionerCfg.Organization, p.configFile.ProjectName)
		parentNameOnly := pApi.CollapseStackReference(parentFullReference)

		// get template from parent
		templateRef := stackDescriptorTemplateName(parentFullReference, templateName)
		var stackDesc api.StackDescriptor
		stackDescYaml, err := pApi.GetStringValueFromStack(ctx, fmt.Sprintf("%s-template", parentFullReference), parentFullReference, templateRef, true)
		if err != nil {
			return errors.Wrapf(err, "failed to get template descriptpor for stack %q in %q", parentStack, params.Environment)
		}
		if stackDescYaml == "" {
			return errors.Errorf("no template descriptor for stack %q in %q, consider re-provisioning of parent stack", parentStack, params.Environment)
		}
		err = yaml.Unmarshal([]byte(stackDescYaml), &stackDesc)
		if err != nil {
			return errors.Wrapf(err, "failed to serialize template's %q descriptor", templateName)
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

		dnsPreference := &pApi.DnsPreference{}
		if a, dnsAware := clientStackDesc.Config.Config.(api.DnsConfigAware); dnsAware {
			dnsPreference.BaseZone = a.OverriddenBaseZone()
		}
		if err := p.initRegistrar(ctx, stack, dnsPreference); err != nil {
			return errors.Errorf("failed to init registrar for stack %q in env %q", fullStackName, params.Environment)
		}

		uses := make(map[string]bool)
		if a, resAware := clientStackDesc.Config.Config.(api.ResourceAware); resAware {
			uses = lo.Associate(a.Uses(), func(resName string) (string, bool) {
				return resName, true
			})
		}

		var dependsOnResourcesList []api.StackConfigDependencyResource
		if info, withDependsOnResources := clientStackDesc.Config.Config.(api.WithDependsOnResources); withDependsOnResources {
			dependsOnResourcesList = append(dependsOnResourcesList, info.DependsOnResources()...)
		}
		dependsOn := lo.Associate(dependsOnResourcesList, func(dep api.StackConfigDependencyResource) (string, bool) {
			return dep.Resource, true
		})

		p.logger.Debug(ctx.Context(), "converted compose to cloud compose input: %q", clientStackDesc)

		collector := pApi.NewComputeContextCollector(ctx.Context(), p.logger, stack.Name, params.Environment)
		for resName, res := range stack.Server.Resources.Resources[params.Environment].Resources {
			if res.Name == "" {
				res.Name = resName
			}
			if !uses[resName] && !dependsOn[resName] {
				p.logger.Info(ctx.Context(), "stack %q does not use or depend on resource %q, skipping...", stack.Name, resName)
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
					StackName:     parentNameOnly,
					FullReference: parentFullReference,
				}
				provisionParams.UseResources = uses
				provisionParams.DependOnResources = dependsOnResourcesList
				provisionParams.StackDescriptor = clientStackDesc
				provisionParams.ComputeContext = collector
				_, err = fnc(ctx, stack, api.ResourceInput{
					Descriptor:  &res,
					StackParams: &params,
				}, collector, provisionParams)
				if err != nil {
					return errors.Wrapf(err, "failed to process compute context for resource %q of env %q", resName, params.Environment)
				}
			}
		}

		deployResOut := sdk.ToArrayOutput(collector.Outputs()).ApplyTWithContext(ctx.Context(), func(args []any) (string, error) {
			// resolve resource-dependent client placeholders that have remained after initial resolve of basic values
			if err := collector.ResolvePlaceholders(&clientStackDesc.Config.Config); err != nil {
				p.logger.Error(ctx.Context(), "failed to resolve placeholders for secrets in stack %q in %q: %v", stack.Name, params.Environment, err)
				return "failure", errors.Wrapf(err, "failed to resolve placeholders for secrets in stack %q in %q", stack.Name, params.Environment)
			}

			resDesc := api.ResourceDescriptor{
				Type:   clientStackDesc.Type,
				Name:   fullStackName,
				Config: clientStackDesc.Config,
			}

			provisionParams, err := p.getProvisionParams(ctx, stack, resDesc, params.Environment)
			if err != nil {
				p.logger.Error(ctx.Context(), "failed to init provision params for %q: %v", resDesc.Type, err)
				return "failure", errors.Wrapf(err, "failed to init provision params for %q", resDesc.Type)
			}
			provisionParams.ComputeContext = collector
			provisionParams.StackDescriptor = clientStackDesc
			provisionParams.ParentStack = &pApi.ParentInfo{
				StackName:     parentNameOnly,
				FullReference: parentFullReference,
			}

			if fnc, ok := pApi.ProvisionFuncByType[resDesc.Type]; !ok {
				p.logger.Error(ctx.Context(), "unknown resource type %q", resDesc.Type)
				return "failure", errors.Errorf("unknown resource type %q", resDesc.Type)
			} else if _, err := fnc(ctx, stack, api.ResourceInput{
				Descriptor:  &resDesc,
				StackParams: &params,
			}, provisionParams); err != nil {
				p.logger.Error(ctx.Context(), "failed to provision stack %q in env %q: %v", fullStackName, params.Environment, err)
				return "failure", errors.Wrapf(err, "failed to provision stack %q in env %q", fullStackName, params.Environment)
			}
			return "success", nil
		})
		ctx.Export(fmt.Sprintf("%s-%s-outcome", params.StackName, params.Environment), deployResOut)
		return nil
	}
}
