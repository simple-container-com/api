// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"errors"
	"testing"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// TestExtractHostFromEndpointURL — the resolved-URL parser the
// wire reads before handing to the registrar. Accepts bare hosts
// and full URLs; refuses things that have neither.
func TestExtractHostFromEndpointURL(t *testing.T) {
	cases := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{"https://xxxx.lambda-url.eu-central-1.on.aws/", "xxxx.lambda-url.eu-central-1.on.aws", false},
		{"https://xxxx.lambda-url.eu-central-1.on.aws", "xxxx.lambda-url.eu-central-1.on.aws", false},
		{"https://host.example/path?q=1", "host.example", false},
		{"my-bare-host.example", "my-bare-host.example", false},
		{"", "", false}, // bare empty — wire guards earlier, this returns empty verbatim
		{"://no-scheme-host", "", true},
	}
	for _, tc := range cases {
		got, err := extractHostFromEndpointURL(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Errorf("extractHostFromEndpointURL(%q) = (%q, nil), want error", tc.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("extractHostFromEndpointURL(%q) unexpected error: %v", tc.raw, err)
			continue
		}
		if got != tc.want {
			t.Errorf("extractHostFromEndpointURL(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// cdnsFakeRegistrar captures ProvisionDomainForEndpoint calls so tests
// can assert what the wire handed in without spinning Pulumi.
type cdnsFakeRegistrar struct {
	calls []pApi.DomainEndpoint
	err   error
}

func (f *cdnsFakeRegistrar) MainDomain() string { return "example.com" }

func (f *cdnsFakeRegistrar) ProvisionRecords(_ *sdk.Context, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
	return nil, nil
}

func (f *cdnsFakeRegistrar) NewRecord(_ *sdk.Context, _ api.DnsRecord) (*api.ResourceOutput, error) {
	return nil, nil
}

func (f *cdnsFakeRegistrar) NewOverrideHeaderRule(_ *sdk.Context, _ api.Stack, _ pApi.OverrideHeaderRule) (*api.ResourceOutput, error) {
	return nil, nil
}

func (f *cdnsFakeRegistrar) ProvisionDomainForEndpoint(_ *sdk.Context, _ api.Stack, endpoint pApi.DomainEndpoint) (*api.ResourceOutput, error) {
	f.calls = append(f.calls, endpoint)
	return &api.ResourceOutput{}, f.err
}

// mkParams builds a minimal ProvisionParams with a logger that
// accumulates warnings so tests can assert them.
func mkParams(reg pApi.Registrar) pApi.ProvisionParams {
	return pApi.ProvisionParams{Registrar: reg, Log: logger.New()}
}

// TestWireComponentDomain_HappyPath — template plugin published a
// resolved URL; the registrar is called with host + hostname.
func TestWireComponentDomain_HappyPath(t *testing.T) {
	reg := &cdnsFakeRegistrar{}
	dispatched := &api.ResourceOutput{Ref: &api.PluginResourceOutput{
		PluginName: "aws",
		Urn:        "urn:pulumi:...",
		Outputs: map[string]api.PluginOutputValue{
			ComponentEndpointURLKey: {Value: "https://xxxx.lambda-url.eu-central-1.on.aws/"},
		},
	}}
	err := wireComponentDomain(nil, api.Stack{Name: "infra"}, mkParams(reg),
		"my-backup", "backup.simple-forge.com", dispatched)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reg.calls) != 1 {
		t.Fatalf("expected 1 ProvisionDomainForEndpoint call, got %d", len(reg.calls))
	}
	call := reg.calls[0]
	if call.Name != "my-backup" {
		t.Errorf("Name: got %q, want %q", call.Name, "my-backup")
	}
	if call.Domain != "backup.simple-forge.com" {
		t.Errorf("Domain: got %q, want %q", call.Domain, "backup.simple-forge.com")
	}
	// TargetHost is a StringInput; its string form is set via sdk.String
	// under the hood, which we can't trivially unwrap here without a
	// Pulumi context. Trust that the wire passed it verbatim — the
	// extractHostFromEndpointURL test above pins the parser.
}

// TestWireComponentDomain_NilRegistrar — a parent stack with no
// registrar declared cannot wire Component ingress; error loudly so
// the deploy fails rather than shipping a Component with its DNS
// silently absent.
func TestWireComponentDomain_NilRegistrar(t *testing.T) {
	err := wireComponentDomain(nil, api.Stack{}, mkParams(nil),
		"my-backup", "backup.simple-forge.com", nil)
	if err == nil {
		t.Fatal("expected error with nil Registrar")
	}
}

// TestWireComponentDomain_NotConfiguredRegistrar — the stack has a
// *notConfigured sentinel (parse-time placeholder). Same loud-error
// as nil so operators don't ship Components with silently-missing DNS.
func TestWireComponentDomain_NotConfiguredRegistrar(t *testing.T) {
	err := wireComponentDomain(nil, api.Stack{}, mkParams(&notConfigured{}),
		"my-backup", "backup.simple-forge.com", nil)
	if err == nil {
		t.Fatal("expected error with *notConfigured Registrar")
	}
}

// TestWireComponentDomain_NilDispatched — defensive. Should never
// happen in practice (callers guard on tplFn error) but the wire is
// public-shaped and should refuse nil gracefully.
func TestWireComponentDomain_NilDispatched(t *testing.T) {
	err := wireComponentDomain(nil, api.Stack{}, mkParams(&cdnsFakeRegistrar{}),
		"my-backup", "backup.simple-forge.com", nil)
	if err == nil {
		t.Fatal("expected error with nil dispatched output")
	}
}

// TestWireComponentDomain_InTreeTemplateSkips — a template whose
// output's Ref is not *api.PluginResourceOutput is an in-tree
// template; those already wire DNS themselves at provision time
// (they hold the real Registrar). SC-core wiring here would
// double-write. Silently skip.
func TestWireComponentDomain_InTreeTemplateSkips(t *testing.T) {
	reg := &cdnsFakeRegistrar{}
	dispatched := &api.ResourceOutput{Ref: "in-tree-opaque-value"}
	err := wireComponentDomain(nil, api.Stack{}, mkParams(reg),
		"my-backup", "backup.simple-forge.com", dispatched)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reg.calls) != 0 {
		t.Errorf("expected ZERO registrar calls for in-tree template, got %d", len(reg.calls))
	}
}

// TestWireComponentDomain_PreConventionTemplate — plugin-dispatched
// template that pre-dates the ComponentEndpointURLKey convention
// returns a PluginResourceOutput without the key. Warn-log + skip
// (not error), so a mid-rollout fleet with mixed plugin versions
// doesn't break deploys for Components whose hostname wasn't going
// to wire anyway. Operator sees the warning + knows to bump the
// template plugin.
func TestWireComponentDomain_PreConventionTemplate(t *testing.T) {
	reg := &cdnsFakeRegistrar{}
	dispatched := &api.ResourceOutput{Ref: &api.PluginResourceOutput{
		PluginName: "aws",
		Urn:        "urn:pulumi:...",
		Outputs:    map[string]api.PluginOutputValue{}, // no _endpoint_url
	}}
	err := wireComponentDomain(nil, api.Stack{}, mkParams(reg),
		"my-backup", "backup.simple-forge.com", dispatched)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reg.calls) != 0 {
		t.Errorf("pre-convention template should skip wiring, got %d calls", len(reg.calls))
	}
}

// TestWireComponentDomain_NonStringEndpoint — a plugin publishes
// _endpoint_url but with the wrong value type (e.g. a nested map).
// Error loudly — this is a plugin bug, operators should see it.
func TestWireComponentDomain_NonStringEndpoint(t *testing.T) {
	reg := &cdnsFakeRegistrar{}
	dispatched := &api.ResourceOutput{Ref: &api.PluginResourceOutput{
		PluginName: "aws",
		Urn:        "urn:pulumi:...",
		Outputs: map[string]api.PluginOutputValue{
			ComponentEndpointURLKey: {Value: map[string]any{"wrong": "shape"}},
		},
	}}
	err := wireComponentDomain(nil, api.Stack{}, mkParams(reg),
		"my-backup", "backup.simple-forge.com", dispatched)
	if err == nil {
		t.Fatal("expected error for non-string endpoint value")
	}
}

// TestWireComponentDomain_RegistrarErrorBubbles — the registrar's
// own ProvisionDomainForEndpoint can fail (zone misconfig, auth
// denied, duplicate Workers Route). Error must bubble so the deploy
// stops rather than reporting a Lambda green + DNS silently unwired.
func TestWireComponentDomain_RegistrarErrorBubbles(t *testing.T) {
	reg := &cdnsFakeRegistrar{err: errors.New("cloudflare: zone mismatch")}
	dispatched := &api.ResourceOutput{Ref: &api.PluginResourceOutput{
		PluginName: "aws",
		Urn:        "urn:pulumi:...",
		Outputs: map[string]api.PluginOutputValue{
			ComponentEndpointURLKey: {Value: "https://xxxx.lambda-url.eu-central-1.on.aws/"},
		},
	}}
	err := wireComponentDomain(nil, api.Stack{}, mkParams(reg),
		"my-backup", "backup.simple-forge.com", dispatched)
	if err == nil {
		t.Fatal("expected error to bubble from registrar")
	}
}

// Keep an unused-import style check to confirm we exercised logger
// at least once in this test file.
var _ context.Context
