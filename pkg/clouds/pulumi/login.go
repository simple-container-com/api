package pulumi

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"

	"github.com/pulumi/pulumi/pkg/v3/backend"
	"github.com/pulumi/pulumi/pkg/v3/backend/display"
	"github.com/pulumi/pulumi/pkg/v3/backend/diy"
	"github.com/pulumi/pulumi/pkg/v3/backend/httpstate"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/git/path_util"
	"github.com/simple-container-com/api/pkg/api/logger/color"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

const (
	ConfigPassphraseEnvVar  = "PULUMI_CONFIG_PASSPHRASE"
	DefaultPulumiPassphrase = "simple-container.com"
)

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
		if pulumiHome, err = path_util.ReplaceTildeWithHome("~/.pulumi"); err != nil {
			p.logger.Warn(ctx, "failed to replace tilde with home: %q", err.Error())
		} else if err := os.Setenv("PATH", fmt.Sprintf("%s/bin:%s", pulumiHome, os.Getenv("PATH"))); err != nil {
			p.logger.Warn(ctx, "failed to set %s var", "PATH")
		}
		// override pulumi home, so that it does not interfere when executed concurrently
		newPulumiHome := filepath.Join("~/.pulumi", "sc", cfg.ProjectName, stack.Name)
		if overridePulumiHome, err := homedir.Expand(newPulumiHome); err != nil {
			p.logger.Warn(ctx, "failed to expand overridden pulumi home %q: %v", newPulumiHome, err)
		} else if err := os.Setenv(workspace.PulumiHomeEnvVar, overridePulumiHome); err != nil {
			p.logger.Warn(ctx, "failed to override %q to %q: %v", workspace.PulumiHomeEnvVar, newPulumiHome, err)
		}
	}

	project := &workspace.Project{
		Name: tokens.PackageName(cfg.ProjectName),
	}

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
	} else if fnc, ok := pApi.InitStateStoreFuncByType[authCfg.ProviderType()]; !ok {
		p.logger.Warn(ctx, "could not find init state storage function for provider %q, skipping init", authCfg.ProviderType())
	} else if err := fnc(ctx, stateStorageCfg); err != nil {
		return errors.Wrapf(err, "failed to init state storage for provider %q", authCfg.ProviderType())
	}

	switch provisionerCfg.StateStorage.Type {
	case BackendTypePulumiCloud:
		cloudUrl := "https://api.pulumi.com"
		_, err = httpstate.NewLoginManager().Login(ctx, cloudUrl, false, "pulumi", "Pulumi stacks", httpstate.WelcomeUser, true /*current*/, display.Options{
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

	stackRefString := fmt.Sprintf("%s/%s/%s", organization, project.Name, stack.Name)
	secretsProviderStackRefString := fmt.Sprintf("%s--sc", stackRefString)

	secretsProviderCfg, ok := provisionerCfg.SecretsProvider.Config.Config.(api.SecretsProviderConfig)
	if !ok {
		return errors.Errorf("secrets provider config is not of type api.SecretsProviderConfig for %q", provisionerCfg.SecretsProvider.Type)
	}
	ref, err := be.ParseStackReference(stackRefString)
	if err != nil {
		return err
	}
	p.stackRef = ref
	p.provisionerCfg = provisionerCfg

	secretsProviderUrlExportName := fmt.Sprintf("%s-%s-sc", cfg.ProjectName, stack.Name)

	if secretsProviderCfg.IsProvisionEnabled() && secretsProviderCfg.ProviderType() != BackendTypePulumiCloud && p.secretsProviderUrl == "" {
		defer p.withPulumiPassphrase(ctx)()
		secretsStackRef, err := be.ParseStackReference(secretsProviderStackRefString)
		if err != nil {
			return errors.Wrapf(err, "failed to parse secrets provider stack reference %q", secretsProviderStackRefString)
		}
		p.secretsStackRef = secretsStackRef
		if secretsProviderStackSource, err := p.prepareStackForOperations(ctx, secretsStackRef, cfg, func(ctx *sdk.Context) error {
			return p.provisionSecretsProvider(ctx, provisionerCfg, stack, secretsProviderUrlExportName)
		}); err != nil {
			return errors.Wrapf(err, "failed to prepare secrets stack for operations for stack %q", stackRefString)
		} else if out, err := secretsProviderStackSource.Outputs(ctx); err != nil {
			return errors.Wrapf(err, "failed to get outputs for stack %q before update", stackRefString)
		} else if e, ok := out[secretsProviderUrlExportName]; !ok || e.Value == nil {
			p.logger.Info(ctx, color.GreenFmt("init secrets provider for stack %q...", stackRefString))
			if err != nil {
				return errors.Wrapf(err, "failed to init secrets provider stack %q", secretsProviderStackSource.Name())
			}
			upRes, err := secretsProviderStackSource.Up(ctx, optup.EventStreams(p.watchEvents(WithContextAction(ctx, ActionContextInit))))
			if err != nil {
				return errors.Wrapf(err, "failed to provision secrets provider stack %q", secretsProviderStackSource.Name())
			}
			p.logger.Debug(ctx, color.GreenFmt("Update secrets provider result: \n%s", p.toUpdateResult(secretsProviderStackSource.Name(), upRes)))
			out, err := secretsProviderStackSource.Outputs(ctx)
			if err != nil {
				return errors.Wrapf(err, "failed to get outputs from secrets provider stack %q", secretsProviderStackSource.Name())
			}
			if e, ok := out[secretsProviderUrlExportName]; !ok {
				return errors.Errorf("failed to get secrets provider url from stack %q", secretsProviderStackSource.Name())
			} else if e.Value == nil || e.Value.(string) == "" {
				return errors.Errorf("secrets provider url is empty from stack %q", secretsProviderStackSource.Name())
			} else {
				p.wsOpts = append(p.wsOpts, auto.SecretsProvider(e.Value.(string)))
				p.secretsProviderUrl = e.Value.(string)
			}
		} else if e.Value == nil {
			return errors.Errorf("secrets provider url is empty for %q in stack %q", secretsProviderCfg.ProviderType(), stack.Name)
		} else {
			p.wsOpts = append(p.wsOpts, auto.SecretsProvider(e.Value.(string)))
			p.secretsProviderUrl = e.Value.(string)
		}

	} else if secretsProviderCfg.ProviderType() != BackendTypePulumiCloud && p.secretsProviderUrl == "" {
		if secretsProviderCfg.KeyUrl() == "" {
			return errors.Errorf("secrets provider key url is empty for %q in stack %q", secretsProviderCfg.ProviderType(), stack.Name)
		}
		p.wsOpts = append(p.wsOpts, auto.SecretsProvider(secretsProviderCfg.KeyUrl()))
		p.secretsProviderUrl = secretsProviderCfg.KeyUrl()
	}

	name, apiOrgs, tokenInfo, err := be.CurrentUser()
	if err != nil {
		return err
	}
	p.logger.Debug(ctx, "name: %s, orgs: [%s], tokenInfo: %s", name, strings.Join(apiOrgs, ","), tokenInfo)

	p.configFile = cfg
	p.backend = be
	p.project = project

	return nil
}

func (p *pulumi) withPulumiPassphrase(ctx context.Context) func() {
	if os.Getenv(ConfigPassphraseEnvVar) == "" {
		if err := os.Setenv(ConfigPassphraseEnvVar, DefaultPulumiPassphrase); err != nil {
			p.logger.Warn(ctx, "failed to set %s var", ConfigPassphraseEnvVar)
		}
	}
	return func() {
		_ = os.Unsetenv(ConfigPassphraseEnvVar)
	}
}

func (p *pulumi) getProvisionerConfig(stack api.Stack) (*ProvisionerConfig, error) {
	provisionerCfg, valid := stack.Server.Provisioner.Config.Config.(*ProvisionerConfig)

	if !valid {
		return nil, errors.Errorf("provisioner config is not of type %T", &ProvisionerConfig{})
	}
	return provisionerCfg, nil
}
