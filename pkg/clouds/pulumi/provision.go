package pulumi

import (
	"context"

	"api/pkg/clouds/pulumi/gcp"

	"github.com/pkg/errors"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"api/pkg/api"
)

func (p *pulumi) provisionStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack) error {
	provisionerCfg, err := p.getProvisionerConfig(stack)
	if err != nil {
		return err
	}

	sdkCtx, be, stackRef, err := p.login(ctx, cfg, stack)
	if err != nil {
		return err
	}
	s, err := be.GetStack(ctx, stackRef)
	if err != nil {
		return err
	}

	p.logger.Info(ctx, "Found stack %q", s.Ref().String())

	return sdk.RunWithContext(sdkCtx, func(ctx *sdk.Context) error {
		if err := p.provisionSecretsProvider(ctx, provisionerCfg, stack); err != nil {
			return err
		}
		return nil
	})
}

func (p *pulumi) provisionSecretsProvider(ctx *sdk.Context, provisionerCfg *ProvisionerConfig, stack api.Stack) error {
	if !provisionerCfg.SecretsProvider.Provision {
		p.logger.Info(ctx.Context(), "Skipping provisioning of secrets provider for stack %q", stack.Name)
	}
	switch provisionerCfg.SecretsProvider.Type {
	case SecretsProviderTypeGcpKms:
		if key, err := gcp.ProvisionKmsKey(ctx, stack, gcp.KmsKeyInput{
			KeyRingName:       stack.Name,
			KeyName:           provisionerCfg.SecretsProvider.KeyName,
			KeyLocation:       provisionerCfg.SecretsProvider.KeyLocation,
			KeyRotationPeriod: provisionerCfg.SecretsProvider.KeyRotationPeriod,
		}); err != nil {
			return err
		} else {
			provisionerCfg.kmsKey = key
		}
	default:
		return errors.Errorf("unknown secrets provider type %q", provisionerCfg.SecretsProvider.Type)
	}
	return nil
}
