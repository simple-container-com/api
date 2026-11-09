// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"strings"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"

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
		{
			// 2026-11-08, same stand: adding the shared ACME store's
			// credentials to the Caddy deployment. pulumi-kubernetes treats
			// `stringData` as replace-forcing, so this is EVERY credential
			// rotation and every added secret env var on every SC Kubernetes
			// stack — the guard refused all of them from the day it merged.
			// Exempt under rule 2, not rule 1: the object really is briefly
			// absent, and that is acceptable only because its whole content
			// comes from the client's config on each apply.
			name: "Secret content change → safe (content is regenerated from config)",
			meta: apitype.StepEventMetadata{
				Op:   apitype.OpReplace,
				URN:  "urn:pulumi:fleet::sck3s::kubernetes:core/v1:Secret::caddy-staging-secrets",
				Type: "kubernetes:core/v1:Secret",
				Old:  &apitype.StepEventStateMetadata{ID: "caddy/caddy-staging-secrets"},
				New:  &apitype.StepEventStateMetadata{},
				Keys: []string{"stringData"},
			},
			want: false,
		},
		{
			name: "ConfigMap content change → safe (same rule)",
			meta: apitype.StepEventMetadata{
				Op:   apitype.OpReplace,
				URN:  "urn:pulumi:fleet::sck3s::kubernetes:core/v1:ConfigMap::caddy-staging-caddyfile",
				Type: "kubernetes:core/v1:ConfigMap",
				Old:  &apitype.StepEventStateMetadata{ID: "caddy/caddy-staging-caddyfile"},
				New:  &apitype.StepEventStateMetadata{},
				Keys: []string{"data"},
			},
			want: false,
		},
		{
			// The exemption is per TYPE and must not generalise to "anything
			// Kubernetes". A PVC replace loses the volume.
			name: "PVC replace → destructive (a Kubernetes type is not automatically safe)",
			meta: apitype.StepEventMetadata{
				Op:   apitype.OpReplace,
				URN:  "urn:pulumi:fleet::sck3s::kubernetes:core/v1:PersistentVolumeClaim::caddy-staging-data",
				Type: "kubernetes:core/v1:PersistentVolumeClaim",
				Old:  &apitype.StepEventStateMetadata{ID: "caddy/caddy-staging-data"},
				New:  &apitype.StepEventStateMetadata{},
				Keys: []string{"spec"},
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

// An operator reading this message has one decision to make and the
// message has to give them the question, not just the override. Setting
// the variable was the only actionable sentence in it until 2026-11-08,
// which is how an escape hatch becomes a routine step and stops being
// read at all.
func TestDestructiveReplaceError_TellsTheOperatorHowToDecide(t *testing.T) {
	t.Setenv(envAllowDestructiveReplace, "")
	err := destructiveReplaceErrorFrom([]destructiveReplace{{
		URN:  "urn:pulumi:staging::proj::aws:rds/instance:Instance::db",
		Type: "aws:rds/instance:Instance",
		Keys: []string{"engineVersion"},
	}})
	if err == nil {
		t.Fatal("expected a destructive-replace error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{
		// The question that decides it.
		"what the replace actually loses",
		// Where a false positive is fixed for every stack, instead of
		// per-deploy with the override.
		"pkg/api/safe_to_replace.go",
		"safe_to_replace_types",
		// And the override still named, scoped to a reviewed intent.
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
	// The guard's own package pins what the guard depends on being exempt.
	// `pkg/api`'s test pins exact membership and the rule each entry sits
	// under; this one is "at least", and names the entries whose absence
	// would make a specific deploy start refusing again — the ECS task
	// definition (forge-baas run 42077847984) and the two Kubernetes types
	// whose content change IS a replace (every credential rotation).
	want := map[string]bool{
		"aws:ecs/taskDefinition:TaskDefinition": true,
		"kubernetes:core/v1:Secret":             true,
		"kubernetes:core/v1:ConfigMap":          true,
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

// Rule 3 through the guard, not through the registry: the exact event
// that stopped the 3-VPS fleet stand on 2026-11-09 must now be recorded
// as nothing at all.
//
// A `pulumi:providers:*` replace is the shape EVERY credential change
// takes — the kubeconfig below, but equally a rotated cloud key on any
// provider in any stack — so a regression here does not fail one smoke,
// it makes `SC_ALLOW_DESTRUCTIVE_REPLACE=1` the documented way to rotate
// a credential.
func TestRecordDestructiveReplace_ExemptsProviderResources(t *testing.T) {
	p := &pulumi{}
	p.recordDestructiveReplace(&apitype.ResourcePreEvent{Metadata: apitype.StepEventMetadata{
		Op:   apitype.OpReplace,
		URN:  "urn:pulumi:fleet::scsmoke::pulumi:providers:kubernetes::smoke-caddy--staging-caddy-kubeprovider",
		Type: "pulumi:providers:kubernetes",
		Old:  &apitype.StepEventStateMetadata{ID: "live"},
		New:  &apitype.StepEventStateMetadata{},
		Keys: []string{"kubeconfig"},
	}})
	if got := p.drainDestructiveReplaces(); len(got) != 0 {
		t.Fatalf("a provider replace was recorded as destructive: %+v", got)
	}
}

// The prefix is matched as a prefix of the TYPE, which is the only
// reason a single entry can cover every provider package. A type that
// merely contains the string somewhere else is not exempt — that is the
// difference between a reserved namespace and a substring.
func TestEngineOnlyTypes_PrefixNotSubstring(t *testing.T) {
	for _, typ := range []string{
		"pulumi:providers:kubernetes",
		"pulumi:providers:aws",
		"pulumi:providers:yandex",
		"pulumi:providers:command",
	} {
		if !api.IsEngineOnlyType(typ) {
			t.Errorf("%q should be engine-only", typ)
		}
		if api.SafeToReplaceRule(typ) != api.ReplaceRuleEngineOnly {
			t.Errorf("%q: rule = %q, want engine-only", typ, api.SafeToReplaceRule(typ))
		}
	}
	for _, typ := range []string{
		"kubernetes:core/v1:Secret",            // exempt, but under rule 2
		"gcp:container/cluster:Cluster",        // not exempt at all
		"mycloud:index/pulumi:providers:Thing", // contains it, does not start with it
		"pulumi-providers:kubernetes",          // near miss
	} {
		if api.IsEngineOnlyType(typ) {
			t.Errorf("%q must not be engine-only", typ)
		}
	}
}

// Rule 3 is deliberately absent from the enumerable snapshot: a prefix
// denotes an open set. A caller that asks the map instead of the
// predicate would wrongly conclude a provider replace is destructive,
// which is exactly the bug this guards.
func TestSafeToReplaceTypes_DoesNotEnumerateRule3(t *testing.T) {
	snap := api.SafeToReplaceTypes()
	for typ := range snap {
		if api.IsEngineOnlyType(typ) {
			t.Errorf("snapshot enumerates prefix-matched type %q", typ)
		}
	}
	if _, ok := snap["pulumi:providers:kubernetes"]; ok {
		t.Error("snapshot must not claim to list provider types")
	}
	// But the guard's own question still answers yes.
	if !api.IsSafeToReplace("pulumi:providers:kubernetes") {
		t.Error("IsSafeToReplace must honour rule 3")
	}
}

// The seven-resource refusal measured on the 3-VPS fleet stand (2026-11-09). Every one
// of these replaces named `provider` as its sole key, with an identical provider URN and
// the unknown sentinel as the new ID — and the apply that followed only updated them.
func TestRecordDestructiveReplace_ExemptsUnknownProviderIDOnly(t *testing.T) {
	const provURN = "urn:pulumi:fleet::scsmoke::pulumi:providers:kubernetes::smoke-caddy--staging-caddy-kubeprovider"

	for _, resType := range []string{
		"kubernetes:apps/v1:Deployment",
		"kubernetes:core/v1:Namespace",
		"kubernetes:core/v1:Service",
		"kubernetes:core/v1:ServiceAccount",
		"kubernetes:policy/v1:PodDisruptionBudget",
		"kubernetes:rbac.authorization.k8s.io/v1:ClusterRole",
		"kubernetes:rbac.authorization.k8s.io/v1:ClusterRoleBinding",
	} {
		p := &pulumi{}
		p.recordDestructiveReplace(&apitype.ResourcePreEvent{Metadata: apitype.StepEventMetadata{
			Op:   apitype.OpReplace,
			URN:  "urn:pulumi:fleet::scsmoke::" + resType + "::caddy-staging",
			Type: resType,
			Old:  &apitype.StepEventStateMetadata{ID: "live", Provider: provURN + "::d85914e8-7282-41fc-82a9-ae3d49f014a6"},
			New:  &apitype.StepEventStateMetadata{Provider: provURN + "::04da6b54-80e4-46f7-96ec-b56ff0331ba9"},
			Keys: []string{"provider"},
		}})
		if got := p.drainDestructiveReplaces(); len(got) != 0 {
			t.Fatalf("%s: an unknown-provider-ID replace was recorded as destructive: %+v", resType, got)
		}
	}
}

// Each of these is one mutation away from the exempt case above, and each must stay
// guarded. The exemption is narrow by construction; this is what holds it narrow.
func TestUnknownProviderIDOnly_StaysGuarded(t *testing.T) {
	const provURN = "urn:pulumi:fleet::scsmoke::pulumi:providers:kubernetes::caddy-kubeprovider"
	const otherURN = "urn:pulumi:fleet::scsmoke::pulumi:providers:kubernetes::other-kubeprovider"
	const unknown = "::04da6b54-80e4-46f7-96ec-b56ff0331ba9"
	const realID = "::d85914e8-7282-41fc-82a9-ae3d49f014a6"

	for name, meta := range map[string]apitype.StepEventMetadata{
		// The fault the guard exists for: a resource moved to a different
		// cluster's provider. Unknown ID or not, the URN changed.
		"a different provider URN": {
			Old:  &apitype.StepEventStateMetadata{ID: "live", Provider: provURN + realID},
			New:  &apitype.StepEventStateMetadata{Provider: otherURN + unknown},
			Keys: []string{"provider"},
		},
		// A provider that genuinely was replaced and whose new ID is known.
		"a known but different provider ID": {
			Old:  &apitype.StepEventStateMetadata{ID: "live", Provider: provURN + realID},
			New:  &apitype.StepEventStateMetadata{Provider: provURN + "::11111111-2222-3333-4444-555555555555"},
			Keys: []string{"provider"},
		},
		// A real input moved too. That input's replace is not excused by the
		// provider's ID being unknown alongside it.
		"provider plus a real input": {
			Old:  &apitype.StepEventStateMetadata{ID: "live", Provider: provURN + realID},
			New:  &apitype.StepEventStateMetadata{Provider: provURN + unknown},
			Keys: []string{"provider", "metadata"},
		},
		// Not the provider reference at all — a property that happens to be
		// named "provider" on some resource type.
		"a non-provider key": {
			Old:  &apitype.StepEventStateMetadata{ID: "live", Provider: provURN + realID},
			New:  &apitype.StepEventStateMetadata{Provider: provURN + unknown},
			Keys: []string{"zoneId"},
		},
		// A malformed reference must fail closed, not open.
		"a provider reference with no id separator": {
			Old:  &apitype.StepEventStateMetadata{ID: "live", Provider: "not-a-reference"},
			New:  &apitype.StepEventStateMetadata{Provider: provURN + unknown},
			Keys: []string{"provider"},
		},
	} {
		meta.Op = apitype.OpReplace
		meta.URN = "urn:pulumi:fleet::scsmoke::kubernetes:core/v1:Service::caddy-staging"
		meta.Type = "kubernetes:core/v1:Service"

		p := &pulumi{}
		p.recordDestructiveReplace(&apitype.ResourcePreEvent{Metadata: meta})
		if got := p.drainDestructiveReplaces(); len(got) != 1 {
			t.Fatalf("%s: expected the replace to stay guarded, recorded %+v", name, got)
		}
	}
}

// The URN contains `::` as its own field separator, so splitting on the FIRST
// occurrence would compare a fragment of the URN against the sentinel and make the
// exemption fire for unrelated providers.
func TestSplitProviderRef_SplitsOnTheLastSeparator(t *testing.T) {
	urn, id, ok := splitProviderRef("urn:pulumi:fleet::scsmoke::pulumi:providers:kubernetes::kubeprovider::abc-123")
	if !ok {
		t.Fatal("a well-formed reference was rejected")
	}
	if id != "abc-123" {
		t.Fatalf("id = %q, want abc-123", id)
	}
	if urn != "urn:pulumi:fleet::scsmoke::pulumi:providers:kubernetes::kubeprovider" {
		t.Fatalf("urn = %q", urn)
	}
	if _, _, ok := splitProviderRef("no-separator"); ok {
		t.Fatal("a reference with no separator was accepted")
	}
}

// pulumiUnknownValue is hardcoded so this file needs only apitype. That is only safe
// while it agrees with the SDK, and the SDK is already a dependency — so pin it.
func TestPulumiUnknownValueMatchesTheSDK(t *testing.T) {
	if pulumiUnknownValue != plugin.UnknownStringValue {
		t.Fatalf("pulumiUnknownValue = %q, SDK says %q — the sentinel moved", pulumiUnknownValue, plugin.UnknownStringValue)
	}
}
