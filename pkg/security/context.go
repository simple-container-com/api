package security

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
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

	// Always attempt OIDC resolution — SIGSTORE_ID_TOKEN may be set by a
	// developer running `sc image sign --keyless` or `sc provenance attach
	// --keyless` locally (outside any detected CI provider). GetOIDCToken
	// checks the env var first, then falls back to the GitHub-Actions
	// request endpoint when applicable. A failure here is non-fatal:
	// OIDC is only needed for keyless flows, and those will return their
	// own clearer error when execCtx.OIDCToken is empty.
	if err := execCtx.GetOIDCToken(ctx); err != nil {
		if execCtx.IsCI {
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

		token, err := requestOIDCTokenWithRetry(ctx, requestURL, requestToken, defaultOIDCRetryPolicy(), sleepContext)
		if err != nil {
			return err
		}
		e.OIDCToken = token
		return nil
	}

	return fmt.Errorf("OIDC token not available")
}

type oidcRetryPolicy struct {
	Attempts          int
	PerAttemptTimeout time.Duration
	BaseBackoff       time.Duration
	MaxBackoff        time.Duration
}

func defaultOIDCRetryPolicy() oidcRetryPolicy {
	p := oidcRetryPolicy{
		Attempts:          4,
		PerAttemptTimeout: 20 * time.Second,
		BaseBackoff:       1 * time.Second,
		MaxBackoff:        8 * time.Second,
	}
	if v := os.Getenv("SC_OIDC_TOKEN_REQUEST_ATTEMPTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.Attempts = n
		}
	}
	if v := os.Getenv("SC_OIDC_TOKEN_REQUEST_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			p.PerAttemptTimeout = d
		}
	}
	return p
}

func requestOIDCTokenWithRetry(ctx context.Context, requestURL, requestToken string, policy oidcRetryPolicy, sleep func(context.Context, time.Duration) error) (string, error) {
	var lastErr error
	for attempt := 0; attempt < policy.Attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if attempt > 0 {
			if err := sleep(ctx, oidcBackoff(policy, attempt)); err != nil {
				return "", err
			}
		}
		token, retryable, err := doOIDCTokenRequest(ctx, requestURL, requestToken, policy.PerAttemptTimeout)
		if err == nil {
			return token, nil
		}
		lastErr = err
		if !retryable {
			return "", err
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("requesting OIDC token failed after %d attempts: %w", policy.Attempts, lastErr)
}

func doOIDCTokenRequest(ctx context.Context, requestURL, requestToken string, timeout time.Duration) (string, bool, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	reqURL, err := oidcTokenURL(requestURL)
	if err != nil {
		return "", false, err
	}
	req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", false, fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+requestToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", true, fmt.Errorf("requesting OIDC token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", retryableOIDCStatus(resp.StatusCode), fmt.Errorf("OIDC token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", true, fmt.Errorf("reading token response: %w", err)
	}
	var tokenResp struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", false, fmt.Errorf("parsing OIDC token response: %w", err)
	}
	if tokenResp.Value == "" {
		return "", false, fmt.Errorf("OIDC token response has empty value field")
	}
	return tokenResp.Value, false, nil
}

func retryableOIDCStatus(status int) bool {
	if status == http.StatusRequestTimeout || status == http.StatusTooManyRequests {
		return true
	}
	return status >= 500 && status <= 599
}

func oidcTokenURL(requestURL string) (string, error) {
	u, err := url.Parse(requestURL)
	if err != nil {
		return "", fmt.Errorf("invalid OIDC request URL: %w", err)
	}
	q := u.Query()
	q.Set("audience", "sigstore")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func oidcBackoff(policy oidcRetryPolicy, attempt int) time.Duration {
	maxBackoff := policy.BaseBackoff << (attempt - 1)
	if maxBackoff <= 0 || maxBackoff > policy.MaxBackoff {
		maxBackoff = policy.MaxBackoff
	}
	if maxBackoff <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(maxBackoff)))
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
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
