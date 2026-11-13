// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"strings"
	"sync"
)

// The guard in `pkg/clouds/pulumi` exempts a type under one of THREE
// rules, and they are deliberately separate registries rather than one
// list: the question each answers is different, and merging them would
// let an entry that only satisfies the weaker rule read as if it
// satisfied the stronger one.
//
//	RULE 1 (safeToReplaceTypes) — the replace is ADDITIVE. The provider
//	publishes a new revision, the old one stays registered, and the
//	consuming resource re-references the new id on the same deploy.
//	Nothing is ever absent. `aws:ecs/taskDefinition:TaskDefinition`.
//
//	RULE 2 (configRegeneratedTypes) — the replace IS a delete and a
//	create of the same name, and that is acceptable only because the
//	resource's entire content comes from the client's config on every
//	apply. There is nothing in it that SC could fail to reproduce.
//	`kubernetes:core/v1:{Secret,ConfigMap}`.
//
//	RULE 3 (engineOnlyTypePrefixes) — the "resource" is not a cloud
//	object at all. A `pulumi:providers:*` resource is a configured
//	client that lives only in the engine's state; replacing it issues
//	no API call to any cloud, so there is nothing to destroy and no
//	window in which anything is absent.
//
// Rule 2 is the narrower claim it looks like, not a softer version of
// rule 1: it admits a brief window where the object does not exist, so
// it holds ONLY where no data is at stake. A database or a bucket fails
// it on the first clause and must keep refusing.
//
// Rule 3 is the widest exemption and the only one matched by PREFIX,
// which is safe for exactly one reason: `pulumi:providers:` is a
// namespace reserved by the Pulumi engine, not a cloud's, and no member
// of it has a remote counterpart. It is NOT a licence to prefix-match
// anything else.

