package docker

import (
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

func TestEcrDockerLogin(t *testing.T) {
	tests := []struct {
		name     string
		imageRef string
		wantECR  bool
	}{
		{"ecr image", "471112843480.dkr.ecr.eu-central-1.amazonaws.com/repo/img@sha256:abc123", true},
		{"docker hub", "docker.io/library/ubuntu:latest", false},
		{"ghcr", "ghcr.io/org/image:v1", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ecrDockerLogin(tt.imageRef)
			if tt.wantECR && result == "" {
				t.Errorf("ecrDockerLogin(%q) = empty, want ECR login command", tt.imageRef)
			}
			if !tt.wantECR && result != "" {
				t.Errorf("ecrDockerLogin(%q) = %q, want empty", tt.imageRef, result)
			}
			if tt.wantECR && result != "" {
				if !strings.Contains(result, "eu-central-1") {
					t.Errorf("ecrDockerLogin() missing region, got: %s", result)
				}
				if !strings.Contains(result, "aws ecr get-login-password") {
					t.Errorf("ecrDockerLogin() missing aws command, got: %s", result)
				}
			}
		})
	}
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
