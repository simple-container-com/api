package pulumi

import (
	"context"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"sync"

	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"

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
	registrar               pApi.Registrar

	secretsProviderOutput *SecretsProviderOutput
	fieldConfigReader     api.ProvisionerFieldConfigReaderFunc
	pParamsMutex          sync.RWMutex
	pParamsMap            map[string]pApi.ProvisionParams
	provisionerCfg        *ProvisionerConfig
	configFile            *api.ConfigFile
	project               *workspace.Project
}

func InitPulumiProvisioner(config api.Config, opts ...api.ProvisionerOption) (api.Provisioner, error) {
	res := &pulumi{
		logger:     logger.New(),
		pParamsMap: make(map[string]pApi.ProvisionParams),
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

func (p *pulumi) ProvisionStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack) error {
	if err := p.createStackIfNotExists(ctx, cfg, stack); err != nil {
		return errors.Wrapf(err, "failed to create stack %q if not exists", stack.Name)
	}
	return p.provisionStack(ctx, cfg, stack)
}

func (p *pulumi) SetPublicKey(pubKey string) {
	p.pubKey = pubKey
}

func (p *pulumi) DestroyParentStack(ctx context.Context, cfg *api.ConfigFile, parentStack api.Stack) error {
	_, err := p.getStack(ctx, cfg, parentStack)
	if err != nil {
		return errors.Wrapf(err, "failed to get parent stack %q", parentStack.Name)
	}
	return p.destroyParentStack(ctx, cfg, parentStack)
}

func (p *pulumi) DestroyChildStack(ctx context.Context, cfg *api.ConfigFile, parentStack api.Stack, params api.DestroyParams) error {
	_, err := p.getStack(ctx, cfg, parentStack)
	if err != nil {
		return errors.Wrapf(err, "failed to get parent stack %q", parentStack.Name)
	}
	childStack := parentStack.ChildStack(params.ParentStack)
	if _, err = p.getStack(ctx, cfg, childStack); err != nil {
		return errors.Wrapf(err, "failed to get child stack %q", childStack.Name)
	}
	return p.destroyChildStack(ctx, cfg, childStack, params)
}

func (p *pulumi) PreviewStack(ctx context.Context, cfg *api.ConfigFile, parentStack api.Stack, params api.DeployParams) (*api.PreviewResult, error) {
	childStack, err := p.initChildStackForDeploy(ctx, cfg, parentStack, params)
	if err != nil {
		return nil, err
	}
	return p.previewStack(ctx, cfg, *childStack, params)
}

func (p *pulumi) initChildStackForDeploy(ctx context.Context, cfg *api.ConfigFile, parentStack api.Stack, params api.DeployParams) (*api.Stack, error) {
	_, err := p.getStack(ctx, cfg, parentStack)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get parent stack %q", parentStack.Name)
	}
	childStack := parentStack.ChildStack(params.StackName)
	if err = p.createStackIfNotExists(ctx, cfg, childStack); err != nil {
		return nil, errors.Wrapf(err, "failed to create stack %q if not exists", childStack.Name)
	}
	return &childStack, nil
}

func (p *pulumi) CancelStack(ctx context.Context, cfg *api.ConfigFile, parentStack api.Stack, params api.DeployParams) error {
	childStack, err := p.initChildStackForDeploy(ctx, cfg, parentStack, params)
	if err != nil {
		return err
	}
	return p.cancelStack(ctx, cfg, *childStack, params)
}

func (p *pulumi) DeployStack(ctx context.Context, cfg *api.ConfigFile, parentStack api.Stack, params api.DeployParams) error {
	childStack, err := p.initChildStackForDeploy(ctx, cfg, parentStack, params)
	if err != nil {
		return err
	}
	return p.deployStack(ctx, cfg, *childStack, params)
}
