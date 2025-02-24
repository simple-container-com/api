package yandex

import (
	"context"
	"github.com/pkg/errors"
	pYandex "github.com/pulumi/pulumi-yandex/sdk/go/yandex"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func InitStateStore(ctx context.Context, stateStoreCfg api.StateStorageConfig, log logger.Logger) error {
	_, ok := stateStoreCfg.(api.AuthConfig)
	if !ok {
		return errors.Errorf("failed to convert yandex state storage config to api.AuthConfig")
	}
	if !stateStoreCfg.IsProvisionEnabled() {
		return nil
	}

	return nil
}

func Provider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	pcfg, ok := input.Descriptor.Config.Config.(api.AuthConfig)
	if !ok {
		return nil, errors.Errorf("failed to cast config to api.AuthConfig")
	}

	creds := pcfg.CredentialsValue()
	projectId := pcfg.ProjectIdValue()

	provider, err := pYandex.NewProvider(ctx, input.ToResName(input.Descriptor.Name), &pYandex.ProviderArgs{
		ServiceAccountKeyFile: sdk.String(creds),
		CloudId:               sdk.String(projectId),
	})
	return &api.ResourceOutput{
		Ref: provider,
	}, err

}
