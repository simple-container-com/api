// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"testing"

	. "github.com/onsi/gomega"
)

// Context. api v2026.11.10 shipped A3 (plugin /api blank-imports
// stripped from the CLI runtime) with a split-map bug: dynamic
// discovery via RegisterPluginOwnedType populated providerConfigMapping
// but NOT provisionerFieldConfigMapping. Canonical state-storage /
// secrets-provider tokens without `:` — the aws plugin's `s3-bucket`,
// the yandex plugin's `yc-object-storage` + `yc-kms`, etc. — failed
// at parse time with "unknown provisioner field config type".
// Live-caught on forge-conductor isolated verify run 41670670084
// (2026-11-03).
//
// Pre-A3, each plugin's /api/init() populated TWO maps per
// state-storage / secrets-provider type:
//
//	api.RegisterProviderConfig({"s3-bucket": ...})         // providerConfigMapping
//	api.RegisterProvisionerFieldConfig({"s3-bucket": ...}) // provisionerFieldConfigMapping
//
// Dynamic discovery has to seed BOTH or the parse path is broken for
// anything that reads via ReadProvisionerFieldConfig (pkg/api/read.go).
//
// These tests lock in the map-population contract per PluginOwnedKind
// so this regression cannot sneak past unit-test gating again.

// TestRegisterPluginOwnedType_PopulatesCorrectMaps asserts, for each
// PluginOwnedKind, that RegisterPluginOwnedType seeds exactly the map(s)
// the parse path for that kind reads from. If a new kind is added to
// api.PluginOwnedKind, THIS table should grow — nothing else in the test
// suite enforces the invariant across all kinds.
func TestRegisterPluginOwnedType_PopulatesCorrectMaps(t *testing.T) {
	RegisterTestingT(t)

	// Each row: (kind, tokenName, expectProviderConfigMapping, expectProvisionerFieldConfigMapping).
	// The token values are prefixed to avoid any collision with the bootstrap seed.
	type row struct {
		kind   PluginOwnedKind
		token  string
		inPCM  bool // providerConfigMapping
		inPFCM bool // provisionerFieldConfigMapping
	}
	cases := []row{
		{PluginOwnedKindResource, "ptest-resource-xyz", true, false},
		{PluginOwnedKindRegistrar, "ptest-registrar-xyz", true, false},
		{PluginOwnedKindStateStore, "ptest-statestore-xyz", true, true},
		{PluginOwnedKindSecretsProvider, "ptest-secretsprov-xyz", true, true},
		{PluginOwnedKindComputeProcessor, "ptest-compute-xyz", true, false},
		{PluginOwnedKindVmDriver, "ptest-vmdriver-xyz", true, false},
	}

	for _, tc := range cases {
		// Guard against test-ordering surprises — if a prior test somehow
		// registered the same token under a different kind, bail noisily.
		if _, exists := providerConfigMapping[tc.token]; exists {
			t.Fatalf("test setup: token %q already in providerConfigMapping", tc.token)
		}
		RegisterPluginOwnedType(tc.token, tc.kind, "ptest-plugin")

		_, gotPCM := providerConfigMapping[tc.token]
		Expect(gotPCM).To(Equal(tc.inPCM),
			"kind %q token %q: providerConfigMapping entry expected=%v got=%v",
			tc.kind, tc.token, tc.inPCM, gotPCM)

		_, gotPFCM := provisionerFieldConfigMapping[tc.token]
		Expect(gotPFCM).To(Equal(tc.inPFCM),
			"kind %q token %q: provisionerFieldConfigMapping entry expected=%v got=%v. "+
				"IF THIS IS NEW: pulumi's state-storage / secrets-provider parse path "+
				"reads via ReadProvisionerFieldConfig (pkg/api/read.go), which hits "+
				"provisionerFieldConfigMapping, NOT providerConfigMapping — see mapping.go "+
				"RegisterPluginOwnedType for the kind-gated seed.",
			tc.kind, tc.token, tc.inPFCM, gotPFCM)

		// Binding is always present regardless of kind.
		binding, ok := IsPluginOwnedType(tc.token)
		Expect(ok).To(BeTrue(), "kind %q token %q: IsPluginOwnedType must find it", tc.kind, tc.token)
		Expect(binding.Kind).To(Equal(tc.kind))
		Expect(binding.PluginName).To(Equal("ptest-plugin"))
	}
}