// safeToReplaceTypes is rule 1: `OpReplace` is NOT destruction because
// the provider publishes a new revision additively and the consuming
// resource re-references the new id on the same deploy. Consumed by the
// destructive-replace guard in `pkg/clouds/pulumi` (S4 of
// cloudflare-plugin-wrong-zone).
//
// Populated from two sources:
//
//  1. Built-in entries added here at init() time — one canonical
//     entry: `aws:ecs/taskDefinition:TaskDefinition`, the type that
//     catalysed the allowlist on 2026-11-05 (forge-baas run
//     42077847984).
//
//  2. Plugin advertisements via `scpluginv1.SchemaResponse.
//     safe_to_replace_types`, merged by `pkg/stackread.
//     DiscoverPluginTypes` through `RegisterSafeToReplaceType`.
//     Each plugin author knows which of their types are
//     revision-based; this path is how the authority stays with the
//     plugin instead of forcing an api release per new revision-
//     based type.
//
// See forge/docs/roadmap/items/sc-safe-to-replace-via-plugin-schema.md
// for the design + the full inclusion rule. Guarded by a mutex: a
// concurrent parse of multiple stacks could otherwise race on a
// slice-backed map write.
var (
	safeToReplaceMu    sync.RWMutex
	safeToReplaceTypes = map[string]struct{}{
		// ECS task definitions are revision-based. The TF provider's
		// `aws_ecs_task_definition` resource maps `containerDefinitions`
		// and `family` as ForceNew; `pulumi-aws` follows. A "replace"
		// here publishes revision N+1 — AWS ECS keeps revision N
		// registered until no running task uses it, and the parent
		// FargateService's taskDefinition ARN output is re-wired
		// automatically on the same deploy. Not destruction.
		//
		// Live-caught on forge-baas run 42077847984 the moment api
		// 2026.11.19 landed in the fleet. Fixed in api #556.
		//
		// Kept as a built-in rather than deferred to sc-plugin-aws's
		// Schema advertisement because sc-plugin-aws does NOT wrap the
		// ECS Fargate stack today — it stays in-tree. If that changes
		// and sc-plugin-aws starts owning `aws:ecs/*` types, move the
		// entry into its SchemaResponse.SafeToReplaceTypes and drop
		// this line.
		"aws:ecs/taskDefinition:TaskDefinition": {},
	}

	// configRegeneratedTypes is rule 2. NOT plugin-registerable, and
	// not meant to grow often: every entry is a standing promise that
	// a delete-then-create of that type loses nothing, which is a claim
	// about the resource's semantics that the destructive-replace guard
	// exists precisely to stop anyone from making casually. A plugin
	// author can advertise a revision-based type (rule 1) because
	// "does the provider keep the old revision" is a fact about their
	// provider; "is this resource's content fully reproducible from
	// config" is a judgement about the operator's data, and belongs in
	// a reviewed api change.
	//
	// Both entries below: pulumi-kubernetes treats a Secret's
	// `stringData` and a ConfigMap's `data` as replace-forcing, so
	// ANY content change — rotating a credential, adding a secret env
	// var, changing a mounted file — is an `OpReplace`. The guard
	// refused every one of those from the moment it merged, which made
	// `SC_ALLOW_DESTRUCTIVE_REPLACE=1` a normal step in a credential
	// rotation and eroded the signal the variable carries. Live-caught
	// 2026-11-08 on the 3-VPS fleet stand, adding the shared ACME
	// store's credentials to the Caddy deployment:
	//
	//	kubernetes:core/v1:Secret  caddy-staging-secret-volumes  keys=[stringData]
	//	kubernetes:core/v1:Secret  caddy-staging-secrets         keys=[stringData]
	//
	// What the exemption accepts, knowingly: between the delete and
	// the create, a Pod that STARTS in that window cannot mount the
	// volume. A Pod already running keeps its projected copy and never
	// notices. So the blast radius is "pods that happen to start during
	// the replace", not the workload — and the alternative was an
	// operator typing the escape hatch often enough to stop reading it.
	//
	// The genuinely correct fix is content-addressed names, which makes
	// the change additive and satisfies rule 1 instead; it also leaves
	// orphans to collect. Not done here.
	//
	// `kubernetes:batch/v1:Job` is the third entry. A Job's pod template is
	// immutable, so ANY change to what it runs (a proxy container moving from a
	// sidecar to a native init container, a script edit) is an `OpReplace`.
	// Everything in the Job comes from SC's config on every apply, and what a
	// replace costs is that the Job runs again. The Jobs SC creates are the
	// per-stack init Jobs (the Cloud SQL `db-user-init`: GRANT statements, idempotent
	// by construction), so running again is the point, not a loss. Found on a
	// preview of a deployed fleet: the stacks last deployed before that Job changed
	// shape were refused, while the SC before the guard replaced the Job without
	// comment and the stacks deployed fine.
	//
	// What the exemption accepts, knowingly: a Job someone ELSE adds to a client
	// stack through the same path would also be allowed to re-run on a change. SC
	// has no such path today; if one appears and its Job is not idempotent, it
	// belongs on its own type or name, not on this entry.
	configRegeneratedTypes = map[string]struct{}{
		"kubernetes:core/v1:Secret":    {},
		"kubernetes:core/v1:ConfigMap": {},
		"kubernetes:batch/v1:Job":      {},
	}

	// engineOnlyTypePrefixes is rule 3, and the only prefix-matched
	// registry. Not plugin-registerable and not meant to gain a second
	// entry: the claim is "this namespace has no cloud counterpart",
	// which is true of Pulumi's provider namespace and of nothing a
	// plugin author owns.
	//
	// A `pulumi:providers:<pkg>` resource is a configured SDK client.
	// The engine holds its inputs in state and hands dependent
	// resources a `urn::id` reference to it; a replace mints a new
	// client, re-points the dependents on the same apply, and makes no
	// remote call — nothing in any cloud is destroyed, which is what
	// earns the whole namespace a pass.
	//
	// Measured on the 3-VPS fleet stand (2026-11-09), and the first
	// reading of that measurement was wrong, so it is written out here.
	// The dependents are NOT absent from the plan: a preview that
	// replaced `smoke-caddy--staging-caddy-kubeprovider` also planned a
	// replace for all seven resources under it (Namespace, Deployment,
	// Service, ServiceAccount, PodDisruptionBudget, ClusterRole,
	// ClusterRoleBinding), each with `provider` as its sole
	// replace-forcing key. What makes those safe is a different fact —
	// their new provider reference carries an IDENTICAL URN with
	// Pulumi's unknown sentinel as the id, because the provider's own id
	// is not computed yet at preview time, and the apply only `update`s
	// them. That case is handled where it belongs, by
	// `isUnknownProviderIDOnly` in pkg/clouds/pulumi, NOT by this rule.
	//
	// This rule covers only the provider resource itself, and it has to,
	// because there is no signature available for it: its
	// replace-forcing input is `kubeconfig`, a secret, and the step
	// event redacts BOTH sides to the byte-identical
	// `{"4dabf…":"1b470…","ciphertext":"[secret]"}` — there is nothing
	// to compare. The apply, measured, only `update`d it.
	//
	// What made this urgent: ANY change to a provider's credentials is
	// replace-forcing. So the guard refused
	//
	//	pulumi:providers:kubernetes  smoke-caddy--staging-caddy-kubeprovider  keys=[kubeconfig]
	//
	// on the first apply that resolved a `${resource:<cluster>.kubeconfig}`
	// reference to its real value — i.e. the guard blocked the kubeconfig-
	// reference feature from ever landing on an existing stack. The same
	// refusal fires for a rotated cloud credential on any provider in any
	// stack, which is a credential rotation turned into an
	// `SC_ALLOW_DESTRUCTIVE_REPLACE=1` ritual for no gain.
	engineOnlyTypePrefixes = []string{
		"pulumi:providers:",
	}
)

