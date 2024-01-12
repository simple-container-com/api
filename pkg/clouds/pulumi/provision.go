package pulumi

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"api/pkg/api"
	"api/pkg/clouds/pulumi/gcp"
)

func (p *pulumi) provisionStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack) error {
	provisionerCfg, err := p.getProvisionerConfig(stack)
	if err != nil {
		return err
	}

	_, be, stackRef, err := p.login(ctx, cfg, stack)
	if err != nil {
		return err
	}
	s, err := be.GetStack(ctx, stackRef)
	if err != nil {
		return err
	}

	p.logger.Info(ctx, "Found stack %q", s.Ref().String())

	stackSource, err := auto.UpsertStackInlineSource(ctx, stack.Name, cfg.ProjectName, func(ctx *sdk.Context) error {
		// TODO: provision resources for stack with the use of secrets provider output
		p.logger.Info(ctx.Context(), "secrets provider output: %v", provisionerCfg.secretsProviderOutput)

		// stack.Server.Resources.Resources

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

func (p *pulumi) provisionSecretsProvider(ctx *sdk.Context, provisionerCfg *ProvisionerConfig, stack api.Stack) error {
	if !provisionerCfg.SecretsProvider.Provision {
		p.logger.Info(ctx.Context(), "Skipping provisioning of secrets provider for stack %q", stack.Name)
	}
	p.logger.Info(ctx.Context(), "Provisioning secrets provider of type %s for stack %q...", provisionerCfg.SecretsProvider.Type, stack.Name)
	switch provisionerCfg.SecretsProvider.Type {
	case SecretsProviderTypeGcpKms:
		return p.provisionSecretsProviderGcpKms(ctx, provisionerCfg, stack)
	default:
		return errors.Errorf("unknown secrets provider type %q", provisionerCfg.SecretsProvider.Type)
	}
}

type SecretsProviderOutput struct {
	Provider sdk.ProviderResource
	Resource sdk.ComponentResource
}

func (p *pulumi) provisionSecretsProviderGcpKms(ctx *sdk.Context, provisionerCfg *ProvisionerConfig, stack api.Stack) error {
	gcpProvider, err := gcp.ProvisionProvider(ctx, gcp.ProviderInput{
		Name:        fmt.Sprintf("%s-secrets-provider", stack.Name),
		Credentials: provisionerCfg.SecretsProvider.Credentials,
		ProjectId:   provisionerCfg.SecretsProvider.ProjectId,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to provision gcp provider")
	}
	if key, err := gcp.ProvisionKmsKey(ctx, stack, gcp.KmsKeyInput{
		KeyRingName:       stack.Name,
		KeyName:           provisionerCfg.SecretsProvider.KeyName,
		KeyLocation:       provisionerCfg.SecretsProvider.KeyLocation,
		KeyRotationPeriod: provisionerCfg.SecretsProvider.KeyRotationPeriod,
		Provider:          gcpProvider.Provider,
	}); err != nil {
		return err
	} else {
		provisionerCfg.secretsProviderOutput = &SecretsProviderOutput{
			Provider: gcpProvider.Provider,
			Resource: key,
		}
	}
	return nil
}
