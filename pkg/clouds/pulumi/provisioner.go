package pulumi

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi/pkg/v3/backend"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/mongodb"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

//go:generate ../../../bin/mockery --name Pulumi --output ./mocks --filename pulumi_mock.go --outpkg pulumi_mocks --structname PulumiMock
type Pulumi interface {
	api.Provisioner
}

type pulumi struct {
	logger logger.Logger
	pubKey string

	preProvisionProgram       func(ctx *sdk.Context) error
	backend                   backend.Backend
	stackRef                  backend.StackReference
	secretsStackRef           backend.StackReference
	secretsProviderUrl        string
	secretsProviderPassphrase string
	registrar                 pApi.Registrar
	wsOpts                    []auto.LocalWorkspaceOption

	fieldConfigReader api.ProvisionerFieldConfigReaderFunc
	provisionerCfg    *ProvisionerConfig
	configFile        *api.ConfigFile
	project           *workspace.Project
}

func InitPulumiProvisioner(config api.Config, opts ...api.ProvisionerOption) (api.Provisioner, error) {
	res := &pulumi{
		logger: logger.New(),
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

func (p *pulumi) ProvisionStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.ProvisionParams) error {
	if err := p.createStackIfNotExists(ctx, cfg, stack); err != nil {
		return errors.Wrapf(err, "failed to create stack %q if not exists", stack.Name)
	}
	return p.provisionStack(ctx, cfg, stack, params)
}

func (p *pulumi) SetPublicKey(pubKey string) {
	p.pubKey = pubKey
}

func (p *pulumi) DestroyParentStack(ctx context.Context, cfg *api.ConfigFile, parentStack api.Stack, params api.DestroyParams, preview bool) error {
	s, err := p.selectStack(ctx, cfg, parentStack)
	if err != nil {
		return errors.Wrapf(err, "failed to get parent stack %q", parentStack.Name)
	}
	return p.destroyStack(ctx, cfg, s, params, p.provisionProgram(parentStack, cfg), preview)
}

func (p *pulumi) DestroyChildStack(ctx context.Context, cfg *api.ConfigFile, parentStack api.Stack, params api.DestroyParams, preview bool) error {
	_, err := p.selectStack(ctx, cfg, parentStack)
	if err != nil {
		return errors.Wrapf(err, "failed to get parent stack %q", parentStack.Name)
	}
	childStack := toChildStack(parentStack, params.StackParams)
	s, err := p.selectStack(ctx, cfg, childStack)
	if err != nil {
		return errors.Wrapf(err, "failed to get child stack %q", childStack.Name)
	}
	program := p.deployStackProgram(childStack, params.StackParams, parentStack.Name, s.Ref().FullyQualifiedName().String())
	return p.destroyStack(ctx, cfg, s, params, program, preview, func(stackSource auto.Stack) {
		p.dropMongoDbIfEnabled(ctx, childStack, params, stackSource)
	})
}

// dropMongoDbIfEnabled checks if mongoDbDestroyDatabase is enabled in cloudExtras and if so,
// reads MongoDB credentials from the stack outputs and drops the database before destroy.
func (p *pulumi) dropMongoDbIfEnabled(ctx context.Context, childStack api.Stack, params api.DestroyParams, stackSource auto.Stack) {
	clientDesc := childStack.Client.Stacks[params.Environment]
	if clientDesc.Config.Config == nil {
		return
	}

	type mongoDbDestroyExtras struct {
		MongoDBDestroyDatabase *bool `json:"mongoDbDestroyDatabase" yaml:"mongoDbDestroyDatabase"`
	}

	// clientDesc.Config.Config is a raw map[string]interface{} from YAML parsing,
	// so use ConvertDescriptor to extract cloudExtras regardless of the underlying type
	type stackConfigWithCloudExtras struct {
		CloudExtras *any `json:"cloudExtras" yaml:"cloudExtras"`
	}
	cfgWithExtras := &stackConfigWithCloudExtras{}
	if _, err := api.ConvertDescriptor(clientDesc.Config.Config, cfgWithExtras); err != nil || cfgWithExtras.CloudExtras == nil {
		return
	}

	extras := &mongoDbDestroyExtras{}
	converted, err := api.ConvertDescriptor(*cfgWithExtras.CloudExtras, extras)
	if err != nil || converted == nil || converted.MongoDBDestroyDatabase == nil || !*converted.MongoDBDestroyDatabase {
		return
	}

	outputs, err := stackSource.Outputs(ctx)
	if err != nil {
		p.logger.Warn(ctx, "mongoDbDestroyDatabase: failed to get stack outputs: %v", err)
		return
	}

	for key, output := range outputs {
		if !strings.HasSuffix(key, "-service-user") {
			continue
		}
		dbUserJson, ok := output.Value.(string)
		if !ok {
			continue
		}
		var dbUser mongodb.DbUserOutput
		if err := json.Unmarshal([]byte(dbUserJson), &dbUser); err != nil {
			p.logger.Warn(ctx, "mongoDbDestroyDatabase: failed to parse service user output %q: %v", key, err)
			continue
		}
		// dbName == userName: both are set to stack.Name in appendUsesResourceContext
		dbName := dbUser.UserName
		fullUri := mongodb.AppendUserPasswordAndDBToMongoUri(dbUser.DbUri, dbUser.UserName, dbUser.Password, dbName)
		p.logger.Info(ctx, "mongoDbDestroyDatabase: dropping MongoDB database %q...", dbName)
		if err := mongodb.DropDatabase(ctx, fullUri, dbName); err != nil {
			p.logger.Warn(ctx, "mongoDbDestroyDatabase: failed to drop database %q: %v", dbName, err)
		} else {
			p.logger.Info(ctx, "mongoDbDestroyDatabase: successfully dropped MongoDB database %q", dbName)
		}
	}
}

func (p *pulumi) PreviewStack(ctx context.Context, cfg *api.ConfigFile, parentStack api.Stack, params api.ProvisionParams) (*api.PreviewResult, error) {
	err := p.createStackIfNotExists(ctx, cfg, parentStack)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get parent stack %q", parentStack.Name)
	}
	return p.previewStack(ctx, cfg, parentStack, params)
}

func (p *pulumi) PreviewChildStack(ctx context.Context, cfg *api.ConfigFile, parentStack api.Stack, params api.DeployParams) (*api.PreviewResult, error) {
	childStack, err := p.initChildStackForDeploy(ctx, cfg, parentStack, params)
	if err != nil {
		return nil, err
	}
	return p.previewChildStack(ctx, cfg, *childStack, params)
}

func (p *pulumi) initChildStackForDeploy(ctx context.Context, cfg *api.ConfigFile, parentStack api.Stack, params api.DeployParams) (*api.Stack, error) {
	_, err := p.selectStack(ctx, cfg, parentStack)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get parent stack %q", parentStack.Name)
	}
	childStack := toChildStack(parentStack, params.StackParams)
	if err = p.createStackIfNotExists(ctx, cfg, childStack); err != nil {
		return nil, errors.Wrapf(err, "failed to create stack %q if not exists", childStack.Name)
	}
	return &childStack, nil
}

func toChildStack(parentStack api.Stack, params api.StackParams) api.Stack {
	return parentStack.ChildStack(pApi.StackNameInEnv(params.StackName, params.Environment))
}

func (p *pulumi) DeployStack(ctx context.Context, cfg *api.ConfigFile, parentStack api.Stack, params api.DeployParams) error {
	childStack, err := p.initChildStackForDeploy(ctx, cfg, parentStack, params)
	if err != nil {
		return err
	}
	return p.deployStack(ctx, cfg, *childStack, params)
}
