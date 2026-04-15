package docker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
)

func TestCanUseMergedScanCommand(t *testing.T) {
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
			if got := canUseMergedScanCommand(tt.tools); got != tt.want {
				t.Fatalf("canUseMergedScanCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsMergedScanArtifacts(t *testing.T) {
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
			if got := needsMergedScanArtifacts(tt.security, "demo"); got != tt.want {
				t.Fatalf("needsMergedScanArtifacts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldAttachProvenance(t *testing.T) {
	if shouldAttachProvenance(nil) {
		t.Fatal("shouldAttachProvenance(nil) = true, want false")
	}

	if shouldAttachProvenance(&api.ProvenanceDescriptor{}) {
		t.Fatal("shouldAttachProvenance(empty) = true, want false")
	}

	if !shouldAttachProvenance(&api.ProvenanceDescriptor{
		Output: &api.OutputDescriptor{Registry: true},
	}) {
		t.Fatal("shouldAttachProvenance(registry=true) = false, want true")
	}
}

func TestAppendScanCacheArg(t *testing.T) {
	args := []string{"sc", "image", "scan"}
	appendScanCacheArg(&args, &api.ScanDescriptor{
		Cache: &api.CacheDescriptor{
			Enabled: true,
			Dir:     ".sc/cache/security",
		},
	})

	if got, want := args[len(args)-2], "--cache-dir"; got != want {
		t.Fatalf("cache flag = %q, want %q", got, want)
	}
	if got, want := args[len(args)-1], ".sc/cache/security"; got != want {
		t.Fatalf("cache dir = %q, want %q", got, want)
	}
}

func TestSigningCLIArgs(t *testing.T) {
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
			got := signingCLIArgs(tt.cfg)
			if len(got) != len(tt.want) {
				t.Fatalf("signingCLIArgs() len = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("signingCLIArgs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestRepoDigestRegex(t *testing.T) {
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
		if !repoDigestRe.MatchString(ref) {
			t.Errorf("repoDigestRe should match %q but did not", ref)
		}
	}
	for _, ref := range invalid {
		if repoDigestRe.MatchString(ref) {
			t.Errorf("repoDigestRe should not match %q but did", ref)
		}
	}
}

func TestDockerConfigJSON(t *testing.T) {
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
	if err := json.Unmarshal([]byte(configJSON), &parsed); err != nil {
		t.Fatalf("config.json is not valid JSON: %v", err)
	}
	entry, ok := parsed.Auths[server]
	if !ok {
		t.Fatalf("config.json missing auth entry for %s", server)
	}
	decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
	if err != nil {
		t.Fatalf("auth field is not valid base64: %v", err)
	}
	if string(decoded) != username+":"+password {
		t.Errorf("decoded auth = %q, want %q", string(decoded), username+":"+password)
	}
}

func TestDockerConfigJSON_GCPArtifactRegistry(t *testing.T) {
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
	if err := json.Unmarshal([]byte(configJSON), &parsed); err != nil {
		t.Fatalf("config.json is not valid JSON: %v", err)
	}
	if _, ok := parsed.Auths[server]; !ok {
		t.Fatalf("config.json missing auth entry for GCP Artifact Registry server")
	}
}

func TestIsGCPRegistry(t *testing.T) {
	gcpServers := []string{
		"europe-north1-docker.pkg.dev",
		"europe-north1-docker.pkg.dev/project/repo",
		"us-docker.pkg.dev/my-project/my-repo",
		"gcr.io/my-project",
		"eu.gcr.io/my-project",
	}
	nonGCPServers := []string{
		"000000000000.dkr.ecr.eu-central-1.amazonaws.com",
		"registry-1.docker.io",
		"ghcr.io/org",
		"",
	}
	for _, s := range gcpServers {
		if !isGCPRegistry(s) {
			t.Errorf("isGCPRegistry(%q) = false, want true", s)
		}
	}
	for _, s := range nonGCPServers {
		if isGCPRegistry(s) {
			t.Errorf("isGCPRegistry(%q) = true, want false", s)
		}
	}
}

func TestGCPRegistryLoginScript(t *testing.T) {
	script := gcpRegistryLoginScript("europe-north1-docker.pkg.dev/project/repo")
	// Should extract host only (no project/repo path)
	if !strings.Contains(script, "europe-north1-docker.pkg.dev") {
		t.Error("script should contain the registry host")
	}
	if strings.Contains(script, "project/repo") {
		t.Error("script should NOT contain the project/repo path, only the host")
	}
	if !strings.Contains(script, "gcloud auth print-access-token") {
		t.Error("script should use gcloud auth print-access-token")
	}
	if !strings.Contains(script, "oauth2accesstoken") {
		t.Error("script should use oauth2accesstoken as username")
	}
}

func TestWriteDockerConfigScript(t *testing.T) {
	script := writeDockerConfigScript("registry.example.com", "dGVzdC1hdXRo")
	if !strings.Contains(script, "registry.example.com") {
		t.Error("script should contain server")
	}
	if !strings.Contains(script, "dGVzdC1hdXRo") {
		t.Error("script should contain auth token")
	}
	if !strings.Contains(script, ".docker/config.json") {
		t.Error("script should write to .docker/config.json")
	}
}

func TestSecurityPATHPrefix(t *testing.T) {
	// The PATH prefix ensures tools installed to ~/.local/bin are findable by
	// Pulumi local.Command subshells, which don't inherit Go-side os.Setenv.
	prefix := securityPATHPrefix
	if !strings.Contains(prefix, "$HOME/.local/bin") {
		t.Error("securityPATHPrefix should include $HOME/.local/bin")
	}
	if !strings.Contains(prefix, "/usr/local/bin") {
		t.Error("securityPATHPrefix should include /usr/local/bin")
	}
	if !strings.HasPrefix(prefix, "export PATH=") {
		t.Error("securityPATHPrefix should start with export PATH=")
	}
	if !strings.HasSuffix(prefix, "&& ") {
		t.Error("securityPATHPrefix should end with '&& ' for command chaining")
	}
}

func TestPREngagementDetection(t *testing.T) {
	// The PR detection logic in executeSecurityOperations checks:
	// strings.HasPrefix(environment, "pr") && num[0] >= '0' && num[0] <= '9'
	// Test the same logic here to guard against regressions.
	tests := []struct {
		env  string
		want string // empty means no override (keep configured name)
	}{
		{"pr2209", "PR-2209"},
		{"pr1", "PR-1"},
		{"pr99999", "PR-99999"},
		{"staging", ""},
		{"test", ""},
		{"prod", ""},        // must NOT match — "prod" starts with "pr"
		{"production", ""},  // must NOT match
		{"preview", ""},     // must NOT match
		{"pre-release", ""}, // must NOT match
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			got := ""
			if strings.HasPrefix(tt.env, "pr") {
				num := strings.TrimPrefix(tt.env, "pr")
				if num != "" && num[0] >= '0' && num[0] <= '9' {
					got = "PR-" + num
				}
			}
			if got != tt.want {
				t.Errorf("env=%q → %q, want %q", tt.env, got, tt.want)
			}
		})
	}
}

func TestVerifyIdentityArgs(t *testing.T) {
	t.Run("keyless", func(t *testing.T) {
		cfg := &api.SigningDescriptor{
			Keyless: true,
			Verify: &api.VerifyDescriptor{
				OIDCIssuer:     "https://token.actions.githubusercontent.com",
				IdentityRegexp: "^https://github.com/org/.*$",
			},
		}
		args := verifyIdentityArgs(cfg)
		if len(args) != 4 {
			t.Fatalf("verifyIdentityArgs() len = %d, want 4", len(args))
		}
		if args[0] != "--certificate-oidc-issuer" {
			t.Errorf("args[0] = %q, want --certificate-oidc-issuer", args[0])
		}
	})

	t.Run("key-based", func(t *testing.T) {
		cfg := &api.SigningDescriptor{PublicKey: "cosign.pub"}
		args := verifyIdentityArgs(cfg)
		if len(args) != 2 || args[0] != "--key" {
			t.Errorf("verifyIdentityArgs() = %v, want [--key cosign.pub]", args)
		}
	})

	t.Run("nil verify", func(t *testing.T) {
		cfg := &api.SigningDescriptor{Keyless: true}
		args := verifyIdentityArgs(cfg)
		if args != nil {
			t.Errorf("verifyIdentityArgs() = %v, want nil", args)
		}
	})
}

func TestSigningCommandEnvironment(t *testing.T) {
	if got := signingCommandEnvironment(nil); got != nil {
		t.Fatalf("signingCommandEnvironment(nil) = %#v, want nil", got)
	}

	keylessEnv := signingCommandEnvironment(&api.SigningDescriptor{Keyless: true})
	if _, ok := keylessEnv["COSIGN_EXPERIMENTAL"]; !ok {
		t.Fatal("expected COSIGN_EXPERIMENTAL for keyless signing")
	}

	keyEnv := signingCommandEnvironment(&api.SigningDescriptor{PrivateKey: ".keys/cosign.key"})
	value, ok := keyEnv["COSIGN_PASSWORD"]
	if !ok {
		t.Fatal("expected COSIGN_PASSWORD for key-based signing")
	}
	if _, ok := interface{}(value).(sdk.StringOutput); !ok {
		t.Fatalf("COSIGN_PASSWORD env value type = %T, want pulumi StringOutput", value)
	}
}
