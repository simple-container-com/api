// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"os"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi/pkg/v3/backend"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

//go:generate ../../../bin/mockery --name Pulumi --output ./mocks --filename pulumi_mock.go --outpkg pulumi_mocks --structname PulumiMock
type Pulumi interface {
	api.Provisioner
}

type pulumi struct {
	logger logger.Logger
	pubKey string

	preProvisionProgram       func(ctx *sdk.Context) error
	backend                   backend.Backend
	stackRef                  backend.StackReference
	secretsStackRef           backend.StackReference
	secretsProviderUrl        string
	secretsProviderPassphrase string
	registrar                 pApi.Registrar
	wsOpts                    []auto.LocalWorkspaceOption

	fieldConfigReader api.ProvisionerFieldConfigReaderFunc
	provisionerCfg    *ProvisionerConfig
	configFile        *api.ConfigFile
	project           *workspace.Project

	// preEnginePlugins is populated at the start of a ProvisionStack call
	// (or Destroy*/Preview*) when stack.Server.Plugins is non-empty. The
	// pre-engine phase registers InitStateStore sidecars from these plugins'
	// Schemas, so SC's login flow picks them up transparently. The in-engine
	// phase (pluginSession, created inside provisionProgram) RE-USES these
	// plugin subprocesses rather than spawning twice. See
	// pkg/clouds/pulumi/plugin_preengine.go.
	preEnginePlugins *preEnginePluginBundle
}

func InitPulumiProvisioner(config api.Config, opts ...api.ProvisionerOption) (api.Provisioner, error) {
	res := &pulumi{
		logger: logger.New(),
	}
	for _, opt := range opts {
		if err := opt(res); err != nil {
			return nil, err
		}
	}
	return readProvisionerFields(config, res)
}

func readProvisionerFields(config api.Config, res *pulumi) (api.Provisioner, error) {
	if res.fieldConfigReader != nil {
		if pConfig, ok := config.Config.(*ProvisionerConfig); ok {
			if stateStorageCfg, err := res.fieldConfigReader(pConfig.StateStorage.Type, &pConfig.StateStorage.Config); err != nil {
				return res, errors.Wrapf(err, "failed to read state storage config")
			} else {
				pConfig.StateStorage.Config = stateStorageCfg
			}
			if secretsProviderCfg, err := res.fieldConfigReader(pConfig.SecretsProvider.Type, &pConfig.SecretsProvider.Config); err != nil {
				return res, errors.Wrapf(err, "failed to read secrets provider config")
			} else {
				pConfig.SecretsProvider.Config = secretsProviderCfg
			}
			config.Config = pConfig
		}
	}
	return res, nil
}

func (p *pulumi) SetConfigReader(f api.ProvisionerFieldConfigReaderFunc) {
	p.fieldConfigReader = f
}

func (p *pulumi) ProvisionStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.ProvisionParams) error {
	// Phase 1 of the plugin two-phase lifecycle — see Addendum C in
	// forge/docs/design/2026-10-24/sc-plugin-architecture/04-no-destroy-invariant.md.
	// Pre-engine: spawn plugins, register state-store sidecars BEFORE the
	// engine starts, so login flow can route to them.
	if err := p.loadPreEnginePlugins(ctx, stack); err != nil {
		return errors.Wrap(err, "pre-engine plugin load")
	}
	defer p.closePreEnginePlugins()

	if err := p.createStackIfNotExists(ctx, cfg, stack); err != nil {
		return errors.Wrapf(err, "failed to create stack %q if not exists", stack.Name)
	}
	return p.provisionStack(ctx, cfg, stack, params)
}

// loadPreEnginePlugins is a no-op when stack has no plugins declared.
func (p *pulumi) loadPreEnginePlugins(ctx context.Context, stack api.Stack) error {
	if len(stack.Server.Plugins) == 0 {
		return nil
	}
	b := newPreEnginePluginBundle(p.logger)
	if err := b.Load(ctx, stack.Server.Plugins); err != nil {
		b.Close()
		return err
	}
	if err := b.RegisterSidecars(); err != nil {
		b.Close()
		return errors.Wrap(err, "register plugin sidecars")
	}
	p.preEnginePlugins = b

	// If the provisioner's secrets-provider type is now owned by a plugin
	// sidecar, invoke it to resolve the pulumi provider URL / passphrase.
	// Sets p.secretsProviderUrl + p.secretsProviderPassphrase which
	// prepareStackForOperations consumes. Reads the provisioner config from
	// the stack descriptor directly — p.provisionerCfg isn't populated
	// until login() runs, which happens after this.
	if pCfg, ok := stack.Server.Provisioner.Config.Config.(*ProvisionerConfig); ok {
		if fn, ok := pApi.InitSecretsProviderFuncByType[pCfg.SecretsProvider.Type]; ok {
			url, passphrase, env, err := fn(ctx, p.logger)
			if err != nil {
				return errors.Wrapf(err, "plugin secrets-provider init for %q",
					pCfg.SecretsProvider.Type)
			}
			if url != "" {
				p.secretsProviderUrl = url
			}
			if passphrase != "" {
				p.secretsProviderPassphrase = passphrase
			}
			for k, v := range env {
				if err := os.Setenv(k, v); err != nil {
					p.logger.Warn(ctx, "set env %q from secrets-provider plugin: %v", k, err)
				}
			}
		}
	}
	return nil
}

