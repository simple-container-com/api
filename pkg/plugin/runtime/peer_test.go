// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// TestRegisterPeerCapability_Lookup — happy path: register a handler,
// dispatch via HandlePluginInvoke, reply bytes round-trip.
func TestRegisterPeerCapability_Lookup(t *testing.T) {
	t.Cleanup(ResetPeerCapabilitiesForTesting)

	called := false
	err := RegisterPeerCapability("mint-scoped-user", func(_ context.Context, caller string, payload []byte) ([]byte, error) {
		called = true
		if string(payload) != `{"database":"forge_app"}` {
			t.Errorf("payload: %s", payload)
		}
		if caller != "sc-component-backup" {
			t.Errorf("caller: %q", caller)
		}
		return []byte(`{"username":"backup-x","password":"pw"}`), nil
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	resp, err := HandlePluginInvoke(context.Background(), &scpluginv1.PluginInvokeRequest{
		Capability:    "mint-scoped-user",
		PayloadJson:   []byte(`{"database":"forge_app"}`),
		CallingPlugin: "sc-component-backup",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !called {
		t.Fatal("handler not invoked")
	}
	if string(resp.GetReplyJson()) != `{"username":"backup-x","password":"pw"}` {
		t.Errorf("reply: %s", resp.GetReplyJson())
	}
}

// TestHandlePluginInvoke_UnregisteredCapability — SC core's broker
// enforces advertisement, but the plugin may have ADVERTISED a
// capability + forgotten to RegisterPeerCapability. HandlePluginInvoke
// surfaces that as a distinct, named error.
func TestHandlePluginInvoke_UnregisteredCapability(t *testing.T) {
	t.Cleanup(ResetPeerCapabilitiesForTesting)

	_, err := HandlePluginInvoke(context.Background(), &scpluginv1.PluginInvokeRequest{
		Capability:  "orphan-capability",
		PayloadJson: []byte(`{}`),
	})
	if err == nil {
		t.Fatal("expected error for unregistered capability")
	}
	if !strings.Contains(err.Error(), "orphan-capability") {
		t.Errorf("error should name the missing capability: %v", err)
	}
	if !strings.Contains(err.Error(), "RegisterPeerCapability") {
		t.Errorf("error should hint at the fix: %v", err)
	}
}

// TestHandlePluginInvoke_EmptyCapabilityRefused — guards a malformed
// request (empty capability). SC core's broker already refuses this
// upstream, but defense-in-depth.
func TestHandlePluginInvoke_EmptyCapabilityRefused(t *testing.T) {
	t.Cleanup(ResetPeerCapabilitiesForTesting)

	_, err := HandlePluginInvoke(context.Background(), &scpluginv1.PluginInvokeRequest{
		Capability: "",
	})
	if err == nil || !strings.Contains(err.Error(), "capability is empty") {
		t.Fatalf("expected empty-capability refusal, got: %v", err)
	}
}

// TestHandlePluginInvoke_HandlerErrorWrapped — the handler can return
// an error (e.g. upstream Atlas REST failed). The dispatcher wraps it
// with the capability name so the calling side can see both legs.
func TestHandlePluginInvoke_HandlerErrorWrapped(t *testing.T) {
	t.Cleanup(ResetPeerCapabilitiesForTesting)

	_ = RegisterPeerCapability("fail-me", func(context.Context, string, []byte) ([]byte, error) {
		return nil, errors.New("atlas REST 500")
	})
	_, err := HandlePluginInvoke(context.Background(), &scpluginv1.PluginInvokeRequest{
		Capability: "fail-me",
	})
	if err == nil {
		t.Fatal("expected handler error to surface")
	}
	if !strings.Contains(err.Error(), "atlas REST 500") {
		t.Errorf("underlying error missing: %v", err)
	}
	if !strings.Contains(err.Error(), "fail-me") {
		t.Errorf("wrapper should name the capability: %v", err)
	}
}

// TestRegisterPeerCapability_Replace — re-registering the same name
// replaces the handler. Supports test-time replacement without an
// explicit "unregister" step.
func TestRegisterPeerCapability_Replace(t *testing.T) {
	t.Cleanup(ResetPeerCapabilitiesForTesting)

	_ = RegisterPeerCapability("replaceable", func(context.Context, string, []byte) ([]byte, error) {
		return []byte("v1"), nil
	})
	_ = RegisterPeerCapability("replaceable", func(context.Context, string, []byte) ([]byte, error) {
		return []byte("v2"), nil
	})
	resp, err := HandlePluginInvoke(context.Background(), &scpluginv1.PluginInvokeRequest{Capability: "replaceable"})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.GetReplyJson()) != "v2" {
		t.Errorf("replace failed: got %s", resp.GetReplyJson())
	}
}

// TestRegisterPeerCapability_EmptyInputs — defensive: empty capability
// or nil handler both refused at registration.
func TestRegisterPeerCapability_EmptyInputs(t *testing.T) {
	t.Cleanup(ResetPeerCapabilitiesForTesting)

	if err := RegisterPeerCapability("", func(context.Context, string, []byte) ([]byte, error) { return nil, nil }); err == nil {
		t.Error("empty capability name should be refused")
	}
	if err := RegisterPeerCapability("x", nil); err == nil {
		t.Error("nil handler should be refused")
	}
}
