// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import "testing"

// Pin the built-in membership. Plugin advertisements add to this set at
// stackread time; this test runs with no plugins loaded, so the registry
// must contain exactly the built-in seed. A silent addition — someone
// pastes a new entry into safe_to_replace.go without the roadmap item
// update — fails here before it reaches main.
func TestSafeToReplace_BuiltinSeed(t *testing.T) {
	want := map[string]bool{
		"aws:ecs/taskDefinition:TaskDefinition": true,
	}
	got := SafeToReplaceTypes()
	if len(got) != len(want) {
		t.Fatalf("built-in safe-to-replace seed drifted: got %v (len=%d), want len=%d", got, len(got), len(want))
	}
	for typ := range want {
		if _, ok := got[typ]; !ok {
			t.Errorf("safe-to-replace seed missing expected entry %q", typ)
		}
	}
	for typ := range got {
		if !want[typ] {
			t.Errorf("safe-to-replace seed gained unexpected entry %q — if intentional, update the test AND the roadmap item", typ)
		}
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
