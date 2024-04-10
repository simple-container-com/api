package provisioner

import (
	"context"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

func (p *provisioner) Preview(ctx context.Context, params api.DeployParams) (*api.PreviewResult, error) {
	cfg, stack, pv, err := p.initProvisionerForDeploy(ctx, params.StackParams)
	if err != nil {
		return nil, err
	}
	if pv == nil {
		return nil, errors.Errorf("provisioner is not initialized properly")
	}
	if params.StacksDir == "" {
		params.StacksDir = p.getStacksDir(cfg, params.StacksDir)
	}

	return pv.PreviewChildStack(ctx, cfg, *stack, params)
}

func (p *provisioner) Outputs(ctx context.Context, params api.StackParams) (*api.OutputsResult, error) {
	cfg, err := api.ReadConfigFile(p.rootDir, p.profile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read config file for profile %q", p.profile)
	}

	if err := p.ReadStacks(ctx, cfg, api.ProvisionParams{
		StacksDir: params.StacksDir,
		Profile:   params.Profile,
	}, false); err != nil {
		return nil, errors.Wrapf(err, "failed to read stacks")
	}
	stack, ok := p.stacks[params.StackName]
	if !ok {
		return nil, errors.Errorf("stack %q is not configured", params.StackName)
	}

	pv, err := p.getProvisionerForStack(ctx, stack)

	return pv.OutputsStack(ctx, cfg, stack, params)
}

func (p *provisioner) PreviewProvision(ctx context.Context, params api.ProvisionParams) ([]*api.PreviewResult, error) {
	cfg, err := p.readConfigForProvision(ctx, params)
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
