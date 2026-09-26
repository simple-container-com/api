// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"testing"

	. "github.com/onsi/gomega"
)

// Yandex Container Registry enforces the legacy Docker repository grammar, and
// answers a bare 400 to the blob HEAD when a name violates it — so these cases
// are transcribed from a live probe against cr.yandex (2026-09-26) rather than
// from the registry's documentation, which does not state the rule.
func TestToRepositoryName(t *testing.T) {
	RegisterTestingT(t)

	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		// The case that matters: every SC client stack is named <stack>--<env>,
		// so this is the shape every Forge service would push under.
		{name: "stack--env", in: "ycdemo--smoke", want: "ycdemo-smoke"},
		{name: "fleet service", in: "forge-storage--staging", want: "forge-storage-staging"},
		{name: "several runs", in: "aa--bb--cc", want: "aa-bb-cc"},
		{name: "longer run collapses too", in: "a----b", want: "a-b"},
		// A run collapses to its first character rather than always to "-", so a
		// name that used underscores keeps using them.
		{name: "double underscore", in: "a__b", want: "a_b"},
		{name: "mixed run keeps the leading separator", in: "a-_b", want: "a-b"},
		// Legal names must pass through untouched.
		{name: "single hyphen", in: "a-b-c", want: "a-b-c"},
		{name: "single underscore", in: "a_b", want: "a_b"},
		{name: "dot", in: "a.b", want: "a.b"},
		// Leading/trailing separators are rejected by the same grammar.
		{name: "trims edges", in: "-a--b-", want: "a-b"},
		{name: "uppercase is not a legal repository name", in: "Ycdemo--Smoke", want: "ycdemo-smoke"},
		{name: "nothing left", in: "--", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(toRepositoryName(tc.in)).To(Equal(tc.want))
		})
	}
}
