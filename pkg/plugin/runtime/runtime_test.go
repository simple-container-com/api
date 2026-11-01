// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package runtime

import (
	"context"
	"testing"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// TestLinknameResolves is a compile-time regression check for the
// `//go:linkname pulumiContextWait → (*Context).wait` in wait.go. If the
// pulumi SDK renames or removes `(*Context).wait` in a future release, this
// test's package fails to link entirely — Go reports an unresolved external
// at test-build time, which is loud and immediate. The body is intentionally
// trivial; the value is in the test binary getting built at all.
func TestLinknameResolves(t *testing.T) {
	t.Parallel()
	// Reference pulumiContextWait so the compiler emits the external symbol
	// into this test binary (otherwise an unused private function could be
	// dead-stripped and the linkname never resolved).
	fn := pulumiContextWait
	if fn == nil {
		t.Fatal("pulumiContextWait resolved to nil — the linkname target has changed")
	}
}

// TestResourceInputForComputeProcess_ParentEnvSwap exercises the
// child-env vs parent-env lookup-key preference that compute processors
// depend on. See forge-storage isolated run 41290064279 for the live-caught
// failure this guards against.
func TestResourceInputForComputeProcess_ParentEnvSwap(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		childEnv   string
		parentEnv  string
		wantEnv    string
		parentNil  bool
		resourceTy string
	}{
		{
			name:       "parent env present wins over child env",
			childEnv:   "108b3f",
			parentEnv:  "staging",
			wantEnv:    "staging",
			resourceTy: "bucket",
		},
		{
			name:       "parent env empty falls back to child env",
			childEnv:   "staging",
			parentEnv:  "",
			wantEnv:    "staging",
			resourceTy: "bucket",
		},
		{
			name:      "nil parent info falls back to child env",
			childEnv:  "staging",
			parentNil: true,
			wantEnv:   "staging",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := &scpluginv1.ComputeProcessRequest{
				Header: &scpluginv1.RequestHeader{
					StackName:   "forge-storage",
					Environment: c.childEnv,
				},
				ResourceType: c.resourceTy,
				ResourceName: "bucket",
			}
			var parent *pApi.ParentInfo
			if !c.parentNil {
				parent = &pApi.ParentInfo{ParentEnv: c.parentEnv}
			}
			got := ResourceInputForComputeProcess(req, parent, nil)
			if got.StackParams.Environment != c.wantEnv {
				t.Fatalf("Environment = %q, want %q", got.StackParams.Environment, c.wantEnv)
			}
			if got.StackParams.StackName != "forge-storage" {
				t.Fatalf("StackName = %q, want forge-storage", got.StackParams.StackName)
			}
			if got.Descriptor.Type != c.resourceTy {
				t.Fatalf("Descriptor.Type = %q, want %q", got.Descriptor.Type, c.resourceTy)
			}
		})
	}
}

// TestAnyToInputValue covers every scalar branch of the Secret-preserving
// harvest path. The companion secret-flag test lives in the plugin smoke
// (an sdk.Output requires a running Pulumi monitor), but the serialisation
// surface is testable here.
func TestAnyToInputValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   any
		kind string // which oneof branch should be set
	}{
		{"string", "hello", "string"},
		{"empty string drops", "", ""},
		{"bool", true, "bool"},
		{"int", 42, "int"},
		{"int64", int64(42), "int"},
		{"float64", 3.14, "double"},
		{"bytes", []byte("raw"), "bytes"},
		{"nil", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := anyToInputValue(c.in)
			if c.kind == "" {
				if got != nil {
					t.Fatalf("expected nil, got %#v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected non-nil, got nil")
			}
			switch c.kind {
			case "string":
				if _, ok := got.Value.(*scpluginv1.InputValue_StringValue); !ok {
					t.Fatalf("want StringValue, got %T", got.Value)
				}
			case "bool":
				if _, ok := got.Value.(*scpluginv1.InputValue_BoolValue); !ok {
					t.Fatalf("want BoolValue, got %T", got.Value)
				}
			case "int":
				if _, ok := got.Value.(*scpluginv1.InputValue_IntValue); !ok {
					t.Fatalf("want IntValue, got %T", got.Value)
				}
			case "double":
				if _, ok := got.Value.(*scpluginv1.InputValue_DoubleValue); !ok {
					t.Fatalf("want DoubleValue, got %T", got.Value)
				}
			case "bytes":
				if _, ok := got.Value.(*scpluginv1.InputValue_BytesValue); !ok {
					t.Fatalf("want BytesValue, got %T", got.Value)
				}
			}
		})
	}
}

// TestNewContext_FactoryHook exercises the test-seam path. Production code
// paths (local engine fallback, sdk.NewContext against a real monitor)
// require live gRPC, so they're covered by plugin smokes rather than here.
func TestNewContext_FactoryHook(t *testing.T) {
	t.Parallel()
	called := false
	rctx, err := NewContext(context.Background(), &scpluginv1.RequestHeader{},
		WithContextFactory(func(_ context.Context, _ *scpluginv1.RequestHeader) (*sdk.Context, error) {
			called = true
			return nil, nil
		}))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if !called {
		t.Fatal("factory hook not invoked")
	}
	if rctx == nil {
		t.Fatal("returned Context is nil")
	}
	// Done() returns the deferred teardown closure even for a nil pctx.
	if rctx.Done() == nil {
		t.Fatal("Done() returned nil")
	}
}

// TestSettle_NilSafety confirms Settle handles the no-op cases without
// panicking. Called with a nil Context (handler took the error path before
// NewContext returned) or a Context whose done is nil (same reason).
func TestSettle_NilSafety(t *testing.T) {
	t.Parallel()
	var resp *scpluginv1.ProvisionResponse
	var err error
	Settle(nil, &resp, &err)
	Settle(&Context{}, &resp, &err)
	if err != nil {
		t.Fatalf("nil-safe Settle set err: %v", err)
	}
}
