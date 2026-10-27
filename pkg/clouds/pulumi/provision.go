// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi/pkg/v3/backend"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/internal/build"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger/color"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func (p *pulumi) provisionStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.ProvisionParams) error {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return err
	}

	// Plugin sessions attached by provisionProgram live until the engine is
	// done with this stack — see startPluginSession.
	defer closePluginSessions()

	p.logger.Info(ctx, "Found stack %q", s.Ref().FullyQualifiedName())

	stackSource, err := p.prepareStackForOperations(ctx, s.Ref(), cfg, p.provisionProgram(stack, cfg))
	if err != nil {
		return err
	}
	if err := checkRefreshCLI(stackSource.Workspace().PulumiVersion(), params.SkipRefresh); err != nil {
		return err
	}

	if timeoutDuration, err := time.ParseDuration(params.Timeouts.ExecutionTimeout); err == nil {
		p.logger.Info(ctx, "%s", color.YellowFmt("Setting timeout on whole execution %q...", timeoutDuration.String()))
		ctxWithTimeout, cancel := context.WithTimeout(ctx, timeoutDuration)
		ctx = ctxWithTimeout
		defer cancel()
	}

	if !params.SkipPreview {
		p.logger.Info(ctx, "%s", color.GreenFmt("Previewing stack %q...", s.Ref().FullyQualifiedName()))

		if params.DetailedDiff {
			p.logger.Info(ctx, "🔍 Diff enabled - showing granular changes for nested properties")
		}
		previewResult, err := stackSource.Preview(ctx, p.previewOptions(ctx, ActionContextPreview, params.DetailedDiff)...)
		if err != nil {
			return err
		}
		p.logger.Info(ctx, "%s", color.GreenFmt("Preview summary: \n%s", p.toPreviewResult(stackSource.Name(), previewResult)))
	}
	updateRes, err := stackSource.Up(ctx, p.upOptions(ctx, ActionContextProvision, params.SkipRefresh, params.DetailedDiff)...)
	if err != nil {
		return err
	}
	p.logger.Info(ctx, "%s", color.GreenFmt("Update summary: \n%s", p.toUpdateResult(stackSource.Name(), updateRes)))
	return nil
}

func (p *pulumi) prepareStackForOperations(ctx context.Context, ref backend.StackReference, cfg *api.ConfigFile, program sdk.RunFunc) (auto.Stack, error) {
	var stackSource auto.Stack
	var err error
	if program != nil {
		stackSource, err = auto.UpsertStackInlineSource(ctx, ref.FullyQualifiedName().String(), cfg.ProjectName, program, p.wsOpts...)
		if err != nil {
			return stackSource, err
		}
		if p.secretsProviderUrl != "" {
			if err = stackSource.ChangeSecretsProvider(ctx, p.secretsProviderUrl, nil); err != nil {
				return stackSource, err
			}
		}

	} else {
		stackSource, err = auto.SelectStackInlineSource(ctx, ref.FullyQualifiedName().String(), cfg.ProjectName, nil, p.wsOpts...)
		if err != nil {
			return stackSource, err
		}
	}
	if stackSource.Workspace() != nil {
		envVars := make(map[string]string)
		for _, kv := range os.Environ() {
			split := strings.SplitN(kv, "=", 2)
			envVars[split[0]] = split[1]
		}
		if err := stackSource.Workspace().SetEnvVars(envVars); err != nil {
			p.logger.Error(ctx, "failed to set environment variables for pulumi workspace: %v", err)
		}
		if p.secretsProviderPassphrase != "" {
			if err := os.Setenv(pApi.ConfigPassphraseEnvVar, p.secretsProviderPassphrase); err != nil {
				p.logger.Warn(ctx, "failed to set %s var", pApi.ConfigPassphraseEnvVar)
			}
			err := stackSource.Workspace().ChangeStackSecretsProvider(ctx, ref.FullyQualifiedName().String(), "passphrase", &auto.ChangeSecretsProviderOptions{
				NewPassphrase: lo.ToPtr(p.secretsProviderPassphrase),
			})
			if err != nil {
				p.logger.Error(ctx, "failed to set passphrase secrets provider for stack: %v", err)
				return stackSource, err
			}
		}
	}
	return stackSource, nil
}

