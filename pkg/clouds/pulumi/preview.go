// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/api/logger/color"
)

func (p *pulumi) previewStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, _ api.ProvisionParams) (*api.PreviewResult, error) {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return nil, err
	}
	defer closePluginSessions() // see startPluginSession for why not in the RunFunc
	p.logger.Info(ctx, "%s", color.GreenFmt("Previewing parent stack %q...", s.Ref().FullyQualifiedName().String()))
	stackSource, err := p.prepareStackForOperations(ctx, s.Ref(), cfg, p.provisionProgram(stack, cfg))
	if err != nil {
		return nil, err
	}
	p.logger.Info(ctx, "Preview parent stack %q...", stackSource.Name())
	previewResult, err := stackSource.Preview(ctx, p.previewOptions(ctx, ActionContextPreview, true)...)
	if err != nil {
		return nil, annotateNullSecretCheckpointError(err)
	}
	if err := destructiveReplaceErrorFrom(p.drainDestructiveReplaces()); err != nil {
		return nil, err
	}
	p.logger.Info(ctx, "%s", color.GreenFmt("Preview parent summary: %q", p.toPreviewResult(stackSource.Name(), previewResult)))
	return p.toPreviewResult(stackSource.Name(), previewResult), nil
}

func (p *pulumi) previewChildStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.DeployParams) (*api.PreviewResult, error) {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return nil, err
	}
	defer closePluginSessions() // see startPluginSession for why not in the RunFunc
	p.logger.Info(ctx, "%s", color.GreenFmt("Previewing child stack %q...", s.Ref().FullyQualifiedName().String()))

	parentStack := stack.Client.Stacks[params.Environment].ParentStack
	fullStackName := s.Ref().FullyQualifiedName().String()

	program := p.deployStackProgram(stack, params.StackParams, parentStack, fullStackName)
	stackSource, err := p.prepareStackForOperations(ctx, s.Ref(), cfg, program)
	if err != nil {
		return nil, err
	}
	p.logger.Info(ctx, "%s", color.GreenFmt("Preview child stack %q...", stackSource.Name()))
	previewResult, err := stackSource.Preview(ctx, p.previewOptions(ctx, ActionContextPreview, true)...)
	if err != nil {
		return nil, annotateNullSecretCheckpointError(err)
	}
	if err := destructiveReplaceErrorFrom(p.drainDestructiveReplaces()); err != nil {
		return nil, err
	}
	p.logger.Info(ctx, "%s", color.GreenFmt("Preview child summary: %q", p.toPreviewResult(stackSource.Name(), previewResult)))
	return p.toPreviewResult(stackSource.Name(), previewResult), nil
}

func (p *pulumi) OutputsStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.StackParams) (*api.OutputsResult, error) {
	// Reading outputs needs the backend, and when the state storage is
	// plugin-owned only the sidecar knows its URL: an `s3-bucket`
	// state-storage declares `bucketName:` and the plugin assembles
	// `s3://<bucket>` at InitStateStore time, publishing it through
	// PULUMI_BACKEND_URL for login() to pick up (see the default branch of
	// the switch there). With no pre-engine load the sidecar never runs, so
	// StorageUrl() is the empty string *api.PluginFieldConfig returns and
	// diy.Login rejects it with `diy URL  has an illegal prefix` — which
	// names no stack, no plugin and no field, so it reads like a malformed
	// config rather than a missing plugin load. Live-caught on
	// `sc stack outputs -s infra` against forge's s3-bucket backend.
	//
	// Loaded from the PARENT descriptor, before the toChildStack rewrite
	// below: `plugins:` is declared on the parent server.yaml, same as
	// DestroyChildStack does it.
	//
	// This is the full load, which spawns every declared plugin and asks each
	// for its Schema — forge's infra declares six, so an `outputs` call that
	// was ~200ms becomes a couple of seconds. The cache-aware variant below
	// trims the plugin set to the ones whose sidecars actually contribute to
	// backend open (state-store, secrets-provider) using the on-disk schema
	// cache the pre-parse already writes; a plugin whose cached schema has
	// neither is skipped. Cache miss falls back to full spawn (preserves the
	// f306d4c2 correctness fix for empty-URL on Outputs/Cancel).
	if err := p.loadPreEnginePluginsForBackend(ctx, stack); err != nil {
		return nil, errors.Wrap(err, "pre-engine plugin load")
	}
	defer p.closePreEnginePlugins()

	if params.Environment != "" && params.StackName != "" {
		stack = toChildStack(stack, params)
	}
	s, err := p.selectStack(ctx, cfg, stack)
	if err != nil {
		return nil, err
	}

	stackSource, err := p.prepareStackForOperations(ctx, s.Ref(), cfg, nil)
	if err != nil {
		return nil, err
	}

	res, err := stackSource.Outputs(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get outputs")
	}
	return p.toOutputsResult(stackSource.Name(), res), nil
}

func (p *pulumi) toOutputsResult(stackName string, result auto.OutputMap) *api.OutputsResult {
	return &api.OutputsResult{
		StackName: stackName,
		Outputs: lo.MapValues(result, func(value auto.OutputValue, key string) any {
			var res string
			if s, ok := value.Value.(string); ok {
				res = s
			} else if m, ok := value.Value.(map[string]any); ok {
				j, _ := json.Marshal(m)
				res = string(j)
			} else if value.Secret {
				j, _ := json.Marshal(value)
				res = string(j)
			}
			return res
		}),
	}
}

func (p *pulumi) toUpdateResult(stackName string, result auto.UpResult) *api.UpdateResult {
	changes := map[string]int{}
	if result.Summary.ResourceChanges != nil {
		changes = *result.Summary.ResourceChanges
	}
	return &api.UpdateResult{
		StackName:  stackName,
		Summary:    logger.Redact(result.StdOut),
		Operations: changes,
	}
}

func (p *pulumi) toPreviewResult(stackName string, result auto.PreviewResult) *api.PreviewResult {
	return &api.PreviewResult{
		StackName: stackName,
		Summary:   logger.Redact(result.StdOut),
		Operations: lo.MapKeys(result.ChangeSummary, func(value int, key apitype.OpType) string {
			return string(key)
		}),
	}
}

func (p *pulumi) toDestroyResult(result auto.DestroyResult) *api.DestroyResult {
	return &api.DestroyResult{
		Operations: lo.MapValues(*result.Summary.ResourceChanges, func(value int, key string) int {
			return int(value)
		}),
	}
}

func (p *pulumi) toRefreshResult(result auto.RefreshResult) *api.RefreshResult {
	return &api.RefreshResult{
		Operations: lo.MapValues(*result.Summary.ResourceChanges, func(value int, key string) int {
			return int(value)
		}),
	}
}
