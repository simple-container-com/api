package security

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
)

func TestNewExecutionContextHonoursSIGSTOREIDTokenOutsideCI(t *testing.T) {
	RegisterTestingT(t)

	// Codex review caught a regression in keyless attach / sign: when a
	// developer runs `sc provenance attach --keyless` locally with
	// SIGSTORE_ID_TOKEN already exported, NewExecutionContext used to skip
	// OIDC resolution entirely because DetectCI returned false. The CLI
	// then surfaced "OIDC token not available" even though the env var was
	// set, blocking local keyless workflows. NewExecutionContext now
	// always tries GetOIDCToken (which checks SIGSTORE_ID_TOKEN first),
	// regardless of IsCI.
	clearCIEnv(t)
	t.Setenv("SIGSTORE_ID_TOKEN", "test-token-not-real")

	execCtx, err := NewExecutionContext(context.Background())
	Expect(err).ToNot(HaveOccurred())
	Expect(execCtx.IsCI).To(BeFalse(), "test setup: must not be detected as CI")
	Expect(execCtx.OIDCToken).To(Equal("test-token-not-real"),
		"NewExecutionContext must surface SIGSTORE_ID_TOKEN even outside CI")
}

func TestNewExecutionContextNoTokenOutsideCIIsNonFatal(t *testing.T) {
	RegisterTestingT(t)

	// Sanity guard: with neither SIGSTORE_ID_TOKEN nor a CI provider, the
	// constructor must still return successfully — keyless callers raise
	// their own clearer error from the empty OIDCToken.
	clearCIEnv(t)
	t.Setenv("SIGSTORE_ID_TOKEN", "")

	execCtx, err := NewExecutionContext(context.Background())
	Expect(err).ToNot(HaveOccurred())
	Expect(execCtx.OIDCToken).To(BeEmpty())
}

func clearCIEnv(t *testing.T) {
	t.Helper()
	// t.Setenv("", "") would panic; explicitly unset each var so detection
	// falls through to the local-dev branch regardless of the surrounding
	// shell (e.g., running this test from within a GitHub Actions runner).
	for _, key := range []string{
		"GITHUB_ACTIONS",
		"GITLAB_CI",
		"ACTIONS_ID_TOKEN_REQUEST_URL",
		"ACTIONS_ID_TOKEN_REQUEST_TOKEN",
	} {
		t.Setenv(key, "")
	}
}
