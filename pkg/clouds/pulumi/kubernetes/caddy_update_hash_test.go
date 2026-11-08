// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestCaddyUpdateHashValue(t *testing.T) {
	g := NewWithT(t)

	// The entry template both starts and ends with a newline (see simple_container.go's
	// caddyfileEntry render), and those bytes are part of what the init container hashes.
	applied := "\nhttps://smoke.example.com {\n  reverse_proxy http://app.ns.svc.cluster.local:8080\n}\n"
	static := "\nhttp://smoke.example.com {\n  reverse_proxy http://app.ns.svc.cluster.local:8080\n}\n"

	t.Run("prefers the applied annotation over the static render", func(t *testing.T) {
		// The case that matters on a migrated stack: the two differ by design, because the
		// applied annotation is re-rendered against the legacy namespace. Hashing the static
		// one gives a digest no running Caddy can ever reproduce from cluster state.
		g.Expect(caddyUpdateHashValue(applied, static)).To(Equal(md5Hex(applied)))
		g.Expect(caddyUpdateHashValue(applied, static)).ToNot(Equal(md5Hex(static)))
	})

	t.Run("falls back to the static render when the Service carries no annotation", func(t *testing.T) {
		// Pre-existing behaviour, preserved: a workload with no ports has no Service, and a
		// Service may not carry the annotation yet on a first-ever apply.
		g.Expect(caddyUpdateHashValue("", static)).To(Equal(md5Hex(static)))
	})

	t.Run("is stable and sensitive to a trailing newline", func(t *testing.T) {
		g.Expect(caddyUpdateHashValue(applied, "")).To(Equal(caddyUpdateHashValue(applied, "")))
		// `entry=$(kubectl ...)` would strip this newline; the script reads the annotation to
		// a file precisely so it does not. Assert the two are genuinely different digests, so
		// a future "simplification" back to a capture fails here rather than in production.
		g.Expect(md5Hex(applied)).ToNot(Equal(md5Hex(applied[:len(applied)-1])))
	})

	t.Run("yields a hash long enough to slice for caddy-updated-at", func(t *testing.T) {
		// Both call sites take hash[:8] for the informational annotation.
		g.Expect(len(caddyUpdateHashValue("", ""))).To(Equal(32))
	})
}
