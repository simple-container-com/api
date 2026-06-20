package security

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/signing"
)

// ---- errors.go ----

func TestSecurityError(t *testing.T) {
	RegisterTestingT(t)

	t.Run("with wrapped error", func(t *testing.T) {
		RegisterTestingT(t)
		inner := errors.New("boom")
		serr := NewSecurityError("scan", "failed to run grype", inner)
		Expect(serr.Operation).To(Equal("scan"))
		Expect(serr.Message).To(Equal("failed to run grype"))
		Expect(serr.Error()).To(Equal("scan failed: failed to run grype: boom"))
		Expect(serr.Unwrap()).To(BeIdenticalTo(inner))
		// errors.Is reaches the wrapped error via Unwrap.
		Expect(errors.Is(serr, inner)).To(BeTrue())
	})

	t.Run("without wrapped error", func(t *testing.T) {
		RegisterTestingT(t)
		serr := NewSecurityError("sign", "missing key", nil)
		Expect(serr.Error()).To(Equal("sign failed: missing key"))
		Expect(serr.Unwrap()).To(BeNil())
	})
}

// ---- config.go: previously-uncovered validators ----

func TestPRCommentConfigValidate(t *testing.T) {
	RegisterTestingT(t)

	t.Run("disabled is valid", func(t *testing.T) {
		RegisterTestingT(t)
		Expect((&PRCommentConfig{Enabled: false}).Validate()).To(Succeed())
	})
	t.Run("enabled is valid", func(t *testing.T) {
		RegisterTestingT(t)
		Expect((&PRCommentConfig{Enabled: true, Output: "x.md"}).Validate()).To(Succeed())
	})
}