func (p *pulumi) provisionProgram(stack api.Stack, cfg *api.ConfigFile) func(ctx *sdk.Context) error {
	program := func(ctx *sdk.Context) error {
		// Phase 1 complete — in-engine plugin session.
		// Pre-engine plugin loading + sidecar registration already happened in
		// ProvisionStack entry (loadPreEnginePlugins); here we just stand up
		// the MonitorProxy and attach the session for the sidecars to find.
		// Teardown is NOT deferred here — provisionStack/previewStack/
		// destroyStack defer closePluginSessions, since sidecar dispatches can
		// outlive this RunFunc inside output continuations.
		if err := p.startPluginSession(ctx); err != nil {
			return err
		}

		if p.preProvisionProgram != nil {
			if err := p.preProvisionProgram(ctx); err != nil {
				return errors.Wrapf(err, "failed to provision init program")
			}
		}

		if err := p.initRegistrar(ctx, stack, nil); err != nil {
			return errors.Wrapf(err, "failed to init registar")
		}

		if _, nc := p.registrar.(*notConfigured); !nc && p.registrar != nil {
			if _, err := p.registrar.ProvisionRecords(ctx, pApi.ProvisionParams{
				Log: p.logger,
			}); err != nil {
				return errors.Wrapf(err, "failed to provision base DNS records for stack %q", stack.Name)
			}
		}

		for env, resources := range stack.Server.Resources.Resources {
			p.logger.Info(ctx.Context(), "configure resources for stack %q in env %q...", stack.Name, env)
			collector := pApi.NewComputeContextCollector(ctx.Context(), p.logger, stack.Name, env)

			resourcesWithoutDeps := make(map[string]api.ResourceDescriptor)
			resourcesWithDeps := make(map[string]api.ResourceDescriptor)

			// figure out dependencies first
			for resName, resource := range resources.Resources {
				if withDeps, ok := resource.Config.Config.(api.WithParentDependencies); ok {
					deps := lo.Map(withDeps.DependsOnResources(), func(d api.ParentResourceDependency, _ int) string {
						return d.Name
					})
					p.logger.Info(ctx.Context(), "resource %q in stack %q in env %q has dependencies on %q", resName, stack.Name, env, deps)
					resourcesWithDeps[resName] = resource
				} else {
					resourcesWithoutDeps[resName] = resource
				}
			}

			// TODO: validate there are no cycles in dependencies

			// provision resources without deps
			outs := make(map[string]*api.ResourceOutput)
			for resName, resource := range resourcesWithoutDeps {
				if out, err := p.configureResource(ctx, stack, env, resName, resource, collector, outs); err != nil {
					return errors.Wrapf(err, "failed to provision resource %q of env %q", resName, env)
				} else {
					outs[resName] = out
				}
			}

			// provision resources with deps
			for resName, resource := range resourcesWithDeps {
				if _, err := p.configureResource(ctx, stack, env, resName, resource, collector, outs); err != nil {
					return errors.Wrapf(err, "failed to provision resource %q of env %q", resName, env)
				}
			}
		}
		for templateName, stackDesc := range stack.Server.Templates {
			serializedStackDesc, err := yaml.Marshal(stackDesc)
			if err != nil {
				return errors.Wrapf(err, "failed to serialize template's %q descriptor", templateName)
			}

			fullStackRef := pApi.ExpandStackReference(stack.Name, p.provisionerCfg.Organization, cfg.ProjectName)
			outputName := stackDescriptorTemplateName(fullStackRef, templateName)
			p.logger.Debug(ctx.Context(), "preserving template %q in the stack's %q outputs as %q...", templateName, stack.Name, outputName)
			secretOutput := sdk.ToSecret(string(serializedStackDesc))
			ctx.Export(outputName, secretOutput)
		}

		return nil
	}
	return program
}