// RegisterSafeToReplaceType adds a Pulumi resource type to the
// safe-to-replace allowlist. Safe to call concurrently. Idempotent.
//
// Called by `pkg/stackread.DiscoverPluginTypes` for every entry in a
// plugin's `scpluginv1.SchemaResponse.safe_to_replace_types`; may
// also be called directly from init() by in-tree code that owns a
// revision-based type (keep that path rare — plugin advertisement is
// the preferred shape).
func RegisterSafeToReplaceType(resourceType string) {
	if resourceType == "" {
		return
	}
	safeToReplaceMu.Lock()
	defer safeToReplaceMu.Unlock()
	safeToReplaceTypes[resourceType] = struct{}{}
}

// IsSafeToReplace reports whether `OpReplace` on this resource type
// should be exempt from the destructive-replace guard — under either
// rule. This is the single question the guard asks.
func IsSafeToReplace(resourceType string) bool {
	return SafeToReplaceRule(resourceType) != ReplaceRuleNone
}

// ReplaceRule names which exemption a type qualifies under, so a
// diagnostic can say WHY a replace was let through. The guard itself
// only needs the boolean.
type ReplaceRule string

const (
	ReplaceRuleNone ReplaceRule = ""
	// ReplaceRuleRevision: the old revision survives the replace.
	ReplaceRuleRevision ReplaceRule = "revision-based"
	// ReplaceRuleConfigRegenerated: the object is briefly absent, and
	// its whole content comes from config on every apply.
	ReplaceRuleConfigRegenerated ReplaceRule = "regenerated-from-config"
	// ReplaceRuleEngineOnly: the resource exists only in the engine's
	// state and the replace makes no remote call.
	ReplaceRuleEngineOnly ReplaceRule = "engine-only"
)

// IsEngineOnlyType reports whether the type is rule 3 — a resource with
// no cloud counterpart. Exported separately because it is the one rule
// SafeToReplaceTypes cannot enumerate: a prefix is not a finite set.
func IsEngineOnlyType(resourceType string) bool {
	for _, prefix := range engineOnlyTypePrefixes {
		if strings.HasPrefix(resourceType, prefix) {
			return true
		}
	}
	return false
}

// SafeToReplaceRule reports the rule under which this type is exempt,
// or ReplaceRuleNone. Checked in rule order: where a type somehow
// satisfies more than one, the narrower claim is the true one, so the
// diagnostic never overstates what is known about the replace.
func SafeToReplaceRule(resourceType string) ReplaceRule {
	safeToReplaceMu.RLock()
	defer safeToReplaceMu.RUnlock()
	if _, ok := safeToReplaceTypes[resourceType]; ok {
		return ReplaceRuleRevision
	}
	if _, ok := configRegeneratedTypes[resourceType]; ok {
		return ReplaceRuleConfigRegenerated
	}
	// Not under the lock's protection by design: the prefix list is a
	// compile-time constant with no registration path, so there is
	// nothing to race on. Held anyway because the RLock above already
	// covers the whole function and splitting it would buy nothing.
	if IsEngineOnlyType(resourceType) {
		return ReplaceRuleEngineOnly
	}
	return ReplaceRuleNone
}

// SafeToReplaceTypes returns a snapshot of every exempt type named
// EXACTLY — rules 1 and 2 — for tests and for operator-visible
// diagnostics ("sc explain"-style). Returned map is a copy; callers may
// mutate it without affecting the registry.
//
// Rule 3 is absent and cannot be added: it is prefix-matched, so it
// denotes an open set with no enumeration. A caller deciding whether a
// specific type is exempt must ask SafeToReplaceRule or
// IsSafeToReplace, never membership of this map.
func SafeToReplaceTypes() map[string]struct{} {
	safeToReplaceMu.RLock()
	defer safeToReplaceMu.RUnlock()
	out := make(map[string]struct{}, len(safeToReplaceTypes)+len(configRegeneratedTypes))
	for k := range safeToReplaceTypes {
		out[k] = struct{}{}
	}
	for k := range configRegeneratedTypes {
		out[k] = struct{}{}
	}
	return out
}
