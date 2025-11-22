package pulumi

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi/pkg/v3/backend"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optrefresh"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/internal/build"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger/color"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func (p *pulumi) provisionStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.ProvisionParams) error {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return err
	}

	p.logger.Info(ctx, "Found stack %q", s.Ref().FullyQualifiedName())

	stackSource, err := p.prepareStackForOperations(ctx, s.Ref(), cfg, p.provisionProgram(stack, cfg))
	if err != nil {
		return err
	}

	if timeoutDuration, err := time.ParseDuration(params.Timeouts.ExecutionTimeout); err == nil {
		p.logger.Info(ctx, color.YellowFmt("Setting timeout on whole execution %q...", timeoutDuration.String()))
		ctxWithTimeout, cancel := context.WithTimeout(ctx, timeoutDuration)
		ctx = ctxWithTimeout
		defer cancel()
	}

	if !params.SkipRefresh {
		p.logger.Info(ctx, "Refreshing stack %q...", s.Ref().FullyQualifiedName())
		refreshResult, err := stackSource.Refresh(ctx, optrefresh.EventStreams(p.watchEvents(WithContextAction(ctx, ActionContextRefresh))))
		if err != nil {
			return err
		}
		p.logger.Info(ctx, "Refresh summary: \n%s", p.toRefreshResult(refreshResult))
	}
	if !params.SkipPreview {
		p.logger.Info(ctx, color.GreenFmt("Previewing stack %q...", s.Ref().FullyQualifiedName()))

		previewOpts := []optpreview.Option{
			optpreview.EventStreams(p.watchEvents(WithContextAction(ctx, ActionContextPreview))),
		}

		// Add detailed diff option if requested (default to true for better visibility)
		if params.DetailedDiff {
			previewOpts = append(previewOpts, optpreview.Diff())
			p.logger.Info(ctx, "🔍 Diff enabled - showing granular changes for nested properties")
		}

		previewResult, err := stackSource.Preview(ctx, previewOpts...)
		if err != nil {
			return err
		}
		p.logger.Info(ctx, color.GreenFmt("Preview summary: \n%s", p.toPreviewResult(stackSource.Name(), previewResult)))
	}
	upOpts := []optup.Option{
		optup.EventStreams(p.watchEvents(WithContextAction(ctx, ActionContextProvision))),
	}

	// Add detailed diff option if requested
	if params.DetailedDiff {
		upOpts = append(upOpts, optup.Diff())
		p.logger.Info(ctx, "🔍 Diff enabled for update operation - showing granular changes")
	}

	updateRes, err := stackSource.Up(ctx, upOpts...)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, color.GreenFmt("Update summary: \n%s", p.toUpdateResult(stackSource.Name(), updateRes)))
	return nil
}

func (p *pulumi) prepareStackForOperations(ctx context.Context, ref backend.StackReference, cfg *api.ConfigFile, program sdk.RunFunc) (auto.Stack, error) {
	var stackSource auto.Stack
	var err error
	if program != nil {
		stackSource, err = auto.UpsertStackInlineSource(ctx, ref.FullyQualifiedName().String(), cfg.ProjectName, program, p.wsOpts...)
		if err != nil {
			return stackSource, err
		}
		if p.secretsProviderUrl != "" {
			if err = stackSource.ChangeSecretsProvider(ctx, p.secretsProviderUrl, nil); err != nil {
				return stackSource, err
			}
		}

	} else {
		stackSource, err = auto.SelectStackInlineSource(ctx, ref.FullyQualifiedName().String(), cfg.ProjectName, nil, p.wsOpts...)
		if err != nil {
			return stackSource, err
		}
	}
	if stackSource.Workspace() != nil {
		envVars := make(map[string]string)
		for _, kv := range os.Environ() {
			split := strings.SplitN(kv, "=", 2)
			envVars[split[0]] = split[1]
		}
		if err := stackSource.Workspace().SetEnvVars(envVars); err != nil {
			p.logger.Error(ctx, "failed to set environment variables for pulumi workspace: %v", err)
		}
		if p.secretsProviderPassphrase != "" {
			if err := os.Setenv(pApi.ConfigPassphraseEnvVar, p.secretsProviderPassphrase); err != nil {
				p.logger.Warn(ctx, "failed to set %s var", pApi.ConfigPassphraseEnvVar)
			}
			err := stackSource.Workspace().ChangeStackSecretsProvider(ctx, ref.FullyQualifiedName().String(), "passphrase", &auto.ChangeSecretsProviderOptions{
				NewPassphrase: lo.ToPtr(p.secretsProviderPassphrase),
			})
			if err != nil {
				p.logger.Error(ctx, "failed to set passphrase secrets provider for stack: %v", err)
				return stackSource, err
			}
		}
	}
	return stackSource, nil
}

