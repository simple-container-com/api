// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"strings"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
)

// isDestructiveReplace is the single predicate the tracker + the
// tests share. Pinning its behaviour per-op makes a refactor that
// silently changes which ops abort the preview impossible to merge.
func TestIsDestructiveReplace_OpCoverage(t *testing.T) {
	tests := []struct {
		name string
		meta apitype.StepEventMetadata
		want bool
	}{
		{
			name: "replace on existing resource with non-empty Old.ID → destructive",
			meta: apitype.StepEventMetadata{
				Op:   apitype.OpReplace,
				URN:  "urn:pulumi:stack::proj::cloudflare:index/record:Record::notifier-record",
				Old:  &apitype.StepEventStateMetadata{ID: "efe05167e6c81424a237a2f50a33dbff"},
				New:  &apitype.StepEventStateMetadata{},
				Keys: []string{"zoneId"},
			},
			want: true,
		},
		{
			name: "replace with no Old (resource never created) → not destructive",
			meta: apitype.StepEventMetadata{
				Op:  apitype.OpReplace,
				URN: "urn:pulumi:stack::proj::foo::new",
				New: &apitype.StepEventStateMetadata{},
			},
			want: false,
		},
		{
			name: "replace with empty Old.ID (resource flagged for replace before first create) → not destructive",
			meta: apitype.StepEventMetadata{
				Op:  apitype.OpReplace,
				URN: "urn:pulumi:stack::proj::foo::pending",
				Old: &apitype.StepEventStateMetadata{ID: ""},
				New: &apitype.StepEventStateMetadata{},
			},
			want: false,
		},
		{
			name: "create-replacement (the create half of a replace) → tracked only via the OpReplace summary",
			meta: apitype.StepEventMetadata{
				Op:  apitype.OpCreateReplacement,
				URN: "urn:pulumi:stack::proj::foo::half",
				Old: &apitype.StepEventStateMetadata{ID: "live-id"},
				New: &apitype.StepEventStateMetadata{},
			},
			want: false,
		},
		{
			name: "delete-replaced (the delete half) → also not tracked here to avoid double-count",
			meta: apitype.StepEventMetadata{
				Op:  apitype.OpDeleteReplaced,
				URN: "urn:pulumi:stack::proj::foo::half",
				Old: &apitype.StepEventStateMetadata{ID: "live-id"},
			},
			want: false,
		},
		{
			name: "plain update → not a replace at all",
			meta: apitype.StepEventMetadata{
				Op:  apitype.OpUpdate,
				URN: "urn:pulumi:stack::proj::foo::patch",
				Old: &apitype.StepEventStateMetadata{ID: "live-id"},
				New: &apitype.StepEventStateMetadata{},
			},
			want: false,
		},
		{
			name: "same → no change",
			meta: apitype.StepEventMetadata{
				Op:  apitype.OpSame,
				URN: "urn:pulumi:stack::proj::foo::stable",
				Old: &apitype.StepEventStateMetadata{ID: "live-id"},
			},
			want: false,
		},
		{
			// forge-baas 2026-11-05: every deploy published a new ECS
			// task-definition revision; before this exemption the
			// `OpReplace && Old.ID != ''` predicate flagged it as
			// destruction and blocked the deploy.
			name: "ECS task definition replace → safe (revision-based, allowlisted)",
			meta: apitype.StepEventMetadata{
				Op:   apitype.OpReplace,
				URN:  "urn:pulumi:forge-baas--staging::forge-baas::awsx:ecs:FargateService$awsx:ecs:FargateTaskDefinition$aws:ecs/taskDefinition:TaskDefinition::forge-baas--staging-staging-service",
				Type: "aws:ecs/taskDefinition:TaskDefinition",
				Old:  &apitype.StepEventStateMetadata{ID: "arn:aws:ecs:eu-central-1:645119433725:task-definition/forge-baas--staging-staging-service:42"},
				New:  &apitype.StepEventStateMetadata{},
				Keys: []string{"containerDefinitions", "family"},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDestructiveReplace(tt.meta); got != tt.want {
				t.Fatalf("isDestructiveReplace() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Reproduces the forge-notifier 2026-10-28 outage shape: an unchanged URN
// with a changed zoneId forces replace, and the live record would be
// destroyed. Pre-S4 the preview exited green on this; post-S4 the tracker
// fires and the sentinel error names both the URN and the key.
func TestDestructiveReplaceErrorFrom_NotifierOutage(t *testing.T) {
	t.Setenv(envAllowDestructiveReplace, "")
	err := destructiveReplaceErrorFrom([]destructiveReplace{{
		URN:  "urn:pulumi:staging::forge-notifier::cloudflare:index/record:Record::notifier.simple-forge.com-record",
		Type: "cloudflare:index/record:Record",
		Keys: []string{"zoneId"},
	}})
	if err == nil {
		t.Fatal("expected a destructive-replace error, got nil")
	}
	msg := err.Error()
	// URN + type + key are all surfaced so the operator can trace the
	// destruction to a specific resource and the input that forced it.
	for _, want := range []string{
		"notifier.simple-forge.com-record",
		"cloudflare:index/record:Record",
		"zoneId",
		envAllowDestructiveReplace,
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message does not contain %q; got:\n%s", want, msg)
		}
	}
}

// The escape hatch MUST be the only way to bypass the check. Spelling
// variations of the truthy value should all work; anything else blocks.
func TestDestructiveReplaceErrorFrom_EscapeHatch(t *testing.T) {
	replaces := []destructiveReplace{{
		URN:  "urn:pulumi:stack::proj::res",
		Type: "t",
		Keys: []string{"region"},
	}}
	for _, truthy := range []string{"1", "t", "true", "y", "yes", "on", "True", "YES", " 1 "} {
		t.Run("allowed/"+truthy, func(t *testing.T) {
			t.Setenv(envAllowDestructiveReplace, truthy)
			if err := destructiveReplaceErrorFrom(replaces); err != nil {
				t.Fatalf("escape hatch %q should bypass the check, got err: %v", truthy, err)
			}
		})
	}
	for _, blocked := range []string{"", "0", "false", "no", "off", "maybe"} {
		t.Run("blocked/"+blocked, func(t *testing.T) {
			t.Setenv(envAllowDestructiveReplace, blocked)
			if err := destructiveReplaceErrorFrom(replaces); err == nil {
				t.Fatalf("value %q must NOT bypass the check", blocked)
			}
		})
	}
}

// Empty list is a no-op whether or not the escape hatch is set: the
// escape hatch only disables the check, it cannot invent destructive
// ops out of nowhere.
func TestDestructiveReplaceErrorFrom_EmptyList(t *testing.T) {
	for _, env := range []string{"", "1"} {
		t.Run("env="+env, func(t *testing.T) {
			t.Setenv(envAllowDestructiveReplace, env)
			if err := destructiveReplaceErrorFrom(nil); err != nil {
				t.Fatalf("empty list must not error (env=%q): %v", env, err)
			}
		})
	}
}

// Multiple replaces come out sorted by URN so an operator reading the
// message or a CI log parser grep-ing on "- urn:" sees deterministic
// lines across runs.
func TestDestructiveReplaceError_StableOrder(t *testing.T) {
	t.Setenv(envAllowDestructiveReplace, "")
	err := destructiveReplaceErrorFrom([]destructiveReplace{
		{URN: "urn:z-last", Type: "t", Keys: []string{"k"}},
		{URN: "urn:a-first", Type: "t", Keys: []string{"k"}},
		{URN: "urn:m-middle", Type: "t", Keys: []string{"k"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	first := strings.Index(msg, "urn:a-first")
	middle := strings.Index(msg, "urn:m-middle")
	last := strings.Index(msg, "urn:z-last")
	if !(first < middle && middle < last) {
		t.Fatalf("expected URN order a-first < m-middle < z-last; got:\n%s", msg)
	}
}

// recordDestructiveReplace must only append when the metadata actually
// flags an input-driven destructive replace. Testing the recorder (not
// just the predicate) catches a refactor that forgets the nil-check.
func TestRecordDestructiveReplace_IgnoresNonDestructiveEvents(t *testing.T) {
	p := &pulumi{}
	// nil pre-event
	p.recordDestructiveReplace(nil)
	// same op
	p.recordDestructiveReplace(&apitype.ResourcePreEvent{Metadata: apitype.StepEventMetadata{Op: apitype.OpSame}})
	// create with no Old
	p.recordDestructiveReplace(&apitype.ResourcePreEvent{Metadata: apitype.StepEventMetadata{Op: apitype.OpCreate}})
	if got := p.drainDestructiveReplaces(); len(got) != 0 {
		t.Fatalf("expected no recorded replaces, got %d: %v", len(got), got)
	}
}

// The allowlist must stay tight — adding a type here suppresses the
// guard for EVERY deploy, so a wrong entry silently reopens the
// no-destroy hole S4 exists to close. Pin the current membership so
// an unintended addition fails this test rather than reaching main.
func TestSafeToReplaceTypes_PinsCurrentMembership(t *testing.T) {
	want := map[string]bool{
		"aws:ecs/taskDefinition:TaskDefinition": true,
	}
	if len(safeToReplaceTypes) != len(want) {
		t.Fatalf("safeToReplaceTypes membership drifted: got %v (len=%d), want len=%d", keysOfSafeToReplaceTypes(), len(safeToReplaceTypes), len(want))
	}
	for typ := range want {
		if _, ok := safeToReplaceTypes[typ]; !ok {
			t.Errorf("safeToReplaceTypes missing expected entry %q", typ)
		}
	}
	for typ := range safeToReplaceTypes {
		if !want[typ] {
			t.Errorf("safeToReplaceTypes gained unexpected entry %q — update the test if intentional", typ)
		}
	}
}

func keysOfSafeToReplaceTypes() []string {
	out := make([]string, 0, len(safeToReplaceTypes))
	for k := range safeToReplaceTypes {
		out = append(out, k)
	}
	return out
}

func TestRecordDestructiveReplace_AppendsOnMatch(t *testing.T) {
	p := &pulumi{}
	p.recordDestructiveReplace(&apitype.ResourcePreEvent{Metadata: apitype.StepEventMetadata{
		Op:   apitype.OpReplace,
		URN:  "urn:pulumi:staging::stack::res",
		Type: "cloudflare:index/record:Record",
		Old:  &apitype.StepEventStateMetadata{ID: "live"},
		New:  &apitype.StepEventStateMetadata{},
		Keys: []string{"zoneId"},
	}})
	got := p.drainDestructiveReplaces()
	if len(got) != 1 {
		t.Fatalf("expected 1 recorded replace, got %d", len(got))
	}
	if got[0].URN != "urn:pulumi:staging::stack::res" || got[0].Keys[0] != "zoneId" {
		t.Fatalf("unexpected record: %+v", got[0])
	}
	// drain clears the tracker
	if got2 := p.drainDestructiveReplaces(); len(got2) != 0 {
		t.Fatalf("expected tracker to be cleared after drain, got %d", len(got2))
	}
}