func (p *pulumi) configureResource(ctx *sdk.Context, stack api.Stack, env string, resName string, res api.ResourceDescriptor, collector pApi.ComputeContext, outs pApi.ResourcesOutputs) (*api.ResourceOutput, error) {
	p.logger.Info(ctx.Context(), "configure resource %q for stack %q in env %q", resName, stack.Name, env)
	if res.Name == "" {
		res.Name = resName
	}

	// Plugin dispatch short-circuit: a resource type with `:` prefix came
	// through the config-tier plugin fall-through in pkg/api/read.go, which
	// means a plugin owns it (its sidecar is in pApi.ProvisionFuncByType).
	// SKIP initProvider — plugin-owned resources rely on the Pulumi engine's
	// auto-default-provider or on an explicit provider the plugin registered
	// via its Provider RPC.
	if strings.Count(res.Type, ":") >= 2 {
		if fnc, ok := pApi.ProvisionFuncByType[res.Type]; ok {
			return fnc(ctx, stack, api.ResourceInput{
				Descriptor: &res,
				StackParams: &api.StackParams{
					StackName:   stack.Name,
					Environment: env,
				},
			}, pApi.ProvisionParams{
				Log:             p.logger,
				Registrar:       p.registrar,
				ComputeContext:  collector,
				ResourceOutputs: outs,
			})
		}
	}

	// Belt-and-suspenders: if the type is owned by an out-of-tree plugin
	// but the dispatch map has no entry for it, something slipped past the
	// parse-time validator (unknown root cause — maybe a hand-crafted
	// state replay). Fail with the same actionable YAML block the
	// validator would have produced, rather than the opaque "unsupported
	// resource type" below. Addendum J.
	if _, ok := pApi.ProvisionFuncByType[res.Type]; !ok {
		if req, isPlugin := api.LookupPluginRequirement(res.Type); isPlugin {
			return nil, errors.Errorf(
				"resource type %q is owned by sc-plugin %q but no `plugins.%s` block "+
					"is configured. Add the following to server.yaml (or inherit from parent):\n\n"+
					"plugins:\n  %s:\n    source:\n      type: %s\n      repository: %s\n      version: v<pick-a-release-tag>\n\n"+
					"Pin a specific release from https://github.com/%s/releases",
				res.Type, req.PluginName, req.PluginName,
				req.PluginName, req.DefaultSource.Type, req.DefaultSource.Repository,
				req.DefaultSource.Repository,
			)
		}
	}

	provisionParams, err := p.getProvisionParams(ctx, stack, res, env, "")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to init provision params for %q", res.Type)
	}
	provisionParams.ComputeContext = collector
	provisionParams.ResourceOutputs = outs

	fnc, ok := pApi.ProvisionFuncByType[res.Type]
	if !ok {
		return nil, errors.Errorf("unknown resource type %q", res.Type)
	}
	if out, err := fnc(ctx, stack, api.ResourceInput{
		Descriptor: &res,
		StackParams: &api.StackParams{
			StackName:   stack.Name,
			Environment: env,
		},
	}, provisionParams); err != nil {
		return nil, errors.Wrapf(err, "failed to provision resource %q of env %q", resName, env)
	} else {
		return out, nil
	}
}

func (p *pulumi) initRegistrar(ctx *sdk.Context, stack api.Stack, dnsPreference *pApi.DnsPreference) error {
	registrars, err := stack.Server.Resources.AllRegistrars()
	if err != nil {
		return errors.Wrapf(err, "invalid registrar configuration of stack %q", stack.Name)
	}

	if len(registrars) > 1 {
		p.logger.Info(ctx.Context(), "configure %d registrars for stack %q...", len(registrars), stack.Name)
		p.registrar = newMultiRegistrar(stack.Name, registrars, dnsPreference, p.logger)
		return nil
	}

	// With one registrar — or none, which resolves to the not-configured null object —
	// assign it directly rather than wrapping it. That keeps every stack written before
	// `registrars:` byte-identical, including callers that type-assert on the concrete
	// registrar (notably *notConfigured in provisionProgram).
	registrar := stack.Server.Resources.Registrar
	for _, desc := range registrars {
		registrar = desc
	}
	p.logger.Info(ctx.Context(), "configure registrar of type %q for stack %q...", registrar.Type, stack.Name)
	if registrarInit, ok := pApi.RegistrarFuncByType[registrar.Type]; !ok {
		return errors.Errorf("unsupported registrar type %q for stack %q", registrar.Type, stack.Name)
	} else if reg, err := registrarInit(ctx, registrar, pApi.ProvisionParams{
		Log:           p.logger,
		DnsPreference: dnsPreference,
	}); err != nil {
		return errors.Wrapf(err, "failed to init registrar for stack %q", stack.Name)
	} else {
		p.registrar = reg
	}
	return nil
}

