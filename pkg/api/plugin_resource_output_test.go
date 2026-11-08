// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"strings"
	"testing"
)

func pluginOut() *PluginResourceOutput {
	return &PluginResourceOutput{
		PluginName: "gcp",
		Urn:        "urn:pulumi:staging::forge::gcp:storage/bucket:Bucket::acme-store",
		Outputs: map[string]PluginOutputValue{
			"acme-store-bucket-name":        {Value: "acme-store-staging"},
			"acme-store-bucket-location":    {Value: "US-CENTRAL1"},
			"acme-store-hmac-access-key-id": {Value: "GOOG1EEXAMPLE"},
			"acme-store-hmac-secret-key":    {Value: "s3cr3t", Secret: true},
			"acme-store-versioned":          {Value: true},
		},
	}
}

// The old placeholder was the string `plugin:<name>:<urn>`. Anything that logged or
// formatted it must read identically now that .Ref holds a struct — and the values
// must stay out of that rendering, because one of them is secret.
func TestPluginResourceOutput_StringRendersTheOldPlaceholder(t *testing.T) {
	got := pluginOut().String()
	want := "plugin:gcp:urn:pulumi:staging::forge::gcp:storage/bucket:Bucket::acme-store"
	if got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if strings.Contains(got, "s3cr3t") {
		t.Errorf("String() leaked a secret output value: %q", got)
	}
}

// The secret flag is the whole reason PluginOutputValue is a struct rather than an
// `any`: a consumer rendering the HMAC secret into a Caddyfile has to wrap it in
// sdk.ToSecret, and it can only know to do that if the flag survived the harvest.
func TestPluginResourceOutput_SecretFlagSurvives(t *testing.T) {
	out := pluginOut()
	secret, found := out.Lookup("acme-store-hmac-secret-key")
	if !found {
		t.Fatal("hmac secret key not published")
	}
	if !secret.Secret {
		t.Error("hmac secret key lost its secret flag")
	}
	plain, found := out.Lookup("acme-store-hmac-access-key-id")
	if !found {
		t.Fatal("hmac access key id not published")
	}
	if plain.Secret {
		t.Error("hmac access key id was marked secret, which it is not")
	}
	if _, found := out.Lookup("acme-store-endpoint"); found {
		t.Error("Lookup reported a key the plugin never published")
	}
}

func TestPluginResourceOutput_StringOf(t *testing.T) {
	out := pluginOut()

	got, err := out.StringOf("acme-store-bucket-name")
	if err != nil {
		t.Fatalf("StringOf(bucket-name): %v", err)
	}
	if got != "acme-store-staging" {
		t.Errorf("StringOf(bucket-name) = %q", got)
	}

	// The realistic failure is a plugin version that predates the key, so the
	// error has to say what IS available — otherwise there is no way to find out.
	_, err = out.StringOf("acme-store-endpoint")
	if err == nil {
		t.Fatal("StringOf on an unpublished key returned no error")
	}
	for _, want := range []string{"acme-store-endpoint", "acme-store-bucket-name", "plugin:gcp:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}

	// A non-string value is a different mistake and gets a different message.
	_, err = out.StringOf("acme-store-versioned")
	if err == nil {
		t.Fatal("StringOf on a bool output returned no error")
	}
	if !strings.Contains(err.Error(), "not a string") {
		t.Errorf("error %q does not say the value is not a string", err)
	}
}

func TestPluginOutputsOf(t *testing.T) {
	want := pluginOut()
	outs := map[string]*ResourceOutput{
		"acme-store": {Ref: want},
		"nil-output": nil,
		"in-tree":    {Ref: "some in-tree typed output"},
	}

	got, err := PluginOutputsOf(outs, "acme-store")
	if err != nil {
		t.Fatalf("PluginOutputsOf(acme-store): %v", err)
	}
	if got != want {
		t.Errorf("PluginOutputsOf returned %v, want the stored output", got)
	}

	// Undeclared dependency and nil entry are the same operator mistake, and the
	// error names the fix (DependsOnResources) rather than reporting an absence.
	for _, name := range []string{"never-declared", "nil-output"} {
		_, err := PluginOutputsOf(outs, name)
		if err == nil {
			t.Fatalf("PluginOutputsOf(%s) returned no error", name)
		}
		if !strings.Contains(err.Error(), "DependsOnResources") {
			t.Errorf("error for %s does not name DependsOnResources: %v", name, err)
		}
	}

	// An in-tree resource puts its own typed output on .Ref; asking it for plugin
	// outputs is a programming error and must not be reported as "no outputs".
	_, err = PluginOutputsOf(outs, "in-tree")
	if err == nil {
		t.Fatal("PluginOutputsOf on an in-tree output returned no error")
	}
	if !strings.Contains(err.Error(), "not provisioned by a plugin") {
		t.Errorf("error %q does not say the resource is not plugin-owned", err)
	}
}

// A nil receiver is reachable: PluginOutputsOf's own error path returns one, and a
// caller that logs the error and carries on with the value would otherwise panic.
func TestPluginResourceOutput_NilIsSafe(t *testing.T) {
	var out *PluginResourceOutput
	if _, found := out.Lookup("anything"); found {
		t.Error("Lookup on a nil output reported a hit")
	}
	if _, err := out.StringOf("anything"); err == nil {
		t.Error("StringOf on a nil output returned no error")
	}
	if out.String() == "" {
		t.Error("String() on a nil output rendered nothing")
	}
}
