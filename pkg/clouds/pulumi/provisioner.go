package pulumi

import (
	"context"
	"sync"

	"github.com/simple-container-com/api/pkg/clouds/pulumi/params"

	"github.com/pulumi/pulumi/pkg/v3/backend"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
)

//go:generate ../../../bin/mockery --name Pulumi --output ./mocks --filename pulumi_mock.go --outpkg pulumi_mocks --structname PulumiMock
type Pulumi interface {
	api.Provisioner
}

type pulumi struct {
	logger logger.Logger
	pubKey string

	initialProvisionProgram func(ctx *sdk.Context) error
	stack                   *auto.Stack
	backend                 backend.Backend
	stackRef                backend.StackReference

	secretsProviderOutput *SecretsProviderOutput
	fieldConfigReader     api.ProvisionerFieldConfigReaderFunc
	pParamsMutex          sync.RWMutex
	pParamsMap            map[string]params.ProvisionParams
}

func InitPulumiProvisioner(config api.Config, opts ...api.ProvisionerOption) (api.Provisioner, error) {
	res := &pulumi{
		logger:     logger.New(),
		pParamsMap: make(map[string]params.ProvisionParams),
	}
	for _, opt := range opts {
		if err := opt(res); err != nil {
			return nil, err
		}
	}
	return readProvisionerFields(config, res)
}

func readProvisionerFields(config api.Config, res *pulumi) (api.Provisioner, error) {
	if res.fieldConfigReader != nil {
		if pConfig, ok := config.Config.(*ProvisionerConfig); ok {
			if stateStorageCfg, err := res.fieldConfigReader(pConfig.StateStorage.Type, &pConfig.StateStorage.Config); err != nil {
				return res, errors.Wrapf(err, "failed to read state storage config")
			} else {
				pConfig.StateStorage.Config = stateStorageCfg
			}
			if secretsProviderCfg, err := res.fieldConfigReader(pConfig.SecretsProvider.Type, &pConfig.SecretsProvider.Config); err != nil {
				return res, errors.Wrapf(err, "failed to read secrets provider config")
			} else {
				pConfig.SecretsProvider.Config = secretsProviderCfg
			}
			config.Config = pConfig
		}
	}
	return res, nil
}

func (p *pulumi) SetConfigReader(f api.ProvisionerFieldConfigReaderFunc) {
	p.fieldConfigReader = f
}

func (p *pulumi) ProvisionStack(ctx context.Context, cfg *api.ConfigFile, pubKey string, stack api.Stack) error {
	p.pubKey = pubKey // find better way of setting this
	if err := p.createStackIfNotExists(ctx, cfg, stack); err != nil {
		return errors.Wrapf(err, "failed to create stack %q if not exists", stack.Name)
	}
	if err := p.provisionStack(ctx, cfg, stack); err != nil {
		return err
	}
	return nil
}
