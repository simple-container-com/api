package reporting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ProductType int    `json:"prod_type"`
}

// DefectDojoTest represents a DefectDojo test
type DefectDojoTest struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Engagement  int    `json:"engagement"`
	TestType    int    `json:"test_type"`
	TargetStart string `json:"target_start"`
	TargetEnd   string `json:"target_end"`
}

// ImportScanResponse represents the response from importing a scan
type ImportScanResponse struct {
	ID               int `json:"id"`
	Test             int `json:"test"`
	Product          int `json:"product"`
	Engagement       int `json:"engagement"`
	NumberOfFindings int `json:"number_of_findings"`
}

// NewDefectDojoClient creates a new DefectDojo client
func NewDefectDojoClient(baseURL, apiKey string) *DefectDojoClient {
	return &DefectDojoClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
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

	sarifData, err := NewSARIFFromScanResult(result, imageRef)
	if err != nil {
		return nil, fmt.Errorf("encoding SARIF: %w", err)
	}

	// Import scan
	importResp, err := c.importScan(ctx, engagementID, sarifData, imageRef, config)
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

	if config.EngagementName != "" {
		lookupConfig, err := c.engagementLookupConfig(ctx, config)
		if err != nil {
			return 0, fmt.Errorf("resolving engagement lookup config: %w", err)
		}
		engagements, err := c.listEngagements(ctx, lookupConfig)
		if err != nil {
			return 0, fmt.Errorf("listing engagements: %w", err)
		}
		if len(engagements) > 0 {
			return engagements[0].ID, nil
		}
	}

	// If auto-create is enabled, create engagement
	if config.AutoCreate {
		return c.createEngagement(ctx, config)
	}

	return 0, fmt.Errorf("engagement ID %d not found and auto-create is disabled", config.EngagementID)
}

func (c *DefectDojoClient) engagementLookupConfig(ctx context.Context, config *DefectDojoUploaderConfig) (*DefectDojoUploaderConfig, error) {
	if config == nil {
		return nil, nil
	}

	lookup := *config
	if lookup.ProductID > 0 || lookup.ProductName == "" {
		return &lookup, nil
	}

	products, err := c.listProducts(ctx, lookup.ProductName)
	if err != nil {
		return nil, err
	}
	if len(products) > 0 {
		lookup.ProductID = products[0].ID
	}

	return &lookup, nil
}

