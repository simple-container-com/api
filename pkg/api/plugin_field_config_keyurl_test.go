// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"testing"

	. "github.com/onsi/gomega"
)

// A plugin-owned secrets provider with `provision: false` names its key in
// keyName. Login reads it through KeyUrl and fails with "secrets provider key
// url is empty" when it comes back empty, which it did for every such config
// once the typed gcp-kms and aws-kms configs left core.
func TestPluginFieldConfig_KeyUrl(t *testing.T) {
	RegisterTestingT(t)

	byo := &PluginFieldConfig{Raw: map[string]any{"keyName": "gcpkms://projects/p/locations/global/keyRings/r/cryptoKeys/k"}}
	Expect(byo.KeyUrl()).To(Equal("gcpkms://projects/p/locations/global/keyRings/r/cryptoKeys/k"))

	explicit := &PluginFieldConfig{URL: "awskms://alias/x", Raw: map[string]any{"keyName": "ignored"}}
	Expect(explicit.KeyUrl()).To(Equal("awskms://alias/x"), "an explicit url wins")

	Expect((&PluginFieldConfig{}).KeyUrl()).To(BeEmpty())
	Expect((&PluginFieldConfig{Raw: map[string]any{"keyName": 42}}).KeyUrl()).To(BeEmpty(), "a non-string keyName is not a URL")
}
