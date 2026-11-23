// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"testing"

	. "github.com/onsi/gomega"
)

// TestPluginFieldConfigDependsOnResources covers the shapes a Component's
// `.Raw` actually arrives in. The nested cases are the point: the ordering
// has to agree with `ResolvePlaceholders`, which walks the whole object
// graph reflectively, so a scan that only looked at top-level fields would
// order a deploy the resolver then resolves anyway.
func TestPluginFieldConfigDependsOnResources(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]any
		want []string
	}{
		{
			name: "no references",
			raw:  map[string]any{"deployTo": "lambda", "maxMemory": 512},
			want: nil,
		},
		{
			name: "nil raw",
			raw:  nil,
			want: nil,
		},
		{
			name: "top-level reference",
			raw:  map[string]any{"notifyUrl": "${resource:my-notifier.self-url}"},
			want: []string{"my-notifier"},
		},
		{
			name: "reference nested under a map",
			raw: map[string]any{
				"notify": map[string]any{"url": "${resource:my-notifier.self-url}"},
			},
			want: []string{"my-notifier"},
		},
		{
			name: "reference nested inside a list of maps",
			raw: map[string]any{
				"channels": []any{
					map[string]any{"name": "ops", "url": "${resource:my-notifier.self-url}"},
				},
			},
			want: []string{"my-notifier"},
		},
		{
			name: "reference under a yaml.v2 map[any]any node",
			raw: map[string]any{
				"notify": map[any]any{"url": "${resource:my-notifier.self-url}"},
			},
			want: []string{"my-notifier"},
		},
		{
			name: "two references to the same resource collapse to one dependency",
			raw: map[string]any{
				"a": "${resource:my-notifier.self-url}",
				"b": "${resource:my-notifier.ingest-path}",
			},
			want: []string{"my-notifier"},
		},
		{
			name: "two tokens in one string yield both names",
			raw: map[string]any{
				"url": "${resource:my-notifier.self-url}/t/${resource:my-backup.tenant-id}",
			},
			want: []string{"my-notifier", "my-backup"},
		},
		{
			name: "a self-reference is reported; provision_order drops it by name",
			raw:  map[string]any{"url": "${resource:my-backup.self-url}"},
			want: []string{"my-backup"},
		},
		{
			name: "token with no dot contributes nothing",
			raw:  map[string]any{"url": "${resource:my-notifier}"},
			want: nil,
		},
		{
			name: "token with an empty name contributes nothing",
			raw:  map[string]any{"url": "${resource:.self-url}"},
			want: nil,
		},
		{
			name: "token with an empty key contributes nothing",
			raw:  map[string]any{"url": "${resource:my-notifier.}"},
			want: nil,
		},
		{
			name: "unterminated token contributes nothing",
			raw:  map[string]any{"url": "${resource:my-notifier.self-url"},
			want: nil,
		},
		{
			name: "a ${dependency:…} token is not a sibling reference",
			raw:  map[string]any{"url": "${dependency:stack.res.uri}"},
			want: nil,
		},
		{
			name: "a ${secret:…} token is not a sibling reference",
			raw:  map[string]any{"token": "${secret:sc-notifier-tg-token}"},
			want: nil,
		},
		{
			// Reaches the empty-name/no-dot guard, not the nested-token one:
			// the scan stops at the FIRST `}`, which is the one closing
			// `${name}`, leaving a body of `${name` with no dot at all.
			name: "an unsubstituted ${name} token contributes nothing",
			raw:  map[string]any{"url": "${resource:${name}.self-url}"},
			want: nil,
		},
		{
			// The case that actually reaches the nested-`${` guard: the
			// first `}` closes a token that itself contains a dot, so the
			// body parses as name=`${a`, key=`b`. Without the guard that
			// yields a dependency on a resource called `${a`. Found by
			// inverting the guard — the case above left it untested.
			name: "a nested token carrying a dot contributes nothing",
			raw:  map[string]any{"url": "${resource:${a.b}.self-url}"},
			want: nil,
		},
		{
			name: "a non-string leaf next to a reference does not break the walk",
			raw: map[string]any{
				"maxMemory": 512,
				"enabled":   true,
				"notify":    map[string]any{"url": "${resource:my-notifier.self-url}"},
			},
			want: []string{"my-notifier"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			cfg := &PluginFieldConfig{TypeName: "sc-backup", Raw: tt.raw}
			got := cfg.DependsOnResources()

			names := make([]string, 0, len(got))
			for _, d := range got {
				names = append(names, d.Name)
			}
			if tt.want == nil {
				Expect(names).To(BeEmpty())
				return
			}
			// Order matters for the two-tokens-in-one-string case; the
			// single-key cases have one element so it is moot.
			Expect(names).To(Equal(tt.want))
		})
	}
}

// TestPluginFieldConfigDependsOnResourcesNilReceiver guards the ordering
// call site, which type-asserts and then calls without a nil check.
func TestPluginFieldConfigDependsOnResourcesNilReceiver(t *testing.T) {
	RegisterTestingT(t)

	var cfg *PluginFieldConfig
	Expect(cfg.DependsOnResources()).To(BeEmpty())
}

// TestPluginFieldConfigSatisfiesWithParentDependencies is the assertion
// that actually matters for the bug: `resourceProvisionOrder` reaches this
// method only through the `api.WithParentDependencies` type assertion, so
// a signature that drifts from the interface silently restores the
// alphabetical-ordering behaviour with every test above still passing.
func TestPluginFieldConfigSatisfiesWithParentDependencies(t *testing.T) {
	RegisterTestingT(t)

	var cfg any = &PluginFieldConfig{Raw: map[string]any{
		"notify": map[string]any{"url": "${resource:my-notifier.self-url}"},
	}}

	withDeps, ok := cfg.(WithParentDependencies)
	Expect(ok).To(BeTrue(), "PluginFieldConfig must satisfy WithParentDependencies or provision ordering never consults it")
	Expect(withDeps.DependsOnResources()).To(HaveLen(1))
	Expect(withDeps.DependsOnResources()[0].Name).To(Equal("my-notifier"))
}