func (p *pulumi) closePreEnginePlugins() {
	if p.preEnginePlugins != nil {
		p.preEnginePlugins.Close()
		p.preEnginePlugins = nil
	}
}

func (p *pulumi) SetPublicKey(pubKey string) {
	p.pubKey = pubKey
}

func (p *pulumi) DestroyParentStack(ctx context.Context, cfg *api.ConfigFile, parentStack api.Stack, params api.DestroyParams, preview bool) error {
	// Pre-engine plugin load — same as ProvisionStack. Destroy still needs the
	// state-storage sidecar in InitStateStoreFuncByType to find the backend.
	if err := p.loadPreEnginePlugins(ctx, parentStack); err != nil {
		return errors.Wrap(err, "pre-engine plugin load")
	}
	defer p.closePreEnginePlugins()

	s, err := p.selectStack(ctx, cfg, parentStack)
	if err != nil {
		return errors.Wrapf(err, "failed to get parent stack %q", parentStack.Name)
	}
	return p.destroyStack(ctx, cfg, s, params, p.provisionProgram(parentStack, cfg), preview, func(stackSource auto.Stack) {
		// Phase 1 — pre-destroy hooks also fire on parent-stack destroy so
		// plugin-side cleanup runs symmetrically for infrastructure teardown,
		// not only child-stack deploys.
		for _, hook := range pApi.PreDestroyHookFuncs {
			hook(ctx, parentStack, params, stackSource, p.logger)
		}
	})
}

func (p *pulumi) DestroyChildStack(ctx context.Context, cfg *api.ConfigFile, parentStack api.Stack, params api.DestroyParams, preview bool) error {
	if err := p.loadPreEnginePlugins(ctx, parentStack); err != nil {
		return errors.Wrap(err, "pre-engine plugin load")
	}
	defer p.closePreEnginePlugins()

	_, err := p.selectStack(ctx, cfg, parentStack)
	if err != nil {
		return errors.Wrapf(err, "failed to get parent stack %q", parentStack.Name)
	}
	childStack := toChildStack(parentStack, params.StackParams)
	s, err := p.selectStack(ctx, cfg, childStack)
	if err != nil {
		return errors.Wrapf(err, "failed to get child stack %q", childStack.Name)
	}
	if s == nil {
		p.logger.Info(ctx, "child stack %q not found; nothing to destroy", childStack.Name)
		return nil
	}
	program := p.deployStackProgram(childStack, params.StackParams, parentStack.Name, s.Ref().FullyQualifiedName().String())
	return p.destroyStack(ctx, cfg, s, params, program, preview, func(stackSource auto.Stack) {
		for _, hook := range pApi.PreDestroyHookFuncs {
			hook(ctx, childStack, params, stackSource, p.logger)
		}
	})
}

func (p *pulumi) PreviewStack(ctx context.Context, cfg *api.ConfigFile, parentStack api.Stack, params api.ProvisionParams) (*api.PreviewResult, error) {
	if err := p.loadPreEnginePlugins(ctx, parentStack); err != nil {
		return nil, errors.Wrap(err, "pre-engine plugin load")
	}
	defer p.closePreEnginePlugins()

	err := p.createStackIfNotExists(ctx, cfg, parentStack)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get parent stack %q", parentStack.Name)
	}
	return p.previewStack(ctx, cfg, parentStack, params)
}

func (p *pulumi) PreviewChildStack(ctx context.Context, cfg *api.ConfigFile, parentStack api.Stack, params api.DeployParams) (*api.PreviewResult, error) {
	if err := p.loadPreEnginePlugins(ctx, parentStack); err != nil {
		return nil, errors.Wrap(err, "pre-engine plugin load")
	}
	defer p.closePreEnginePlugins()

	childStack, err := p.initChildStackForDeploy(ctx, cfg, parentStack, params)
	if err != nil {
		return nil, err
	}
	return p.previewChildStack(ctx, cfg, *childStack, params)
}

func (p *pulumi) initChildStackForDeploy(ctx context.Context, cfg *api.ConfigFile, parentStack api.Stack, params api.DeployParams) (*api.Stack, error) {
	_, err := p.selectStack(ctx, cfg, parentStack)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get parent stack %q", parentStack.Name)
	}
	childStack := toChildStack(parentStack, params.StackParams)
	if err = p.createStackIfNotExists(ctx, cfg, childStack); err != nil {
		return nil, errors.Wrapf(err, "failed to create stack %q if not exists", childStack.Name)
	}
	return &childStack, nil
}

func toChildStack(parentStack api.Stack, params api.StackParams) api.Stack {
	return parentStack.ChildStack(pApi.StackNameInEnv(params.StackName, params.Environment))
}

func (p *pulumi) DeployStack(ctx context.Context, cfg *api.ConfigFile, parentStack api.Stack, params api.DeployParams) error {
	// Same two-phase plugin bootstrap as ProvisionStack — see Addendum C of
	// 04-no-destroy-invariant.md. Plugins declared on the PARENT stack are
	// loaded before the engine; the inline deploy program (via pluginSession)
	// re-uses the same clients for in-engine resource dispatch.
	if err := p.loadPreEnginePlugins(ctx, parentStack); err != nil {
		return errors.Wrap(err, "pre-engine plugin load")
	}
	defer p.closePreEnginePlugins()

	childStack, err := p.initChildStackForDeploy(ctx, cfg, parentStack, params)
	if err != nil {
		return err
	}
	return p.deployStack(ctx, cfg, *childStack, params)
}
