// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"reflect"
	"strings"
	"testing"
)

// TestInterpolateDefaults_SelfDerivedTokens — the primary case: a
// plugin default string with $$sc:-prefix carrying ${name}, ${env},
// ${stack}, ${template.name} resolves against the DefaultsContext.
func TestInterpolateDefaults_SelfDerivedTokens(t *testing.T) {
	defaults := map[string]any{
		"storage": map[string]any{
			"bucket": "$$sc:sc-backup-${name}-archives--${env}",
		},
		"worker": map[string]any{
			"cluster": "$$sc:sc-backup-${name}-cluster",
		},
	}
	ctx := DefaultsContext{Name: "my-backup", Env: "staging"}
	got := InterpolateDefaults(defaults, ctx)
	s := got["storage"].(map[string]any)
	if s["bucket"] != "sc-backup-my-backup-archives--staging" {
		t.Errorf("bucket: %v", s["bucket"])
	}
	w := got["worker"].(map[string]any)
	if w["cluster"] != "sc-backup-my-backup-cluster" {
		t.Errorf("cluster: %v", w["cluster"])
	}
}

// TestInterpolateDefaults_NonSentinelPassesThrough — a string WITHOUT
// the sentinel is returned unchanged. Operator-written values + any
// plugin-default literal strings (no templating) take this path.
func TestInterpolateDefaults_NonSentinelPassesThrough(t *testing.T) {
	defaults := map[string]any{
		"metadata": map[string]any{"type": "colocated"},
		"storage":  map[string]any{"type": "s3", "region": "eu-central-1"},
	}
	ctx := DefaultsContext{Name: "x", Env: "y"}
	got := InterpolateDefaults(defaults, ctx)
	s := got["storage"].(map[string]any)
	if s["region"] != "eu-central-1" {
		t.Errorf("literal region string changed: %v", s["region"])
	}
}

// TestInterpolateDefaults_UnknownTokenLeftInPlace — a plugin default
// referencing a token not in the context (e.g. a typo, or a `${foo}`)
// is left unresolved. Fail-open contract: a plugin author's mistake
// doesn't break every dispatch for that component type.
func TestInterpolateDefaults_UnknownTokenLeftInPlace(t *testing.T) {
	defaults := map[string]any{
		"typo": "$$sc:${nam}-oops",
	}
	got := InterpolateDefaults(defaults, DefaultsContext{Name: "x"})
	// sentinel stripped (no ${parent:…} remaining); unknown token stays.
	if got["typo"] != "${nam}-oops" {
		t.Errorf("typo path: %v", got["typo"])
	}
}

// TestInterpolateDefaults_ParentTokenRetainsSentinel — `${parent:…}`
// tokens are left untouched (incl. keeping the sentinel) so the
// future parent-output resolver picks them up on a second pass.
func TestInterpolateDefaults_ParentTokenRetainsSentinel(t *testing.T) {
	defaults := map[string]any{
		"worker": map[string]any{
			"subnetIds":        "$$sc:${parent:default-vpc.subnets}",
			"combinedWithSelf": "$$sc:${parent:default-vpc.subnets}--${env}",
		},
	}
	got := InterpolateDefaults(defaults, DefaultsContext{Env: "prod"})
	w := got["worker"].(map[string]any)
	// Parent-only token: sentinel + token preserved.
	if w["subnetIds"] != "$$sc:${parent:default-vpc.subnets}" {
		t.Errorf("parent-only: %v", w["subnetIds"])
	}
	// Mixed (parent + self): self resolves, sentinel retained because
	// parent token remains.
	if w["combinedWithSelf"] != "$$sc:${parent:default-vpc.subnets}--prod" {
		t.Errorf("mixed: %v", w["combinedWithSelf"])
	}
}

// TestInterpolateDefaults_TemplateConfigTokens — `${template.<key>}`
// reads TemplateConfig[<key>] when it's a string. Non-string values
// are intentionally skipped.
func TestInterpolateDefaults_TemplateConfigTokens(t *testing.T) {
	defaults := map[string]any{
		"storage": map[string]any{
			"region":  "$$sc:${template.region}",
			"account": "$$sc:${template.account}",
			// Non-string key — token stays unresolved.
			"port": "$$sc:${template.port}",
		},
	}
	got := InterpolateDefaults(defaults, DefaultsContext{
		TemplateConfig: map[string]any{
			"region":  "eu-central-1",
			"account": "645119433725",
			"port":    8080, // int — skipped
		},
	})
	s := got["storage"].(map[string]any)
	if s["region"] != "eu-central-1" {
		t.Errorf("region: %v", s["region"])
	}
	if s["account"] != "645119433725" {
		t.Errorf("account: %v", s["account"])
	}
	if s["port"] != "${template.port}" {
		t.Errorf("non-string template.port should stay unresolved, got: %v", s["port"])
	}
}

