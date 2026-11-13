// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package sbom

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

const testDigestRef = "registry.example/app@sha256:88a3fa17655840cbe33b8a6991098c51519cbacde47490f0abd25060829e2324"

func TestSyftSource(t *testing.T) {
	cases := []struct {
		name    string
		image   string
		inLocal bool
		want    string
	}{
		{"digest held by the daemon is read locally", testDigestRef, true, "docker:" + testDigestRef},
		{"digest the daemon lacks is read from the registry", testDigestRef, false, "registry:" + testDigestRef},
		{"a tag can name different content locally, so it is never read from the daemon", "registry.example/app:v1", true, "registry:registry.example/app:v1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			g := NewSyftGenerator()
			g.localImageExists = func(context.Context, string) bool { return tc.inLocal }
			Expect(g.source(context.Background(), tc.image)).To(Equal(tc.want))
		})
	}
}

func TestSyftSource_NoDockerMeansRegistry(t *testing.T) {
	RegisterTestingT(t)
	t.Setenv("PATH", t.TempDir())
	Expect(NewSyftGenerator().source(context.Background(), testDigestRef)).To(Equal("registry:" + testDigestRef))
}

// Generate must hand syft the source that source() picked: a regression to a
// hardcoded registry: prefix downloads every freshly pushed image again.
func TestGenerate_PassesChosenSourceToSyft(t *testing.T) {
	for _, inLocal := range []bool{true, false} {
		RegisterTestingT(t)
		bin := t.TempDir()
		argsFile := filepath.Join(bin, "args")
		script := "#!/bin/sh\nif [ \"$1\" = version ]; then echo 'Version: 1.0.0'; exit 0; fi\n" +
			"echo \"$@\" > " + argsFile + "\necho '{\"components\":[]}'\n"
		Expect(os.WriteFile(filepath.Join(bin, "syft"), []byte(script), 0o755)).To(Succeed())
		t.Setenv("PATH", bin)

		g := NewSyftGenerator()
		g.localImageExists = func(context.Context, string) bool { return inLocal }
		_, err := g.Generate(context.Background(), testDigestRef, FormatCycloneDXJSON)
		Expect(err).ToNot(HaveOccurred())

		got, err := os.ReadFile(argsFile)
		Expect(err).ToNot(HaveOccurred())
		prefix := "registry:"
		if inLocal {
			prefix = "docker:"
		}
		Expect(strings.Fields(string(got))[0]).To(Equal(prefix + testDigestRef))
	}
}