func (p *pulumi) provisionProgram(stack api.Stack, cfg *api.ConfigFile) func(ctx *sdk.Context) error {
	program := func(ctx *sdk.Context) error {
		if p.preProvisionProgram != nil {
			if err := p.preProvisionProgram(ctx); err != nil {
				return errors.Wrapf(err, "failed to provision init program")
			}
		}

		if err := p.initRegistrar(ctx, stack, nil); err != nil {
			return errors.Wrapf(err, "failed to init registar")
		}

		if _, nc := p.registrar.(*notConfigured); !nc && p.registrar != nil {
			if _, err := p.registrar.ProvisionRecords(ctx, pApi.ProvisionParams{
				Log: p.logger,
			}); err != nil {
				return errors.Wrapf(err, "failed to provision base DNS records for stack %q", stack.Name)
			}
		}

		for env, resources := range stack.Server.Resources.Resources {
			p.logger.Info(ctx.Context(), "configure resources for stack %q in env %q...", stack.Name, env)
			collector := pApi.NewComputeContextCollector(ctx.Context(), p.logger, stack.Name, env)

			resourcesWithoutDeps := make(map[string]api.ResourceDescriptor)
			resourcesWithDeps := make(map[string]api.ResourceDescriptor)

			// figure out dependencies first
			for resName, resource := range resources.Resources {
				if withDeps, ok := resource.Config.Config.(api.WithParentDependencies); ok {
					deps := lo.Map(withDeps.DependsOnResources(), func(d api.ParentResourceDependency, _ int) string {
						return d.Name
					})
					p.logger.Info(ctx.Context(), "resource %q in stack %q in env %q has dependencies on %q", resName, stack.Name, env, deps)
					resourcesWithDeps[resName] = resource
				} else {
					resourcesWithoutDeps[resName] = resource
				}
			}

			// TODO: validate there are no cycles in dependencies

			// provision resources without deps
			outs := make(map[string]*api.ResourceOutput)
			for resName, resource := range resourcesWithoutDeps {
				if out, err := p.configureResource(ctx, stack, env, resName, resource, collector, outs); err != nil {
					return errors.Wrapf(err, "failed to provision resource %q of env %q", resName, env)
				} else {
					outs[resName] = out
				}
			}

			// provision resources with deps
			for resName, resource := range resourcesWithDeps {
				if _, err := p.configureResource(ctx, stack, env, resName, resource, collector, outs); err != nil {
					return errors.Wrapf(err, "failed to provision resource %q of env %q", resName, env)
				}
			}
		}
		for templateName, stackDesc := range stack.Server.Templates {
			serializedStackDesc, err := yaml.Marshal(stackDesc)
			if err != nil {
				return errors.Wrapf(err, "failed to serialize template's %q descriptor", templateName)
			}

			fullStackRef := pApi.ExpandStackReference(stack.Name, p.provisionerCfg.Organization, cfg.ProjectName)
			outputName := stackDescriptorTemplateName(fullStackRef, templateName)
			p.logger.Debug(ctx.Context(), "preserving template %q in the stack's %q outputs as %q...", templateName, stack.Name, outputName)
			secretOutput := sdk.ToSecret(string(serializedStackDesc))
			ctx.Export(outputName, secretOutput)
		}

		return nil
	}
	return program
}

