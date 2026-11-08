// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"testing"

	. "github.com/onsi/gomega"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// TestIsPulumiOutputSet pins the distinction the Caddyfile-prefix branch turns on.
//
// Only the GCP/GKE path assigns CaddyfilePrefixOut (the GCS storage block). Everywhere else
// the field is the zero value and the static `caddyfilePrefix`/`trustedProxies` branch must
// win. The predicate used to be `ElementType() != nil`, which is a VALUE method on
// StringOutput returning a constant type — non-nil on the zero value too. So the dynamic
// branch always won, CADDYFILE_PREFIX was always rendered as the empty string, and both
// config fields were silently dropped on every non-GKE cluster.
func TestIsPulumiOutputSet(t *testing.T) {
	g := NewGomegaWithT(t)

	t.Run("zero value is not set", func(t *testing.T) {
		var zero sdk.StringOutput
		// The regression guard: this is exactly what the GKE-only field looks like on a
		// k3s or kubernetes-cloudrun stack.
		g.Expect(isPulumiOutputSet(zero)).To(BeFalse())
		// And the reason the old predicate could not tell the two apart.
		g.Expect(zero.ElementType()).ToNot(BeNil())
	})

	t.Run("assigned output is set", func(t *testing.T) {
		g.Expect(isPulumiOutputSet(sdk.String("{\n\tstorage s3 {\n\t}\n}").ToStringOutput())).To(BeTrue())
	})

	t.Run("assigned empty output is still set", func(t *testing.T) {
		// Assignment, not content, is the question — a caller that computed an empty
		// prefix has still spoken, and the static branch must not override it.
		g.Expect(isPulumiOutputSet(sdk.String("").ToStringOutput())).To(BeTrue())
	})
}
