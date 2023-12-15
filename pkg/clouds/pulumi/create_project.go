package pulumi

import (
	"context"
	"fmt"
	"os"
	"strings"

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
)

func (p *pulumi) createStackIfNotExists(ctx context.Context, cfg *api.ConfigFile, stack api.Stack) error {
	sdkCtx, be, err := p.login(ctx, cfg, stack)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, be.Name())

	name, apiOrgs, tokenInfo, err := be.CurrentUser()
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "name: %s, orgs: [%s], tokenInfo: %s", name, strings.Join(apiOrgs, ","), tokenInfo)

	ref, err := be.ParseStackReference(fmt.Sprintf("%s/%s/%s", sdkCtx.Organization(), sdkCtx.Project(), stack.Name))
	if err != nil {
		return err
	}

	if s, err := be.GetStack(ctx, ref); err != nil {
		return err
	} else if s != nil {
		p.logger.Debug(ctx, "found stack %q, not going to create", ref.String())
		return nil
	} else {
		p.logger.Debug(ctx, "stack %q not found, creating...", ref.String())
		s, err = be.CreateStack(ctx, ref, "", nil)
		if err != nil {
			return errors.Wrapf(err, "failed to create stack %q", ref.String())
		} else if s != nil {
			p.logger.Info(ctx, "created stack %q", s.Ref().String())
		}
	}
	return nil
}

func (p *pulumi) login(ctx context.Context, cfg *api.ConfigFile, stack api.Stack) (*sdk.Context, backend.Backend, error) {
	cmdutil.DisableInteractive = true
	provisionerCfg, valid := stack.Server.Provisioner.Config.Config.(*ProvisionerConfig)

	if !valid {
		return nil, nil, errors.Errorf("provisioner config is not of type %T", &ProvisionerConfig{})
	}

	var organization string
	if provisionerCfg.Organization == "" {
		p.logger.Warn(ctx, "pulumi organization is empty, assuming 'organization'")
		organization = "organization"
	} else {
		organization = provisionerCfg.Organization
	}

	sdkCtx, err := sdk.NewContext(ctx, sdk.RunInfo{
		Organization: organization,
		Project:      cfg.ProjectName,
		Stack:        stack.Name,
		MonitorAddr:  "",
		EngineAddr:   "",
	})
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to init pulumi provisioner context")
	}

	project := &workspace.Project{
		Name: tokens.PackageName(cfg.ProjectName),
	}
	var be backend.Backend
	creds := provisionerCfg.StateStorage.Credentials

	if creds == "" {
		return nil, nil, errors.Errorf("credentials for pulumi backend must not be empty")
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
			return nil, nil, err
		}
		be, err = httpstate.New(cmdutil.Diag(), cloudUrl, project, false)
	default:
		return nil, nil, errors.Errorf("unsupported state storage type %q", provisionerCfg.StateStorage.Type)
	}
	return sdkCtx, be, nil
}
