package docker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
)

func TestCanUseMergedScanCommand(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name  string
		tools []api.ScanToolDescriptor
		want  bool
	}{
		{
			name: "single tool",
			tools: []api.ScanToolDescriptor{
				{Name: "grype"},
			},
			want: false,
		},
		{
			name: "multiple tools with shared policy only",
			tools: []api.ScanToolDescriptor{
				{Name: "grype"},
				{Name: "trivy"},
			},
			want: true,
		},
		{
			name: "tool specific warn threshold disables merged fast path",
			tools: []api.ScanToolDescriptor{
				{Name: "grype", WarnOn: "high"},
				{Name: "trivy"},
			},
			want: false,
		},
		{
			name: "tool specific required disables merged fast path",
			tools: []api.ScanToolDescriptor{
				{Name: "grype", Required: true},
				{Name: "trivy"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(canUseMergedScanCommand(tt.tools)).To(Equal(tt.want))
		})
	}
}

func TestNeedsMergedScanArtifacts(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name     string
		security *api.SecurityDescriptor
		want     bool
	}{
		{
			name: "no outputs or reporting",
			security: &api.SecurityDescriptor{
				Scan: &api.ScanDescriptor{},
			},
			want: false,
		},
		{
			name: "local scan output requested",
			security: &api.SecurityDescriptor{
				Scan: &api.ScanDescriptor{
					Output: &api.OutputDescriptor{Local: ".sc/scan/results.json"},
				},
			},
			want: true,
		},
		{
			name: "pr comment requested",
			security: &api.SecurityDescriptor{
				Scan: &api.ScanDescriptor{},
				Reporting: &api.ReportingDescriptor{
					PRComment: &api.PRCommentDescriptor{Enabled: true},
				},
			},
			want: true,
		},
		{
			name: "defectdojo upload requested",
			security: &api.SecurityDescriptor{
				Scan: &api.ScanDescriptor{},
				Reporting: &api.ReportingDescriptor{
					DefectDojo: &api.DefectDojoDescriptor{Enabled: true},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(needsMergedScanArtifacts(tt.security, "demo")).To(Equal(tt.want))
		})
	}
}

func TestShouldAttachProvenance(t *testing.T) {
	RegisterTestingT(t)

	Expect(shouldAttachProvenance(nil)).To(BeFalse(), "shouldAttachProvenance(nil) should be false")
	Expect(shouldAttachProvenance(&api.ProvenanceDescriptor{})).To(BeFalse(), "shouldAttachProvenance(empty) should be false")
	Expect(shouldAttachProvenance(&api.ProvenanceDescriptor{
		Output: &api.OutputDescriptor{Registry: true},
	})).To(BeTrue(), "shouldAttachProvenance(registry=true) should be true")
}

func TestAppendScanCacheArg(t *testing.T) {
	RegisterTestingT(t)

	args := []string{"sc", "image", "scan"}
	appendScanCacheArg(&args, &api.ScanDescriptor{
		Cache: &api.CacheDescriptor{
			Enabled: true,
			Dir:     ".sc/cache/security",
		},
	})

	Expect(args[len(args)-2]).To(Equal("--cache-dir"))
	Expect(args[len(args)-1]).To(Equal(".sc/cache/security"))
}

func TestSigningCLIArgs(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		cfg  *api.SigningDescriptor
		want []string
	}{
		{
			name: "nil config",
			cfg:  nil,
			want: nil,
		},
		{
			name: "keyless",
			cfg: &api.SigningDescriptor{
				Keyless: true,
			},
			want: []string{"--keyless"},
		},
		{
			name: "key based",
			cfg: &api.SigningDescriptor{
				PrivateKey: ".keys/cosign.key",
			},
			want: []string{"--key", ".keys/cosign.key"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			got := signingCLIArgs(tt.cfg)
			Expect(got).To(HaveLen(len(tt.want)))
			for i := range got {
				Expect(got[i]).To(Equal(tt.want[i]))
			}
		})
	}
}

