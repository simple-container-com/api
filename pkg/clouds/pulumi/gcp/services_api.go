package gcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/serviceusage/v1"

	"github.com/pkg/errors"
)

// servicesAPIClient interface for enabling GCP services
type servicesAPIClient interface {
	EnableService(ctx context.Context, authConfig any, apiName string) error
}

// defaultServicesAPIClient implements servicesAPIClient using real GCP APIs
type defaultServicesAPIClient struct{}

// EnableService enables a GCP service API
func (c *defaultServicesAPIClient) EnableService(ctx context.Context, authConfig any, apiName string) error {
	return enableServicesAPIImpl(ctx, authConfig, apiName)
}

// mockServicesAPIClient implements servicesAPIClient for testing
type mockServicesAPIClient struct {
	// enabledServices tracks which services have been "enabled"
	enabledServices map[string]bool
	// shouldFail controls whether to simulate failures
	shouldFail map[string]bool
}

// newMockServicesAPIClient creates a new mock client
func newMockServicesAPIClient() *mockServicesAPIClient {
	return &mockServicesAPIClient{
		enabledServices: make(map[string]bool),
		shouldFail:      make(map[string]bool),
	}
}

// EnableService mocks enabling a GCP service API
func (c *mockServicesAPIClient) EnableService(ctx context.Context, authConfig any, apiName string) error {
	if c.shouldFail[apiName] {
		return fmt.Errorf("mock failure for service %s", apiName)
	}
	c.enabledServices[apiName] = true
	return nil
}

// simulateFailure configures the mock to fail for a specific service
func (c *mockServicesAPIClient) simulateFailure(apiName string, shouldFail bool) {
	c.shouldFail[apiName] = shouldFail
}

// isServiceEnabled checks if a service was enabled
func (c *mockServicesAPIClient) isServiceEnabled(apiName string) bool {
	return c.enabledServices[apiName]
}

// Context key for services API client
type servicesAPIClientKey struct{}

// Global variable for dependency injection (used when context doesn't have client)
var globalServicesAPIClient servicesAPIClient = &defaultServicesAPIClient{}

// enableServicesAPI enables a GCP service API using the client from context, global, or default
func enableServicesAPI(ctx context.Context, authConfig any, apiName string) error {
	client := getServicesAPIClient(ctx)
	return client.EnableService(ctx, authConfig, apiName)
}

// getServicesAPIClient gets the services API client from context, global, or returns default
func getServicesAPIClient(ctx context.Context) servicesAPIClient {
	// First try to get from context
	if client, ok := ctx.Value(servicesAPIClientKey{}).(servicesAPIClient); ok {
		return client
	}
	// Fall back to global variable
	if globalServicesAPIClient != nil {
		return globalServicesAPIClient
	}
	// Final fallback to default
	return &defaultServicesAPIClient{}
}

// setGlobalServicesAPIClient sets the global services API client (for testing)
func setGlobalServicesAPIClient(client servicesAPIClient) {
	globalServicesAPIClient = client
}

// resetGlobalServicesAPIClient resets the global services API client to default
func resetGlobalServicesAPIClient() {
	globalServicesAPIClient = &defaultServicesAPIClient{}
}

// enableServicesAPIImpl is the original implementation moved here
func enableServicesAPIImpl(ctx context.Context, authConfig any, apiName string) error {
	svc, err := initServicesAPIClient(ctx, authConfig)
	if err != nil {
		return errors.Wrapf(err, "failed to init services API client")
	}

	if info, err := svc.Services.Get(apiName).Do(); err == nil {
		if info.State == "ENABLED" {
			// already enabled
			return nil
		}
	}
	op, err := svc.Services.Enable(apiName, &serviceusage.EnableServiceRequest{}).Do()
	if err != nil {
		return errors.Wrapf(err, "failed to enable %s", apiName)
	}

	// Handle immediate completion or operations that don't need polling
	if op.Done {
		// Operation completed immediately
		if op.Error != nil {
			return errors.Errorf("failed to enable API %q: %s", apiName, op.Error.Message)
		}
		return nil
	}

	// Poll for operation completion with improved error handling
	maxRetries := 60 // Maximum 60 seconds
	for i := 0; i < maxRetries; i++ {
		// Add context cancellation check
		select {
		case <-ctx.Done():
			return errors.Wrapf(ctx.Err(), "context cancelled while enabling API %q", apiName)
		default:
		}

		op, err = svc.Operations.Get(op.Name).Do()
		if err != nil {
			// Handle specific "DONE_OPERATION" error
			if strings.Contains(err.Error(), "DONE_OPERATION") {
				// Operation is already done, check final state
				if info, checkErr := svc.Services.Get(apiName).Do(); checkErr == nil {
					if info.State == "ENABLED" {
						return nil
					}
				}
				return errors.Wrapf(err, "API enablement operation completed with error for %q", apiName)
			}
			return errors.Wrapf(err, "failed to check operation status for API %q", apiName)
		}

		if op.Done {
			if op.Error != nil {
				return errors.Errorf("failed to enable API %q: %s", apiName, op.Error.Message)
			}
			return nil
		}

		time.Sleep(1 * time.Second)
	}

	return errors.Errorf("timeout waiting for API %q to be enabled after %d seconds", apiName, maxRetries)
}
