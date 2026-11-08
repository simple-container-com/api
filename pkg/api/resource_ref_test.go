// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import "testing"

// The parser's three answers are deliberately distinct — not a reference, a
// valid reference, a malformed reference — because collapsing the third into
// either of the others is how a typo becomes a credential. `${resource:oops}`
// read as "not a reference" would be handed to a Kubernetes provider as the
// kubeconfig.
func TestParseResourceRef(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      string
		wantRef ResourceRef
		isRef   bool
		wantErr bool
	}{
		{
			name:    "the motivating case",
			in:      "${resource:my-k3s.kubeconfig}",
			wantRef: ResourceRef{ResourceName: "my-k3s", Key: "kubeconfig"},
			isRef:   true,
		},
		{
			name:    "surrounding whitespace is a yaml artefact, not intent",
			in:      "  ${resource:my-k3s.kubeconfig}\n",
			wantRef: ResourceRef{ResourceName: "my-k3s", Key: "kubeconfig"},
			isRef:   true,
		},
		{
			// The first dot is the boundary: a resource name cannot contain one
			// (it is a server.yaml map key) but an output key might.
			name:    "a dotted output key stays whole",
			in:      "${resource:store.hmac.access-key}",
			wantRef: ResourceRef{ResourceName: "store", Key: "hmac.access-key"},
			isRef:   true,
		},
		{
			name:  "a literal kubeconfig is not a reference",
			in:    "apiVersion: v1\nclusters: []\n",
			isRef: false,
		},
		{
			// Resolved long before the provisioning tier; it must pass through
			// untouched, not be mistaken for a sibling reference.
			name:  "a secret placeholder is not a resource reference",
			in:    "${secret:K3S_KUBECONFIG}",
			isRef: false,
		},
		{
			name:  "empty is not a reference",
			in:    "",
			isRef: false,
		},
		{
			name:  "an unterminated reference is not one",
			in:    "${resource:my-k3s.kubeconfig",
			isRef: false,
		},
		{
			name:    "no output named",
			in:      "${resource:my-k3s}",
			isRef:   true,
			wantErr: true,
		},
		{
			name:    "no resource named",
			in:      "${resource:.kubeconfig}",
			isRef:   true,
			wantErr: true,
		},
		{
			name:    "no output after the dot",
			in:      "${resource:my-k3s.}",
			isRef:   true,
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, isRef, err := ParseResourceRef(tc.in)
			if isRef != tc.isRef {
				t.Fatalf("ParseResourceRef(%q) isRef=%v, want %v", tc.in, isRef, tc.isRef)
			}
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseResourceRef(%q) returned no error", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseResourceRef(%q): %v", tc.in, err)
			}
			if got != tc.wantRef {
				t.Errorf("ParseResourceRef(%q) = %+v, want %+v", tc.in, got, tc.wantRef)
			}
		})
	}
}

// String has to round-trip, because every error message about a reference quotes
// it back and an operator matches that against their server.yaml by eye.
func TestResourceRef_StringRoundTrips(t *testing.T) {
	want := ResourceRef{ResourceName: "my-k3s", Key: "kubeconfig"}
	got, isRef, err := ParseResourceRef(want.String())
	if err != nil || !isRef {
		t.Fatalf("ParseResourceRef(%q) = isRef %v, err %v", want.String(), isRef, err)
	}
	if got != want {
		t.Errorf("round trip gave %+v, want %+v", got, want)
	}
}