func (p *pulumi) validateStateAndGetStack(ctx context.Context) (backend.Stack, error) {
	if p.backend == nil {
		return nil, errors.Errorf("backend is nil")
	}
	if p.stackRef == nil {
		return nil, errors.Errorf("stackRef is nil")
	}
	if p.stackRef.FullyQualifiedName() == "" {
		return nil, errors.Errorf("stack reference is not set")
	}

	if s, err := p.backend.GetStack(ctx, p.stackRef); err != nil {
		return nil, errors.Errorf("failed to get stack %q", p.stackRef.FullyQualifiedName())
	} else if s != nil {
		return s, nil
	} else {
		return nil, errors.Errorf("failed to get stack %q: ref is nil", p.stackRef.FullyQualifiedName())
	}
}

func (p *pulumi) getProvisionParams(ctx *sdk.Context, stack api.Stack, res api.ResourceDescriptor, environment string, suffix string) (pApi.ProvisionParams, error) {
	envVariables := map[string]string{
		api.ComputeEnv.StackName:               stack.Name,
		api.ComputeEnv.StackEnv:                environment,
		api.ScContainerResourceTypeEnvVariable: res.Type,
	}

	// Plugin-dispatched resource types own their provider construction
	// internally (in the plugin's own pctx — ProviderResource handles don't
	// cross contexts). Skip initProvider; the plugin's Provision handler
	// constructs the provider using SC-core's naming formula so the
	// resulting Pulumi provider URN stays byte-identical to pre-split. See
	// forge/docs/design/2026-10-24/sc-plugin-architecture/04 Addendum H.
	var (
		providerType string
		provider     sdk.ProviderResource
	)
	if pApi.IsPluginType(res.Type) {
		providerType = res.Type
	} else {
		var err error
		providerType, provider, err = p.initProvider(ctx, stack, lo.If(suffix != "", fmt.Sprintf("%s--%s", res.Name, suffix)).Else(res.Name), res.Type, res.Config, environment)
		if err != nil {
			return pApi.ProvisionParams{}, errors.Wrapf(err, "failed to init main provider for resource %q of type %q in stack %q", res.Name, res.Type, stack.Name)
		}
	}

	dependencyProviders := make(map[string]pApi.DependencyProvider)

	if depProviders, ok := res.Config.Config.(api.WithDependencyProviders); ok {
		for name, pCfg := range depProviders.DependencyProviders() {
			_, dProvider, err := p.initProvider(ctx, stack, res.Name+"-dep-"+name, res.Type, pCfg.Config, environment)
			if err != nil {
				return pApi.ProvisionParams{}, errors.Wrapf(err, "failed to init dependency provider for resource %q of type %q in stack %q", res.Name, res.Type, stack.Name)
			}
			dependencyProviders[name] = pApi.DependencyProvider{
				Provider: dProvider,
				Config:   pCfg.Config,
			}
		}
	}

	return pApi.ProvisionParams{
		Provider:            provider,
		Registrar:           p.registrar,
		Log:                 p.logger,
		BaseEnvVariables:    envVariables,
		HelpersImage:        p.cloudHelpersImage(providerType),
		DependencyProviders: dependencyProviders,
	}, nil
}

