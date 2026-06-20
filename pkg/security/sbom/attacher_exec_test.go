// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package sbom

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/signing"
)

// fakeCosign installs a shell script named "cosign" into a fresh temp dir and
// prepends it to PATH. The script ignores its arguments, prints stdout/stderr,
// and exits with exitCode. This drives Attacher.Attach / Attacher.Verify
// without a real cosign install, registry, or signing keys.
func fakeCosign(t *testing.T, stdout, stderr string, exitCode int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake cosign shell script requires a POSIX shell")
	}

	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"printf '%s' " + shellQuote(stdout) + "\n" +
		"printf '%s' " + shellQuote(stderr) + " 1>&2\n" +
		"exit " + itoa(exitCode) + "\n"

	path := filepath.Join(dir, "cosign")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake cosign: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestAttacherAttach_FakeCosign(t *testing.T) {
	RegisterTestingT(t)

	sbomContent := []byte(`{"bomFormat":"CycloneDX","components":[]}`)
	sbomObj := NewSBOM(FormatCycloneDXJSON, sbomContent, "img", &Metadata{ToolName: "syft", ToolVersion: "1.0.0"})

	t.Run("Keyless attest succeeds", func(t *testing.T) {
		RegisterTestingT(t)
		fakeCosign(t, "", "tlog entry created", 0)
		a := NewAttacher(&signing.Config{Enabled: true, Keyless: true, OIDCToken: "tok"})
		Expect(a.Attach(context.Background(), sbomObj, "registry.io/app:v1")).To(Succeed())
	})

	t.Run("Key-based attest succeeds", func(t *testing.T) {
		RegisterTestingT(t)
		fakeCosign(t, "", "", 0)
		a := NewAttacher(&signing.Config{Enabled: true, Keyless: false, PrivateKey: "/tmp/cosign.key", Password: "pw"})
		Expect(a.Attach(context.Background(), sbomObj, "registry.io/app:v1")).To(Succeed())
	})

	t.Run("Nil signing config attest succeeds", func(t *testing.T) {
		RegisterTestingT(t)
		fakeCosign(t, "", "", 0)
		a := NewAttacher(nil)
		Expect(a.Attach(context.Background(), sbomObj, "registry.io/app:v1")).To(Succeed())
	})

	t.Run("cosign failure surfaces stderr", func(t *testing.T) {
		RegisterTestingT(t)
		fakeCosign(t, "", "signing failed: no identity token", 1)
		a := NewAttacher(&signing.Config{Enabled: true, Keyless: true})
		err := a.Attach(context.Background(), sbomObj, "registry.io/app:v1")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("cosign attest failed"))
		Expect(err.Error()).To(ContainSubstring("no identity token"))
	})
}

func TestAttacherVerify_FakeCosign(t *testing.T) {
	RegisterTestingT(t)

	t.Run("Verify succeeds and parses predicate", func(t *testing.T) {
		RegisterTestingT(t)
		predicate := json.RawMessage(`{"bomFormat":"CycloneDX","components":[]}`)
		statement, _ := json.Marshal(map[string]json.RawMessage{"predicate": predicate})
		envelope, _ := json.Marshal(map[string]string{"payload": base64.StdEncoding.EncodeToString(statement)})

		fakeCosign(t, string(envelope), "Verification for app:v1 --\n", 0)
		a := NewAttacher(&signing.Config{
			Enabled:        true,
			Keyless:        true,
			IdentityRegexp: "user@example.com",
			OIDCIssuer:     "https://token.actions.githubusercontent.com",
		})

		sbom, err := a.Verify(context.Background(), "registry.io/app@sha256:"+
			"abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890", FormatCycloneDXJSON)
		Expect(err).ToNot(HaveOccurred())
		Expect(sbom).ToNot(BeNil())
		Expect(string(sbom.Content)).To(Equal(string(predicate)))
		Expect(sbom.ImageDigest).To(Equal("sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"))
	})

	t.Run("cosign verify failure surfaces stderr", func(t *testing.T) {
		RegisterTestingT(t)
		fakeCosign(t, "", "no matching attestations", 1)
		a := NewAttacher(&signing.Config{Enabled: true, Keyless: true})
		_, err := a.Verify(context.Background(), "registry.io/app:v1", FormatCycloneDXJSON)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("verify-attestation failed"))
		Expect(err.Error()).To(ContainSubstring("no matching attestations"))
	})

	t.Run("Verify with unparseable cosign output errors at parse", func(t *testing.T) {
		RegisterTestingT(t)
		// cosign exits 0 but emits no JSON attestation -> parseAttestationOutput fails.
		fakeCosign(t, "Verification succeeded but no JSON here", "", 0)
		a := NewAttacher(&signing.Config{Enabled: true, Keyless: false, PublicKey: "/tmp/cosign.pub"})
		_, err := a.Verify(context.Background(), "registry.io/app:v1", FormatCycloneDXJSON)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to parse attestation output"))
	})
}

func TestAttacherBuildSigningEnv_KeylessToken(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name   string
		config *signing.Config
		want   []string
	}{
		{
			name:   "Keyless with OIDC token exports SIGSTORE_ID_TOKEN",
			config: &signing.Config{Keyless: true, OIDCToken: "id-token-123"},
			want:   []string{"SIGSTORE_ID_TOKEN=id-token-123"},
		},
		{
			name:   "Keyless without token exports nothing",
			config: &signing.Config{Keyless: true},
			want:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			a := &Attacher{SigningConfig: tt.config}
			got := a.buildSigningEnv()
			Expect(got).To(HaveLen(len(tt.want)))
			for i := range got {
				Expect(got[i]).To(Equal(tt.want[i]))
			}
		})
	}
}