func (p *pulumi) configureResource(ctx *sdk.Context, stack api.Stack, env string, resName string, res api.ResourceDescriptor, collector pApi.ComputeContext, outs pApi.ResourcesOutputs) (*api.ResourceOutput, error) {
	p.logger.Info(ctx.Context(), "configure resource %q for stack %q in env %q", resName, stack.Name, env)
	if res.Name == "" {
		res.Name = resName
	}
	provisionParams, err := p.getProvisionParams(ctx, stack, res, env, "")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to init provision params for %q", res.Type)
	}
	provisionParams.ComputeContext = collector
	provisionParams.ResourceOutputs = outs

	if fnc, ok := pApi.ProvisionFuncByType[res.Type]; !ok {
		return nil, errors.Errorf("unknown resource type %q", res.Type)
	} else if out, err := fnc(ctx, stack, api.ResourceInput{
		Descriptor: &res,
		StackParams: &api.StackParams{
			StackName:   stack.Name,
			Environment: env,
		},
	}, provisionParams); err != nil {
		return nil, errors.Wrapf(err, "failed to provision resource %q of env %q", resName, env)
	} else {
		return out, nil
	}
}

func (p *pulumi) initRegistrar(ctx *sdk.Context, stack api.Stack, dnsPreference *pApi.DnsPreference) error {
	registrarType := stack.Server.Resources.Registrar.Type
	p.logger.Info(ctx.Context(), "configure registrar of type %q for stack %q...", registrarType, stack.Name)
	if registrarInit, ok := pApi.RegistrarFuncByType[registrarType]; !ok {
		return errors.Errorf("unsupported registrar type %q for stack %q", registrarType, stack.Name)
	} else if reg, err := registrarInit(ctx, stack.Server.Resources.Registrar, pApi.ProvisionParams{
		Log:           p.logger,
		DnsPreference: dnsPreference,
	}); err != nil {
		return errors.Wrapf(err, "failed to init registrar for stack %q", stack.Name)
	} else {
		p.registrar = reg
	}
	return nil
}

func (p *pulumi) validateStateAndGetStack(ctx context.Context) (backend.Stack, error) {
	if p.backend == nil {
		return nil, errors.Errorf("backend is nil")
	}
	if p.stackRef == nil {
		return nil, errors.Errorf("stackRef is nil")
	}
	if p.stackRef.FullyQualifiedName() == "" {
		return nil, errors.Errorf("stack reference is not set")
	}

	if s, err := p.backend.GetStack(ctx, p.stackRef); err != nil {
		return nil, errors.Errorf("failed to get stack %q", p.stackRef.FullyQualifiedName())
	} else if s != nil {
		return s, nil
	} else {
		return nil, errors.Errorf("failed to get stack %q: ref is nil", p.stackRef.FullyQualifiedName())
	}
}

func (p *pulumi) getProvisionParams(ctx *sdk.Context, stack api.Stack, res api.ResourceDescriptor, environment string, suffix string) (pApi.ProvisionParams, error) {
	envVariables := map[string]string{
		api.ComputeEnv.StackName:               stack.Name,
		api.ComputeEnv.StackEnv:                environment,
		api.ScContainerResourceTypeEnvVariable: res.Type,
	}

	providerType, provider, err := p.initProvider(ctx, stack, lo.If(suffix != "", fmt.Sprintf("%s--%s", res.Name, suffix)).Else(res.Name), res.Type, res.Config, environment)
	if err != nil {
		return pApi.ProvisionParams{}, errors.Wrapf(err, "failed to init main provider for resource %q of type %q in stack %q", res.Name, res.Type, stack.Name)
	}

	dependencyProviders := make(map[string]pApi.DependencyProvider)

	if depProviders, ok := res.Config.Config.(api.WithDependencyProviders); ok {
		for name, pCfg := range depProviders.DependencyProviders() {
			_, dProvider, err := p.initProvider(ctx, stack, res.Name+"-dep-"+name, res.Type, pCfg.Config, environment)
			if err != nil {
				return pApi.ProvisionParams{}, errors.Wrapf(err, "failed to init dependency provider for resource %q of type %q in stack %q", res.Name, res.Type, stack.Name)
			}
			dependencyProviders[name] = pApi.DependencyProvider{
				Provider: dProvider,
				Config:   pCfg.Config,
			}
		}
	}

	return pApi.ProvisionParams{
		Provider:            provider,
		Registrar:           p.registrar,
		Log:                 p.logger,
		BaseEnvVariables:    envVariables,
		HelpersImage:        p.cloudHelpersImage(providerType),
		DependencyProviders: dependencyProviders,
	}, nil
}

