package reporting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/simple-container-com/api/pkg/security/scan"
)

// DefectDojoClient handles interactions with DefectDojo API
// API Documentation: https://defectdojo.github.io/django-DefectDojo/rest/api/
type DefectDojoClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// DefectDojoEngagement represents a DefectDojo engagement
type DefectDojoEngagement struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Product     int    `json:"product"`
	TargetStart string `json:"target_start"`
	TargetEnd   string `json:"target_end"`
	Status      string `json:"status"`
}

// DefectDojoProduct represents a DefectDojo product
type DefectDojoProduct struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	ProductType  int    `json:"prod_type"`
}

// DefectDojoTest represents a DefectDojo test
type DefectDojoTest struct {
	ID              int    `json:"id"`
	Title           string `json:"title"`
	Engagement      int    `json:"engagement"`
	TestType        int    `json:"test_type"`
	TargetStart     string `json:"target_start"`
	TargetEnd       string `json:"target_end"`
}

// ImportScanRequest represents a request to import scan results
type ImportScanRequest struct {
	ScanType        string            `json:"scan_type"`
	EngagementID    int               `json:"engagement"`
	ProductID       int               `json:"product,omitempty"`
	SHA256          string            `json:"sha256,omitempty"`
	Branch          string            `json:"branch,omitempty"`
	FileName        string            `json:"file_name,omitempty"`
	File            []byte            `json:"file,omitempty"`
	ScanDate        string            `json:"scan_date,omitempty"`
	MinimumSeverity string            `json:"minimum_severity,omitempty"`
	Active          bool              `json:"active,omitempty"`
	Verified        bool              `json:"verified,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	Environment     string            `json:"environment,omitempty"`
}

// ImportScanResponse represents the response from importing a scan
type ImportScanResponse struct {
	ID            int    `json:"id"`
	Test          int    `json:"test"`
	Product       int    `json:"product"`
	Engagement    int    `json:"engagement"`
	NumberOfFindings int `json:"number_of_findings"`
}

// NewDefectDojoClient creates a new DefectDojo client
func NewDefectDojoClient(baseURL, apiKey string) *DefectDojoClient {
	return &DefectDojoClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// UploadScanResult uploads scan results to DefectDojo
func (c *DefectDojoClient) UploadScanResult(ctx context.Context, result *scan.ScanResult, imageRef string, config *DefectDojoUploaderConfig) (*ImportScanResponse, error) {
	// Get or create engagement
	engagementID, err := c.getOrCreateEngagement(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("getting engagement: %w", err)
	}

	// Convert scan result to DefectDojo format
	scanData := c.convertScanToDefectDojoFormat(result, imageRef)

	// Import scan
	importResp, err := c.importScan(ctx, engagementID, scanData, config)
	if err != nil {
		return nil, fmt.Errorf("importing scan: %w", err)
	}

	return importResp, nil
}

// getOrCreateEngagement gets an existing engagement or creates a new one
func (c *DefectDojoClient) getOrCreateEngagement(ctx context.Context, config *DefectDojoUploaderConfig) (int, error) {
	// If engagement ID is provided, verify it exists
	if config.EngagementID > 0 {
		exists, err := c.engagementExists(ctx, config.EngagementID)
		if err != nil {
			return 0, fmt.Errorf("checking engagement existence: %w", err)
		}
		if exists {
			return config.EngagementID, nil
		}
	}

	// If auto-create is enabled, create engagement
	if config.AutoCreate {
		return c.createEngagement(ctx, config)
	}

	return 0, fmt.Errorf("engagement ID %d not found and auto-create is disabled", config.EngagementID)
}

// engagementExists checks if an engagement exists
func (c *DefectDojoClient) engagementExists(ctx context.Context, engagementID int) (bool, error) {
	req, err := c.createRequest(ctx, "GET", fmt.Sprintf("/api/v2/engagements/%d/", engagementID), nil)
	if err != nil {
		return false, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	body, _ := io.ReadAll(resp.Body)
	return false, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
}

// createEngagement creates a new engagement
func (c *DefectDojoClient) createEngagement(ctx context.Context, config *DefectDojoUploaderConfig) (int, error) {
	// First get or create product
	productID, err := c.getOrCreateProduct(ctx, config)
	if err != nil {
		return 0, fmt.Errorf("getting product: %w", err)
	}

	// Create engagement
	engagement := map[string]interface{}{
		"name":        config.EngagementName,
		"product":     productID,
		"engagement_type": 1, // CI/CD
		"target_start": time.Now().Format("2006-01-02"),
		"target_end":   time.Now().Add(24 * time.Hour).Format("2006-01-02"),
		"status":       "In Progress",
	}

	body, err := json.Marshal(engagement)
	if err != nil {
		return 0, fmt.Errorf("marshaling engagement: %w", err)
	}

	req, err := c.createRequest(ctx, "POST", "/api/v2/engagements/", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	var createdEngagement DefectDojoEngagement
	if err := json.NewDecoder(resp.Body).Decode(&createdEngagement); err != nil {
		return 0, fmt.Errorf("decoding response: %w", err)
	}

	return createdEngagement.ID, nil
}

// getOrCreateProduct gets an existing product or creates a new one
func (c *DefectDojoClient) getOrCreateProduct(ctx context.Context, config *DefectDojoUploaderConfig) (int, error) {
	if config.ProductID > 0 {
		return config.ProductID, nil
	}

	// Try to find product by name
	products, err := c.listProducts(ctx, config.ProductName)
	if err != nil {
		return 0, err
	}

	if len(products) > 0 {
		return products[0].ID, nil
	}

	// Create new product
	return c.createProduct(ctx, config)
}

// listProducts lists products by name
func (c *DefectDojoClient) listProducts(ctx context.Context, name string) ([]DefectDojoProduct, error) {
	req, err := c.createRequest(ctx, "GET", "/api/v2/products/?name="+name, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var response struct {
		Results []DefectDojoProduct `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return response.Results, nil
}