func (p *pulumi) initProvider(ctx *sdk.Context, stack api.Stack, resName string, resType string, pCfg api.Config, environment string) (string, sdk.ProviderResource, error) {
	var provider sdk.ProviderResource
	var providerType string
	if authCfg, ok := pCfg.Config.(api.AuthConfig); !ok {
		return "", nil, errors.Errorf("failed to cast config to api.AuthConfig for %q of type %q in stack %q (given %T)", resName, resType, stack.Name, pCfg.Config)
	} else if providerFunc, ok := pApi.ProviderFuncByType[authCfg.ProviderType()]; !ok {
		return "", nil, errors.Errorf("unsupported provider type %q for resource %q of type %q in stack %q", authCfg.ProviderType(), resName, resType, stack.Name)
	} else {
		providerType = authCfg.ProviderType()
		providerName := fmt.Sprintf("%s--%s--%s--%s--provider", stack.Name, resType, resName, environment)
		if out, err := providerFunc(ctx, stack, api.ResourceInput{
			Descriptor: &api.ResourceDescriptor{
				Type:   authCfg.ProviderType(),
				Name:   providerName,
				Config: pCfg,
			},
			StackParams: &api.StackParams{
				StackName:   stack.Name,
				Environment: environment,
			},
		}, pApi.ProvisionParams{
			Log:       p.logger,
			Registrar: p.registrar,
		}); err != nil {
			return "", nil, errors.Wrapf(err, "failed to init provider of type %q for resource %q of type %q in stack %q", providerType, resName, resType, stack.Name)
		} else if provider, ok = out.Ref.(sdk.ProviderResource); !ok {
			return "", nil, errors.Errorf("failed to cast ref to sdk.ProviderResource for type %q of resource %q of type %q in stack %q", providerType, resName, resType, stack.Name)
		} else {
			providerType = authCfg.ProviderType()
		}
	}
	return providerType, provider, nil
}

func (p *pulumi) cloudHelpersImage(providerType string) string {
	return fmt.Sprintf("docker.io/simplecontainer/cloud-helpers:%s-%s", providerType, build.Version)
}

func stackDescriptorTemplateName(stackName, templateName string) string {
	return fmt.Sprintf("%s/%s", stackName, templateName)
}

func (p *pulumi) provisionSecretsProvider(ctx *sdk.Context, provisionerCfg *ProvisionerConfig, stack api.Stack, exportName string) error {
	secretsProviderCfg, ok := provisionerCfg.SecretsProvider.Config.Config.(api.SecretsProviderConfig)
	if !ok {
		return errors.Errorf("secrets provider config is not of type api.SecretsProviderConfig for %q", provisionerCfg.SecretsProvider.Type)
	}

	if !secretsProviderCfg.IsProvisionEnabled() {
		p.logger.Info(ctx.Context(), "skip provisioning of secrets provider for stack %q", stack.Name)
		return nil
	}
	p.logger.Info(ctx.Context(), "configure secrets provider of type %s for stack %q...", provisionerCfg.SecretsProvider.Type, stack.Name)

	resDescriptor := api.ResourceDescriptor{
		Type:   provisionerCfg.SecretsProvider.Type,
		Name:   exportName,
		Config: provisionerCfg.SecretsProvider.Config,
	}
	provisionParams, err := p.getProvisionParams(ctx, stack, resDescriptor, "", "")
	if err != nil {
		return errors.Wrapf(err, "failed to init provision params for %q in stack %q", provisionerCfg.SecretsProvider.Type, stack.Name)
	}

	if fnc, ok := pApi.ProvisionFuncByType[provisionerCfg.SecretsProvider.Type]; ok {
		_, err := fnc(ctx, stack, api.ResourceInput{
			Descriptor: &resDescriptor,
			StackParams: &api.StackParams{
				StackName: stack.Name,
			},
		}, provisionParams)
		if err != nil {
			return errors.Wrapf(err, "failed to provision secrets provider of type %q", provisionerCfg.SecretsProvider.Type)
		}
		return nil
	}
	return errors.Errorf("unknown secrets provider type %q", provisionerCfg.SecretsProvider.Type)
}
