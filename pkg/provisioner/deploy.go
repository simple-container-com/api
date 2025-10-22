package provisioner

import (
	"context"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/internal/build"
	"github.com/simple-container-com/api/pkg/api"
)

func (p *provisioner) Deploy(ctx context.Context, params api.DeployParams) error {
	p.logWelcome(ctx, &params)

	cfg, stack, pv, err := p.prepareForChildStack(ctx, &params.StackParams)
	if err != nil {
		return err
	}
	return pv.DeployStack(ctx, cfg, *stack, params)
}

func (p *provisioner) logWelcome(ctx context.Context, deployParams *api.DeployParams) {
	p.log.Info(ctx, color.GreenString("Simple Container CLI version: %s", build.Version))
	if deployParams != nil {
		p.log.Info(ctx, color.GreenString("Deploy version: %s", deployParams.Version))
	}
}

func (p *provisioner) prepareForChildStack(ctx context.Context, params *api.StackParams) (*api.ConfigFile, *api.Stack, api.Provisioner, error) {
	cfg, stack, pv, err := p.initProvisionerForDeploy(ctx, *params)
	if err != nil {
		return nil, nil, nil, err
	}
	if pv == nil {
		return nil, nil, nil, errors.Errorf("provisioner is not initialized properly for stack %q", params.StackName)
	}
	if params.StacksDir == "" {
		params.StacksDir = p.getStacksDir(cfg, params.StacksDir)
	}
	return cfg, stack, pv, nil
}

func (p *provisioner) initProvisioner(ctx context.Context, params api.StackParams) (*api.ConfigFile, *api.Stack, api.Provisioner, error) {
	cfg, err := api.ReadConfigFile(p.rootDir, p.profile)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to read config file for profile %q", p.profile)
	}

	if params.StackName == "" {
		return nil, nil, nil, errors.Errorf("stack must be specified")
	}

	readOpts := api.ReadOpts{
		IgnoreServerMissing:  true,
		IgnoreClientMissing:  true,
		IgnoreSecretsMissing: true,
	}
	if !params.Parent {
		readOpts.RequireClientConfigs = []string{params.StackName}
	}
	if err := p.ReadStacks(ctx, cfg, api.ProvisionParams{
		StacksDir: params.StacksDir,
		Profile:   params.Profile,
	}, readOpts); err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to read stacks")
	}

	// first we resolve existing placeholders
	if err := p.resolvePlaceholders(); err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to resolve placeholders for %q in %q", params.StackName, params.Environment)
	}

	// now we reconcile with parent references, if environment was specified
	p.logger.Debug(ctx, "🔍 Reconciliation check: Environment=%s, Parent=%t", params.Environment, params.Parent)

	if params.Environment != "" && !params.Parent {
		p.logger.Debug(ctx, "🔧 Running reconciliation for client stack deployment...")
		if stacks, err := p.stacks.ReconcileForDeploy(params); err != nil {
			return nil, nil, nil, errors.Wrapf(err, "failed to reconcile stacks for %q in %q", params.StackName, params.Environment)
		} else {
			p.stacks = *stacks
			p.logger.Debug(ctx, "✅ Reconciliation completed successfully")
		}
	} else {
		p.logger.Debug(ctx, "⚠️  Skipping reconciliation - this is a parent stack or no environment specified")
		if params.Parent {
			p.logger.Debug(ctx, "🏗️  Parent stack - secrets should have been revealed during parent repository setup")
		}
	}

	// now we resolve placeholders with reconciled parent
	if err := p.resolvePlaceholders(); err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to resolve placeholders for %q in %q", params.StackName, params.Environment)
	}
	stack, ok := p.stacks[params.StackName]
	if !ok {
		return nil, nil, nil, errors.Errorf("stack %q is not configured", params.StackName)
	}

	pv, err := p.getProvisionerForStack(ctx, stack)
	return cfg, &stack, pv, err
}

func (p *provisioner) initProvisionerForDeploy(ctx context.Context, params api.StackParams) (*api.ConfigFile, *api.Stack, api.Provisioner, error) {
	if params.Environment == "" {
		return nil, nil, nil, errors.Errorf("environment must be specified")
	}

	cfg, stack, pv, err := p.initProvisioner(ctx, params)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to init provisioner for stack %q", params.StackName)
	}

	_, ok := p.stacks[params.StackName].Client.Stacks[params.Environment]
	if !ok {
		return nil, nil, nil, errors.Errorf("environment %q for stack %q is not configured", params.Environment, stack.Name)
	}
	return cfg, stack, pv, err
}

func (p *provisioner) getStacksDir(cfg *api.ConfigFile, providedDir string) string {
	stacksDir := providedDir

	if stacksDir == "" {
		stacksDir = cfg.StacksDir
	}

	if stacksDir == "" {
		stacksDir = DefaultStacksRootDir
	}
	if filepath.IsAbs(stacksDir) {
		return stacksDir
	}
	return filepath.Join(p.rootDir, stacksDir)
}
