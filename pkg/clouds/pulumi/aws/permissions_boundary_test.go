// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package aws

import (
	"testing"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// permissionsBoundaryPtr's contract: an empty/whitespace-only ARN yields nil
// (no boundary set — the backward-compat guarantee for stacks that don't opt
// in), and a real ARN yields the trimmed value (a downstream enforcement flip
// matches the boundary EXACTLY, so a wrong/padded value must not slip through).
func TestPermissionsBoundaryPtr(t *testing.T) {
	tests := []struct {
		name    string
		arn     string
		wantNil bool
		wantVal string // expected sdk.String value when !wantNil
	}{
		{name: "empty means no boundary", arn: "", wantNil: true},
		{name: "whitespace means no boundary", arn: "   ", wantNil: true},
		{name: "arn sets a boundary", arn: "arn:aws:iam::123456789012:policy/my-workload-boundary", wantNil: false, wantVal: "arn:aws:iam::123456789012:policy/my-workload-boundary"},
		{name: "surrounding whitespace is trimmed", arn: "  arn:aws:iam::123456789012:policy/my-workload-boundary  ", wantNil: false, wantVal: "arn:aws:iam::123456789012:policy/my-workload-boundary"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := permissionsBoundaryPtr(tc.arn)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("permissionsBoundaryPtr(%q) = non-nil, want nil (would attach an unintended boundary)", tc.arn)
				}
				return
			}
			if got == nil {
				t.Fatalf("permissionsBoundaryPtr(%q) = nil, want the boundary to be set", tc.arn)
			}
			// Assert the exact value, not just non-nil: a bug returning a wrong or
			// untrimmed ARN would break the exact-match enforcement flip.
			s, ok := got.(sdk.String)
			if !ok {
				t.Fatalf("permissionsBoundaryPtr(%q) is %T, want sdk.String", tc.arn, got)
			}
			if string(s) != tc.wantVal {
				t.Fatalf("permissionsBoundaryPtr(%q) = %q, want %q", tc.arn, string(s), tc.wantVal)
			}
		})
	}
}
