package pulumi

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/params"
)

func (p *pulumi) provisionStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack) error {
	if p.backend == nil {
		return errors.Errorf("backend is nil")
	}
	if p.stackRef == nil {
		return errors.Errorf("stackRef is nil")
	}

	s, err := p.backend.GetStack(ctx, p.stackRef)
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
			p.logger.Info(ctx.Context(), "provisioning resources for env %q...", env)
			for resName, res := range resources.Resources {
				p.logger.Info(ctx.Context(), "provisioning resource %q of env %q", resName, env)

				provisionParams, err := p.getProvisionParams(ctx, res)
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

func (p *pulumi) getProvisionParams(ctx *sdk.Context, res api.ResourceDescriptor) (params.ProvisionParams, error) {
	p.pParamsMutex.Lock()
	defer p.pParamsMutex.Unlock()

	var provider sdk.ProviderResource
	providerName := fmt.Sprintf("%s-provider", res.Type)

	// check cache and return if found
	if pParams, ok := p.pParamsMap[providerName]; ok {
		return pParams, nil
	}

	if fnc, ok := providerByType[res.Type]; !ok {
		return params.ProvisionParams{}, errors.Errorf("unsupported resource type %q", res.Type)
	} else if out, err := fnc(ctx, params.ProviderInput{
		Name:     providerName,
		Resource: res.Config.Config,
	}); err != nil {
		return params.ProvisionParams{}, errors.Errorf("failed to provision provider for resource %q", res.Type)
	} else {
		provider = out.Provider
	}
	pParams := params.ProvisionParams{
		Provider: provider,
	}
	p.pParamsMap[providerName] = pParams
	return pParams, nil
}

func (p *pulumi) provisionSecretsProvider(ctx *sdk.Context, provisionerCfg *ProvisionerConfig, stack api.Stack) error {
	if !provisionerCfg.SecretsProvider.IsProvisionEnabled() {
		p.logger.Info(ctx.Context(), "Skipping provisioning of secrets provider for stack %q", stack.Name)
		return nil
	}
	p.logger.Info(ctx.Context(), "Provisioning secrets provider of type %s for stack %q...", provisionerCfg.SecretsProvider.Type, stack.Name)

	resDescriptor := api.ResourceDescriptor{
		Type:   provisionerCfg.SecretsProvider.Type,
		Config: provisionerCfg.SecretsProvider.Config,
	}
	provisionParams, err := p.getProvisionParams(ctx, resDescriptor)
	if err != nil {
		return errors.Wrapf(err, "failed to init provision params for %q", provisionerCfg.SecretsProvider.Type)
	}

	if fnc, ok := provisionFuncByType[provisionerCfg.SecretsProvider.Type]; ok {
		_, err := fnc(ctx, stack, api.ResourceInput{
			Log:        p.logger,
			Descriptor: &resDescriptor,
		}, provisionParams)
		if err != nil {
			return errors.Wrapf(err, "failed to provision secrets provider of type %q", provisionerCfg.SecretsProvider.Type)
		}
		return nil
	}
	return errors.Errorf("unknown secrets provider type %q", provisionerCfg.SecretsProvider.Type)
}

type SecretsProviderOutput struct {
	Provider sdk.ProviderResource
	Resource sdk.ComponentResource
}
