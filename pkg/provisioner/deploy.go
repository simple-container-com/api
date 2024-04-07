package provisioner

import (
	"context"

	"github.com/pkg/errors"
	"github.com/simple-container-com/api/pkg/api"
)

func (p *provisioner) Deploy(ctx context.Context, params api.DeployParams) error {
	cfg, stack, pv, err := p.initProvisionerForDeploy(ctx, params)
	if err != nil {
		return err
	}
	return pv.DeployStack(ctx, cfg, *stack, params)
}

func (p *provisioner) initProvisionerForDeploy(ctx context.Context, params api.DeployParams) (*api.ConfigFile, *api.Stack, api.Provisioner, error) {
	cfg, err := api.ReadConfigFile(params.RootDir, p.profile)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to read config file for profile %q", p.profile)
	}

	if err := p.ReadStacks(ctx, api.ProvisionParams{
		RootDir: params.RootDir,
		Profile: params.Profile,
	}, true); err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to read stacks")
	}
	stack, ok := p.stacks[params.ParentStack]
	if !ok {
		return nil, nil, nil, errors.Errorf("stack %q is not configured", params.ParentStack)
	}

	_, ok = p.stacks[params.ParentStack].Server.Resources.Resources[params.Environment]
	if !ok {
		return nil, nil, nil, errors.Errorf("resources for stack %q are not configured in env %q", stack.Name, params.Environment)
	}

	_, ok = p.stacks[params.ParentStack].Client.Stacks[params.Environment]
	if !ok {
		return nil, nil, nil, errors.Errorf("environment %q for stack %q is not configured", params.Environment, stack.Name)
	}
	pv, err := p.getProvisionerForStack(ctx, stack)
	return cfg, &stack, pv, nil
}
