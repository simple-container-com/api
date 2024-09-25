package provisioner

import (
	"context"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

func (p *provisioner) Destroy(ctx context.Context, params api.DestroyParams, preview bool) error {
	p.logWelcome(ctx, nil)

	cfg, stack, pv, err := p.prepareForChildStack(ctx, &params.StackParams)
	if err != nil {
		return err
	}
	return pv.DestroyChildStack(ctx, cfg, *stack, params, preview)
}

func (p *provisioner) DestroyParent(ctx context.Context, params api.DestroyParams, preview bool) error {
	cfg, err := p.prepareForParentStack(ctx, params.ToProvisionParams())
	if err != nil {
		return err
	}

	for _, stack := range p.stacks {
		pv, err := p.getProvisionerForStack(ctx, stack)
		if err != nil {
			return errors.Wrapf(err, "failed to get provisioner for stack %q", stack.Name)
		}
		if err := pv.DestroyParentStack(ctx, cfg, stack, params, preview); err != nil {
			return errors.Wrapf(err, "failed to preview for stack %q", stack.Name)
		}
	}
	return nil
}