// TestInterpolateDefaults_NestedListStrings — a list of
// $$sc:-prefixed strings gets each element resolved. Guards against
// a shallow-walk regression (skipping into list elements).
func TestInterpolateDefaults_NestedListStrings(t *testing.T) {
	defaults := map[string]any{
		"worker": map[string]any{
			"roles": []any{
				"$$sc:role-${name}",
				"literal-role",
			},
		},
	}
	got := InterpolateDefaults(defaults, DefaultsContext{Name: "backup"})
	roles := got["worker"].(map[string]any)["roles"].([]any)
	if roles[0] != "role-backup" {
		t.Errorf("first: %v", roles[0])
	}
	if roles[1] != "literal-role" {
		t.Errorf("literal passed through changed: %v", roles[1])
	}
}

// TestInterpolateDefaults_DeepRecursion — three-level nesting with a
// token at the leaf. Walker must reach it.
func TestInterpolateDefaults_DeepRecursion(t *testing.T) {
	defaults := map[string]any{
		"outer": map[string]any{
			"mid": map[string]any{
				"leaf": "$$sc:${name}-${env}",
			},
		},
	}
	got := InterpolateDefaults(defaults, DefaultsContext{Name: "x", Env: "y"})
	leaf := got["outer"].(map[string]any)["mid"].(map[string]any)["leaf"]
	if leaf != "x-y" {
		t.Errorf("leaf: %v", leaf)
	}
}

// TestInterpolateDefaults_NilOrEmpty — defensive: nil + empty maps
// are no-ops.
func TestInterpolateDefaults_NilOrEmpty(t *testing.T) {
	if got := InterpolateDefaults(nil, DefaultsContext{}); got != nil {
		t.Errorf("nil: %v", got)
	}
	if got := InterpolateDefaults(map[string]any{}, DefaultsContext{}); len(got) != 0 {
		t.Errorf("empty: %v", got)
	}
}

// TestInterpolateDefaults_MutationIsolated — the input map is not
// modified by interpolation; the returned map is a fresh copy.
// Regression guard against a sloppy in-place walker.
func TestInterpolateDefaults_MutationIsolated(t *testing.T) {
	defaults := map[string]any{"k": "$$sc:${name}"}
	original := "$$sc:${name}"
	got := InterpolateDefaults(defaults, DefaultsContext{Name: "x"})
	if defaults["k"] != original {
		t.Errorf("input mutated: %v", defaults["k"])
	}
	if got["k"] != "x" {
		t.Errorf("output: %v", got["k"])
	}
}

// TestRefuseSentinelInOperatorRaw_Flat — an operator-written top-level
// string with the sentinel prefix is refused. Prevents the operator
// from forging a plugin-authored default value.
func TestRefuseSentinelInOperatorRaw_Flat(t *testing.T) {
	raw := map[string]any{"deployTo": "$$sc:${name}-smuggled"}
	err := RefuseSentinelInOperatorRaw(raw)
	if err == nil || !strings.Contains(err.Error(), "deployTo") {
		t.Errorf("expected refusal naming deployTo, got %v", err)
	}
	if !strings.Contains(err.Error(), DefaultsSentinel) {
		t.Errorf("error must name the reserved prefix: %v", err)
	}
}

// TestRefuseSentinelInOperatorRaw_Nested — the walker reaches down
// into nested maps + lists. Pins that operator-forged sentinels can't
// hide deeper in the yaml.
func TestRefuseSentinelInOperatorRaw_Nested(t *testing.T) {
	raw := map[string]any{
		"worker": map[string]any{
			"roles": []any{
				"literal",
				"$$sc:${name}-smuggled",
			},
		},
	}
	err := RefuseSentinelInOperatorRaw(raw)
	if err == nil {
		t.Fatal("expected refusal, got nil")
	}
	// Path should include worker.roles.[1]
	if !strings.Contains(err.Error(), "worker") || !strings.Contains(err.Error(), "roles") {
		t.Errorf("error path should include worker.roles: %v", err)
	}
}