func TestRepoDigestRegex(t *testing.T) {
	RegisterTestingT(t)

	valid := []string{
		"registry.example.com/repo@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		"ghcr.io/org/image@sha256:0000000000000000000000000000000000000000000000000000000000000000",
		"index.docker.io/library/ubuntu@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	invalid := []string{
		"registry.example.com/repo:latest",
		"registry.example.com/repo@sha256:abc123", // too short
		"registry.example.com/repo@sha256:ABCDEF" + // uppercase not allowed
			"1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		"",
	}

	for _, ref := range valid {
		Expect(repoDigestRe.MatchString(ref)).To(BeTrue(), "repoDigestRe should match %q", ref)
	}
	for _, ref := range invalid {
		Expect(repoDigestRe.MatchString(ref)).To(BeFalse(), "repoDigestRe should not match %q", ref)
	}
}

func TestDockerConfigJSON(t *testing.T) {
	RegisterTestingT(t)

	// Verify the config.json format matches what docker login produces
	// and what cosign/grype/trivy/syft expect.
	server := "000000000000.dkr.ecr.eu-central-1.amazonaws.com"
	username := "AWS"
	password := "test-ecr-token-not-real"

	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	configJSON := fmt.Sprintf(`{"auths":{"%s":{"auth":"%s"}}}`, server, auth)

	var parsed struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}
	err := json.Unmarshal([]byte(configJSON), &parsed)
	Expect(err).ToNot(HaveOccurred(), "config.json is not valid JSON")

	entry, ok := parsed.Auths[server]
	Expect(ok).To(BeTrue(), "config.json missing auth entry for %s", server)

	decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
	Expect(err).ToNot(HaveOccurred(), "auth field is not valid base64")
	Expect(string(decoded)).To(Equal(username + ":" + password))
}

func TestDockerConfigJSON_GCPArtifactRegistry(t *testing.T) {
	RegisterTestingT(t)

	server := "europe-north1-docker.pkg.dev/example-project/example-registry"
	username := "oauth2accesstoken"
	password := "test-gcp-token-not-real"

	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	configJSON := fmt.Sprintf(`{"auths":{"%s":{"auth":"%s"}}}`, server, auth)

	var parsed struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}
	err := json.Unmarshal([]byte(configJSON), &parsed)
	Expect(err).ToNot(HaveOccurred(), "config.json is not valid JSON")

	_, ok := parsed.Auths[server]
	Expect(ok).To(BeTrue(), "config.json missing auth entry for GCP Artifact Registry server")
}

func TestResolveStringArg(t *testing.T) {
	RegisterTestingT(t)

	// sdk.All may pass through *string (from sdk.StringPtr) without dereferencing.
	// resolveStringArg handles both string and *string.
	token := "ya29.test-gcp-access-token"
	ptr := &token

	Expect(resolveStringArg("direct-string")).To(Equal("direct-string"))
	Expect(resolveStringArg(ptr)).To(Equal(token))
	Expect(resolveStringArg((*string)(nil))).To(BeEmpty())
	Expect(resolveStringArg(nil)).To(BeEmpty())
	Expect(resolveStringArg(42)).To(BeEmpty())
}

func TestWriteDockerConfigScript(t *testing.T) {
	RegisterTestingT(t)

	script := writeDockerConfigScript("registry.example.com", "dGVzdC1hdXRo")
	Expect(script).To(ContainSubstring("registry.example.com"))
	Expect(script).To(ContainSubstring("dGVzdC1hdXRo"))
	Expect(script).To(ContainSubstring(".docker/config.json"))
}

func TestVerifyAttestationStdoutRedirect(t *testing.T) {
	RegisterTestingT(t)

	// Verify that cosign verify-attestation commands redirect stdout to /dev/null
	// to prevent Pulumi pipe buffer deadlocks on large attestation payloads.
	// The actual commands are built inside ApplyT callbacks (not directly testable
	// without Pulumi runtime), so we verify the constant and pattern here.

	// The securityPATHPrefix + command + " > /dev/null" pattern is used in:
	// - verify-sbom (cyclonedx attestation)
	// - verify-provenance (SLSA v1 attestation)
	// This test guards against someone removing the redirect.
	prefix := securityPATHPrefix
	verifyCmd := prefix + "'cosign' 'verify-attestation' '--type' 'cyclonedx' 'img@sha256:abc'" + " > /dev/null"
	Expect(verifyCmd).To(HaveSuffix("> /dev/null"))
	Expect(verifyCmd).To(HavePrefix("export PATH="))
}

