package pulumi

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/backend/diy"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/pkg/v3/backend"
	"github.com/pulumi/pulumi/pkg/v3/backend/display"
	"github.com/pulumi/pulumi/pkg/v3/backend/httpstate"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/git/path_util"
)

const ConfigPassphraseEnvVar = "PULUMI_CONFIG_PASSPHRASE"

func (p *pulumi) login(ctx context.Context, cfg *api.ConfigFile, stack api.Stack) error {
	cmdutil.DisableInteractive = true

	provisionerCfg, err := p.getProvisionerConfig(stack)
	if err != nil {
		return err
	}

	var organization string
	if provisionerCfg.Organization == "" {
		p.logger.Debug(ctx, "pulumi organization is empty, assuming 'organization'")
		organization = "organization"
	} else {
		organization = provisionerCfg.Organization
	}

	if err != nil {
		return errors.Wrapf(err, "failed to init pulumi provisioner context")
	}

	var pulumiHome string
	if os.Getenv(workspace.PulumiHomeEnvVar) == "" {
		// TODO: detect pulumi home
		if pulumiHome, err = path_util.ReplaceTildeWithHome("~/.pulumi"); err != nil {
			p.logger.Warn(ctx, "failed to replace tilde with home: %q", err.Error())
		} else if err := os.Setenv("PATH", fmt.Sprintf("%s/bin:%s", pulumiHome, os.Getenv("PATH"))); err != nil {
			p.logger.Warn(ctx, "failed to set %s var", "PATH")
		}
	}
	if os.Getenv(ConfigPassphraseEnvVar) == "" {
		// TODO: figure out how to set this properly
		if err := os.Setenv(ConfigPassphraseEnvVar, cfg.ProjectName); err != nil {
			p.logger.Warn(ctx, "failed to set %s var", ConfigPassphraseEnvVar)
		}
	}

	p.initialProvisionProgram = func(ctx *sdk.Context) error {
		if err := p.provisionSecretsProvider(ctx, provisionerCfg, stack); err != nil {
			return err
		}
		return nil
	}
	project := &workspace.Project{
		Name: tokens.PackageName(cfg.ProjectName),
	}
	stackRefString := fmt.Sprintf("%s/%s/%s", organization, project.Name, stack.Name)
	pStack, _ := auto.SelectStackInlineSource(ctx, stackRefString, cfg.ProjectName, p.initialProvisionProgram)
	var be backend.Backend
	stateStorageCfg, ok := provisionerCfg.StateStorage.Config.Config.(api.StateStorageConfig)
	if !ok {
		return errors.Errorf("state storage config is not of type api.StateStorageConfig for %q", provisionerCfg.StateStorage.Type)
	}
	creds := stateStorageCfg.CredentialsValue()

	if creds == "" {
		return errors.Errorf("credentials for pulumi backend must not be empty")
	}

	if authCfg, ok := provisionerCfg.StateStorage.Config.Config.(api.AuthConfig); !ok {
		p.logger.Warn(ctx, "state storage config is not of type api.AuthConfig")
	} else if fnc, ok := initStateStoreFuncByType[authCfg.ProviderType()]; !ok {
		p.logger.Warn(ctx, "could not find init state storage function for provider %q, skipping init", authCfg.ProviderType())
	} else if err := fnc(ctx, authCfg); err != nil {
		return errors.Wrapf(err, "failed to init state storage for provider %q", authCfg.ProviderType())
	}

	switch provisionerCfg.StateStorage.Type {
	case BackendTypePulumiCloud:
		cloudUrl := "https://api.pulumi.com"
		_, err := httpstate.NewLoginManager().Login(ctx, cloudUrl, false, "pulumi", "Pulumi stacks", httpstate.WelcomeUser, true /*current*/, display.Options{
			Color: cmdutil.GetGlobalColorization(),
		})
		if err != nil {
			return err
		}
		be, err = httpstate.New(cmdutil.Diag(), cloudUrl, project, false)
	default:
		be, err = diy.Login(ctx, cmdutil.Diag(), stateStorageCfg.StorageUrl(), project)
	}
	if err != nil {
		return err
	}

	name, apiOrgs, tokenInfo, err := be.CurrentUser()
	if err != nil {
		return err
	}
	p.logger.Debug(ctx, "name: %s, orgs: [%s], tokenInfo: %s", name, strings.Join(apiOrgs, ","), tokenInfo)

	ref, err := be.ParseStackReference(stackRefString)
	if err != nil {
		return err
	}
	p.stackRef = ref

	p.provisionerCfg = provisionerCfg
	p.configFile = cfg
	p.stack = &pStack
	p.backend = be
	p.project = project

	return nil
}

func (p *pulumi) getProvisionerConfig(stack api.Stack) (*ProvisionerConfig, error) {
	provisionerCfg, valid := stack.Server.Provisioner.Config.Config.(*ProvisionerConfig)

	if !valid {
		return nil, errors.Errorf("provisioner config is not of type %T", &ProvisionerConfig{})
	}
	return provisionerCfg, nil
}
