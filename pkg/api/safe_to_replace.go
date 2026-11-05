// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import "sync"

// safeToReplaceTypes is SC's set of Pulumi resource types where
// `OpReplace` is NOT destruction — the provider publishes a new
// revision additively and the consuming resource re-references the
// new id on the same deploy. Consumed by the destructive-replace
// guard in `pkg/clouds/pulumi` (S4 of cloudflare-plugin-wrong-zone).
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
// should be exempt from the destructive-replace guard.
func IsSafeToReplace(resourceType string) bool {
	safeToReplaceMu.RLock()
	defer safeToReplaceMu.RUnlock()
	_, ok := safeToReplaceTypes[resourceType]
	return ok
}

// SafeToReplaceTypes returns a snapshot of the current allowlist, for
// tests and for operator-visible diagnostics ("sc explain"-style).
// Returned map is a copy — callers may mutate it without affecting
// the registry.
func SafeToReplaceTypes() map[string]struct{} {
	safeToReplaceMu.RLock()
	defer safeToReplaceMu.RUnlock()
	out := make(map[string]struct{}, len(safeToReplaceTypes))
	for k := range safeToReplaceTypes {
		out[k] = struct{}{}
	}
	return out
}
