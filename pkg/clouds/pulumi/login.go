package pulumi

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/pkg/v3/backend"
	"github.com/pulumi/pulumi/pkg/v3/backend/display"
	"github.com/pulumi/pulumi/pkg/v3/backend/filestate"
	"github.com/pulumi/pulumi/pkg/v3/backend/httpstate"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"api/pkg/api"
	"api/pkg/api/git/path_util"
)

const ConfigPassphraseEnvVar = "PULUMI_CONFIG_PASSPHRASE"

func (p *pulumi) login(ctx context.Context, cfg *api.ConfigFile, stack api.Stack) (*auto.Stack, backend.Backend, backend.StackReference, error) {
	cmdutil.DisableInteractive = true

	provisionerCfg, err := p.getProvisionerConfig(stack)
	if err != nil {
		return nil, nil, nil, err
	}

	var organization string
	if provisionerCfg.Organization == "" {
		p.logger.Warn(ctx, "pulumi organization is empty, assuming 'organization'")
		organization = "organization"
	} else {
		organization = provisionerCfg.Organization
	}

	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to init pulumi provisioner context")
	}

	if os.Getenv(workspace.PulumiHomeEnvVar) == "" {
		// TODO: detect pulumi home
		if pulumiHome, err := path_util.ReplaceTildeWithHome("~/.pulumi"); err != nil {
			p.logger.Warn(ctx, "failed to replace tilde with home: %q", err.Error())
		} else if err := os.Setenv("PATH", fmt.Sprintf("%s/bin:%s", pulumiHome, os.Getenv("PATH"))); err != nil {
			p.logger.Warn(ctx, "failed to set %s var", "PATH")
		}
	}
	if os.Getenv(ConfigPassphraseEnvVar) == "" {
		// TODO: figure out how to set this properly
		if err := os.Setenv(ConfigPassphraseEnvVar, p.pubKey); err != nil {
			p.logger.Warn(ctx, "failed to set %s var", ConfigPassphraseEnvVar)
		}
	}

	pStack, err := auto.UpsertStackInlineSource(ctx, stack.Name, cfg.ProjectName, func(ctx *sdk.Context) error {
		if err := p.provisionSecretsProvider(ctx, provisionerCfg, stack); err != nil {
			return err
		}
		return nil
	})

	upRes, err := pStack.Up(ctx)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to provision stack %q", pStack.Name())
	}
	p.logger.Info(ctx, fmt.Sprint(upRes.Outputs))

	project := &workspace.Project{
		Name: tokens.PackageName(cfg.ProjectName),
	}
	var be backend.Backend
	creds := provisionerCfg.StateStorage.Credentials

	if creds == "" {
		return nil, nil, nil, errors.Errorf("credentials for pulumi backend must not be empty")
	}

	switch provisionerCfg.StateStorage.Type {
	case StateStorageTypeGcpBucket:
		// hackily set google creds env variable, so that bucket can access it (see github.com/pulumi/pulumi/pkg/v3/authhelpers/gcpauth.go:28)
		if err := os.Setenv("GOOGLE_CREDENTIALS", creds); err != nil {
			p.logger.Warn(ctx, "failed to set %q value: %q", httpstate.AccessTokenEnvVar, err.Error())
		}

		be, err = filestate.Login(ctx, cmdutil.Diag(), fmt.Sprintf("gs://%s", provisionerCfg.StateStorage.BucketName), project)
	case StateStorageTypePulumiCloud:
		// hackily set access token env variable, so that lm can access it
		if err := os.Setenv(httpstate.AccessTokenEnvVar, creds); err != nil {
			p.logger.Warn(ctx, "failed to set %q value: %q", httpstate.AccessTokenEnvVar, err.Error())
		}
		cloudUrl := "https://api.pulumi.com"
		_, err := httpstate.NewLoginManager().Login(ctx, cloudUrl, false, "pulumi", "Pulumi stacks", httpstate.WelcomeUser, true /*current*/, display.Options{
			Color: cmdutil.GetGlobalColorization(),
		})
		if err != nil {
			return nil, nil, nil, err
		}
		be, err = httpstate.New(cmdutil.Diag(), cloudUrl, project, false)
	default:
		return nil, nil, nil, errors.Errorf("unsupported state storage type %q", provisionerCfg.StateStorage.Type)
	}

	name, apiOrgs, tokenInfo, err := be.CurrentUser()
	if err != nil {
		return nil, nil, nil, err
	}
	p.logger.Info(ctx, "name: %s, orgs: [%s], tokenInfo: %s", name, strings.Join(apiOrgs, ","), tokenInfo)

	ref, err := be.ParseStackReference(fmt.Sprintf("%s/%s/%s", organization, project.Name, stack.Name))
	if err != nil {
		return nil, nil, nil, err
	}

	return &pStack, be, ref, nil
}

func (p *pulumi) getProvisionerConfig(stack api.Stack) (*ProvisionerConfig, error) {
	provisionerCfg, valid := stack.Server.Provisioner.Config.Config.(*ProvisionerConfig)

	if !valid {
		return nil, errors.Errorf("provisioner config is not of type %T", &ProvisionerConfig{})
	}
	return provisionerCfg, nil
}