func (p *pulumi) initProvider(ctx *sdk.Context, stack api.Stack, resName string, resType string, pCfg api.Config, environment string) (string, sdk.ProviderResource, error) {
	var provider sdk.ProviderResource
	var providerType string
	if authCfg, ok := pCfg.Config.(api.AuthConfig); !ok {
		return "", nil, errors.Errorf("failed to cast config to api.AuthConfig for %q of type %q in stack %q (given %T)", resName, resType, stack.Name, pCfg.Config)
	} else if providerFunc, ok := pApi.ProviderFuncByType[authCfg.ProviderType()]; !ok {
		return "", nil, errors.Errorf("unsupported provider type %q for resource %q of type %q in stack %q", authCfg.ProviderType(), resName, resType, stack.Name)
	} else {
		providerType = authCfg.ProviderType()
		providerName := fmt.Sprintf("%s--%s--%s--%s--provider", stack.Name, resType, resName, environment)
		if out, err := providerFunc(ctx, stack, api.ResourceInput{
			Descriptor: &api.ResourceDescriptor{
				Type:   authCfg.ProviderType(),
				Name:   providerName,
				Config: pCfg,
			},
			StackParams: &api.StackParams{
				StackName:   stack.Name,
				Environment: environment,
			},
		}, pApi.ProvisionParams{
			Log:       p.logger,
			Registrar: p.registrar,
		}); err != nil {
			return "", nil, errors.Wrapf(err, "failed to init provider of type %q for resource %q of type %q in stack %q", providerType, resName, resType, stack.Name)
		} else if provider, ok = out.Ref.(sdk.ProviderResource); !ok {
			return "", nil, errors.Errorf("failed to cast ref to sdk.ProviderResource for type %q of resource %q of type %q in stack %q", providerType, resName, resType, stack.Name)
		} else {
			providerType = authCfg.ProviderType()
		}
	}
	return providerType, provider, nil
}

func (p *pulumi) cloudHelpersImage(providerType string) string {
	return fmt.Sprintf("docker.io/simplecontainer/cloud-helpers:%s-%s", providerType, build.Version)
}

func stackDescriptorTemplateName(stackName, templateName string) string {
	return fmt.Sprintf("%s/%s", stackName, templateName)
}

func (p *pulumi) provisionSecretsProvider(ctx *sdk.Context, provisionerCfg *ProvisionerConfig, stack api.Stack, exportName string) error {
	secretsProviderCfg, ok := provisionerCfg.SecretsProvider.Config.Config.(api.SecretsProviderConfig)
	if !ok {
		return errors.Errorf("secrets provider config is not of type api.SecretsProviderConfig for %q", provisionerCfg.SecretsProvider.Type)
	}

	if !secretsProviderCfg.IsProvisionEnabled() {
		p.logger.Info(ctx.Context(), "skip provisioning of secrets provider for stack %q", stack.Name)
		return nil
	}
	p.logger.Info(ctx.Context(), "configure secrets provider of type %s for stack %q...", provisionerCfg.SecretsProvider.Type, stack.Name)

	resDescriptor := api.ResourceDescriptor{
		Type:   provisionerCfg.SecretsProvider.Type,
		Name:   exportName,
		Config: provisionerCfg.SecretsProvider.Config,
	}
	provisionParams, err := p.getProvisionParams(ctx, stack, resDescriptor, "", "")
	if err != nil {
		return errors.Wrapf(err, "failed to init provision params for %q in stack %q", provisionerCfg.SecretsProvider.Type, stack.Name)
	}

	if fnc, ok := pApi.ProvisionFuncByType[provisionerCfg.SecretsProvider.Type]; ok {
		_, err := fnc(ctx, stack, api.ResourceInput{
			Descriptor: &resDescriptor,
			StackParams: &api.StackParams{
				StackName: stack.Name,
			},
		}, provisionParams)
		if err != nil {
			return errors.Wrapf(err, "failed to provision secrets provider of type %q", provisionerCfg.SecretsProvider.Type)
		}
		return nil
	}
	return errors.Errorf("unknown secrets provider type %q", provisionerCfg.SecretsProvider.Type)
}
