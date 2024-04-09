package pulumi

import (
	"context"
	"fmt"

	"github.com/pulumi/pulumi/pkg/v3/backend"
	"gopkg.in/yaml.v3"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func (p *pulumi) provisionStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack) error {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return err
	}

	p.logger.Info(ctx, "Found stack %q", s.Ref().FullyQualifiedName())

	stackSource, err := auto.UpsertStackInlineSource(ctx, s.Ref().FullyQualifiedName().String(), cfg.ProjectName, p.provisionProgram(stack))
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Refreshing stack %q...", s.Ref().FullyQualifiedName())
	refreshResult, err := stackSource.Refresh(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Refresh summary: %q", p.toRefreshResult(refreshResult))
	p.logger.Info(ctx, "Previewing stack %q...", s.Ref().FullyQualifiedName())
	previewResult, err := stackSource.Preview(ctx)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "Preview summary: %q", p.toPreviewResult(stackSource.Name(), previewResult))
	_, err = stackSource.Up(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (p *pulumi) provisionProgram(stack api.Stack) func(ctx *sdk.Context) error {
	program := func(ctx *sdk.Context) error {
		if err := p.initialProvisionProgram(ctx); err != nil {
			return errors.Wrapf(err, "failed to provision init program")
		}

		p.logger.Debug(ctx.Context(), "secrets provider output: %v", p.secretsProviderOutput)

		if err := p.initRegistrar(ctx, stack, ""); err != nil {
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
			p.logger.Info(ctx.Context(), "provisioning resources for stack %q in env %q...", stack.Name, env)
			collector := pApi.NewComputeContextCollector(stack.Name, env)
			for resName, res := range resources.Resources {
				p.logger.Info(ctx.Context(), "provisioning resource %q for stack %q in env %q", resName, stack.Name, env)

				provisionParams, err := p.getProvisionParams(ctx, stack, res, env)
				if err != nil {
					return errors.Wrapf(err, "failed to init provision params for %q", res.Type)
				}
				provisionParams.ComputeContext = collector

				if fnc, ok := provisionFuncByType[res.Type]; !ok {
					return errors.Errorf("unknown resource type %q", res.Type)
				} else if _, err := fnc(ctx, stack, api.ResourceInput{
					Descriptor: &res,
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

			outputName := stackDescriptorTemplateName(stack.Name, templateName)
			p.logger.Debug(ctx.Context(), "preserving template %q in the stack's %q outputs as %q...", templateName, stack.Name, outputName)
			secretOutput := sdk.ToSecret(string(serializedStackDesc))
			ctx.Export(outputName, secretOutput)
		}

		return nil
	}
	return program
}

func (p *pulumi) initRegistrar(ctx *sdk.Context, stack api.Stack, environment string) error {
	registrarType := stack.Server.Resources.Registrar.Type
	p.logger.Info(ctx.Context(), "provisioning registrar of type %q for stack %q...", registrarType, stack.Name)
	if registrarInit, ok := registrarInitFuncByType[registrarType]; !ok {
		return errors.Errorf("unsupported registrar type %q for stack %q", registrarType, stack.Name)
	} else if reg, err := registrarInit(ctx, stack.Server.Resources.Registrar, pApi.ProvisionParams{
		Log: p.logger,
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
		return nil, errors.Errorf("failed to get stack %q: ref is nil", p.stackRef)
	}
}

func (p *pulumi) getProvisionParams(ctx *sdk.Context, stack api.Stack, res api.ResourceDescriptor, environment string) (pApi.ProvisionParams, error) {
	var provider sdk.ProviderResource
	providerName := fmt.Sprintf("%s-%s-provider", stack.Name, res.Type)

	if authCfg, ok := res.Config.Config.(api.AuthConfig); !ok {
		return pApi.ProvisionParams{}, errors.Errorf("failed to cast config to api.AuthConfig for %q in stack %q", res.Type, stack.Name)
	} else if providerFunc, ok := providerFuncByType[authCfg.ProviderType()]; !ok {
		return pApi.ProvisionParams{}, errors.Errorf("unsupported provider type %q for resource type %q in stack %q", authCfg.ProviderType(), res.Type, stack.Name)
	} else if out, err := providerFunc(ctx, stack, api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type:   res.Type,
			Name:   providerName,
			Config: res.Config,
		},
	}, pApi.ProvisionParams{
		Log:       p.logger,
		Registrar: p.registrar,
	}); err != nil {
	} else if provider, ok = out.Ref.(sdk.ProviderResource); !ok {
		return pApi.ProvisionParams{}, errors.Errorf("failed to cast ref to sdk.ProviderResource for %q in stack %q", res.Type, stack.Name)
	}
	return pApi.ProvisionParams{
		Provider:  provider,
		Registrar: p.registrar,
		Log:       p.logger,
	}, nil
}

func stackDescriptorTemplateName(stackName, templateName string) string {
	return fmt.Sprintf("%s/%s", stackName, templateName)
}

func stackOutputValuesName(stackName string, env string) string {
	return fmt.Sprintf("%s/%s", stackName, env)
}

func (p *pulumi) provisionSecretsProvider(ctx *sdk.Context, provisionerCfg *ProvisionerConfig, stack api.Stack) error {
	secretsProviderCfg, ok := provisionerCfg.SecretsProvider.Config.Config.(api.SecretsProviderConfig)
	if !ok {
		return errors.Errorf("secrets provider config is not of type api.SecretsProviderConfig for %q", provisionerCfg.SecretsProvider.Type)
	}

	if !secretsProviderCfg.IsProvisionEnabled() {
		p.logger.Info(ctx.Context(), "Skipping provisioning of secrets provider for stack %q", stack.Name)
		return nil
	}
	p.logger.Info(ctx.Context(), "Provisioning secrets provider of type %s for stack %q...", provisionerCfg.SecretsProvider.Type, stack.Name)

	if provisionerCfg.SecretsProvider.Type == "pulumi-cloud" {
		p.logger.Info(ctx.Context(), "Do not need to provision secrets provider of type %s for stack %q...", provisionerCfg.SecretsProvider.Type, stack.Name)
		return nil
	}

	resDescriptor := api.ResourceDescriptor{
		Type:   provisionerCfg.SecretsProvider.Type,
		Name:   fmt.Sprintf("%s-secrets-provider", stack.Name),
		Config: provisionerCfg.SecretsProvider.Config,
	}
	provisionParams, err := p.getProvisionParams(ctx, stack, resDescriptor, "")
	if err != nil {
		return errors.Wrapf(err, "failed to init provision params for %q in stack %q", provisionerCfg.SecretsProvider.Type, stack.Name)
	}

	if fnc, ok := provisionFuncByType[provisionerCfg.SecretsProvider.Type]; ok {
		out, err := fnc(ctx, stack, api.ResourceInput{
			Descriptor: &resDescriptor,
		}, provisionParams)
		if err != nil {
			return errors.Wrapf(err, "failed to provision secrets provider of type %q", provisionerCfg.SecretsProvider.Type)
		}
		p.secretsProviderOutput = &SecretsProviderOutput{
			Provider: provisionParams.Provider,
			Resource: out.Ref.(sdk.ComponentResource),
		}
		return nil
	}
	return errors.Errorf("unknown secrets provider type %q", provisionerCfg.SecretsProvider.Type)
}

type SecretsProviderOutput struct {
	Provider sdk.ProviderResource
	Resource sdk.ComponentResource
}
