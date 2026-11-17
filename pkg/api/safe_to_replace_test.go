// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import "testing"

// Pin the built-in membership, AND the rule each entry is exempt under.
// Plugin advertisements add to rule 1 at stackread time; this test runs
// with no plugins loaded, so the registry must contain exactly the
// built-in seed. A silent addition — someone pastes a new entry into
// safe_to_replace.go without the roadmap item update — fails here before
// it reaches main.
//
// The rule is pinned and not just the membership because the two rules
// are different promises. A type filed under rule 1 is asserted to keep
// its old revision alive through the replace; moving a Secret there
// would be claiming something untrue of it, and nothing but this would
// notice.
func TestSafeToReplace_BuiltinSeed(t *testing.T) {
	want := map[string]ReplaceRule{
		"aws:ecs/taskDefinition:TaskDefinition":          ReplaceRuleRevision,
		"kubernetes:core/v1:Secret":                      ReplaceRuleConfigRegenerated,
		"kubernetes:core/v1:ConfigMap":                   ReplaceRuleConfigRegenerated,
		"kubernetes:batch/v1:Job":                        ReplaceRuleConfigRegenerated,
		"aws:secretsmanager/secretVersion:SecretVersion": ReplaceRuleConfigRegenerated,
		"cloudflare:index/workersRoute:WorkersRoute":     ReplaceRuleConfigRegenerated,
	}
	got := SafeToReplaceTypes()
	if len(got) != len(want) {
		t.Fatalf("built-in safe-to-replace seed drifted: got %v (len=%d), want len=%d", got, len(got), len(want))
	}
	for typ, rule := range want {
		if _, ok := got[typ]; !ok {
			t.Errorf("safe-to-replace seed missing expected entry %q", typ)
		}
		if gotRule := SafeToReplaceRule(typ); gotRule != rule {
			t.Errorf("%q is exempt under rule %q, want %q", typ, gotRule, rule)
		}
	}
	for typ := range got {
		if _, ok := want[typ]; !ok {
			t.Errorf("safe-to-replace seed gained unexpected entry %q — if intentional, update the test AND the roadmap item", typ)
		}
	}
}

// A type nobody exempted must stay refused. Named explicitly because the
// whole value of the guard is in what it keeps saying no to, and a
// widened rule is exactly the change that could take these with it.
func TestSafeToReplace_StatefulTypesStayRefused(t *testing.T) {
	for _, typ := range []string{
		"aws:rds/instance:Instance",
		"aws:s3/bucket:Bucket",
		"kubernetes:core/v1:PersistentVolumeClaim",
		"mongodbatlas:index/cluster:Cluster",
		"kubernetes:apps/v1:Deployment",
	} {
		if IsSafeToReplace(typ) {
			t.Errorf("%q is exempt from the destructive-replace guard; a replace of it loses data", typ)
		}
		if rule := SafeToReplaceRule(typ); rule != ReplaceRuleNone {
			t.Errorf("%q reports rule %q, want none", typ, rule)
		}
	}
}

// Rule 2 is built-in only: "is this resource's content fully reproducible
// from config" is a judgement about the operator's data, not a fact about
// a provider, so a plugin cannot advertise its way into it.
// RegisterSafeToReplaceType registers under RULE 1 (revision-based),
// not rule 2. The two allowlists are distinct and this test pins the
// guarantee that rule 1 and rule 2 stay separate — a wrong claim
// about rule 2 must require RegisterConfigRegeneratedType, not
// RegisterSafeToReplaceType.
func TestSafeToReplace_RegisterSafeToReplaceGoesToRule1(t *testing.T) {
	const typ = "test:pkg/api/safe_to_replace:AdvertisedThing"
	RegisterSafeToReplaceType(typ)
	t.Cleanup(func() {
		safeToReplaceMu.Lock()
		delete(safeToReplaceTypes, typ)
		safeToReplaceMu.Unlock()
	})
	if rule := SafeToReplaceRule(typ); rule != ReplaceRuleRevision {
		t.Fatalf("a registered type reports rule %q, want %q", rule, ReplaceRuleRevision)
	}
}

// Plugins can advertise rule-2 types via SchemaResponse.config_regenerated_types;
// the stackread wire calls RegisterConfigRegeneratedType for each entry.
// The claim deserves careful review (wrong assertions cause silent data
// loss) but is no longer api-core-only.
func TestRegisterConfigRegeneratedType_PluginAdvertiseWorks(t *testing.T) {
	const typ = "test:pkg/api/safe_to_replace:RegeneratedThing"
	if IsSafeToReplace(typ) {
		t.Fatalf("unexpected seed contains %q", typ)
	}
	RegisterConfigRegeneratedType(typ)
	t.Cleanup(func() {
		safeToReplaceMu.Lock()
		delete(configRegeneratedTypes, typ)
		safeToReplaceMu.Unlock()
	})
	if !IsSafeToReplace(typ) {
		t.Fatalf("after RegisterConfigRegeneratedType(%q), IsSafeToReplace returned false", typ)
	}
	if rule := SafeToReplaceRule(typ); rule != ReplaceRuleConfigRegenerated {
		t.Fatalf("registered type reports rule %q, want %q", rule, ReplaceRuleConfigRegenerated)
	}
	// Idempotent.
	RegisterConfigRegeneratedType(typ)
	if !IsSafeToReplace(typ) {
		t.Fatalf("second Register call lost the entry")
	}
	// Empty string is a no-op.
	before := len(SafeToReplaceTypes())
	RegisterConfigRegeneratedType("")
	if after := len(SafeToReplaceTypes()); after != before {
		t.Fatalf("empty string must not register; before=%d after=%d", before, after)
	}
}

func TestSafeToReplace_RegisterAndIs(t *testing.T) {
	const typ = "test:pkg/api/safe_to_replace:Thing"
	if IsSafeToReplace(typ) {
		t.Fatalf("unexpected seed contains %q", typ)
	}
	RegisterSafeToReplaceType(typ)
	t.Cleanup(func() {
		safeToReplaceMu.Lock()
		delete(safeToReplaceTypes, typ)
		safeToReplaceMu.Unlock()
	})
	if !IsSafeToReplace(typ) {
		t.Fatalf("after RegisterSafeToReplaceType(%q), IsSafeToReplace returned false", typ)
	}
	// Idempotent — second call must not panic or duplicate.
	RegisterSafeToReplaceType(typ)
	if !IsSafeToReplace(typ) {
		t.Fatalf("second Register call lost the entry")
	}
	// Empty string is a no-op (a plugin whose Schema returns a blank
	// line should not pollute the registry).
	before := len(SafeToReplaceTypes())
	RegisterSafeToReplaceType("")
	if after := len(SafeToReplaceTypes()); after != before {
		t.Fatalf("empty string must not register; before=%d after=%d", before, after)
	}
}

func TestSafeToReplace_SnapshotIsCopy(t *testing.T) {
	const typ = "test:pkg/api/safe_to_replace:SnapshotCheck"
	RegisterSafeToReplaceType(typ)
	t.Cleanup(func() {
		safeToReplaceMu.Lock()
		delete(safeToReplaceTypes, typ)
		safeToReplaceMu.Unlock()
	})
	snap := SafeToReplaceTypes()
	delete(snap, typ)
	if !IsSafeToReplace(typ) {
		t.Fatalf("mutating the snapshot affected the registry — SafeToReplaceTypes must return a copy")
	}
}
