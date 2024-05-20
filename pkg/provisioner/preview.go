package provisioner

import (
	"context"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

func (p *provisioner) Preview(ctx context.Context, params api.DeployParams) (*api.PreviewResult, error) {
	p.logWelcome(ctx)

	cfg, stack, pv, err := p.prepareForChildStack(ctx, &params.StackParams)
	if err != nil {
		return nil, err
	}
	return pv.PreviewChildStack(ctx, cfg, *stack, params)
}

func (p *provisioner) Outputs(ctx context.Context, params api.StackParams) (*api.OutputsResult, error) {
	if params.Environment != "" {
		cfg, stack, pv, err := p.prepareForChildStack(ctx, &params)
		if err != nil {
			return nil, err
		}
		return pv.OutputsStack(ctx, cfg, *stack, params)
	}
	cfg, err := p.prepareForParentStack(ctx, params.ToProvisionParams())
	if err != nil {
		return nil, err
	}
	if stack, found := p.stacks[params.StackName]; !found {
		return nil, errors.Errorf("stack %q is not found in configurations", params.StackName)
	} else if pv, err := p.getProvisionerForStack(ctx, stack); err != nil {
		return nil, errors.Wrapf(err, "failed to get provisioner for stack %q", stack.Name)
	} else {
		return pv.OutputsStack(ctx, cfg, stack, params)
	}
}

func (p *provisioner) PreviewProvision(ctx context.Context, params api.ProvisionParams) ([]*api.PreviewResult, error) {
	cfg, err := p.prepareForParentStack(ctx, params)
	if err != nil {
		return nil, err
	}

	res := make([]*api.PreviewResult, 0)

	for _, stack := range p.stacks {
		pv, err := p.getProvisionerForStack(ctx, stack)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get provisioner for stack %q", stack.Name)
		}
		if pres, err := pv.PreviewStack(ctx, cfg, stack); err != nil {
			return nil, errors.Wrapf(err, "failed to preview for stack %q", stack.Name)
		} else {
			res = append(res, pres)
		}
	}
	return res, nil
}
