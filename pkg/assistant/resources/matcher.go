package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	"github.com/simple-container-com/api/docs"
)

// SupportedResourcesResult represents the result of getting supported resources
type SupportedResourcesResult struct {
	Resources []string                    `json:"resources"`
	Providers []SupportedResourceProvider `json:"providers"`
}

// SupportedResourceProvider represents a provider with its resources
type SupportedResourceProvider struct {
	Name      string   `json:"name"`
	Resources []string `json:"resources"`
}

// ResourceMatcher maps detected resource types to Simple Container resource types
type ResourceMatcher struct {
	supportedResources []string
}

// NewResourceMatcher creates a new resource matcher using schema-based supported resources
func NewResourceMatcher() *ResourceMatcher {
	rm := &ResourceMatcher{}
	rm.loadSupportedResources()
	return rm
}

// loadSupportedResources loads supported resources from embedded schemas
func (rm *ResourceMatcher) loadSupportedResources() {
	supportedResources, err := GetSupportedResourcesFromSchemas(context.Background())
	if err != nil {
		// Fallback to basic resource types
		rm.supportedResources = []string{"aws-rds-postgres", "aws-rds-mysql", "mongodb-atlas", "s3-bucket", "gcs-bucket", "azure-blob"}
		return
	}

	// Extract resource types from all providers
	var resources []string
	for _, provider := range supportedResources.Providers {
		resources = append(resources, provider.Resources...)
	}

	if len(resources) == 0 {
		// Fallback if no resources found
		rm.supportedResources = []string{"aws-rds-postgres", "aws-rds-mysql", "mongodb-atlas", "s3-bucket", "gcs-bucket", "azure-blob"}
	} else {
		rm.supportedResources = resources
	}
}

// GetBestResourceType returns the best matching supported resource type for a detected resource
func (rm *ResourceMatcher) GetBestResourceType(detectedType string) string {
	detectedLower := strings.ToLower(detectedType)

	// Try to find the best match among supported resources
	for _, resourceType := range rm.supportedResources {
		resourceLower := strings.ToLower(resourceType)

		// Direct matches
		if strings.Contains(resourceLower, detectedLower) {
			return resourceType
		}

		// Specific mappings for common cases
		switch detectedLower {
		case "postgresql", "postgres":
			if strings.Contains(resourceLower, "postgres") {
				return resourceType
			}
		case "mongodb", "mongo":
			if strings.Contains(resourceLower, "mongo") {
				return resourceType
			}
		case "redis":
			if strings.Contains(resourceLower, "redis") || strings.Contains(resourceLower, "cache") {
				return resourceType
			}
		case "s3":
			if strings.Contains(resourceLower, "s3") {
				return resourceType
			}
		case "gcs", "google cloud storage":
			if strings.Contains(resourceLower, "gcs") || strings.Contains(resourceLower, "gcp") {
				return resourceType
			}
		case "azure":
			if strings.Contains(resourceLower, "azure") {
				return resourceType
			}
		case "mysql":
			if strings.Contains(resourceLower, "mysql") {
				return resourceType
			}
		}
	}

	// Generic fallback - create a reasonable resource name
	return fmt.Sprintf("%s-service", detectedLower)
}

// GetSupportedResourcesFromSchemas loads supported resources directly from embedded schemas
func GetSupportedResourcesFromSchemas(ctx context.Context) (*SupportedResourcesResult, error) {
	result := &SupportedResourcesResult{
		Resources: []string{},
		Providers: []SupportedResourceProvider{},
	}

	providerResourceMap := make(map[string][]string)

	// Walk through all provider schemas to build resource list
	err := fs.WalkDir(docs.EmbeddedSchemas, "schemas", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Continue on errors
		}

		if d.Name() == "index.json" && strings.Contains(path, "/") {
			// This is a provider index file
			content, err := fs.ReadFile(docs.EmbeddedSchemas, path)
			if err != nil {
				return nil // Continue on errors
			}

			var providerIndex struct {
				Provider  string `json:"provider"`
				Resources []struct {
					ResourceType string `json:"resourceType"`
					Type         string `json:"type"`
				} `json:"resources"`
			}

			if err := json.Unmarshal(content, &providerIndex); err != nil {
				return nil // Continue on errors
			}

			// Build resource list for this provider
			var providerResources []string
			for _, resource := range providerIndex.Resources {
				if resource.Type == "resource" && resource.ResourceType != "" {
					providerResources = append(providerResources, resource.ResourceType)
					result.Resources = append(result.Resources, resource.ResourceType)
				}
			}

			if len(providerResources) > 0 {
				providerResourceMap[providerIndex.Provider] = providerResources
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk schema directories: %w", err)
	}

	// Convert provider resource map to result format
	for provider, resources := range providerResourceMap {
		result.Providers = append(result.Providers, SupportedResourceProvider{
			Name:      provider,
			Resources: resources,
		})
	}

	// If no resources found, provide fallback
	if len(result.Resources) == 0 {
		fallbackResources := []string{"aws-rds-postgres", "aws-rds-mysql", "mongodb-atlas", "s3-bucket", "gcs-bucket", "azure-blob"}
		result.Resources = fallbackResources
		result.Providers = []SupportedResourceProvider{
			{
				Name:      "fallback",
				Resources: fallbackResources,
			},
		}
	}

	return result, nil
}

// GetAvailableResourceTypes returns a simple list of available resource types
func GetAvailableResourceTypes() []string {
	supportedResources, err := GetSupportedResourcesFromSchemas(context.Background())
	if err != nil {
		// Return basic resource types as fallback
		return []string{"aws-rds-postgres", "s3-bucket", "ecr-repository", "gcp-bucket", "gcp-cloudsql-postgres", "kubernetes-helm-postgres-operator"}
	}

	// Extract resource types from the result
	var resources []string
	for _, provider := range supportedResources.Providers {
		resources = append(resources, provider.Resources...)
	}

	// If no resources found, provide fallback
	if len(resources) == 0 {
		return []string{"aws-rds-postgres", "s3-bucket", "ecr-repository", "gcp-bucket", "gcp-cloudsql-postgres", "kubernetes-helm-postgres-operator"}
	}

	return resources
}
