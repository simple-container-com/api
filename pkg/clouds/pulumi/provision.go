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
	"github.com/simple-container-com/api/pkg/clouds/pulumi/params"
)

func (p *pulumi) provisionStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack) error {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return err
	}

	p.logger.Info(ctx, "Found stack %q", s.Ref().String())

	stackSource, err := auto.UpsertStackInlineSource(ctx, stack.Name, cfg.ProjectName, func(ctx *sdk.Context) error {
		if err := p.initialProvisionProgram(ctx); err != nil {
			return errors.Wrapf(err, "failed to provision init program")
		}

		// TODO: provision resources for stack with the use of secrets provider output
		p.logger.Info(ctx.Context(), "secrets provider output: %v", p.secretsProviderOutput)

		for env, resources := range stack.Server.Resources.Resources {
			p.logger.Info(ctx.Context(), "provisioning resources for stack %q in env %q...", stack.Name, env)
			for resName, res := range resources.Resources {
				p.logger.Info(ctx.Context(), "provisioning resource %q for stack %q in env %q", resName, stack.Name, env)

				provisionParams, err := p.getProvisionParams(ctx, stack, res)
				if err != nil {
					return errors.Wrapf(err, "failed to init provision params for %q", res.Type)
				}

				if fnc, ok := provisionFuncByType[res.Type]; !ok {
					return errors.Errorf("unknown resource type %q", res.Type)
				} else if _, err := fnc(ctx, stack, api.ResourceInput{
					Log:        p.logger,
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

func (p *pulumi) validateStateAndGetStack(ctx context.Context) (backend.Stack, error) {
	if p.backend == nil {
		return nil, errors.Errorf("backend is nil")
	}
	if p.stackRef == nil {
		return nil, errors.Errorf("stackRef is nil")
	}

	if s, err := p.backend.GetStack(ctx, p.stackRef); err != nil {
		return nil, errors.Errorf("failed to get stack %q", p.stackRef.Name())
	} else {
		return s, nil
	}
}

func (p *pulumi) getProvisionParams(ctx *sdk.Context, stack api.Stack, res api.ResourceDescriptor) (params.ProvisionParams, error) {
	p.pParamsMutex.Lock()
	defer p.pParamsMutex.Unlock()

	var provider sdk.ProviderResource
	providerName := fmt.Sprintf("%s-provider", res.Type)

	// check cache and return if found
	if pParams, ok := p.pParamsMap[providerName]; ok {
		return pParams, nil
	}

	if providerArgsFunc, ok := pulumiProviderArgsByType[res.Type]; !ok {
		return params.ProvisionParams{}, errors.Errorf("unsupported provider for resource type %q in stack %q", res.Type, stack.Name)
	} else if providerArgs, err := providerArgsFunc(res.Config); err != nil {
		return params.ProvisionParams{}, errors.Errorf("failed to cast config to provider args for %q in stack %q", res.Type, stack.Name)
	} else if providerFunc, ok := providerFuncByType[res.Type]; !ok {
		return params.ProvisionParams{}, errors.Errorf("unsupported provider for resource type %q in stack %q", res.Type, stack.Name)
	} else if out, err := providerFunc(ctx, stack, api.ResourceInput{
		Log: p.logger,
		Descriptor: &api.ResourceDescriptor{
			Type: res.Type,
			Name: providerName,
			Config: api.Config{
				Config: providerArgs,
			},
		},
	}, params.ProvisionParams{}); err != nil {
	} else if provider, ok = out.Ref.(sdk.ProviderResource); !ok {
		return params.ProvisionParams{}, errors.Errorf("failed to cast ref to sdk.ProviderResource for %q in stack %q", res.Type, stack.Name)
	}
	pParams := params.ProvisionParams{
		Provider: provider,
	}
	p.pParamsMap[providerName] = pParams
	return pParams, nil
}

func stackDescriptorTemplateName(stackName, templateName string) string {
	return fmt.Sprintf("%s/%s", stackName, templateName)
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

	resDescriptor := api.ResourceDescriptor{
		Type:   provisionerCfg.SecretsProvider.Type,
		Name:   fmt.Sprintf("%s-secrets-provider", stack.Name),
		Config: provisionerCfg.SecretsProvider.Config,
	}
	provisionParams, err := p.getProvisionParams(ctx, stack, resDescriptor)
	if err != nil {
		return errors.Wrapf(err, "failed to init provision params for %q in stack %q", provisionerCfg.SecretsProvider.Type, stack.Name)
	}

	if fnc, ok := provisionFuncByType[provisionerCfg.SecretsProvider.Type]; ok {
		out, err := fnc(ctx, stack, api.ResourceInput{
			Log:        p.logger,
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
