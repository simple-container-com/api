package pulumi

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/pkg/v3/backend"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
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

	if !params.SkipRefresh {
		p.logger.Info(ctx, "Refreshing stack %q...", s.Ref().FullyQualifiedName())
		refreshResult, err := stackSource.Refresh(ctx)
		if err != nil {
			return err
		}
		p.logger.Info(ctx, "Refresh summary: \n%s", p.toRefreshResult(refreshResult))
	}
	if !params.SkipPreview {
		p.logger.Info(ctx, color.GreenFmt("Previewing stack %q...", s.Ref().FullyQualifiedName()))
		previewResult, err := stackSource.Preview(ctx)
		if err != nil {
			return err
		}
		p.logger.Info(ctx, color.GreenFmt("Preview summary: \n%s", p.toPreviewResult(stackSource.Name(), previewResult)))
	}
	updateRes, err := stackSource.Up(ctx)
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
	} else {
		stackSource, err = auto.SelectStackInlineSource(ctx, ref.FullyQualifiedName().String(), cfg.ProjectName, nil, p.wsOpts...)
	}
	if err != nil {
		return stackSource, err
	}
	if p.secretsProviderUrl != "" {
		if err = stackSource.ChangeSecretsProvider(ctx, p.secretsProviderUrl, nil); err != nil {
			return stackSource, err
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
			p.logger.Info(ctx.Context(), "configure resources for stack %q in env %q...", color.Yellow(stack.Name), color.Yellow(env))
			collector := pApi.NewComputeContextCollector(ctx.Context(), p.logger, stack.Name, env)
			for resName, res := range resources.Resources {
				p.logger.Info(ctx.Context(), "configure resource %q for stack %q in env %q", color.Yellow(resName), color.Yellow(stack.Name), color.Yellow(env))
				if res.Name == "" {
					res.Name = resName
				}
				provisionParams, err := p.getProvisionParams(ctx, stack, res, env)
				if err != nil {
					return errors.Wrapf(err, "failed to init provision params for %q", res.Type)
				}
				provisionParams.ComputeContext = collector

				if fnc, ok := pApi.ProvisionFuncByType[res.Type]; !ok {
					return errors.Errorf("unknown resource type %q", res.Type)
				} else if _, err := fnc(ctx, stack, api.ResourceInput{
					Descriptor: &res,
					StackParams: &api.StackParams{
						StackName:   stack.Name,
						Environment: env,
					},
				}, provisionParams); err != nil {
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

func (p *pulumi) getProvisionParams(ctx *sdk.Context, stack api.Stack, res api.ResourceDescriptor, environment string) (pApi.ProvisionParams, error) {
	var provider sdk.ProviderResource
	providerName := fmt.Sprintf("%s--%s--%s--%s--provider", stack.Name, res.Type, res.Name, environment)

	envVariables := map[string]string{
		"SIMPLE_CONTAINER_STACK": stack.Name,
		"SIMPLE_CONTAINER_ENV":   environment,
	}
	var providerType string
	if authCfg, ok := res.Config.Config.(api.AuthConfig); !ok {
		return pApi.ProvisionParams{}, errors.Errorf("failed to cast config to api.AuthConfig for %q in stack %q", res.Type, stack.Name)
	} else if providerFunc, ok := pApi.ProviderFuncByType[authCfg.ProviderType()]; !ok {
		return pApi.ProvisionParams{}, errors.Errorf("unsupported provider type %q for resource type %q in stack %q", authCfg.ProviderType(), res.Type, stack.Name)
	} else if out, err := providerFunc(ctx, stack, api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type:   res.Type,
			Name:   providerName,
			Config: res.Config,
		},
		StackParams: &api.StackParams{
			StackName:   stack.Name,
			Environment: environment,
		},
	}, pApi.ProvisionParams{
		Log:       p.logger,
		Registrar: p.registrar,
	}); err != nil {
		return pApi.ProvisionParams{}, errors.Wrapf(err, "failed to init provider for %q in stack %q", res.Type, stack.Name)
	} else if provider, ok = out.Ref.(sdk.ProviderResource); !ok {
		return pApi.ProvisionParams{}, errors.Errorf("failed to cast ref to sdk.ProviderResource for %q in stack %q", res.Type, stack.Name)
	} else {
		envVariables[api.ScContainerResourceTypeEnvVariable] = res.Type
		providerType = authCfg.ProviderType()
	}
	return pApi.ProvisionParams{
		Provider:         provider,
		Registrar:        p.registrar,
		Log:              p.logger,
		BaseEnvVariables: envVariables,
		HelpersImage:     p.cloudHelpersImage(providerType),
	}, nil
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
	provisionParams, err := p.getProvisionParams(ctx, stack, resDescriptor, "")
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
