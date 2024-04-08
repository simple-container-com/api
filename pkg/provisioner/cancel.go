package provisioner

import (
	"context"

	"github.com/simple-container-com/api/pkg/api"
)

func (p *provisioner) Cancel(ctx context.Context, params api.DeployParams) error {
	cfg, stack, pv, err := p.initProvisionerForDeploy(ctx, params)
	if err != nil {
		return err
	}
	if params.StacksDir == "" {
		params.StacksDir = p.getStacksDir(cfg, params.StacksDir)
	}

	return pv.CancelStack(ctx, cfg, *stack, params)
}
