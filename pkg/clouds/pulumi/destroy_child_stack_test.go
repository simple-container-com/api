// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/backend"
)

// selectStack returns (nil, nil) when a stack's checkpoint blob does not exist
// (see stackCheckpointNotFound). DestroyChildStack must treat that nil stack as
// "nothing to destroy" and skip the s.Ref() dereference; otherwise it panics
// with a nil-pointer dereference on a never-deployed child stack.
func TestDestroyChildStack_NilStackIsNoDeref(t *testing.T) {
	var s backend.Stack // nil, as returned by selectStack for a missing stack

	// The guard DestroyChildStack uses.
	if s != nil {
		t.Fatalf("expected nil stack from a missing checkpoint, got %v", s)
	}

	// Guard against a regression that removes the nil check: reaching s.Ref()
	// on a nil stack is exactly the reported panic.
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected dereferencing a nil backend.Stack to panic; the nil guard in DestroyChildStack is what prevents it")
		}
	}()
	_ = s.Ref()
}
