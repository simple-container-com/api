// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/api/logger/color"
)

func (p *pulumi) previewStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, _ api.ProvisionParams) (*api.PreviewResult, error) {
	s, err := p.validateStateAndGetStack(ctx)
	if err != nil {
		return nil, err
	}
	p.logger.Info(ctx, "%s", color.GreenFmt("Previewing parent stack %q...", s.Ref().FullyQualifiedName().String()))
	stackSource, err := p.prepareStackForOperations(ctx, s.Ref(), cfg, p.provisionProgram(stack, cfg))
	if err != nil {
		return nil, err
	}
	p.logger.Info(ctx, "Preview parent stack %q...", stackSource.Name())
	previewResult, err := stackSource.Preview(ctx, p.previewOptions(ctx, ActionContextPreview, true)...)
	if err != nil {
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
		return nil, err
	}
	p.logger.Info(ctx, "%s", color.GreenFmt("Preview child summary: %q", p.toPreviewResult(stackSource.Name(), previewResult)))
	return p.toPreviewResult(stackSource.Name(), previewResult), nil
}

func (p *pulumi) OutputsStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.StackParams) (*api.OutputsResult, error) {
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
	return p.toOutputsResult(stackSource.Name(), res, params.ShowSecrets), nil
}

// toOutputsResult flattens Pulumi's output map into the shape `sc stack outputs`
// prints, masking anything Pulumi marked secret unless showSecrets is set.
//
// The mask is keyed on value.Secret — Pulumi's own secret tracking, set when the
// program wraps an output in sdk.ToSecret (k3s/cluster.go does exactly that for the
// admin kubeconfig via kubernetes.SecretKubeconfigOutput) — and NOT on
// logger.Redact. Redact is a pattern matcher for PEM blocks and private-key JSON;
// a kubeconfig bearer token or a k3s join token matches none of its regexes, so
// routing this through Redact would mask nothing while looking like it did.
//
// The check comes before the type switch deliberately. The leak this closes was
// that every branch below resolves the secret's own bytes into res — including the
// value.Secret branch, which marshalled the whole OutputValue with Value embedded
// in the clear. A cluster-admin kubeconfig and a join token were both observed
// unmasked in the 2026-10-18 k3s smoke (forge
// docs/roadmap/items/sc-stack-outputs-leaks-secrets-in-clear.md).
func (p *pulumi) toOutputsResult(stackName string, result auto.OutputMap, showSecrets bool) *api.OutputsResult {
	return &api.OutputsResult{
		StackName: stackName,
		Outputs: lo.MapValues(result, func(value auto.OutputValue, key string) any {
			if value.Secret && !showSecrets {
				return api.SecretOutputPlaceholder
			}
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