// TestRefuseSentinelInOperatorRaw_CleanRaw — an operator-written map
// with no sentinel strings passes through.
func TestRefuseSentinelInOperatorRaw_CleanRaw(t *testing.T) {
	raw := map[string]any{
		"deployTo": "lambda",
		"cloudExtras": map[string]any{
			"lambdaRoutingType": "function-url",
			"awsRoles":          []any{"s3:PutObject", "iam:PassRole"},
		},
		"worker": map[string]any{"cpu": 512},
	}
	if err := RefuseSentinelInOperatorRaw(raw); err != nil {
		t.Errorf("clean operator raw refused: %v", err)
	}
	// Idempotent on nil + empty.
	if err := RefuseSentinelInOperatorRaw(nil); err != nil {
		t.Errorf("nil refused: %v", err)
	}
	if err := RefuseSentinelInOperatorRaw(map[string]any{}); err != nil {
		t.Errorf("empty refused: %v", err)
	}
}

// TestInterpolateDefaults_RoundTripWithMerge — the integration case:
// DeepMergeComponentRaw followed by InterpolateDefaults produces the
// final shape the dispatcher ships. Pins that the two don't fight.
func TestInterpolateDefaults_RoundTripWithMerge(t *testing.T) {
	defaults := map[string]any{
		"storage": map[string]any{
			"type":   "s3",
			"bucket": "$$sc:sc-backup-${name}-archives--${env}",
			"region": "$$sc:${template.region}",
		},
		"worker": map[string]any{
			"cluster": "$$sc:sc-backup-${name}-cluster",
			"cpu":     512,
		},
	}
	operator := map[string]any{
		"storage": map[string]any{
			"bucket": "my-explicit-bucket", // operator wins
		},
		"worker": map[string]any{
			"cpu": 2048, // operator wins
		},
	}
	merged := DeepMergeComponentRaw(defaults, operator)
	got := InterpolateDefaults(merged, DefaultsContext{
		Name: "my-backup", Env: "prod",
		TemplateConfig: map[string]any{"region": "eu-central-1"},
	})
	s := got["storage"].(map[string]any)
	if s["bucket"] != "my-explicit-bucket" {
		t.Errorf("operator bucket must win, got: %v", s["bucket"])
	}
	// region: operator did NOT write, so plugin default interpolates.
	if s["region"] != "eu-central-1" {
		t.Errorf("region from template: %v", s["region"])
	}
	w := got["worker"].(map[string]any)
	if w["cpu"] != 2048 {
		t.Errorf("operator cpu must win: %v", w["cpu"])
	}
	// cluster: operator did NOT write, plugin default interpolates.
	if w["cluster"] != "sc-backup-my-backup-cluster" {
		t.Errorf("cluster: %v", w["cluster"])
	}
}

// TestInterpolateDefaults_ParentResolver_ResolvesAllTokens — happy
// path for the Tier-3 parent-outputs follow-up: a plugin default
// references `${parent:<resource>.<field>}`; the dispatcher-supplied
// resolver returns the matching parent-stack output string; result
// has the value spliced in.
func TestInterpolateDefaults_ParentResolver_ResolvesAllTokens(t *testing.T) {
	defaults := map[string]any{
		"worker": map[string]any{
			"subnetIds":        "$$sc:${parent:default-vpc.subnets}",
			"securityGroupIds": "$$sc:${parent:default-vpc.fargate-sg}",
		},
		"storage": map[string]any{
			"bucket": "$$sc:${parent:default-storage.bucket-name}",
		},
	}
	resolver := func(resource, field string) (string, bool) {
		switch resource + "." + field {
		case "default-vpc.subnets":
			return "subnet-aaa,subnet-bbb", true
		case "default-vpc.fargate-sg":
			return "sg-xxx", true
		case "default-storage.bucket-name":
			return "fleet-backup-archives", true
		}
		return "", false
	}
	got := InterpolateDefaults(defaults, DefaultsContext{ParentResolver: resolver})

	w := got["worker"].(map[string]any)
	if w["subnetIds"] != "subnet-aaa,subnet-bbb" {
		t.Errorf("subnets: %v", w["subnetIds"])
	}
	if w["securityGroupIds"] != "sg-xxx" {
		t.Errorf("sg: %v", w["securityGroupIds"])
	}
	s := got["storage"].(map[string]any)
	if s["bucket"] != "fleet-backup-archives" {
		t.Errorf("bucket: %v", s["bucket"])
	}
}

// TestInterpolateDefaults_ParentResolver_UnknownResource — resolver
// returns ok=false; token stays in place AND the sentinel is retained
// so the dispatched template can log the gap.
func TestInterpolateDefaults_ParentResolver_UnknownResource(t *testing.T) {
	defaults := map[string]any{"k": "$$sc:${parent:unknown-res.field}"}
	resolver := func(resource, field string) (string, bool) { return "", false }
	got := InterpolateDefaults(defaults, DefaultsContext{ParentResolver: resolver})
	if got["k"] != "$$sc:${parent:unknown-res.field}" {
		t.Errorf("unknown parent resource should keep sentinel + token: got %v", got["k"])
	}
}

