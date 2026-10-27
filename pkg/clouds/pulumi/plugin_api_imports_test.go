// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	. "github.com/onsi/gomega"
)

// Every out-of-tree sc-plugin `/api` module required by go.mod must be
// blank-imported by this package's init.go. Those init()s are what call
// api.RegisterProviderConfig (so server.yaml can parse the plugin's type
// tokens) and api.RegisterPluginRequirement (so ValidatePluginRequirements
// can emit the copy-paste-ready `plugins:` block).
//
// This check is DELIBERATELY TEXTUAL rather than a registry lookup. A
// behavioural assertion in a _test.go can pass while the shipped binary is
// broken, because a test binary links test-only packages: pkg/api/tests
// imports sc-plugin-cloudflare/api, so `go list -deps -test` and any
// LookupPluginRequirement("cloudflare") call from a test both succeed even
// when `go list -deps ./cmd/sc` does not contain the module at all. That is
// exactly what happened with the #492 cloudflare drop — the module was a
// direct go.mod require, reachable only through test support, and the
// release pipeline's own `publish sc tool` step could not deploy `dist`.
func TestEveryPluginAPIModuleIsBlankImported(t *testing.T) {
	RegisterTestingT(t)

	root := filepath.Join("..", "..", "..")
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	Expect(err).To(BeNil())
	initGo, err := os.ReadFile(filepath.Join("init.go"))
	Expect(err).To(BeNil())

	// e.g. github.com/simple-container-com/sc-plugin-cloudflare/api v0.0.0-...
	required := regexp.MustCompile(`(?m)^\s+(github\.com/simple-container-com/sc-plugin-[\w-]+/api)\s+v`).
		FindAllStringSubmatch(string(goMod), -1)
	Expect(required).NotTo(BeEmpty(), "expected at least one sc-plugin /api module in go.mod")

	for _, m := range required {
		module := m[1]
		Expect(string(initGo)).To(ContainSubstring(`_ "`+module+`"`),
			"go.mod requires %s but pkg/clouds/pulumi/init.go does not blank-import it; "+
				"its type tokens and plugin requirement will not register in the shipped sc binary",
			module)
	}
}
