// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package placeholders

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/template"
)

// Reproduces the forge-conductor failure against api v2026.11.11: a plugin-
// owned state-storage config holds `${auth:aws}` in its credentials field,
// the placeholder resolver walks the stack, and the plugin bus sends the
// serialised config to the sidecar. The failure says the plugin received
// `${auth:aws}` as a literal — i.e. the walker did not resolve
// PluginFieldConfig.Credentials.
//
// This test exercises the resolver against the exact PluginFieldConfig
// shape the plugin bus serialises (`serializeConfigToInputs` in
// pkg/clouds/pulumi). It intentionally stays at the PluginFieldConfig
// level — the full-stack walker drags in provisioner/secrets types that
// are not in this package and are shaped around a different test — to
// isolate whether the walker visits the typed fields and the Raw map
// through the ApplyTemplatesOnObject entry point.
func TestResolve_PluginFieldConfig_CredentialsAndRawResolved(t *testing.T) {
	RegisterTestingT(t)

	cfg := &api.PluginFieldConfig{
		TypeName: "s3-bucket",
		Raw: map[string]any{
			"credentials": "${auth:aws}",
			"bucketName":  "forge-conductor-state",
		},
		Credentials: "${auth:aws}",
	}

	// Stub a minimal auth resolver and call Apply directly on the config,
	// mirroring what Resolve does for each stack. If the walker reaches
	// both the typed Credentials field AND the Raw map, both should
	// contain the resolved blob.
	authBlob := `{"accessKey":"AKIA","secretAccessKey":"S3CR3T","region":"us-east-1"}`
	p := &placeholders{}
	opts := []Option{WithExtensions(map[string]template.Extension{
		"auth": func(noSubs, path string, value *string) (string, error) {
			if path == "aws" {
				return authBlob, nil
			}
			return noSubs, nil
		},
	})}

	Expect(p.Apply(cfg, opts...)).To(Succeed())

	// 1. The typed Credentials field — read by SC-core-side login.go's
	//    `authCfg.CredentialsValue()` — must hold the resolved JSON blob.
	Expect(cfg.Credentials).ToNot(ContainSubstring("${"),
		"PluginFieldConfig.Credentials still contains an unresolved placeholder — forge-conductor regression (run 41680853191)")
	Expect(cfg.Credentials).To(ContainSubstring("accessKey"))

	// 2. The Raw map's credentials key. If the walker misses this, the
	//    plugin's JSON decoder sees `$` and fails with
	//    "invalid character '$' looking for beginning of value".
	Expect(cfg.Raw).To(HaveKey("credentials"))
	Expect(cfg.Raw["credentials"]).ToNot(ContainSubstring("${"),
		"PluginFieldConfig.Raw[credentials] still contains an unresolved placeholder — this is what the plugin bus serializes")

	// 3. JSON round-trip — the shape the plugin bus actually sends.
	//    serializeConfigToInputs lives in pkg/clouds/pulumi; we simulate
	//    its behaviour here by json.Marshal of the config.
	b, err := json.Marshal(cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(b)).ToNot(ContainSubstring("${"),
		"json.Marshal(*PluginFieldConfig) still carries an unresolved placeholder — this is the exact bytes the sidecar sends to the plugin")
	Expect(string(b)).To(ContainSubstring("accessKey"),
		"json.Marshal must include the resolved credentials")
}
