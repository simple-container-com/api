package provisioner

import (
	"context"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

func (p *provisioner) Destroy(ctx context.Context, params api.DestroyParams) error {
	cfg, stack, pv, err := p.initProvisionerForDeploy(ctx, params.StackParams)
	if err != nil {
		return err
	}
	if pv == nil {
		return errors.Errorf("provisioner is not initialized properly")
	}
	if params.StacksDir == "" {
		params.StacksDir = p.getStacksDir(cfg, params.StacksDir)
	}

	return pv.DestroyChildStack(ctx, cfg, *stack, params)
}

func (p *provisioner) DestroyParent(ctx context.Context, params api.DestroyParams) error {
	cfg, err := p.readConfigForProvision(ctx, params.ToProvisionParams())
	if err != nil {
		return err
	}

	for _, stack := range p.stacks {
		pv, err := p.getProvisionerForStack(ctx, stack)
		if err != nil {
			return errors.Wrapf(err, "failed to get provisioner for stack %q", stack.Name)
		}
		if err := pv.DestroyParentStack(ctx, cfg, stack, params); err != nil {
			return errors.Wrapf(err, "failed to preview for stack %q", stack.Name)
		}
	}
	return nil
}
