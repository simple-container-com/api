// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/pulumi/pulumi/sdk/v3/go/auto/events"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"

	"github.com/simple-container-com/api/pkg/api"
)

type contextValueType struct{}

var contextAction contextValueType = struct{}{}

type contextActionValue string

const (
	ActionContextDeploy    contextActionValue = "deploy"
	ActionContextProvision contextActionValue = "provision"
	ActionContextCancel    contextActionValue = "cancel"
	ActionContextRefresh   contextActionValue = "refresh"
	ActionContextPreview   contextActionValue = "preview"
	ActionContextDestroy   contextActionValue = "destroy"
	ActionContextInit      contextActionValue = "init"
)

func WithContextAction(ctx context.Context, action contextActionValue) context.Context {
	return context.WithValue(ctx, contextAction, action)
}

func (p *pulumi) watchEvents(ctx context.Context) chan events.EngineEvent {
	eventChan := make(chan events.EngineEvent)
	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			p.processEvent(ctx, <-eventChan)
		}
	}()
	return eventChan
}

func (p *pulumi) processEvent(ctx context.Context, evt events.EngineEvent) {
	switch {
	case evt.ResOutputsEvent != nil:
		p.procResourceOutputsEvent(ctx, evt)
	case evt.ResourcePreEvent != nil:
		p.procResourcePreEvent(ctx, evt)
	case evt.ResOpFailedEvent != nil:
		p.procResourceFailedEvent(ctx, evt)
	case evt.PolicyEvent != nil:
		p.procPolicyEvent(ctx, evt)
	case evt.DiagnosticEvent != nil:
		p.procDiagnosticEvent(ctx, evt)
	case evt.Error != nil:
		p.logger.Error(ctx, "[pulumi/error] %s", evt.Error)
	default:
		return // other events are not supported, uncomment lines below for debugging
	}
}

func (p *pulumi) contextActionName(ctx context.Context) string {
	action := ctx.Value(contextAction)
	return lo.If(action == nil, "unknown").Else(fmt.Sprintf("%s", action))
}

// diagSeverityWarning is the engine's severity for a warning diagnostic
// (diag.Warning in the pulumi SDK).
const diagSeverityWarning = "warning"

// procDiagnosticEvent prints engine diagnostics. Warnings go out at WARN with
// the resource URN and the whole message: a provider may report a failed
// operation only as a warning (terraform-provider-yandex does so for a failed
// serverless container revision deploy), and the update still succeeds, so
// the warning is the only trace of the failure. Every other severity keeps
// the INFO line, which also carries docker build/push progress.
func (p *pulumi) procDiagnosticEvent(ctx context.Context, evt events.EngineEvent) {
	d := evt.DiagnosticEvent
	if d.Severity == diagSeverityWarning {
		p.logger.Warn(ctx, "[%s][pulumi/warning] %s: %s", p.contextActionName(ctx), d.URN, strings.TrimRight(d.Message, "\n"))
		return
	}
	p.logger.Info(ctx, "[%s][pulumi/diagnostic] %s: %s", p.contextActionName(ctx), d.URN, d.Message)
}

func (p *pulumi) procPolicyEvent(ctx context.Context, evt events.EngineEvent) {
	p.logger.Info(ctx, "[%s][pulumi/policy] %s: %s", p.contextActionName(ctx), evt.PolicyEvent.ResourceURN, evt.PolicyEvent.Message)
}

func (p *pulumi) procResourceFailedEvent(ctx context.Context, evt events.EngineEvent) {
	p.logger.Info(ctx, "[%s][pulumi/failure] %s: %s/%d", p.contextActionName(ctx), evt.ResOpFailedEvent.Metadata.URN, evt.ResOpFailedEvent.Metadata.Op, evt.ResOpFailedEvent.Status)
}

func (p *pulumi) procResourcePreEvent(ctx context.Context, evt events.EngineEvent) {
	pre := evt.ResourcePreEvent
	p.recordDestructiveReplace(pre)
	p.logger.Info(ctx, strings.TrimSpace("[%s][pulumi/pre] %s: %s \n %s"), p.contextActionName(ctx), pre.Metadata.URN, pre.Metadata.Op, p.diffSummary(pre.Metadata.DetailedDiff))
}

