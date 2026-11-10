// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"strings"
	"testing"
)

func TestNewComponentConfig_AcceptedInputs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      any
		wantErr string
	}{
		{"nil", nil, "cfg is nil"},
		{"wrong type", "stringly", "unexpected type string"},
		{"nil PluginFieldConfig pointer", (*PluginFieldConfig)(nil), "is nil"},
		{"empty map", map[string]any{}, ""},
		{"valid PluginFieldConfig", &PluginFieldConfig{Raw: map[string]any{"deployTo": "lambda"}}, ""},
		{"wrapping ComponentConfig", &ComponentConfig{raw: &PluginFieldConfig{Raw: map[string]any{}}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewComponentConfig(tt.in)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got == nil {
					t.Fatal("expected non-nil ComponentConfig")
				}
			} else {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestComponentConfig_DeployTo(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		raw     map[string]any
		want    string
		wantErr string
	}{
		{"happy path", map[string]any{"deployTo": "lambda"}, "lambda", ""},
		{"missing", map[string]any{}, "", "deployTo is required"},
		{"wrong type", map[string]any{"deployTo": 42}, "", "want string"},
		{"empty string", map[string]any{"deployTo": ""}, "", "is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewComponentConfig(tt.raw)
			if err != nil {
				t.Fatalf("NewComponentConfig: %v", err)
			}
			got, err := c.DeployTo()
			assertStringResult(t, got, err, tt.want, tt.wantErr)
		})
	}
}

func TestComponentConfig_Image(t *testing.T) {
	t.Parallel()
	c, err := NewComponentConfig(map[string]any{"image": "simplecontainer/sc-component-dummy:v1"})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	got, err := c.Image()
	if err != nil {
		t.Fatalf("Image: %v", err)
	}
	if got != "simplecontainer/sc-component-dummy:v1" {
		t.Fatalf("Image = %q", got)
	}
}

func TestComponentConfig_SigningKeyRef(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		raw     map[string]any
		want    string
		wantErr string
	}{
		{"absent is ok", map[string]any{}, "", ""},
		{"resolved secret value is accepted (SC resolves before validation)", map[string]any{"signingKey": "a-resolved-base64-string"}, "a-resolved-base64-string", ""},
		{"unresolved secret ref is accepted (test harness bypasses resolution)", map[string]any{"signingKey": "${secret:dummy-key}"}, "${secret:dummy-key}", ""},
		{"wrong type rejected", map[string]any{"signingKey": 123}, "", "want string"},
		{"empty string rejected", map[string]any{"signingKey": ""}, "", "empty string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewComponentConfig(tt.raw)
			if err != nil {
				t.Fatalf("new: %v", err)
			}
			got, err := c.SigningKeyRef()
			assertStringResult(t, got, err, tt.want, tt.wantErr)
		})
	}
}

func TestComponentConfig_ServiceEnv(t *testing.T) {
	t.Parallel()
	t.Run("absent", func(t *testing.T) {
		c, _ := NewComponentConfig(map[string]any{})
		m, err := c.ServiceEnv()
		if err != nil {
			t.Fatalf("ServiceEnv: %v", err)
		}
		if len(m) != 0 {
			t.Fatalf("expected empty map, got %#v", m)
		}
	})
	t.Run("happy path", func(t *testing.T) {
		c, _ := NewComponentConfig(map[string]any{"env": map[string]any{"A": "1", "B": "2"}})
		m, err := c.ServiceEnv()
		if err != nil {
			t.Fatalf("ServiceEnv: %v", err)
		}
		if m["A"] != "1" || m["B"] != "2" {
			t.Fatalf("env = %#v", m)
		}
	})
	t.Run("non-string value rejected", func(t *testing.T) {
		c, _ := NewComponentConfig(map[string]any{"env": map[string]any{"A": 42}})
		_, err := c.ServiceEnv()
		if err == nil || !strings.Contains(err.Error(), "want string") {
			t.Fatalf("expected shape error, got %v", err)
		}
	})
	t.Run("env wrong shape rejected", func(t *testing.T) {
		c, _ := NewComponentConfig(map[string]any{"env": "flat"})
		_, err := c.ServiceEnv()
		if err == nil || !strings.Contains(err.Error(), "want map") {
			t.Fatalf("expected shape error, got %v", err)
		}
	})
}

func TestComponentConfig_Ingress(t *testing.T) {
	t.Parallel()
	t.Run("absent returns empty", func(t *testing.T) {
		c, _ := NewComponentConfig(map[string]any{})
		h, err := c.IngressHostname()
		if err != nil || h != "" {
			t.Fatalf("hostname=%q err=%v", h, err)
		}
		r, err := c.IngressRegistrar()
		if err != nil || r != "" {
			t.Fatalf("registrar=%q err=%v", r, err)
		}
	})
	t.Run("populated", func(t *testing.T) {
		c, _ := NewComponentConfig(map[string]any{
			"ingress": map[string]any{
				"hostname":  "dummy.example.com",
				"registrar": "cloudflare",
			},
		})
		h, _ := c.IngressHostname()
		r, _ := c.IngressRegistrar()
		if h != "dummy.example.com" || r != "cloudflare" {
			t.Fatalf("hostname=%q registrar=%q", h, r)
		}
	})
	t.Run("wrong shape rejected", func(t *testing.T) {
		c, _ := NewComponentConfig(map[string]any{"ingress": "oops"})
		_, err := c.IngressHostname()
		if err == nil || !strings.Contains(err.Error(), "want map") {
			t.Fatalf("expected shape error, got %v", err)
		}
	})
}

func TestComponentConfig_RawCopy(t *testing.T) {
	t.Parallel()
	c, _ := NewComponentConfig(map[string]any{"deployTo": "lambda", "image": "x:1"})
	cp := c.RawCopy()
	cp["deployTo"] = "mutated"
	again, _ := c.DeployTo()
	if again != "lambda" {
		t.Fatalf("RawCopy mutation leaked back into source; got %q", again)
	}
}

func TestIsComponentConfigTopLevelKey(t *testing.T) {
	t.Parallel()
	for _, k := range ComponentConfigTopLevelKeys {
		if !IsComponentConfigTopLevelKey(k) {
			t.Fatalf("expected key %q to be well-known", k)
		}
	}
	if IsComponentConfigTopLevelKey("nonexistent") {
		t.Fatal("expected 'nonexistent' to be unknown")
	}
}

func assertStringResult(t *testing.T, got string, err error, want, wantErr string) {
	t.Helper()
	if wantErr == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		return
	}
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", wantErr)
	}
	if !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("error %q does not contain %q", err.Error(), wantErr)
	}
}
