package provisioner

import (
	"context"
	"github.com/pkg/errors"
	"github.com/simple-container-com/api/pkg/api"
)

func (p *provisioner) Deploy(ctx context.Context, params api.DeployParams) error {
	cfg, err := api.ReadConfigFile(params.RootDir, p.profile)
	if err != nil {
		return errors.Wrapf(err, "failed to read config file for profile %q", p.profile)
	}

	if err := p.ReadStacks(ctx, api.ProvisionParams{
		RootDir: params.RootDir,
		Profile: params.Profile,
	}, true); err != nil {
		return errors.Wrapf(err, "failed to read stacks")
	}
	stack, ok := p.stacks[params.Stack]
	if !ok {
		return errors.Errorf("stack %q is not configured", params.Stack)
	}

	_, ok = p.stacks[params.Stack].Server.Resources.Resources[params.Environment]
	if !ok {
		return errors.Errorf("resources for stack %q are not configured in env %q", stack.Name, params.Environment)
	}

	_, ok = p.stacks[params.Stack].Client.Stacks[params.Environment]
	if !ok {
		return errors.Errorf("environment %q for stack %q is not configured", params.Environment, stack.Name)
	}
	pv, err := p.getProvisionerForStack(ctx, stack)

	return pv.DeployStack(ctx, cfg, stack, params)
}
