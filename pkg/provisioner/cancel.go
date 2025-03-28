package provisioner

import (
	"context"

	"github.com/simple-container-com/api/pkg/api"
)

func (p *provisioner) CancelParent(ctx context.Context, params api.StackParams) error {
	p.logWelcome(ctx, nil)

	cfg, stack, pv, err := p.initProvisioner(ctx, params)
	if err != nil {
		return err
	}
	if params.StacksDir == "" {
		params.StacksDir = p.getStacksDir(cfg, params.StacksDir)
	}

	return pv.CancelStack(ctx, cfg, *stack, params)
}

func (p *provisioner) Cancel(ctx context.Context, params api.StackParams) error {
	p.logWelcome(ctx, nil)

	cfg, stack, pv, err := p.initProvisionerForDeploy(ctx, params)
	if err != nil {
		return err
	}
	if params.StacksDir == "" {
		params.StacksDir = p.getStacksDir(cfg, params.StacksDir)
	}

	return pv.CancelStack(ctx, cfg, *stack, params)
}
