// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"strings"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"

	"github.com/simple-container-com/api/pkg/api"
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
		{
			// 2026-11-08, a 3-node k3s stand: the SECOND `sc provision`
			// refused, naming all nine of the stack's remote commands. SC
			// nonces them on purpose (TriggersForGuardedChange /
			// TriggersForRead) so a host that drifted out of band is
			// repaired, so this shape is every vm / k3s provision, and the
			// engine reports it with Keys empty.
			name: "nonce-triggered remote command with no delete → safe (replacing it runs nothing)",
			meta: apitype.StepEventMetadata{
				Op:   apitype.OpReplace,
				URN:  "urn:pulumi:fleet::sck3s::command:remote:Command::smoke-cluster-install--staging",
				Type: "command:remote:Command",
				Old: &apitype.StepEventStateMetadata{
					ID:     "smoke-cluster-install--staging1a697627",
					Inputs: map[string]any{"create": "curl -sfL https://get.k3s.io | sh -", "triggers": []any{"2026-11-08T13:14:27Z"}},
				},
				New: &apitype.StepEventStateMetadata{
					Inputs: map[string]any{"create": "curl -sfL https://get.k3s.io | sh -", "triggers": []any{"2026-11-08T13:43:39Z"}},
				},
			},
			want: false,
		},
		{
			// The k3s uninstall and the vm ingress teardown are this shape.
			// Both are registered WITHOUT triggers precisely so nothing but
			// a destroy runs them — so a replace here means a real input
			// moved (a connection address, the script itself), and
			// pulumi-command would run that delete against a live machine.
			name: "remote command carrying a delete script → destructive (the delete runs on replace)",
			meta: apitype.StepEventMetadata{
				Op:   apitype.OpReplace,
				URN:  "urn:pulumi:fleet::sck3s::command:remote:Command::smoke-vm-host-0-k3s-uninstall--staging",
				Type: "command:remote:Command",
				Old: &apitype.StepEventStateMetadata{
					ID:     "smoke-vm-host-0-k3s-uninstall--staging0c1d2e3f",
					Inputs: map[string]any{"create": "true", "delete": "/usr/local/bin/k3s-uninstall.sh"},
				},
				New: &apitype.StepEventStateMetadata{
					Inputs: map[string]any{"create": "true", "delete": "/usr/local/bin/k3s-uninstall.sh"},
				},
			},
			want: true,
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

// The built-in allowlist in pkg/api must stay tight — a bad entry
// silently reopens the no-destroy hole S4 exists to close. The full
// runtime membership includes anything a plugin advertises via
// SchemaResponse.SafeToReplaceTypes, so this test pins the built-in
// baseline (the entries pkg/api seeds at init() with no plugin
// discovery having run yet).
//
// Note: this test MUST run BEFORE any TestMain/init that spawns a
// plugin schema in the same process, or it will see plugin-added
// entries too. Today no other test in this package does so.
func TestSafeToReplaceTypes_BuiltinBaseline(t *testing.T) {
	want := map[string]bool{
		"aws:ecs/taskDefinition:TaskDefinition": true,
	}
	got := api.SafeToReplaceTypes()
	if len(got) < len(want) {
		t.Fatalf("built-in safe-to-replace set missing entries: got %v, want at least %v", got, want)
	}
	for typ := range want {
		if _, ok := got[typ]; !ok {
			t.Errorf("safe-to-replace registry missing baseline entry %q", typ)
		}
	}
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

// isStatelessRemoteCommand asks a per-RESOURCE question (does this
// command have a delete script?) rather than the per-TYPE one the
// safe-to-replace allowlist can express, so its edge cases are worth
// pinning directly: the whole fix rests on reading an input.
func TestIsStatelessRemoteCommand(t *testing.T) {
	tests := []struct {
		name string
		meta apitype.StepEventMetadata
		want bool
	}{
		{
			name: "not a remote command → never exempt, whatever its inputs say",
			meta: apitype.StepEventMetadata{
				Type: "cloudflare:index/record:Record",
				Old:  &apitype.StepEventStateMetadata{Inputs: map[string]any{}},
			},
			want: false,
		},
		{
			name: "no delete input at all → exempt",
			meta: apitype.StepEventMetadata{
				Type: remoteCommandType,
				Old:  &apitype.StepEventStateMetadata{Inputs: map[string]any{"create": "true"}},
				New:  &apitype.StepEventStateMetadata{Inputs: map[string]any{"create": "true"}},
			},
			want: true,
		},
		{
			name: "delete present but empty → exempt (pulumi-command skips the step either way)",
			meta: apitype.StepEventMetadata{
				Type: remoteCommandType,
				Old:  &apitype.StepEventStateMetadata{Inputs: map[string]any{"delete": "   "}},
				New:  &apitype.StepEventStateMetadata{Inputs: map[string]any{"delete": ""}},
			},
			want: true,
		},
		{
			// A delete that arrives only on the NEW side still means a
			// subsequent destroy would run a script, and the replace is the
			// moment to look at it.
			name: "delete on the new side only → guarded",
			meta: apitype.StepEventMetadata{
				Type: remoteCommandType,
				Old:  &apitype.StepEventStateMetadata{Inputs: map[string]any{"create": "true"}},
				New:  &apitype.StepEventStateMetadata{Inputs: map[string]any{"delete": "rm -rf /srv/app"}},
			},
			want: false,
		},
		{
			// Not provably empty, so not waved through — a secret
			// placeholder or an unresolved output lands here.
			name: "non-string delete value → treated as present, guarded",
			meta: apitype.StepEventMetadata{
				Type: remoteCommandType,
				Old:  &apitype.StepEventStateMetadata{Inputs: map[string]any{"delete": map[string]any{"4dabf18193072939515e22adb298388d": "secret"}}},
			},
			want: false,
		},
		{
			// The documented failure direction: unreadable inputs exempt a
			// remote command rather than wedging every provision, because
			// its id names no cloud object.
			name: "nil states → exempt",
			meta: apitype.StepEventMetadata{Type: remoteCommandType},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isStatelessRemoteCommand(tt.meta); got != tt.want {
				t.Fatalf("isStatelessRemoteCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}

// The engine leaves Keys empty when a provider forces replacement
// without naming a property. `keys=[]` under a sentence blaming a
// changed input reads as a contradiction; the message must point at
// `triggers` instead.
func TestDestructiveReplaceError_NamesTriggersWhenNoKeys(t *testing.T) {
	err := &destructiveReplaceError{Replaces: []destructiveReplace{{
		URN:  "urn:pulumi:fleet::sck3s::command:remote:Command::smoke-cluster-install--staging",
		Type: remoteCommandType,
	}}}
	msg := err.Error()
	if strings.Contains(msg, "keys=[]") {
		t.Fatalf("message still renders an empty key list:\n%s", msg)
	}
	if !strings.Contains(msg, "`triggers`") {
		t.Fatalf("message does not point at triggers:\n%s", msg)
	}
	// The grep-stable prefix must survive.
	if !strings.HasPrefix(msg, "input-driven destructive replace:") {
		t.Fatalf("message prefix changed:\n%s", msg)
	}
}
