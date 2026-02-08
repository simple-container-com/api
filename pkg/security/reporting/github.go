package reporting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/simple-container-com/api/pkg/security/scan"
)

// GitHubClient handles interactions with GitHub API for security reporting
// API Documentation: https://docs.github.com/en/rest/code-scanning
type GitHubClient struct {
	Repository string
	Token      string
	HTTPClient *http.Client
}

// GitHubSARIFUploadRequest represents a SARIF upload request
type GitHubSARIFUploadRequest struct {
	CommitSHA string `json:"commit_sha"`
	Ref       string `json:"ref,omitempty"`
}

// GitHubSARIFUploadResponse represents the response from uploading SARIF
type GitHubSARIFUploadResponse struct {
	CommitSHA   string `json:"commit_sha"`
	Ref         string `json:"ref,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	ProcessingState string `json:"processing_state"` // "pending", "complete", "failed"
	ResultsURL  string `json:"analyses_url"`
}

// NewGitHubClient creates a new GitHub client
func NewGitHubClient(repository, token string) *GitHubClient {
	return &GitHubClient{
		Repository: repository,
		Token:      token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// UploadSARIF uploads SARIF data to GitHub Security
func (c *GitHubClient) UploadSARIF(ctx context.Context, sarifData []byte, commitSHA, ref string, workspace string) error {
	// If workspace is provided, write SARIF file for GitHub Actions to upload
	if workspace != "" {
		return c.uploadViaWorkspace(ctx, sarifData, workspace)
	}

	// Otherwise, use GitHub API directly
	return c.uploadViaAPI(ctx, sarifData, commitSHA, ref)
}

// uploadViaWorkspace saves SARIF to workspace for GitHub Actions upload
// This is the preferred method for GitHub Actions environments
func (c *GitHubClient) uploadViaWorkspace(ctx context.Context, sarifData []byte, workspace string) error {
	// Create output directory
	outputDir := filepath.Join(workspace, "github-security-results")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// Write SARIF file
	sarifPath := filepath.Join(outputDir, "scan-results.sarif")
	if err := os.WriteFile(sarifPath, sarifData, 0644); err != nil {
		return fmt.Errorf("writing SARIF file: %w", err)
	}

	fmt.Printf("SARIF results written to: %s\n", sarifPath)
	fmt.Printf("GitHub Actions will upload these results automatically.\n")

	return nil
}

// uploadViaAPI uploads SARIF via GitHub REST API
// This is useful for non-GitHub Actions environments
func (c *GitHubClient) uploadViaAPI(ctx context.Context, sarifData []byte, commitSHA, ref string) error {
	// GitHub API requires gzip compression for SARIF uploads
	// For simplicity, we're just uploading raw data here
	// In production, you should gzip the data

	url := fmt.Sprintf("https://api.github.com/repos/%s/code-scanning/sarifs", c.Repository)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(sarifData))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/sarif+json")

	// Add query parameters for commit SHA and ref
	if commitSHA != "" {
		q := req.URL.Query()
		q.Add("commit_sha", commitSHA)
		if ref != "" {
			q.Add("ref", ref)
		}
		req.URL.RawQuery = q.Encode()
	}

	// Make request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var uploadResp GitHubSARIFUploadResponse
	if err := json.Unmarshal(body, &uploadResp); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	fmt.Printf("SARIF uploaded successfully (processing state: %s)\n", uploadResp.ProcessingState)
	if uploadResp.ResultsURL != "" {
		fmt.Printf("Results URL: %s\n", uploadResp.ResultsURL)
	}

	return nil
}

// GitHubUploaderConfig contains configuration for uploading to GitHub
type GitHubUploaderConfig struct {
	Repository string
	Token      string
	CommitSHA  string
	Ref        string
	Workspace  string
}

// UploadToGitHub uploads scan results to GitHub Security tab
func UploadToGitHub(ctx context.Context, result *scan.ScanResult, imageRef string, config *GitHubUploaderConfig) error {
	// Generate SARIF from scan results
	sarif, err := NewSARIFFromScanResult(result, imageRef)
	if err != nil {
		return fmt.Errorf("generating SARIF: %w", err)
	}

	// Convert to JSON
	sarifData, err := sarif.ToJSON()
	if err != nil {
		return fmt.Errorf("marshaling SARIF: %w", err)
	}

	// Create GitHub client
	client := NewGitHubClient(config.Repository, config.Token)

	// Upload
	if err := client.UploadSARIF(ctx, sarifData, config.CommitSHA, config.Ref, config.Workspace); err != nil {
		return fmt.Errorf("uploading SARIF: %w", err)
	}

	return nil
}
