// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"strings"
	"testing"

	"github.com/pkg/errors"
)

func TestParseAuthDescriptor(t *testing.T) {
	registerTestProviders()
	RegisterProviderConfig(ConfigRegisterMap{
		"test-prov-api-fails": func(c *Config) (Config, error) { return Config{}, errors.New("bad config") },
	})
	t.Cleanup(func() { delete(providerConfigMapping, "test-prov-api-fails") })

	got, err := ParseAuthDescriptor("type: " + testProviderType + "\nconfig:\n  projectId: p1\n")
	if err != nil {
		t.Fatalf("valid entry: %v", err)
	}
	if got.Type != testProviderType {
		t.Errorf("Type = %q; want %q", got.Type, testProviderType)
	}
	if m, ok := got.Config.Config.(map[string]any); !ok || m["projectId"] != "p1" {
		t.Errorf("Config = %#v; want the entry's config converted by the provider", got.Config.Config)
	}

	for _, c := range []struct{ name, text, want string }{
		{"not yaml", "type: [unclosed", "not valid YAML"},
		{"inherits", "inherit: parent\n", "cannot inherit"},
		{"no type", "config:\n  projectId: p1\n", "no type"},
		{"unknown type", "type: nobody-registered-this\n", "unknown auth type"},
		{"provider refuses", "type: test-prov-api-fails\n", "bad config"},
	} {
		if _, err := ParseAuthDescriptor(c.text); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v; want it to mention %q", c.name, err, c.want)
		}
	}
}