func (c *DefectDojoClient) listEngagements(ctx context.Context, config *DefectDojoUploaderConfig) ([]DefectDojoEngagement, error) {
	values := url.Values{}
	if config.EngagementName != "" {
		values.Set("name", config.EngagementName)
	}
	if config.ProductID > 0 {
		values.Set("product", fmt.Sprintf("%d", config.ProductID))
	}

	req, err := c.createRequest(ctx, "GET", "/api/v2/engagements/?"+values.Encode(), nil)
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
		Results []DefectDojoEngagement `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return response.Results, nil
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
		"name":            config.EngagementName,
		"product":         productID,
		"engagement_type": "CI/CD",
		"target_start":    time.Now().Format("2006-01-02"),
		"target_end":      time.Now().Add(24 * time.Hour).Format("2006-01-02"),
		"status":          "In Progress",
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
	values := url.Values{}
	if name != "" {
		values.Set("name", name)
	}

	req, err := c.createRequest(ctx, "GET", "/api/v2/products/?"+values.Encode(), nil)
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
func (c *DefectDojoClient) importScan(ctx context.Context, engagementID int, sarifData []byte, imageRef string, config *DefectDojoUploaderConfig) (*ImportScanResponse, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	fields := map[string]string{
		"scan_type":          "SARIF",
		"engagement":         fmt.Sprintf("%d", engagementID),
		"scan_date":          time.Now().Format("2006-01-02"),
		"minimum_severity":   "Info",
		"active":             "true",
		"verified":           "false",
		"close_old_findings": "true",
		"test_title":         c.testTitle(config, imageRef),
	}
	if config.Environment != "" {
		fields["environment"] = config.Environment
	}
	if len(config.Tags) > 0 {
		fields["tags"] = strings.Join(config.Tags, ",")
	}
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			return nil, fmt.Errorf("writing multipart field %s: %w", name, err)
		}
	}

	fileWriter, err := writer.CreateFormFile("file", "scan-results.sarif")
	if err != nil {
		return nil, fmt.Errorf("creating multipart file: %w", err)
	}
	if _, err := fileWriter.Write(sarifData); err != nil {
		return nil, fmt.Errorf("writing SARIF payload: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("closing multipart body: %w", err)
	}

	req, err := c.createRequest(ctx, "POST", "/api/v2/import-scan/", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	importResp := decodeImportScanResponse(respBody)
	importResp.Engagement = engagementID
	if err := c.enrichImportScanResponse(ctx, importResp, engagementID, c.testTitle(config, imageRef)); err != nil {
		return nil, fmt.Errorf("enriching import response: %w", err)
	}

	return importResp, nil
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

func (c *DefectDojoClient) testTitle(config *DefectDojoUploaderConfig, imageRef string) string {
	title := config.TestType
	if title == "" {
		title = "Container Scan"
	}

	// Use a short image reference for the test title — the full ECR URL
	// with digest is too long for DefectDojo's UI. Extract the image name
	// and a short digest suffix.
	short := imageRef
	if idx := strings.LastIndex(imageRef, "/"); idx >= 0 {
		short = imageRef[idx+1:]
	}
	// Truncate digest: "aimeteor-ecr@sha256:737107dc..." → "aimeteor-ecr@sha256:737107dc"
	if idx := strings.Index(short, "@sha256:"); idx >= 0 && len(short) > idx+16 {
		short = short[:idx+16]
	}

	return fmt.Sprintf("%s - %s", title, short)
}

func decodeImportScanResponse(data []byte) *ImportScanResponse {
	resp := &ImportScanResponse{}
	if len(bytes.TrimSpace(data)) == 0 {
		return resp
	}

	_ = json.Unmarshal(data, resp)

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return resp
	}

	if resp.ID == 0 {
		resp.ID = intValue(raw["id"])
	}
	if resp.Test == 0 {
		resp.Test = intValue(raw["test"])
	}
	if resp.Product == 0 {
		resp.Product = intValue(raw["product"])
	}
	if resp.Engagement == 0 {
		resp.Engagement = intValue(raw["engagement"])
	}
	if resp.NumberOfFindings == 0 {
		resp.NumberOfFindings = intValue(raw["number_of_findings"])
	}
	if resp.NumberOfFindings == 0 {
		resp.NumberOfFindings = intValue(raw["findings_count"])
	}
	if resp.NumberOfFindings == 0 {
		if stats, ok := raw["statistics"].(map[string]interface{}); ok {
			for _, key := range []string{"after_count", "count", "total", "new_findings"} {
				if value := intValue(stats[key]); value > 0 {
					resp.NumberOfFindings = value
					break
				}
			}
		}
	}

	return resp
}

func intValue(value interface{}) int {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		v, err := typed.Int64()
		if err == nil {
			return int(v)
		}
	case string:
		v, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return v
		}
	case map[string]interface{}:
		return intValue(typed["id"])
	}

	return 0
}

func (c *DefectDojoClient) enrichImportScanResponse(ctx context.Context, resp *ImportScanResponse, engagementID int, title string) error {
	if resp == nil {
		return nil
	}
	if resp.Engagement == 0 {
		resp.Engagement = engagementID
	}

	if resp.Test == 0 {
		tests, err := c.listTests(ctx, engagementID)
		if err == nil {
			if test := findLatestTestByTitle(tests, title); test != nil {
				resp.Test = test.ID
			}
		}
	}

	if resp.NumberOfFindings == 0 {
		count, err := c.countFindings(ctx, resp.Test, engagementID)
		if err == nil {
			resp.NumberOfFindings = count
		}
	}

	return nil
}

func (c *DefectDojoClient) listTests(ctx context.Context, engagementID int) ([]DefectDojoTest, error) {
	req, err := c.createRequest(ctx, "GET", fmt.Sprintf("/api/v2/tests/?engagement=%d", engagementID), nil)
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
		Results []DefectDojoTest `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return response.Results, nil
}

func findLatestTestByTitle(tests []DefectDojoTest, title string) *DefectDojoTest {
	var match *DefectDojoTest
	for i := range tests {
		if tests[i].Title != title {
			continue
		}
		if match == nil || tests[i].ID > match.ID {
			match = &tests[i]
		}
	}
	return match
}

func (c *DefectDojoClient) countFindings(ctx context.Context, testID, engagementID int) (int, error) {
	if testID > 0 {
		count, err := c.countFindingsByQuery(ctx, fmt.Sprintf("/api/v2/findings/?test=%d", testID))
		if err == nil {
			return count, nil
		}
	}

	if engagementID == 0 {
		return 0, nil
	}

	return c.countFindingsByQuery(ctx, fmt.Sprintf("/api/v2/findings/?test__engagement=%d", engagementID))
}

func (c *DefectDojoClient) countFindingsByQuery(ctx context.Context, path string) (int, error) {
	req, err := c.createRequest(ctx, "GET", path, nil)
	if err != nil {
		return 0, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var response struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return 0, fmt.Errorf("decoding response: %w", err)
	}

	return response.Count, nil
}

// DefectDojoUploaderConfig contains configuration for uploading to DefectDojo
type DefectDojoUploaderConfig struct {
	EngagementID   int
	EngagementName string
	ProductID      int
	ProductName    string
	TestType       string
	Tags           []string
	Environment    string
	AutoCreate     bool
}