func TestCacheConfigValidate(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		cfg     *CacheConfig
		wantErr bool
	}{
		{"nil receiver is valid", nil, false},
		{"disabled is valid even with bad ttl", &CacheConfig{Enabled: false, TTL: "garbage"}, false},
		{"enabled empty ttl is valid", &CacheConfig{Enabled: true}, false},
		{"enabled valid ttl", &CacheConfig{Enabled: true, TTL: "6h"}, false},
		{"enabled invalid ttl errors", &CacheConfig{Enabled: true, TTL: "garbage"}, true},
		{"enabled zero ttl errors", &CacheConfig{Enabled: true, TTL: "0s"}, true},
		{"enabled negative ttl errors", &CacheConfig{Enabled: true, TTL: "-1h"}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tc.cfg.Validate()
			if tc.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestReportingConfigValidate(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		cfg     *ReportingConfig
		wantErr bool
	}{
		{"nil receiver is valid", nil, false},
		{"empty reporting is valid", &ReportingConfig{}, false},
		{
			name:    "valid defectdojo",
			cfg:     &ReportingConfig{DefectDojo: &DefectDojoConfig{Enabled: true, URL: "https://d", APIKey: "k", EngagementID: 1}},
			wantErr: false,
		},
		{
			name:    "invalid defectdojo propagates",
			cfg:     &ReportingConfig{DefectDojo: &DefectDojoConfig{Enabled: true}},
			wantErr: true,
		},
		{
			name:    "disabled defectdojo ignored",
			cfg:     &ReportingConfig{DefectDojo: &DefectDojoConfig{Enabled: false}},
			wantErr: false,
		},
		{
			name:    "valid prComment",
			cfg:     &ReportingConfig{PRComment: &PRCommentConfig{Enabled: true, Output: "x.md"}},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tc.cfg.Validate()
			if tc.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

// SBOMConfig.Validate: cover the generator + registry-compatibility branches
// that config_test.go does not reach.
func TestSBOMConfigValidateGeneratorAndRegistry(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		cfg     *SBOMConfig
		wantErr bool
		substr  string
	}{
		{"valid syft generator", &SBOMConfig{Enabled: true, Generator: "syft"}, false, ""},
		{"invalid generator", &SBOMConfig{Enabled: true, Generator: "bogus"}, true, "generator"},
		{"invalid cache ttl", &SBOMConfig{Enabled: true, Cache: &CacheConfig{Enabled: true, TTL: "bad"}}, true, "cache"},
		{
			name:    "registry output with attach disabled",
			cfg:     &SBOMConfig{Enabled: true, Output: &OutputConfig{Registry: true}, Attach: &AttachConfig{Enabled: false, Sign: true}},
			wantErr: true,
			substr:  "attach.enabled=false",
		},
		{
			name:    "registry output with sign disabled",
			cfg:     &SBOMConfig{Enabled: true, Output: &OutputConfig{Registry: true}, Attach: &AttachConfig{Enabled: true, Sign: false}},
			wantErr: true,
			substr:  "attach",
		},
		{
			name:    "registry output with attach enabled+signed is valid",
			cfg:     &SBOMConfig{Enabled: true, Output: &OutputConfig{Registry: true}, Attach: &AttachConfig{Enabled: true, Sign: true}},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tc.cfg.Validate()
			if tc.wantErr {
				Expect(err).To(HaveOccurred())
				if tc.substr != "" {
					Expect(err.Error()).To(ContainSubstring(tc.substr))
				}
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

// DefectDojoConfig.Validate: cover the missing-URL branch when enabled.
func TestDefectDojoConfigValidateMissingURL(t *testing.T) {
	RegisterTestingT(t)
	err := (&DefectDojoConfig{Enabled: true, APIKey: "k", EngagementID: 1}).Validate()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("url is required"))
}

// ReportingConfig.Validate: cover the enabled-prComment branch.
func TestReportingConfigValidateEnabledPRComment(t *testing.T) {
	RegisterTestingT(t)
	cfg := &ReportingConfig{PRComment: &PRCommentConfig{Enabled: true, Output: "out.md"}}
	Expect(cfg.Validate()).To(Succeed())
}

// SecurityConfig.Validate: drive the provenance / scan / reporting branches
// that the existing config_test.go does not reach.
func TestSecurityConfigValidateSubConfigErrors(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name   string
		cfg    *SecurityConfig
		substr string
	}{
		{
			name:   "invalid provenance bubbles up",
			cfg:    &SecurityConfig{Enabled: true, Provenance: &ProvenanceConfig{Enabled: true, Format: "nope"}},
			substr: "provenance config validation failed",
		},
		{
			name:   "invalid scan bubbles up",
			cfg:    &SecurityConfig{Enabled: true, Scan: &ScanConfig{Enabled: true, Tools: nil}},
			substr: "scan config validation failed",
		},
		{
			name:   "invalid reporting bubbles up",
			cfg:    &SecurityConfig{Enabled: true, Reporting: &ReportingConfig{DefectDojo: &DefectDojoConfig{Enabled: true}}},
			substr: "reporting config validation failed",
		},
		{
			name:   "invalid sbom cache bubbles up",
			cfg:    &SecurityConfig{Enabled: true, SBOM: &SBOMConfig{Enabled: true, Cache: &CacheConfig{Enabled: true, TTL: "bad"}}},
			substr: "sbom config validation failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tc.cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(tc.substr))
		})
	}
}

func TestSecurityConfigValidateSigningError(t *testing.T) {
	RegisterTestingT(t)
	cfg := &SecurityConfig{
		Enabled: true,
		Signing: &signing.Config{Enabled: true, Keyless: true}, // missing OIDCIssuer
	}
	err := cfg.Validate()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("signing config validation failed"))
}

// ScanToolConfig.Validate: cover the failOn/warnOn validation branches.
func TestScanToolConfigValidateSeverities(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		cfg     ScanToolConfig
		wantErr bool
	}{
		{"valid with severities", ScanToolConfig{Name: "grype", FailOn: SeverityHigh, WarnOn: SeverityLow}, false},
		{"bad failOn", ScanToolConfig{Name: "grype", FailOn: "nope"}, true},
		{"bad warnOn", ScanToolConfig{Name: "trivy", WarnOn: "nope"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tc.cfg.Validate()
			if tc.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

// ScanConfig.Validate: cover warnOn + cache validation branches.
func TestScanConfigValidateWarnOnAndCache(t *testing.T) {
	RegisterTestingT(t)

	t.Run("bad warnOn", func(t *testing.T) {
		RegisterTestingT(t)
		err := (&ScanConfig{Enabled: true, WarnOn: "nope", Tools: []ScanToolConfig{{Name: "grype"}}}).Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("warnOn"))
	})

	t.Run("bad cache ttl", func(t *testing.T) {
		RegisterTestingT(t)
		err := (&ScanConfig{Enabled: true, Cache: &CacheConfig{Enabled: true, TTL: "bad"}, Tools: []ScanToolConfig{{Name: "grype"}}}).Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("cache"))
	})
}

// ---- context.go ----

func TestSleepContext(t *testing.T) {
	RegisterTestingT(t)

	t.Run("non-positive duration returns immediately", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(sleepContext(context.Background(), 0)).To(Succeed())
		Expect(sleepContext(context.Background(), -5*time.Second)).To(Succeed())
	})

	t.Run("sleeps then returns nil", func(t *testing.T) {
		RegisterTestingT(t)
		start := time.Now()
		Expect(sleepContext(context.Background(), 10*time.Millisecond)).To(Succeed())
		Expect(time.Since(start)).To(BeNumerically(">=", 5*time.Millisecond))
	})

	t.Run("cancelled context aborts the sleep", func(t *testing.T) {
		RegisterTestingT(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := sleepContext(ctx, time.Hour)
		Expect(err).To(MatchError(context.Canceled))
	})
}

func TestDetectCIAndGitMetadataGitlab(t *testing.T) {
	RegisterTestingT(t)
	clearCIEnv(t)
	t.Setenv("GITLAB_CI", "true")
	t.Setenv("CI_JOB_ID", "job-42")
	t.Setenv("CI_JOB_URL", "https://gitlab/job/42")
	t.Setenv("CI_PROJECT_PATH", "group/project")
	t.Setenv("CI_COMMIT_REF_NAME", "feature/x")
	t.Setenv("CI_COMMIT_SHA", "abcdef1234567890")

	e := &ExecutionContext{}
	e.DetectCI()
	Expect(e.IsCI).To(BeTrue())
	Expect(e.CIProvider).To(Equal("gitlab-ci"))
	Expect(e.BuildID).To(Equal("job-42"))
	Expect(e.BuildURL).To(Equal("https://gitlab/job/42"))

	e.PopulateGitMetadata()
	Expect(e.Repository).To(Equal("group/project"))
	Expect(e.Branch).To(Equal("feature/x"))
	Expect(e.CommitSHA).To(Equal("abcdef1234567890"))
	Expect(e.CommitShort).To(Equal("abcdef1")) // first 7 chars
}

func TestDetectCIAndGitMetadataGithub(t *testing.T) {
	RegisterTestingT(t)
	clearCIEnv(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_RUN_ID", "987")
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	t.Setenv("GITHUB_REPOSITORY", "org/repo")
	t.Setenv("GITHUB_REF_NAME", "main")
	t.Setenv("GITHUB_SHA", "0123456789abcdef")
	t.Setenv("GITHUB_TOKEN", "ghs_token")

	e := &ExecutionContext{}
	e.DetectCI()
	Expect(e.CIProvider).To(Equal("github-actions"))
	Expect(e.BuildID).To(Equal("987"))
	Expect(e.BuildURL).To(Equal("https://github.com/org/repo/actions/runs/987"))

	e.PopulateGitMetadata()
	Expect(e.Repository).To(Equal("org/repo"))
	Expect(e.Branch).To(Equal("main"))
	Expect(e.CommitSHA).To(Equal("0123456789abcdef"))
	Expect(e.CommitShort).To(Equal("0123456"))
	Expect(e.GitHubToken).To(Equal("ghs_token"))
}

func TestDetectCILocal(t *testing.T) {
	RegisterTestingT(t)
	clearCIEnv(t)
	e := &ExecutionContext{}
	e.DetectCI()
	Expect(e.IsCI).To(BeFalse())
	Expect(e.CIProvider).To(Equal("local"))

	// PopulateGitMetadata leaves everything empty for the local provider.
	e.PopulateGitMetadata()
	Expect(e.Repository).To(BeEmpty())
	Expect(e.CommitShort).To(BeEmpty())
}

func TestPopulateGitMetadataShortSHANotTruncatedWhenShort(t *testing.T) {
	RegisterTestingT(t)
	clearCIEnv(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_SHA", "abc") // <= 7 chars => CommitShort stays empty

	e := &ExecutionContext{}
	e.DetectCI()
	e.PopulateGitMetadata()
	Expect(e.CommitSHA).To(Equal("abc"))
	Expect(e.CommitShort).To(BeEmpty())
}

func TestDefaultOIDCRetryPolicy(t *testing.T) {
	RegisterTestingT(t)

	t.Run("defaults when env unset", func(t *testing.T) {
		RegisterTestingT(t)
		t.Setenv("SC_OIDC_TOKEN_REQUEST_ATTEMPTS", "")
		t.Setenv("SC_OIDC_TOKEN_REQUEST_TIMEOUT", "")
		p := defaultOIDCRetryPolicy()
		Expect(p.Attempts).To(Equal(4))
		Expect(p.PerAttemptTimeout).To(Equal(20 * time.Second))
		Expect(p.BaseBackoff).To(Equal(1 * time.Second))
		Expect(p.MaxBackoff).To(Equal(8 * time.Second))
	})

	t.Run("env overrides attempts and timeout", func(t *testing.T) {
		RegisterTestingT(t)
		t.Setenv("SC_OIDC_TOKEN_REQUEST_ATTEMPTS", "7")
		t.Setenv("SC_OIDC_TOKEN_REQUEST_TIMEOUT", "5s")
		p := defaultOIDCRetryPolicy()
		Expect(p.Attempts).To(Equal(7))
		Expect(p.PerAttemptTimeout).To(Equal(5 * time.Second))
	})

	t.Run("invalid env values are ignored", func(t *testing.T) {
		RegisterTestingT(t)
		t.Setenv("SC_OIDC_TOKEN_REQUEST_ATTEMPTS", "-2")    // n>0 required
		t.Setenv("SC_OIDC_TOKEN_REQUEST_TIMEOUT", "notdur") // ParseDuration fails
		p := defaultOIDCRetryPolicy()
		Expect(p.Attempts).To(Equal(4))
		Expect(p.PerAttemptTimeout).To(Equal(20 * time.Second))
	})

	t.Run("non-numeric attempts ignored", func(t *testing.T) {
		RegisterTestingT(t)
		t.Setenv("SC_OIDC_TOKEN_REQUEST_ATTEMPTS", "abc")
		p := defaultOIDCRetryPolicy()
		Expect(p.Attempts).To(Equal(4))
	})
}

func TestOIDCBackoffWithinBounds(t *testing.T) {
	RegisterTestingT(t)

	policy := oidcRetryPolicy{BaseBackoff: time.Second, MaxBackoff: 8 * time.Second}

	// attempt 1: max = base<<0 = 1s; jitter in [0,1s)
	for attempt := 1; attempt <= 6; attempt++ {
		d := oidcBackoff(policy, attempt)
		Expect(d).To(BeNumerically(">=", time.Duration(0)))
		Expect(d).To(BeNumerically("<", policy.MaxBackoff))
	}
}

func TestOIDCBackoffZeroBaseReturnsZero(t *testing.T) {
	RegisterTestingT(t)
	// BaseBackoff 0 and MaxBackoff 0 => maxBackoff stays <=0 => returns 0.
	policy := oidcRetryPolicy{BaseBackoff: 0, MaxBackoff: 0}
	Expect(oidcBackoff(policy, 1)).To(Equal(time.Duration(0)))
}

func TestGetOIDCTokenLocalProviderUnavailable(t *testing.T) {
	RegisterTestingT(t)
	clearCIEnv(t)
	t.Setenv("SIGSTORE_ID_TOKEN", "")

	e := &ExecutionContext{}
	e.DetectCI() // local provider
	err := e.GetOIDCToken(context.Background())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("OIDC token not available"))
}

func TestGetOIDCTokenGitHubMissingRequestEnv(t *testing.T) {
	RegisterTestingT(t)
	clearCIEnv(t)
	t.Setenv("SIGSTORE_ID_TOKEN", "")
	t.Setenv("GITHUB_ACTIONS", "true")
	// ACTIONS_ID_TOKEN_REQUEST_URL / TOKEN cleared by clearCIEnv.

	e := &ExecutionContext{}
	e.DetectCI()
	err := e.GetOIDCToken(context.Background())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("not available"))
}

func TestNewExecutionContextInCIWithoutOIDCIsNonFatal(t *testing.T) {
	RegisterTestingT(t)
	// GitHub Actions detected but the OIDC request env is absent => GetOIDCToken
	// fails; because IsCI is true, NewExecutionContext logs to stderr but must
	// still succeed (OIDC is only needed for keyless flows).
	clearCIEnv(t)
	t.Setenv("SIGSTORE_ID_TOKEN", "")
	t.Setenv("GITHUB_ACTIONS", "true")
	// ACTIONS_ID_TOKEN_REQUEST_URL/TOKEN cleared by clearCIEnv.

	execCtx, err := NewExecutionContext(context.Background())
	Expect(err).ToNot(HaveOccurred())
	Expect(execCtx.IsCI).To(BeTrue())
	Expect(execCtx.CIProvider).To(Equal("github-actions"))
	Expect(execCtx.OIDCToken).To(BeEmpty())
}

// ---- cache.go: isLinkUnsupported ----

func TestIsLinkUnsupported(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"EPERM", syscall.EPERM, true},
		{"ENOTSUP", syscall.ENOTSUP, true},
		{"EXDEV", syscall.EXDEV, true},
		{"EOPNOTSUPP", syscall.EOPNOTSUPP, true},
		{"ENOENT not link-unsupported", syscall.ENOENT, false},
		{"generic error not link-unsupported", errors.New("boom"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(isLinkUnsupported(tc.err)).To(Equal(tc.want))
		})
	}
}
