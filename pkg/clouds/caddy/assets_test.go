// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package caddy

import (
	"testing"

	. "github.com/onsi/gomega"
)

// TestAssetsEmbedded pins the asset tree the two ingress consumers rely on.
// A `//go:embed assets/*` glob that silently stops matching — a file renamed,
// a directory moved — produces an empty FS, not a build error, and both
// consumers would then ship an ingress with no error pages and no snippet
// library. This test is the thing that notices.
func TestAssetsEmbedded(t *testing.T) {
	RegisterTestingT(t)

	for _, path := range []string{
		CaddyfilePath,
		AssetsDir + "/pages/404.html",
		AssetsDir + "/pages/500.html",
		AssetsDir + "/pages/502.html",
		AssetsDir + "/pages/503.html",
	} {
		content, err := FS.ReadFile(path)
		Expect(err).ToNot(HaveOccurred(), "asset %q missing from the embedded FS", path)
		Expect(content).ToNot(BeEmpty(), "asset %q embedded but empty", path)
	}
}
