package security

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// CIProvider represents the CI/CD platform
type CIProvider string

const (
	CIProviderNone          CIProvider = "none"
	CIProviderGitHubActions CIProvider = "github-actions"
	CIProviderGitLabCI      CIProvider = "gitlab-ci"
	CIProviderJenkins       CIProvider = "jenkins"
	CIProviderCircleCI      CIProvider = "circleci"
	CIProviderTravisCI      CIProvider = "travis-ci"
)

// ExecutionContext captures environment information for security operations
type ExecutionContext struct {
	// CI Detection
	CI   CIProvider
	IsCI bool

	// OIDC Information
	OIDCToken  string
	OIDCIssuer string

	// Git Information
	Repository  string // github.com/org/repo
	Branch      string // main, feature/xyz
	CommitSHA   string // Full commit SHA
	CommitShort string // Short commit SHA (7 chars)

	// Build Information
	BuildID  string // CI build/run ID
	BuildURL string // Link to CI build
	Workflow string // Workflow/pipeline name
	Actor    string // User/service account

	// Registry Information
	Registry string // docker.io, gcr.io, etc.

	// Environment
	Environment string // production, staging, development
	ProjectName string // Simple Container project name
	StackName   string // Stack name
}

// NewExecutionContext creates context from environment
func NewExecutionContext() (*ExecutionContext, error) {
	ctx := &ExecutionContext{}

	// Detect CI environment
	if err := ctx.DetectCI(); err != nil {
		return nil, fmt.Errorf("failed to detect CI environment: %w", err)
	}

	return ctx, nil
}

// DetectCI identifies CI provider and extracts metadata
func (ctx *ExecutionContext) DetectCI() error {
	// GitHub Actions
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		ctx.CI = CIProviderGitHubActions
		ctx.IsCI = true
		ctx.detectGitHubActions()
		return nil
	}

	// GitLab CI
	if os.Getenv("GITLAB_CI") == "true" {
		ctx.CI = CIProviderGitLabCI
		ctx.IsCI = true
		ctx.detectGitLabCI()
		return nil
	}

	// Jenkins
	if os.Getenv("JENKINS_URL") != "" {
		ctx.CI = CIProviderJenkins
		ctx.IsCI = true
		ctx.detectJenkins()
		return nil
	}

	// CircleCI
	if os.Getenv("CIRCLECI") == "true" {
		ctx.CI = CIProviderCircleCI
		ctx.IsCI = true
		ctx.detectCircleCI()
		return nil
	}

	// Travis CI
	if os.Getenv("TRAVIS") == "true" {
		ctx.CI = CIProviderTravisCI
		ctx.IsCI = true
		ctx.detectTravisCI()
		return nil
	}

	// Not in CI
	ctx.CI = CIProviderNone
	ctx.IsCI = false
	ctx.detectLocal()

	return nil
}

// detectGitHubActions extracts GitHub Actions environment metadata
func (ctx *ExecutionContext) detectGitHubActions() {
	ctx.OIDCIssuer = "https://token.actions.githubusercontent.com"
	ctx.Repository = os.Getenv("GITHUB_REPOSITORY")
	ctx.Branch = strings.TrimPrefix(os.Getenv("GITHUB_REF"), "refs/heads/")
	ctx.CommitSHA = os.Getenv("GITHUB_SHA")
	if len(ctx.CommitSHA) > 7 {
		ctx.CommitShort = ctx.CommitSHA[:7]
	}
	ctx.BuildID = os.Getenv("GITHUB_RUN_ID")
	ctx.BuildURL = fmt.Sprintf("https://github.com/%s/actions/runs/%s",
		ctx.Repository, ctx.BuildID)
	ctx.Workflow = os.Getenv("GITHUB_WORKFLOW")
	ctx.Actor = os.Getenv("GITHUB_ACTOR")

	// Attempt to get OIDC token if available
	if os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN") != "" {
		token, err := ctx.getGitHubOIDCToken()
		if err == nil {
			ctx.OIDCToken = token
		}
	}
}

// getGitHubOIDCToken requests OIDC token from GitHub Actions
func (ctx *ExecutionContext) getGitHubOIDCToken() (string, error) {
	requestToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	requestURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")

	if requestToken == "" || requestURL == "" {
		return "", ErrOIDCTokenUnavailable
	}

	// Add audience parameter for cosign
	requestURL += "&audience=sigstore"

	httpCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(httpCtx, "GET", requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create OIDC token request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+requestToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to request OIDC token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OIDC token request failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read OIDC token response: %w", err)
	}

	// Parse JSON response to extract token
	// Simple extraction (in production, use proper JSON parsing)
	tokenStr := string(body)
	if strings.Contains(tokenStr, `"value":`) {
		start := strings.Index(tokenStr, `"value":"`) + 9
		end := strings.Index(tokenStr[start:], `"`)
		if end > 0 {
			return tokenStr[start : start+end], nil
		}
	}

	return "", fmt.Errorf("failed to parse OIDC token from response")
}