// TestInterpolateDefaults_ParentResolver_NilResolver — older dispatcher
// without a resolver wired. Parent tokens stay in place with the
// sentinel retained (today's behaviour before this slice). Regression
// guard against the parent-path breaking existing plugins.
func TestInterpolateDefaults_ParentResolver_NilResolver(t *testing.T) {
	defaults := map[string]any{"k": "$$sc:${parent:a.b}"}
	got := InterpolateDefaults(defaults, DefaultsContext{})
	if got["k"] != "$$sc:${parent:a.b}" {
		t.Errorf("nil resolver should leave parent tokens with sentinel: got %v", got["k"])
	}
}

// TestInterpolateDefaults_ParentResolver_MixedTokens — one string
// references both a self-derived token AND a parent token. Both resolve;
// sentinel drops because no `${parent:…}` remains.
func TestInterpolateDefaults_ParentResolver_MixedTokens(t *testing.T) {
	defaults := map[string]any{
		"bucket": "$$sc:sc-backup-${name}-${env}-${parent:default-storage.bucket-suffix}",
	}
	resolver := func(resource, field string) (string, bool) {
		if resource == "default-storage" && field == "bucket-suffix" {
			return "archives", true
		}
		return "", false
	}
	got := InterpolateDefaults(defaults, DefaultsContext{
		Name: "my-backup", Env: "prod", ParentResolver: resolver,
	})
	if got["bucket"] != "sc-backup-my-backup-prod-archives" {
		t.Errorf("mixed: %v", got["bucket"])
	}
}

// TestInterpolateDefaults_ParentResolver_MalformedToken — a token
// without a `.` separator inside is malformed; the parser leaves it in
// place (fail-open) without calling the resolver.
func TestInterpolateDefaults_ParentResolver_MalformedToken(t *testing.T) {
	called := false
	resolver := func(resource, field string) (string, bool) {
		called = true
		return "should-not-return", true
	}
	// No dot inside.
	defaults := map[string]any{"k": "$$sc:${parent:nodotshere}"}
	got := InterpolateDefaults(defaults, DefaultsContext{ParentResolver: resolver})
	if called {
		t.Error("resolver must not be called for malformed token (no `.`)")
	}
	if got["k"] != "$$sc:${parent:nodotshere}" {
		t.Errorf("malformed: %v", got["k"])
	}
}

// TestInterpolateDefaults_ParentResolver_UnclosedToken — a token
// without a `}` is malformed; parser doesn't infinite-loop + leaves
// the rest of the string as-is.
func TestInterpolateDefaults_ParentResolver_UnclosedToken(t *testing.T) {
	resolver := func(string, string) (string, bool) { return "", true }
	defaults := map[string]any{"k": "$$sc:${parent:res.field-without-close"}
	got := InterpolateDefaults(defaults, DefaultsContext{ParentResolver: resolver})
	// The `${parent:` prefix remains, so sentinel retained.
	if got["k"] != "$$sc:${parent:res.field-without-close" {
		t.Errorf("unclosed: %v", got["k"])
	}
}

// TestSubstituteParentTokens_MultipleInOneString — more than one
// `${parent:…}` token in a single string. Each resolves independently.
func TestSubstituteParentTokens_MultipleInOneString(t *testing.T) {
	resolver := func(resource, field string) (string, bool) {
		return resource + "-" + field, true
	}
	defaults := map[string]any{
		"k": "$$sc:a=${parent:x.y} b=${parent:p.q}",
	}
	got := InterpolateDefaults(defaults, DefaultsContext{ParentResolver: resolver})
	if got["k"] != "a=x-y b=p-q" {
		t.Errorf("multi: %v", got["k"])
	}
}

// Shape guard: ensure the public-ish shape is stable + tests don't
// drift silently.
func TestDefaultsContext_ShapeIsExhaustive(t *testing.T) {
	// If this test fails to compile, a new field was added to
	// DefaultsContext and the token substitution path + docs need
	// updating to match.
	_ = DefaultsContext{
		Name:           "",
		Env:            "",
		Stack:          "",
		TemplateName:   "",
		TemplateConfig: nil,
		ParentResolver: nil,
	}
	// Reflection check: the exported field count stays at 6 until we
	// consciously add a new one with the corresponding token wiring.
	ctx := DefaultsContext{}
	typ := reflect.TypeOf(ctx)
	if n := typ.NumField(); n != 6 {
		t.Errorf("DefaultsContext field count changed from 6 to %d — update substituteSelfTokens/substituteParentTokens/docs", n)
	}
}