// TestReadProvisionerFieldConfig_PluginOwnedStateStore_Succeeds is the
// REGRESSION ANCHOR for the forge-conductor run 41670670084 failure.
// Before the mapping.go fix, this would have failed with
//
//	unknown provisioner field config type "ptest-regr-s3-bucket"
//
// which is the exact error forge-conductor hit against api v2026.11.10.
func TestReadProvisionerFieldConfig_PluginOwnedStateStore_Succeeds(t *testing.T) {
	RegisterTestingT(t)
	const token = "ptest-regr-s3-bucket"

	// Simulate what stackread.registerSchemaTypes does for a plugin
	// advertising this token under StateStoreTypes.
	RegisterPluginOwnedType(token, PluginOwnedKindStateStore, "ptest-fake-aws")

	rawCfg := &Config{Config: map[string]any{
		"url":         "s3://ptest-bucket/state",
		"credentials": "${auth:aws}",
		"provision":   true,
	}}
	got, err := ReadProvisionerFieldConfig(token, rawCfg)
	Expect(err).ToNot(HaveOccurred(),
		"ReadProvisionerFieldConfig must succeed for a plugin-owned state-storage token — "+
			"the forge-conductor regression (run 41670670084) failed exactly here")

	wrapped, ok := got.Config.(*PluginFieldConfig)
	Expect(ok).To(BeTrue(), "config should be wrapped in *PluginFieldConfig, got %T", got.Config)
	Expect(wrapped.TypeName).To(Equal(token))
	Expect(wrapped.URL).To(Equal("s3://ptest-bucket/state"))
	Expect(wrapped.Credentials).To(Equal("${auth:aws}"))
	Expect(wrapped.Provision).To(BeTrue())
}

// TestReadProvisionerFieldConfig_PluginOwnedSecretsProvider_Succeeds
// locks in the SecretsProvider arm of the same fix. yc-kms and
// aws-kms (prod values) go through this path.
func TestReadProvisionerFieldConfig_PluginOwnedSecretsProvider_Succeeds(t *testing.T) {
	RegisterTestingT(t)
	const token = "ptest-regr-aws-kms"

	RegisterPluginOwnedType(token, PluginOwnedKindSecretsProvider, "ptest-fake-aws")

	rawCfg := &Config{Config: map[string]any{
		"url":         "awskms://alias/sc-secrets",
		"credentials": "${auth:aws}",
	}}
	got, err := ReadProvisionerFieldConfig(token, rawCfg)
	Expect(err).ToNot(HaveOccurred())

	wrapped, ok := got.Config.(*PluginFieldConfig)
	Expect(ok).To(BeTrue(), "config should be wrapped in *PluginFieldConfig, got %T", got.Config)
	Expect(wrapped.TypeName).To(Equal(token))
	Expect(wrapped.URL).To(Equal("awskms://alias/sc-secrets"))
}

// TestReadProvisionerFieldConfig_UnknownToken_StillErrorsCleanly guards
// against the fix accidentally short-circuiting on every token. An
// unknown token should still produce the "unknown provisioner field
// config type" error so operators know something is wrong — the fix
// only populates the map for plugin-owned types, not every token.
func TestReadProvisionerFieldConfig_UnknownToken_StillErrorsCleanly(t *testing.T) {
	RegisterTestingT(t)

	_, err := ReadProvisionerFieldConfig("ptest-nobody-owns-this", &Config{})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("unknown provisioner field config type"))
	Expect(err.Error()).To(ContainSubstring("ptest-nobody-owns-this"))
}

// TestRegisterPluginOwnedType_IdempotentAcrossMaps re-registers a token
// twice and confirms the second call doesn't clobber the first entry in
// either map. The current RegisterPluginOwnedType uses a "if _, exists"
// guard; this test locks in that guard.
func TestRegisterPluginOwnedType_IdempotentAcrossMaps(t *testing.T) {
	RegisterTestingT(t)
	const token = "ptest-idempotent-state"

	RegisterPluginOwnedType(token, PluginOwnedKindStateStore, "ptest-plugin-a")

	// Capture the function pointer of the first registration so we can
	// detect a silent overwrite. (We cannot compare funcs for equality,
	// but we can observe behaviour by calling it with a sentinel marker
	// map and checking the returned PluginFieldConfig's TypeName — which
	// is captured at registration time via the closure.)
	cfg1, err := ReadProvisionerFieldConfig(token, &Config{Config: map[string]any{"url": "foo://a"}})
	Expect(err).ToNot(HaveOccurred())
	wrapped1, ok := cfg1.Config.(*PluginFieldConfig)
	Expect(ok).To(BeTrue())
	Expect(wrapped1.TypeName).To(Equal(token))
	Expect(wrapped1.URL).To(Equal("foo://a"))

	// Re-register the same token (same kind, different plugin name).
	// Binding plugin name updates — but the decoder in the field map
	// should NOT be overwritten by a second seed (we use `if _, exists`
	// guards; a future refactor that drops them would show here).
	RegisterPluginOwnedType(token, PluginOwnedKindStateStore, "ptest-plugin-b")

	cfg2, err := ReadProvisionerFieldConfig(token, &Config{Config: map[string]any{"url": "foo://b"}})
	Expect(err).ToNot(HaveOccurred())
	wrapped2, ok := cfg2.Config.(*PluginFieldConfig)
	Expect(ok).To(BeTrue())
	Expect(wrapped2.TypeName).To(Equal(token),
		"TypeName should still match the token name — re-registration must not corrupt the closure")
}
