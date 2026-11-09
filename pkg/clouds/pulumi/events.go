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
	p.recordPlanStep(pre)
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
	// every replace/create-replacement step. Empty for causeRename —
	// nothing forced anything, the name simply moved.
	Keys []string
	// Cause selects which explanation the error renders. The zero value
	// is causeInputReplace, so every existing construction (and every
	// test) keeps the original meaning.
	Cause destructiveCause
	// NewURN is the created sibling that makes a delete look like a
	// rename. Set only for causeRename, where it is the most useful
	// datum in the whole message: the deleted URN alone does not show
	// WHY it is being deleted, and the new name usually contains the
	// empty or wrong segment that caused it.
	NewURN string
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
	// A pulumi-command resource (local or remote) that carries no delete
	// script destroys nothing when it is replaced, and SC replaces those on
	// EVERY provision by design. See isStatelessCommand for why neither type
	// can simply go on the safe-to-replace allowlist.
	if isStatelessCommand(meta) {
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
		// Spelled out rather than left to the zero value. Two readers
		// need it: the operator-facing message picks its whole
		// explanation off this field, so the construction site is where
		// you want to see which of the two shapes is being recorded —
		// and `unused` is right to flag a constant that only ever
		// exists implicitly.
		Cause: causeInputReplace,
	})
}

// renameDeleteCandidate is a standalone delete of a resource that
// exists in the world, held until the whole plan has been observed.
//
// On its own a delete is not a fault — removing a resource from a
// stack is a legitimate change, and `sc destroy` is nothing but
// deletes (which is why destroy never calls the guard at all: see
// destroy.go, it has no drainDestructiveReplaces call). What makes a
// delete suspicious is a sibling CREATE of the same type under a
// different name in the SAME plan, because that is the signature of a
// resource whose Pulumi name moved.
type renameDeleteCandidate struct {
	URN  string
	Type string
}

// recordPlanStep accumulates the two step kinds that only mean
// something together. Called for every ResourcePreEvent alongside
// recordDestructiveReplace, which handles the single-step (replace)
// shape.
//
// Only plain OpCreate / OpDelete are collected. The replacement halves
// (OpCreateReplacement / OpDeleteReplaced) are deliberately excluded:
// they belong to an OpReplace that recordDestructiveReplace has
// already judged, and counting them here would report every genuine
// replace twice, once as itself and once as a "rename".
func (p *pulumi) recordPlanStep(pre *apitype.ResourcePreEvent) {
	if pre == nil {
		return
	}
	meta := pre.Metadata
	switch meta.Op {
	case apitype.OpCreate:
		if p.createdURNsByType == nil {
			p.createdURNsByType = map[string][]string{}
		}
		p.createdURNsByType[meta.Type] = append(p.createdURNsByType[meta.Type], meta.URN)
	case apitype.OpDelete:
		if isDestructiveDelete(meta) {
			p.deleteCandidates = append(p.deleteCandidates, renameDeleteCandidate{URN: meta.URN, Type: meta.Type})
		}
	}
}

// isDestructiveDelete reports whether a plain delete step would
// destroy something that cannot be regenerated.
//
// It reuses the replace path's exemptions rather than inventing its
// own, and each one carries over for the same reason it was written:
// a type on the safe-to-replace allowlist is one whose content comes
// back from config on every apply (or whose provider retains the old
// object), so losing the state entry loses nothing; a remote command
// with no `delete` script runs nothing when it goes away. The
// exemptions are, if anything, MORE obviously right for a delete than
// for a replace.
func isDestructiveDelete(meta apitype.StepEventMetadata) bool {
	if meta.Op != apitype.OpDelete {
		return false
	}
	// No Old, or no id in it, means the resource was never created in the
	// world — the same test the replace path makes, and for the same
	// reason: there is nothing out there to destroy.
	if meta.Old == nil || meta.Old.ID == "" {
		return false
	}
	if api.IsSafeToReplace(meta.Type) {
		return false
	}
	if isStatelessCommand(meta) {
		return false
	}
	return true
}

// renameDrivenDeletes pairs each held delete against the plan's
// creates and reports the ones that look like a rename.
//
// The signature is deliberately narrow — a live delete AND a create of
// the same type at a different URN — because the broad reading ("any
// delete of a live resource during a deploy is destructive") would
// demand SC_ALLOW_DESTRUCTIVE_REPLACE for every deliberate resource
// removal, which is a behaviour change for every consumer. That broad
// form is arguably what this guard's stated invariant already
// promises; it is tracked separately rather than smuggled in here.
//
// The known false positive, stated rather than hidden: a plan that
// removes one resource of a type and adds an UNRELATED one of the same
// type in the same deploy is flagged. The message that results is
// still true — a live resource is being deleted — so the operator is
// being asked to confirm something worth confirming, and the escape
// hatch is one deploy wide. Tightening it further would need a notion
// of resource identity the event stream does not carry.
func (p *pulumi) renameDrivenDeletes() []destructiveReplace {
	var out []destructiveReplace
	for _, candidate := range p.deleteCandidates {
		for _, created := range p.createdURNsByType[candidate.Type] {
			if created == candidate.URN {
				continue
			}
			out = append(out, destructiveReplace{
				URN:    candidate.URN,
				Type:   candidate.Type,
				Cause:  causeRename,
				NewURN: created,
			})
			break
		}
	}
	return out
}

// drainDestructiveReplaces returns everything the trackers caught and
// clears them. previewStack / previewChildStack call this once after
// Preview() returns to decide whether to fail the operation. Clearing
// here (not at Preview() start) is deliberate: a `pulumi` instance can
// be re-used across multiple preview+up cycles, and we want each to
// be judged in isolation.
//
// The rename correlation happens HERE and not as events arrive because
// it needs the complete plan: a delete can be emitted before the create
// that explains it.
func (p *pulumi) drainDestructiveReplaces() []destructiveReplace {
	out := append(p.destructiveReplaces, p.renameDrivenDeletes()...)
	p.destructiveReplaces = nil
	p.deleteCandidates = nil
	p.createdURNsByType = nil
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
