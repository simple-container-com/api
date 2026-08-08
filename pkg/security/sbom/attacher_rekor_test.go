// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package sbom

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/signing"
)

// installFakeCosign puts a `cosign` stub first on PATH that fails the first
// failures invocations with a Rekor 409 entry conflict, then succeeds. It counts
// invocations in a file so the count survives across separate process
// executions, and returns that path so the test can assert the attempt count.
func installFakeCosign(t *testing.T, failures int) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-binary PATH harness is POSIX-shell only")
	}
	dir := t.TempDir()
	counter := filepath.Join(dir, "calls")

	conflict := `Error: signing bundle: error signing bundle: [POST /api/v1/log/entries][409] ` +
		`createLogEntryConflict {"code":409,"message":"an equivalent entry already exists in the ` +
		`transparency log with UUID 108e9186e8c5677a2c45c17488e67ac4beb48541bb66419307a9718e2253460406"}`

	script := "#!/bin/sh\n" +
		"n=$(cat " + counter + " 2>/dev/null || echo 0)\n" +
		"n=$((n+1))\n" +
		"echo $n > " + counter + "\n" +
		"if [ \"$n\" -le " + itoa(failures) + " ]; then\n" +
		"  echo '" + conflict + "' >&2\n" +
		"  exit 1\n" +
		"fi\n" +
		"echo 'tlog entry created with index: 123456'\n" +
		"exit 0\n"

	bin := filepath.Join(dir, "cosign")
	Expect(os.WriteFile(bin, []byte(script), 0o755)).To(Succeed())
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return counter
}

func readCallCount(t *testing.T, counter string) string {
	t.Helper()
	data, err := os.ReadFile(counter)
	if err != nil {
		return "0"
	}
	return strings.TrimSpace(string(data))
}

func testAttacher() *Attacher {
	return &Attacher{
		SigningConfig: &signing.Config{Enabled: true, Keyless: true, OIDCToken: "a.b.c"},
		Timeout:       30 * time.Second,
	}
}

const testImage = "registry.example.com/team/app@sha256:f7ed9277c480591d7ec36fe7da13e112b33d898b7687f9bcbcda5c214a242099"

// A Rekor conflict on the first attest must not fail the deploy: a fresh keyless
// invocation mints a new ephemeral cert, so the replayed body differs.
func TestAttach_RetriesRekorConflict(t *testing.T) {
	RegisterTestingT(t)

	counter := installFakeCosign(t, 1)
	sbom := NewSBOM(FormatCycloneDXJSON, []byte(`{"bomFormat":"CycloneDX"}`), "sha256:f7ed9277", nil)

	err := testAttacher().Attach(context.Background(), sbom, testImage)

	Expect(err).ToNot(HaveOccurred())
	Expect(readCallCount(t, counter)).To(Equal("2"), "conflict must trigger exactly one retry")
}

// Regression guard: before the fix a single conflict aborted `sc sbom attach`,
// which failed the Pulumi update even though the workload rollout had already
// completed.
func TestAttach_PersistentConflictStillFails(t *testing.T) {
	RegisterTestingT(t)

	counter := installFakeCosign(t, signing.MaxCosignAttempts+1)
	sbom := NewSBOM(FormatCycloneDXJSON, []byte(`{"bomFormat":"CycloneDX"}`), "sha256:f7ed9277", nil)

	err := testAttacher().Attach(context.Background(), sbom, testImage)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("cosign attest failed"))
	Expect(readCallCount(t, counter)).To(Equal(itoa(signing.MaxCosignAttempts)))
}

func TestAttach_SucceedsWithoutConflict(t *testing.T) {
	RegisterTestingT(t)

	counter := installFakeCosign(t, 0)
	sbom := NewSBOM(FormatCycloneDXJSON, []byte(`{"bomFormat":"CycloneDX"}`), "sha256:f7ed9277", nil)

	err := testAttacher().Attach(context.Background(), sbom, testImage)

	Expect(err).ToNot(HaveOccurred())
	Expect(readCallCount(t, counter)).To(Equal("1"), "no conflict means no retry")
}