// destructiveReplace records a step whose plan would destroy a live
// resource because an input the provider marked as replace-forcing has
// changed. Pulumi emits three events per logical replace (OpReplace
// summary, OpCreateReplacement for the create half, OpDeleteReplaced
// for the delete half); we track only OpReplace to avoid triple-count,
// and only when Old has a non-empty ID (i.e. the resource exists in
// the world — a fresh `replace` with Old.ID == "" is a provider that
// the engine flagged for replacement before it was ever created, which
// cannot destroy anything).
type destructiveReplace struct {
	URN  string
	Type string
	// Keys lists the inputs that the provider reported as forcing the
	// replace. Populated from StepEventMetadata.Keys, which is set for
	// every replace/create-replacement step.
	Keys []string
}

// isDestructiveReplace is the single predicate the tracker and the
// tests share, so the condition is spelled out once and both sides
// agree on what "an input-driven destructive step" is.
func isDestructiveReplace(meta apitype.StepEventMetadata) bool {
	if meta.Op != apitype.OpReplace {
		return false
	}
	if meta.Old == nil {
		return false
	}
	if meta.Old.ID == "" {
		return false
	}
	// A resource whose provider models updates as "replace by publishing
	// a new revision" — ECS task definitions are the canonical case — is
	// NOT a destructive step: the previous revision is retained, the
	// parent (FargateService) re-references the new id on the same
	// deploy. The set is seeded by `pkg/api` and extended at parse time
	// by each plugin's `scpluginv1.SchemaResponse.safe_to_replace_types`.
	// Live-caught on forge-baas run 42077847984 the moment api
	// 2026.11.19 landed in the fleet. See api.IsSafeToReplace for the
	// inclusion rule.
	if api.IsSafeToReplace(meta.Type) {
		return false
	}
	// A remote command that carries no delete script destroys nothing when
	// it is replaced, and SC replaces those on EVERY provision by design.
	// See isStatelessRemoteCommand for why the type cannot simply go on the
	// safe-to-replace allowlist.
	if isStatelessRemoteCommand(meta) {
		return false
	}
	// A replace the engine reports only because it cannot yet name the ID
	// of a provider that is itself in the plan. Same provider URN, new ID
	// is the unknown sentinel, `provider` the sole replace-forcing key —
	// the apply computes the real diff and does not replace these. See
	// isUnknownProviderIDOnly for the measurement; without it the second
	// `sc provision` of any k3s + Caddy stack is refused.
	if isUnknownProviderIDOnly(meta) {
		return false
	}
	// A resource SC itself marked for retention is still physically
	// present post-delete, but the state entry still gets rewritten and
	// a replace still re-points the state at a NEW object — which is
	// exactly the shape that lost the forge-notifier DNS record in the
	// 2026-10-28 outage (Cloudflare's provider sets RetainOnDelete on
	// WorkersRoute by convention; the Record itself did not retain).
	// The invariant is "SC does not silently re-point a resource that
	// exists in the world to a different instance", so retention does
	// not opt out of the check.
	return true
}

func (p *pulumi) recordDestructiveReplace(pre *apitype.ResourcePreEvent) {
	if pre == nil || !isDestructiveReplace(pre.Metadata) {
		return
	}
	p.destructiveReplaces = append(p.destructiveReplaces, destructiveReplace{
		URN:  pre.Metadata.URN,
		Type: pre.Metadata.Type,
		Keys: append([]string(nil), pre.Metadata.Keys...),
	})
}

// drainDestructiveReplaces returns the recorded replaces and clears
// the tracker. previewStack / previewChildStack call this once after
// Preview() returns to decide whether to fail the operation. Clearing
// here (not at Preview() start) is deliberate: a `pulumi` instance can
// be re-used across multiple preview+up cycles, and we want each to
// be judged in isolation.
func (p *pulumi) drainDestructiveReplaces() []destructiveReplace {
	out := p.destructiveReplaces
	p.destructiveReplaces = nil
	return out
}

func (p *pulumi) procResourceOutputsEvent(ctx context.Context, evt events.EngineEvent) {
	outputs := evt.ResOutputsEvent
	p.logger.Info(ctx, strings.TrimSpace("[%s][pulumi/out] %s: %s \n %s"), p.contextActionName(ctx), outputs.Metadata.URN, outputs.Metadata.Op, p.diffSummary(outputs.Metadata.DetailedDiff))
}

func (p *pulumi) diffSummary(diff map[string]apitype.PropertyDiff) string {
	res := strings.Builder{}
	for k, v := range diff {
		res.WriteString(fmt.Sprintf("\t %s : %s", k, v.Kind))
	}
	return res.String()
}
