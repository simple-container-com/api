package provisioner

import (
	"context"

	"github.com/simple-container-com/api/pkg/api"
)

func (p *provisioner) Preview(ctx context.Context, params api.DeployParams) (*api.PreviewResult, error) {
	cfg, stack, pv, err := p.initProvisionerForDeploy(ctx, params)
	if err != nil {
		return nil, err
	}

	return pv.PreviewStack(ctx, cfg, *stack, params)
}
