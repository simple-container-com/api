package pulumi

import (
	"context"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"api/pkg/api"
)

func (p *pulumi) createProject(ctx context.Context, cfg *api.ConfigFile, stack api.Stack) error {
	provisionerCfg, valid := stack.Server.Provisioner.Config.Config.(*ProvisionerConfig)

	if !valid {
		return errors.Errorf("provisioner config is not of type *pulumi.ProvisionerConfig")
	}

	org := lo.If(provisionerCfg.StateStorage.Organization == "", "organization").
		Else(provisionerCfg.StateStorage.Organization)

	_, err := sdk.NewContext(ctx, sdk.RunInfo{
		Project:           cfg.ProjectName,
		Stack:             stack.Name,
		Config:            nil,
		ConfigSecretKeys:  nil,
		ConfigPropertyMap: nil,
		DryRun:            false,
		Organization:      org,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to init pulumi provisioner context")
	}

	return sdk.RunErr(func(ctx *sdk.Context) error {
		ctx.Project()
		return nil
	})
}
