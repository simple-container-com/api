// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import "testing"

// By TYPE, not by marshalling. The static case is the one that matters: the
// domain sits inside a NAMED field tagged `json:",inline"`, which
// encoding/json does not inline, so every JSON-probe approach has to know
// that `Site` becomes a capital-S nested object. A type switch does not.
func TestStackConfigDomainOf(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  any
		want string
	}{
		{"static, pointer", &StackConfigStatic{Site: StaticSiteConfig{Domain: "simple-forge.ru"}}, "simple-forge.ru"},
		{"static, value", StackConfigStatic{Site: StaticSiteConfig{Domain: "simple-forge.ru"}}, "simple-forge.ru"},
		{"compose, pointer", &StackConfigCompose{Domain: "a.example.com"}, "a.example.com"},
		{"compose, value", StackConfigCompose{Domain: "a.example.com"}, "a.example.com"},
		{"single-image, pointer", &StackConfigSingleImage{Domain: "b.example.com"}, "b.example.com"},
		{"single-image, value", StackConfigSingleImage{Domain: "b.example.com"}, "b.example.com"},
		// A worker-only stack. "" is a legitimate answer, not an error.
		{"no domain", &StackConfigCompose{}, ""},
		// Must not panic on either flavour of absent input.
		{"nil interface", nil, ""},
		{"typed nil", (*StackConfigStatic)(nil), ""},
		// An unknown shape answers "" rather than guessing.
		{"unknown type", struct{ Domain string }{Domain: "ignored.example.com"}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := StackConfigDomainOf(tt.cfg); got != tt.want {
				t.Fatalf("StackConfigDomainOf = %q, want %q", got, tt.want)
			}
		})
	}
}