// createProduct creates a new product
func (c *DefectDojoClient) createProduct(ctx context.Context, config *DefectDojoUploaderConfig) (int, error) {
	product := map[string]interface{}{
		"name":        config.ProductName,
		"description": "Auto-created by Simple Container Security",
		"prod_type":   1, // Default product type
	}

	body, err := json.Marshal(product)
	if err != nil {
		return 0, fmt.Errorf("marshaling product: %w", err)
	}

	req, err := c.createRequest(ctx, "POST", "/api/v2/products/", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	var createdProduct DefectDojoProduct
	if err := json.NewDecoder(resp.Body).Decode(&createdProduct); err != nil {
		return 0, fmt.Errorf("decoding response: %w", err)
	}

	return createdProduct.ID, nil
}

// importScan imports scan results into DefectDojo
func (c *DefectDojoClient) importScan(ctx context.Context, engagementID int, scanData map[string]interface{}, config *DefectDojoUploaderConfig) (*ImportScanResponse, error) {
	// Create import scan request
	importReq := ImportScanRequest{
		ScanType:     "SARIF", // We'll convert to SARIF format
		EngagementID: engagementID,
		ScanDate:     time.Now().Format("2006-01-02"),
		MinimumSeverity: "Info",
		Active:       true,
		Verified:     false,
		Tags:         config.Tags,
		Environment:  config.Environment,
	}

	body, err := json.Marshal(importReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling import request: %w", err)
	}

	// Use multipart form data for file upload
	// For simplicity, we're just sending JSON here
	// In a production implementation, you'd use multipart with the actual SARIF file
	req, err := c.createRequest(ctx, "POST", "/api/v2/import-scan/", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	var importResp ImportScanResponse
	if err := json.NewDecoder(resp.Body).Decode(&importResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &importResp, nil
}

// convertScanToDefectDojoFormat converts scan results to DefectDojo format
func (c *DefectDojoClient) convertScanToDefectDojoFormat(result *scan.ScanResult, imageRef string) map[string]interface{} {
	return map[string]interface{}{
		"image_ref":    imageRef,
		"image_digest": result.ImageDigest,
		"tool":         string(result.Tool),
		"scan_date":    result.ScannedAt.Format(time.RFC3339),
		"summary":      result.Summary,
	}
}

// createRequest creates an HTTP request with authentication
func (c *DefectDojoClient) createRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	url := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Token "+c.APIKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// DefectDojoUploaderConfig contains configuration for uploading to DefectDojo
type DefectDojoUploaderConfig struct {
	EngagementID    int
	EngagementName  string
	ProductID       int
	ProductName     string
	TestType        string
	Tags            []string
	Environment     string
	AutoCreate      bool
}
