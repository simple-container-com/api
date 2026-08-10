// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package aws

import "testing"

// permissionsBoundaryPtr's contract is the backward-compat guarantee: an empty
// or whitespace-only ARN must yield nil (no boundary set on the role), so every
// stack that does not opt in is unchanged. A real ARN must yield non-nil.
func TestPermissionsBoundaryPtr(t *testing.T) {
	tests := []struct {
		name    string
		arn     string
		wantNil bool
	}{
		{name: "empty means no boundary", arn: "", wantNil: true},
		{name: "whitespace means no boundary", arn: "   ", wantNil: true},
		{name: "arn sets a boundary", arn: "arn:aws:iam::123456789012:policy/my-workload-boundary", wantNil: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := permissionsBoundaryPtr(tc.arn)
			if tc.wantNil && got != nil {
				t.Fatalf("permissionsBoundaryPtr(%q) = non-nil, want nil (would attach an unintended boundary)", tc.arn)
			}
			if !tc.wantNil && got == nil {
				t.Fatalf("permissionsBoundaryPtr(%q) = nil, want the boundary to be set", tc.arn)
			}
		})
	}
}