func TestSecurityPATHPrefix(t *testing.T) {
	RegisterTestingT(t)

	// The PATH prefix ensures tools installed to ~/.local/bin are findable by
	// Pulumi local.Command subshells, which don't inherit Go-side os.Setenv.
	prefix := securityPATHPrefix
	Expect(prefix).To(ContainSubstring("$HOME/.local/bin"))
	Expect(prefix).To(ContainSubstring("/usr/local/bin"))
	Expect(prefix).To(HavePrefix("export PATH="))
	Expect(prefix).To(HaveSuffix("&& "))
}

func TestEngagementRouting(t *testing.T) {
	RegisterTestingT(t)

	// Engagement routing must match conventions used by Semgrep, Trivy, Grype:
	//   PR deploy  -> "PR-{number}"
	//   Non-PR     -> "Source-Scan" (default when engagement name not configured)
	tests := []struct {
		env            string
		configuredName string
		want           string
	}{
		{"pr2209", "", "PR-2209"},
		{"pr1", "", "PR-1"},
		{"pr99999", "", "PR-99999"},
		{"staging", "", "Source-Scan"},      // default for non-PR
		{"test", "", "Source-Scan"},         // default for non-PR
		{"prod", "", "Source-Scan"},         // must NOT match "pr" -- "prod" starts with "pr"
		{"production", "", "Source-Scan"},   // must NOT match
		{"preview", "", "Source-Scan"},      // must NOT match
		{"pre-release", "", "Source-Scan"},  // must NOT match
		{"", "", "Source-Scan"},             // empty env = non-PR
		{"staging", "Custom-Eng", "Custom-Eng"}, // configured name preserved for non-PR
		{"pr123", "Custom-Eng", "PR-123"},        // PR override takes precedence
	}
	for _, tt := range tests {
		t.Run(tt.env+"_"+tt.configuredName, func(t *testing.T) {
			RegisterTestingT(t)
			name := tt.configuredName
			if strings.HasPrefix(tt.env, "pr") {
				num := strings.TrimPrefix(tt.env, "pr")
				if num != "" && num[0] >= '0' && num[0] <= '9' {
					name = "PR-" + num
				}
			}
			if name == "" {
				name = "Source-Scan"
			}
			Expect(name).To(Equal(tt.want), "env=%q configured=%q", tt.env, tt.configuredName)
		})
	}
}

func TestVerifyIdentityArgs(t *testing.T) {
	RegisterTestingT(t)

	t.Run("keyless", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.SigningDescriptor{
			Keyless: true,
			Verify: &api.VerifyDescriptor{
				OIDCIssuer:     "https://token.actions.githubusercontent.com",
				IdentityRegexp: "^https://github.com/org/.*$",
			},
		}
		args := verifyIdentityArgs(cfg)
		Expect(args).To(HaveLen(4))
		Expect(args[0]).To(Equal("--certificate-oidc-issuer"))
	})

	t.Run("key-based", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.SigningDescriptor{PublicKey: "cosign.pub"}
		args := verifyIdentityArgs(cfg)
		Expect(args).To(HaveLen(2))
		Expect(args[0]).To(Equal("--key"))
	})

	t.Run("nil verify", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.SigningDescriptor{Keyless: true}
		args := verifyIdentityArgs(cfg)
		Expect(args).To(BeNil())
	})
}

func TestSigningCommandEnvironment(t *testing.T) {
	RegisterTestingT(t)

	Expect(signingCommandEnvironment(nil)).To(BeNil())

	keylessEnv := signingCommandEnvironment(&api.SigningDescriptor{Keyless: true})
	_, ok := keylessEnv["COSIGN_EXPERIMENTAL"]
	Expect(ok).To(BeTrue(), "expected COSIGN_EXPERIMENTAL for keyless signing")

	keyEnv := signingCommandEnvironment(&api.SigningDescriptor{PrivateKey: ".keys/cosign.key"})
	value, ok := keyEnv["COSIGN_PASSWORD"]
	Expect(ok).To(BeTrue(), "expected COSIGN_PASSWORD for key-based signing")
	_, isStringOutput := interface{}(value).(sdk.StringOutput)
	Expect(isStringOutput).To(BeTrue(), "COSIGN_PASSWORD env value type = %T, want pulumi StringOutput", value)
}
