// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import "sync"

// The guard in `pkg/clouds/pulumi` exempts a type under either of TWO
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
// Rule 2 is the narrower claim it looks like, not a softer version of
// rule 1: it admits a brief window where the object does not exist, so
// it holds ONLY where no data is at stake. A database or a bucket fails
// it on the first clause and must keep refusing.

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
	configRegeneratedTypes = map[string]struct{}{
		"kubernetes:core/v1:Secret":    {},
		"kubernetes:core/v1:ConfigMap": {},
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
)

// SafeToReplaceRule reports the rule under which this type is exempt,
// or ReplaceRuleNone. Rule 1 is checked first: where a type somehow
// satisfies both, the stronger claim is the true one.
func SafeToReplaceRule(resourceType string) ReplaceRule {
	safeToReplaceMu.RLock()
	defer safeToReplaceMu.RUnlock()
	if _, ok := safeToReplaceTypes[resourceType]; ok {
		return ReplaceRuleRevision
	}
	if _, ok := configRegeneratedTypes[resourceType]; ok {
		return ReplaceRuleConfigRegenerated
	}
	return ReplaceRuleNone
}

// SafeToReplaceTypes returns a snapshot of every exempt type under
// EITHER rule, for tests and for operator-visible diagnostics ("sc
// explain"-style). Returned map is a copy — callers may mutate it
// without affecting the registry. Use SafeToReplaceRule when the rule
// matters.
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
