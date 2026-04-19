package security

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// ExecutionContext contains information about the current execution environment
type ExecutionContext struct {
	IsCI         bool
	CIProvider   string
	Repository   string
	Branch       string
	CommitSHA    string
	CommitShort  string
	BuildID      string
	BuildURL     string
	OIDCToken    string
	OIDCTokenURL string
	GitHubToken  string
	RequestToken string
}

// NewExecutionContext creates a new execution context by detecting the environment
func NewExecutionContext(ctx context.Context) (*ExecutionContext, error) {
	execCtx := &ExecutionContext{}
	execCtx.DetectCI()

	if execCtx.IsCI {
		if err := execCtx.GetOIDCToken(ctx); err != nil {
			// Non-fatal: OIDC token is optional (only needed for keyless signing).
			// Log to stderr so callers that need the token get diagnostic info.
			fmt.Fprintf(os.Stderr, "OIDC token not acquired: %v\n", err)
		}
	}

	execCtx.PopulateGitMetadata()
	return execCtx, nil
}

// DetectCI detects if running in a CI environment and identifies the provider
func (e *ExecutionContext) DetectCI() {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		e.IsCI = true
		e.CIProvider = "github-actions"
		e.BuildID = os.Getenv("GITHUB_RUN_ID")
		e.BuildURL = fmt.Sprintf("%s/%s/actions/runs/%s",
			os.Getenv("GITHUB_SERVER_URL"),
			os.Getenv("GITHUB_REPOSITORY"),
			os.Getenv("GITHUB_RUN_ID"))
	} else if os.Getenv("GITLAB_CI") == "true" {
		e.IsCI = true
		e.CIProvider = "gitlab-ci"
		e.BuildID = os.Getenv("CI_JOB_ID")
		e.BuildURL = os.Getenv("CI_JOB_URL")
	} else {
		e.IsCI = false
		e.CIProvider = "local"
	}
}

// GetOIDCToken attempts to retrieve an OIDC token for keyless signing
func (e *ExecutionContext) GetOIDCToken(ctx context.Context) error {
	// First check for SIGSTORE_ID_TOKEN env var
	if token := os.Getenv("SIGSTORE_ID_TOKEN"); token != "" {
		e.OIDCToken = token
		return nil
	}

	// For GitHub Actions, request token from Actions service
	if e.CIProvider == "github-actions" {
		requestURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
		requestToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")

		if requestURL == "" || requestToken == "" {
			return fmt.Errorf("ACTIONS_ID_TOKEN_REQUEST_URL or ACTIONS_ID_TOKEN_REQUEST_TOKEN not available")
		}

		e.OIDCTokenURL = requestURL
		e.RequestToken = requestToken

		req, err := http.NewRequestWithContext(ctx, "GET", requestURL+"&audience=sigstore", nil)
		if err != nil {
			return fmt.Errorf("creating token request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+requestToken)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("requesting OIDC token: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("OIDC token request failed with status %d: %s", resp.StatusCode, string(body))
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("reading token response: %w", err)
		}

		var tokenResp struct {
			Value string `json:"value"`
		}
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			return fmt.Errorf("parsing OIDC token response: %w", err)
		}
		if tokenResp.Value == "" {
			return fmt.Errorf("OIDC token response has empty value field")
		}
		e.OIDCToken = tokenResp.Value
		return nil
	}

	return fmt.Errorf("OIDC token not available")
}

// PopulateGitMetadata populates git-related metadata from environment
func (e *ExecutionContext) PopulateGitMetadata() {
	if e.CIProvider == "github-actions" {
		e.Repository = os.Getenv("GITHUB_REPOSITORY")
		e.Branch = os.Getenv("GITHUB_REF_NAME")
		e.CommitSHA = os.Getenv("GITHUB_SHA")
		e.GitHubToken = os.Getenv("GITHUB_TOKEN")
		if len(e.CommitSHA) > 7 {
			e.CommitShort = e.CommitSHA[:7]
		}
	} else if e.CIProvider == "gitlab-ci" {
		e.Repository = os.Getenv("CI_PROJECT_PATH")
		e.Branch = os.Getenv("CI_COMMIT_REF_NAME")
		e.CommitSHA = os.Getenv("CI_COMMIT_SHA")
		if len(e.CommitSHA) > 7 {
			e.CommitShort = e.CommitSHA[:7]
		}
	}
}