// detectGitLabCI extracts GitLab CI environment metadata
func (ctx *ExecutionContext) detectGitLabCI() {
	ctx.OIDCIssuer = os.Getenv("CI_SERVER_URL")
	ctx.OIDCToken = os.Getenv("CI_JOB_JWT_V2")
	ctx.Repository = os.Getenv("CI_PROJECT_PATH")
	ctx.Branch = os.Getenv("CI_COMMIT_BRANCH")
	ctx.CommitSHA = os.Getenv("CI_COMMIT_SHA")
	if len(ctx.CommitSHA) > 7 {
		ctx.CommitShort = ctx.CommitSHA[:7]
	}
	ctx.BuildID = os.Getenv("CI_JOB_ID")
	ctx.BuildURL = os.Getenv("CI_JOB_URL")
	ctx.Workflow = os.Getenv("CI_PIPELINE_NAME")
	ctx.Actor = os.Getenv("GITLAB_USER_LOGIN")
}

// detectJenkins extracts Jenkins environment metadata
func (ctx *ExecutionContext) detectJenkins() {
	ctx.Repository = os.Getenv("GIT_URL")
	ctx.Branch = os.Getenv("GIT_BRANCH")
	ctx.CommitSHA = os.Getenv("GIT_COMMIT")
	if len(ctx.CommitSHA) > 7 {
		ctx.CommitShort = ctx.CommitSHA[:7]
	}
	ctx.BuildID = os.Getenv("BUILD_NUMBER")
	ctx.BuildURL = os.Getenv("BUILD_URL")
	ctx.Workflow = os.Getenv("JOB_NAME")
	ctx.Actor = os.Getenv("BUILD_USER")
}

// detectCircleCI extracts CircleCI environment metadata
func (ctx *ExecutionContext) detectCircleCI() {
	ctx.Repository = os.Getenv("CIRCLE_REPOSITORY_URL")
	ctx.Branch = os.Getenv("CIRCLE_BRANCH")
	ctx.CommitSHA = os.Getenv("CIRCLE_SHA1")
	if len(ctx.CommitSHA) > 7 {
		ctx.CommitShort = ctx.CommitSHA[:7]
	}
	ctx.BuildID = os.Getenv("CIRCLE_BUILD_NUM")
	ctx.BuildURL = os.Getenv("CIRCLE_BUILD_URL")
	ctx.Workflow = os.Getenv("CIRCLE_JOB")
	ctx.Actor = os.Getenv("CIRCLE_USERNAME")
}

// detectTravisCI extracts Travis CI environment metadata
func (ctx *ExecutionContext) detectTravisCI() {
	ctx.Repository = os.Getenv("TRAVIS_REPO_SLUG")
	ctx.Branch = os.Getenv("TRAVIS_BRANCH")
	ctx.CommitSHA = os.Getenv("TRAVIS_COMMIT")
	if len(ctx.CommitSHA) > 7 {
		ctx.CommitShort = ctx.CommitSHA[:7]
	}
	ctx.BuildID = os.Getenv("TRAVIS_BUILD_ID")
	ctx.BuildURL = os.Getenv("TRAVIS_BUILD_WEB_URL")
	ctx.Workflow = os.Getenv("TRAVIS_JOB_NAME")
}

// detectLocal extracts local development environment metadata
func (ctx *ExecutionContext) detectLocal() {
	// Try to get git information from local repository
	// This would require git command execution (simplified for now)
	ctx.Repository = "local"
	ctx.Branch = "local"
	ctx.Actor = os.Getenv("USER")
	if ctx.Actor == "" {
		ctx.Actor = "unknown"
	}
}

// BuilderID generates SLSA builder identifier
func (ctx *ExecutionContext) BuilderID() string {
	switch ctx.CI {
	case CIProviderGitHubActions:
		return fmt.Sprintf("https://github.com/%s/.github/workflows/%s@%s",
			ctx.Repository, ctx.Workflow, ctx.Branch)
	case CIProviderGitLabCI:
		return fmt.Sprintf("%s/%s/-/pipelines/%s",
			ctx.OIDCIssuer, ctx.Repository, ctx.BuildID)
	case CIProviderJenkins:
		return fmt.Sprintf("jenkins://%s/job/%s/%s",
			os.Getenv("JENKINS_URL"), ctx.Workflow, ctx.BuildID)
	case CIProviderCircleCI:
		return fmt.Sprintf("https://circleci.com/%s/workflows/%s",
			ctx.Repository, ctx.BuildID)
	case CIProviderTravisCI:
		return fmt.Sprintf("https://travis-ci.org/%s/builds/%s",
			ctx.Repository, ctx.BuildID)
	default:
		return "https://simple-container.com/build/local"
	}
}

// HasOIDCToken returns true if OIDC token is available
func (ctx *ExecutionContext) HasOIDCToken() bool {
	return ctx.OIDCToken != ""
}

// IsKeylessSupportedCI returns true if CI supports keyless signing
func (ctx *ExecutionContext) IsKeylessSupportedCI() bool {
	return ctx.CI == CIProviderGitHubActions || ctx.CI == CIProviderGitLabCI
}
