package security

import (
	"os"
	"testing"
)

func TestDetectCI_GitHubActions(t *testing.T) {
	// Setup
	os.Setenv("GITHUB_ACTIONS", "true")
	os.Setenv("GITHUB_REPOSITORY", "test/repo")
	os.Setenv("GITHUB_SHA", "abc1234567890")
	os.Setenv("GITHUB_RUN_ID", "12345")
	os.Setenv("GITHUB_ACTOR", "testuser")
	defer clearTestEnv()

	// Execute
	ctx := &ExecutionContext{}
	err := ctx.DetectCI()

	// Assert
	if err != nil {
		t.Fatalf("DetectCI failed: %v", err)
	}

	if ctx.CI != CIProviderGitHubActions {
		t.Errorf("Expected CI provider %s, got %s", CIProviderGitHubActions, ctx.CI)
	}

	if !ctx.IsCI {
		t.Error("Expected IsCI to be true")
	}

	if ctx.Repository != "test/repo" {
		t.Errorf("Expected repository 'test/repo', got '%s'", ctx.Repository)
	}

	if ctx.CommitSHA != "abc1234567890" {
		t.Errorf("Expected commit SHA 'abc1234567890', got '%s'", ctx.CommitSHA)
	}

	if ctx.CommitShort != "abc1234" {
		t.Errorf("Expected short commit 'abc1234', got '%s'", ctx.CommitShort)
	}
}

func TestDetectCI_GitLabCI(t *testing.T) {
	// Setup
	os.Setenv("GITLAB_CI", "true")
	os.Setenv("CI_PROJECT_PATH", "group/project")
	os.Setenv("CI_COMMIT_SHA", "def9876543210")
	os.Setenv("CI_JOB_ID", "67890")
	defer clearTestEnv()

	// Execute
	ctx := &ExecutionContext{}
	err := ctx.DetectCI()

	// Assert
	if err != nil {
		t.Fatalf("DetectCI failed: %v", err)
	}

	if ctx.CI != CIProviderGitLabCI {
		t.Errorf("Expected CI provider %s, got %s", CIProviderGitLabCI, ctx.CI)
	}

	if ctx.Repository != "group/project" {
		t.Errorf("Expected repository 'group/project', got '%s'", ctx.Repository)
	}
}

func TestDetectCI_NoCI(t *testing.T) {
	// Setup - clear all CI env vars
	clearTestEnv()

	// Execute
	ctx := &ExecutionContext{}
	err := ctx.DetectCI()

	// Assert
	if err != nil {
		t.Fatalf("DetectCI failed: %v", err)
	}

	if ctx.CI != CIProviderNone {
		t.Errorf("Expected CI provider %s, got %s", CIProviderNone, ctx.CI)
	}

	if ctx.IsCI {
		t.Error("Expected IsCI to be false")
	}
}

func TestBuilderID_GitHubActions(t *testing.T) {
	// Setup
	ctx := &ExecutionContext{
		CI:         CIProviderGitHubActions,
		Repository: "org/repo",
		Workflow:   "ci.yml",
		Branch:     "main",
	}

	// Execute
	builderID := ctx.BuilderID()

	// Assert
	expected := "https://github.com/org/repo/.github/workflows/ci.yml@main"
	if builderID != expected {
		t.Errorf("Expected builder ID '%s', got '%s'", expected, builderID)
	}
}

func TestBuilderID_Local(t *testing.T) {
	// Setup
	ctx := &ExecutionContext{
		CI: CIProviderNone,
	}

	// Execute
	builderID := ctx.BuilderID()

	// Assert
	expected := "https://simple-container.com/build/local"
	if builderID != expected {
		t.Errorf("Expected builder ID '%s', got '%s'", expected, builderID)
	}
}

func TestHasOIDCToken(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected bool
	}{
		{
			name:     "With token",
			token:    "test-token",
			expected: true,
		},
		{
			name:     "Without token",
			token:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &ExecutionContext{
				OIDCToken: tt.token,
			}

			result := ctx.HasOIDCToken()
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsKeylessSupportedCI(t *testing.T) {
	tests := []struct {
		name     string
		provider CIProvider
		expected bool
	}{
		{
			name:     "GitHub Actions",
			provider: CIProviderGitHubActions,
			expected: true,
		},
		{
			name:     "GitLab CI",
			provider: CIProviderGitLabCI,
			expected: true,
		},
		{
			name:     "Jenkins",
			provider: CIProviderJenkins,
			expected: false,
		},
		{
			name:     "Local",
			provider: CIProviderNone,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &ExecutionContext{
				CI: tt.provider,
			}

			result := ctx.IsKeylessSupportedCI()
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func clearTestEnv() {
	// GitHub Actions
	os.Unsetenv("GITHUB_ACTIONS")
	os.Unsetenv("GITHUB_REPOSITORY")
	os.Unsetenv("GITHUB_SHA")
	os.Unsetenv("GITHUB_RUN_ID")
	os.Unsetenv("GITHUB_ACTOR")
	os.Unsetenv("GITHUB_REF")
	os.Unsetenv("GITHUB_WORKFLOW")

	// GitLab CI
	os.Unsetenv("GITLAB_CI")
	os.Unsetenv("CI_PROJECT_PATH")
	os.Unsetenv("CI_COMMIT_SHA")
	os.Unsetenv("CI_JOB_ID")
	os.Unsetenv("CI_COMMIT_BRANCH")

	// Jenkins
	os.Unsetenv("JENKINS_URL")
	os.Unsetenv("GIT_URL")
	os.Unsetenv("GIT_BRANCH")
	os.Unsetenv("GIT_COMMIT")
	os.Unsetenv("BUILD_NUMBER")

	// CircleCI
	os.Unsetenv("CIRCLECI")

	// Travis CI
	os.Unsetenv("TRAVIS")
}
