// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"testing"

	. "github.com/onsi/gomega"
)

// Hazard 5 of the design doc: the three YAML shapes an operator can
// legitimately write for `uses:`, and the shape errors that must be
// refused by naming the offending Go type rather than silently reading
// as "no dependencies" — which would deploy a consumer with no wiring
// and no complaint.
func TestComponentConfigUses_Shapes(t *testing.T) {
	tests := []struct {
		name    string
		raw     map[string]any
		want    []string
		wantErr string
	}{
		{
			name: "absent key",
			raw:  map[string]any{"deployTo": "lambda"},
			want: []string{},
		},
		{
			name: "explicit nil",
			raw:  map[string]any{"uses": nil},
			want: []string{},
		},
		{
			// How it arrives after a YAML decode, i.e. the only shape
			// that happens in production.
			name: "list of any",
			raw:  map[string]any{"uses": []any{"my-notifier", "my-storage"}},
			want: []string{"my-notifier", "my-storage"},
		},
		{
			name: "list of string",
			raw:  map[string]any{"uses": []string{"my-notifier"}},
			want: []string{"my-notifier"},
		},
		{
			name: "bare string",
			raw:  map[string]any{"uses": "my-notifier"},
			want: []string{"my-notifier"},
		},
		{
			// A trailing list item in YAML, not a resource named "".
			name: "blank entries dropped",
			raw:  map[string]any{"uses": []any{"my-notifier", ""}},
			want: []string{"my-notifier"},
		},
		{
			name:    "map instead of list",
			raw:     map[string]any{"uses": map[string]any{"my-notifier": true}},
			wantErr: "uses is map[string]interface {}, want a list of resource names",
		},
		{
			name:    "non-string element",
			raw:     map[string]any{"uses": []any{"my-notifier", 42}},
			wantErr: "uses[1] is int, want string resource name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			cfg, err := NewComponentConfig(&PluginFieldConfig{Raw: tt.raw})
			Expect(err).To(BeNil())
			got, err := cfg.Uses()
			if tt.wantErr != "" {
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring(tt.wantErr))
				return
			}
			Expect(err).To(BeNil())
			Expect(got).To(Equal(tt.want))
		})
	}
}

// `uses` has to be in the well-known key list or the whole stack fails
// to parse with "unknown or disallowed top-level key" — which is how
// writing it behaved before this slice.
func TestComponentConfigUses_IsWellKnownTopLevelKey(t *testing.T) {
	RegisterTestingT(t)
	Expect(IsComponentConfigTopLevelKey("uses")).To(BeTrue())
}

// Hazard 3, at the layer that decides ordering. `uses:` names must
// become provision-order dependencies, because the consumer's wiring
// cannot run before the used resource exists — and the way this fails
// without the edge is the problem: `ResolvePlaceholders` fails open, so
// a too-early consumer deploys the literal `${resource:…}` string
// instead of erroring.
func TestDependsOnResources_IncludesUses(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]any
		want []string
	}{
		{
			name: "uses list",
			raw:  map[string]any{"uses": []any{"my-notifier"}},
			want: []string{"my-notifier"},
		},
		{
			name: "uses bare string",
			raw:  map[string]any{"uses": "my-notifier"},
			want: []string{"my-notifier"},
		},
		{
			name: "uses []string",
			raw:  map[string]any{"uses": []string{"my-notifier", "my-storage"}},
			want: []string{"my-notifier", "my-storage"},
		},
		{
			// Both sources agree on one edge rather than reporting the
			// same name twice.
			name: "uses and a ${resource:} token naming the same resource",
			raw: map[string]any{
				"uses": []any{"my-notifier"},
				"env":  map[string]any{"URL": "${resource:my-notifier.url}"},
			},
			want: []string{"my-notifier"},
		},
		{
			name: "uses plus an unrelated token",
			raw: map[string]any{
				"uses": []any{"my-notifier"},
				"env":  map[string]any{"B": "${resource:my-bucket.name}"},
			},
			want: []string{"my-bucket", "my-notifier"},
		},
		{
			// A malformed `uses:` contributes no edge: Uses() is the
			// accessor that refuses it, with a message naming the type.
			// Two errors for one typo is worse than one.
			name: "malformed uses contributes nothing",
			raw:  map[string]any{"uses": map[string]any{"my-notifier": true}},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			cfg := &PluginFieldConfig{Raw: tt.raw}
			got := cfg.DependsOnResources()
			names := make([]string, 0, len(got))
			for _, d := range got {
				names = append(names, d.Name)
			}
			Expect(names).To(ConsistOf(toAnySlice(tt.want)...))
		})
	}
}

func toAnySlice(in []string) []any {
	out := make([]any, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}
