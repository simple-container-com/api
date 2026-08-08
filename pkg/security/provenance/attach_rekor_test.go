// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package provenance

import (
	"context"
	"fmt"
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
// failures invocations with a Rekor 409 entry conflict, then succeeds.
// Invocation count lives in a file so it survives across separate process
// executions.
func installFakeCosign(t *testing.T, failures int) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-binary PATH harness is POSIX-shell only")
	}
	dir := t.TempDir()
	counter := filepath.Join(dir, "calls")

	conflict := `Error: attaching provenance: cosign attest failed: [POST /api/v1/log/entries][409] ` +
		`createLogEntryConflict {"code":409,"message":"an equivalent entry already exists in the ` +
		`transparency log with UUID 108e9186e8c5677aed8837db0e5e2ae48507be504018ed6aeb9f8bba9d12f6a2"}`

	script := fmt.Sprintf("#!/bin/sh\n"+
		"n=$(cat %[1]s 2>/dev/null || echo 0)\n"+
		"n=$((n+1))\n"+
		"echo $n > %[1]s\n"+
		"if [ \"$n\" -le %[2]d ]; then\n"+
		"  echo '%[3]s' >&2\n"+
		"  exit 1\n"+
		"fi\n"+
		"echo 'tlog entry created with index: 987654'\n"+
		"exit 0\n", counter, failures, conflict)

	bin := filepath.Join(dir, "cosign")
	Expect(os.WriteFile(bin, []byte(script), 0o755)).To(Succeed())
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return counter
}

func callCount(t *testing.T, counter string) string {
	t.Helper()
	data, err := os.ReadFile(counter)
	if err != nil {
		return "0"
	}
	return strings.TrimSpace(string(data))
}

func testStatement() *Statement {
	predicate := []byte(`{"buildDefinition":{"buildType":"https://simple-container.com/build/v1"}}`)
	return NewStatement(FormatSLSAV10, predicate, provTestImage, &Metadata{BuilderID: "sc"})
}

const provTestImage = "registry.example.com/team/app@sha256:e6ba56b60370949f74b515333ea56a827c1775002aa8b808e37797d1f4304309"

func testProvAttacher() *Attacher {
	return &Attacher{
		SigningConfig: &signing.Config{Enabled: true, Keyless: true, OIDCToken: "a.b.c"},
		Timeout:       30 * time.Second,
	}
}

// Provenance attest races the SBOM attest on the same digest, and was observed
// to hit the conflict first, so it needs the same tolerance.
func TestProvenanceAttach_RetriesRekorConflict(t *testing.T) {
	RegisterTestingT(t)

	counter := installFakeCosign(t, 1)

	err := testProvAttacher().Attach(context.Background(), testStatement(), provTestImage)

	Expect(err).ToNot(HaveOccurred())
	Expect(callCount(t, counter)).To(Equal("2"), "conflict must trigger exactly one retry")
}

func TestProvenanceAttach_PersistentConflictStillFails(t *testing.T) {
	RegisterTestingT(t)

	counter := installFakeCosign(t, signing.MaxCosignAttempts+1)

	err := testProvAttacher().Attach(context.Background(), testStatement(), provTestImage)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("cosign attest failed"))
	Expect(callCount(t, counter)).To(Equal(fmt.Sprint(signing.MaxCosignAttempts)))
}

func TestProvenanceAttach_NoRetryOnOtherErrors(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	counter := filepath.Join(dir, "calls")
	script := fmt.Sprintf("#!/bin/sh\n"+
		"n=$(cat %[1]s 2>/dev/null || echo 0)\n"+
		"echo $((n+1)) > %[1]s\n"+
		"echo 'Error: getting signer: retrieving cert: oidc: token expired' >&2\n"+
		"exit 1\n", counter)
	bin := filepath.Join(dir, "cosign")
	Expect(os.WriteFile(bin, []byte(script), 0o755)).To(Succeed())
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := testProvAttacher().Attach(context.Background(), testStatement(), provTestImage)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("token expired"))
	Expect(callCount(t, counter)).To(Equal("1"), "a non-conflict error must fail fast")
}
