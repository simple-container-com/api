package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	// Core API package for registration maps and configuration structs
	"github.com/simple-container-com/api/pkg/api"
	// Import cloud provider packages directly to access their CloudExtras types
	"github.com/simple-container-com/api/pkg/clouds/aws"
	// Import all cloud provider packages to trigger init() functions
	_ "github.com/simple-container-com/api/pkg/clouds/aws"
	_ "github.com/simple-container-com/api/pkg/clouds/cloudflare"
	_ "github.com/simple-container-com/api/pkg/clouds/compose"
	_ "github.com/simple-container-com/api/pkg/clouds/discord"
	_ "github.com/simple-container-com/api/pkg/clouds/docker"
	_ "github.com/simple-container-com/api/pkg/clouds/fs"
	_ "github.com/simple-container-com/api/pkg/clouds/gcloud"
	_ "github.com/simple-container-com/api/pkg/clouds/github"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	_ "github.com/simple-container-com/api/pkg/clouds/mongodb"
	_ "github.com/simple-container-com/api/pkg/clouds/slack"
	_ "github.com/simple-container-com/api/pkg/clouds/telegram"
)

// ResourceDefinition holds metadata about a resource struct
type ResourceDefinition struct {
	Name         string      `json:"name"`
	Type         string      `json:"type"`
	Provider     string      `json:"provider"`
	Description  string      `json:"description"`
	GoPackage    string      `json:"goPackage"`
	GoStruct     string      `json:"goStruct"`
	ResourceType string      `json:"resourceType,omitempty"`
	TemplateType string      `json:"templateType,omitempty"`
	Schema       interface{} `json:"schema"`
}

// SchemaGenerator generates JSON schemas for Simple Container resources
type SchemaGenerator struct {
	outputDir string
}

func NewSchemaGenerator(outputDir string) *SchemaGenerator {
	return &SchemaGenerator{outputDir: outputDir}
}

// marshalJSONDeterministic marshals data to JSON with deterministic ordering
func marshalJSONDeterministic(data interface{}) ([]byte, error) {
	// First, normalize the data structure to ensure consistent ordering
	normalized := normalizeForJSON(data)

	// Use a buffer to build JSON with consistent formatting
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(normalized); err != nil {
		return nil, err
	}

	// Remove the trailing newline added by Encode
	result := buf.Bytes()
	if len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}

	return result, nil
}

// normalizeForJSON recursively sorts all maps and slices for deterministic output
func normalizeForJSON(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		// Create a new map and process keys in sorted order
		normalized := make(map[string]interface{})
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			normalized[k] = normalizeForJSON(v[k])
		}
		return normalized

	case []interface{}:
		// Process each element in the slice
		normalized := make([]interface{}, len(v))
		for i, item := range v {
			normalized[i] = normalizeForJSON(item)
		}
		return normalized

	case []string:
		// Sort string slices for consistency
		normalized := make([]string, len(v))
		copy(normalized, v)
		sort.Strings(normalized)
		return normalized

	default:
		// For primitive types, return as-is
		return v
	}
}

func (sg *SchemaGenerator) Generate() error {
	// Dynamically discover all registered resources
	resources, err := sg.discoverRegisteredResources()
	if err != nil {
		return fmt.Errorf("failed to discover registered resources: %w", err)
	}

	// Generate configuration file schemas
	configSchemas, err := sg.generateConfigurationSchemas()
	if err != nil {
		return fmt.Errorf("failed to generate configuration schemas: %w", err)
	}

	// Generate CloudExtras schemas for different providers
	cloudExtrasSchemas, err := sg.generateCloudExtrasSchemas()
	if err != nil {
		return fmt.Errorf("failed to generate CloudExtras schemas: %w", err)
	}

	// Combine resources, configuration schemas, and CloudExtras schemas for directory creation
	allSchemas := append(resources, configSchemas...)
	allSchemas = append(allSchemas, cloudExtrasSchemas...)

	// Create output directory structure based on all discovered providers
	if err := sg.createDirectories(allSchemas); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Generate schema files for resources
	for _, resource := range resources {
		if err := sg.generateSchemaFile(resource); err != nil {
			return fmt.Errorf("failed to generate schema for %s: %w", resource.Name, err)
		}
	}

	// Generate schema files for configuration files
	for _, configSchema := range configSchemas {
		if err := sg.generateSchemaFile(configSchema); err != nil {
			return fmt.Errorf("failed to generate config schema for %s: %w", configSchema.Name, err)
		}
	}

	// Generate schema files for CloudExtras
	for _, cloudExtrasSchema := range cloudExtrasSchemas {
		if err := sg.generateSchemaFile(cloudExtrasSchema); err != nil {
			return fmt.Errorf("failed to generate CloudExtras schema for %s: %w", cloudExtrasSchema.Name, err)
		}
	}

	// Generate index files
	if err := sg.generateIndexFiles(allSchemas); err != nil {
		return fmt.Errorf("failed to generate index files: %w", err)
	}

	totalSchemas := len(resources) + len(configSchemas) + len(cloudExtrasSchemas)
	fmt.Printf("Successfully generated JSON Schema files for %d total schemas (%d resources, %d config schemas, %d CloudExtras schemas) in %s\n",
		totalSchemas, len(resources), len(configSchemas), len(cloudExtrasSchemas), sg.outputDir)
	return nil
}

// discoverRegisteredResources dynamically discovers all registered resources from the DI framework
func (sg *SchemaGenerator) discoverRegisteredResources() ([]ResourceDefinition, error) {
	var resources []ResourceDefinition

	// Get all registered provider configurations (resources, templates, auth)
	providerConfigs := api.GetRegisteredProviderConfigs()

	// Sort resource types for consistent ordering
	var resourceTypes []string
	for resourceType := range providerConfigs {
		resourceTypes = append(resourceTypes, resourceType)
	}
	sort.Strings(resourceTypes)

	for _, resourceType := range resourceTypes {
		readerFunc := providerConfigs[resourceType]
		// Create a dummy config to call the reader function and get the struct type
		dummyConfig := &api.Config{Config: make(map[string]interface{})}
		result, err := readerFunc(dummyConfig)
		if err != nil {
			// Skip resources that can't be processed with empty config
			continue
		}

		if result.Config != nil {
			structType := reflect.TypeOf(result.Config)
			if structType.Kind() == reflect.Ptr {
				structType = structType.Elem()
			}

			provider := sg.guessProviderFromResourceType(resourceType)
			resourceDef := ResourceDefinition{
				Name:         structType.Name(),
				Type:         sg.guessResourceType(resourceType),
				Provider:     provider,
				Description:  sg.generateDescription(resourceType, structType.Name()),
				GoPackage:    fmt.Sprintf("pkg/clouds/%s/", provider),
				GoStruct:     structType.Name(),
				ResourceType: resourceType,
				Schema:       structType,
			}

			// Set template type for templates
			if strings.Contains(resourceType, "template") || strings.Contains(resourceType, "fargate") || strings.Contains(resourceType, "lambda") || strings.Contains(resourceType, "cloudrun") {
				resourceDef.TemplateType = resourceType
			}

			resources = append(resources, resourceDef)
		}
	}

	// Get all registered provisioner field configurations
	provisionerConfigs := api.GetRegisteredProvisionerFieldConfigs()

	// Sort provisioner resource types for consistent ordering
	var provisionerResourceTypes []string
	for resourceType := range provisionerConfigs {
		provisionerResourceTypes = append(provisionerResourceTypes, resourceType)
	}
	sort.Strings(provisionerResourceTypes)

	for _, resourceType := range provisionerResourceTypes {
		readerFunc := provisionerConfigs[resourceType]
		dummyConfig := &api.Config{Config: make(map[string]interface{})}
		result, err := readerFunc(dummyConfig)
		if err != nil {
			continue
		}

		if result.Config != nil {
			structType := reflect.TypeOf(result.Config)
			if structType.Kind() == reflect.Ptr {
				structType = structType.Elem()
			}

			provider := sg.guessProviderFromResourceType(resourceType)
			resourceDef := ResourceDefinition{
				Name:         structType.Name(),
				Type:         "provisioner",
				Provider:     provider,
				Description:  sg.generateDescription(resourceType, structType.Name()),
				GoPackage:    fmt.Sprintf("pkg/clouds/%s/", provider),
				GoStruct:     structType.Name(),
				ResourceType: resourceType,
				Schema:       structType,
			}

			resources = append(resources, resourceDef)
		}
	}

	// Sort all resources by name for consistent ordering
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Name < resources[j].Name
	})

	fmt.Printf("Discovered %d registered resources dynamically\n", len(resources))
	return resources, nil
}

// generateConfigurationSchemas creates schemas for core configuration files
func (sg *SchemaGenerator) generateConfigurationSchemas() ([]ResourceDefinition, error) {
	var configSchemas []ResourceDefinition

	// Client configuration schemas (client.yaml)
	configSchemas = append(configSchemas, []ResourceDefinition{
		{
			Name:         "ClientDescriptor",
			Type:         "configuration",
			Provider:     "core",
			Description:  "Simple Container client.yaml configuration file schema",
			GoPackage:    "pkg/api/client.go",
			GoStruct:     "ClientDescriptor",
			ResourceType: "client-config",
			Schema:       reflect.TypeOf(api.ClientDescriptor{}),
		},
		{
			Name:         "StackConfigCompose",
			Type:         "configuration",
			Provider:     "core",
			Description:  "Simple Container cloud-compose stack configuration schema",
			GoPackage:    "pkg/api/client.go",
			GoStruct:     "StackConfigCompose",
			ResourceType: "stack-config-compose",
			Schema:       reflect.TypeOf(api.StackConfigCompose{}),
		},
		{
			Name:         "StackConfigSingleImage",
			Type:         "configuration",
			Provider:     "core",
			Description:  "Simple Container single-image stack configuration schema",
			GoPackage:    "pkg/api/client.go",
			GoStruct:     "StackConfigSingleImage",
			ResourceType: "stack-config-single-image",
			Schema:       reflect.TypeOf(api.StackConfigSingleImage{}),
		},
		{
			Name:         "StackConfigStatic",
			Type:         "configuration",
			Provider:     "core",
			Description:  "Simple Container static site stack configuration schema",
			GoPackage:    "pkg/api/client.go",
			GoStruct:     "StackConfigStatic",
			ResourceType: "stack-config-static",
			Schema:       reflect.TypeOf(api.StackConfigStatic{}),
		},
	}...)

	// Server configuration schemas (server.yaml)
	configSchemas = append(configSchemas, []ResourceDefinition{
		{
			Name:         "ServerDescriptor",
			Type:         "configuration",
			Provider:     "core",
			Description:  "Simple Container server.yaml configuration file schema",
			GoPackage:    "pkg/api/server.go",
			GoStruct:     "ServerDescriptor",
			ResourceType: "server-config",
			Schema:       reflect.TypeOf(api.ServerDescriptor{}),
		},
		{
			Name:         "ConfigFile",
			Type:         "configuration",
			Provider:     "core",
			Description:  "Simple Container project configuration file schema",
			GoPackage:    "pkg/api/config.go",
			GoStruct:     "ConfigFile",
			ResourceType: "project-config",
			Schema:       reflect.TypeOf(api.ConfigFile{}),
		},
	}...)

	fmt.Printf("Generated %d configuration file schemas\n", len(configSchemas))
	return configSchemas, nil
}

// generateCloudExtrasSchemas creates schemas for CloudExtras structures from different providers
func (sg *SchemaGenerator) generateCloudExtrasSchemas() ([]ResourceDefinition, error) {
	var cloudExtrasSchemas []ResourceDefinition

	// Use the actual CloudExtras types from imported packages
	// This ensures we use the single source of truth from each package

	// AWS CloudExtras - use the actual type from aws package
	cloudExtrasSchemas = append(cloudExtrasSchemas, ResourceDefinition{
		Name:         "AWSCloudExtras",
		Type:         "cloudextras",
		Provider:     "aws",
		Description:  "AWS-specific cloudExtras configuration for Simple Container deployments including Lambda scheduling, security groups, and load balancer settings",
		GoPackage:    "pkg/clouds/aws/",
		GoStruct:     "CloudExtras",
		ResourceType: "aws-cloudextras",
		Schema:       reflect.TypeOf(aws.CloudExtras{}),
	})

	// Kubernetes CloudExtras - use the actual type from k8s package
	cloudExtrasSchemas = append(cloudExtrasSchemas, ResourceDefinition{
		Name:         "KubernetesCloudExtras",
		Type:         "cloudextras",
		Provider:     "kubernetes",
		Description:  "Kubernetes-specific cloudExtras configuration for Simple Container deployments including probes, VPA, affinity, node selection, and pod disruption budgets",
		GoPackage:    "pkg/clouds/k8s/",
		GoStruct:     "CloudExtras",
		ResourceType: "kubernetes-cloudextras",
		Schema:       reflect.TypeOf(k8s.CloudExtras{}),
	})

	fmt.Printf("Generated %d CloudExtras schemas\n", len(cloudExtrasSchemas))
	return cloudExtrasSchemas, nil
}

func (sg *SchemaGenerator) createDirectories(resources []ResourceDefinition) error {
	// Collect unique providers from discovered resources
	providerSet := make(map[string]bool)
	for _, resource := range resources {
		if resource.Provider != "" && resource.Provider != "unknown" {
			providerSet[resource.Provider] = true
		}
	}

	// Create directories for all discovered providers
	for provider := range providerSet {
		dir := filepath.Join(sg.outputDir, provider)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	fmt.Printf("Created directories for %d discovered providers\n", len(providerSet))
	return nil
}

// guessProviderFromResourceType attempts to determine the provider from the resource type string
func (sg *SchemaGenerator) guessProviderFromResourceType(resourceType string) string {
	switch {
	case strings.Contains(resourceType, "aws") || strings.Contains(resourceType, "s3") || strings.Contains(resourceType, "ecr") || strings.Contains(resourceType, "rds") || strings.Contains(resourceType, "ecs") || strings.Contains(resourceType, "lambda"):
		return "aws"
	case strings.Contains(resourceType, "gcp") || strings.Contains(resourceType, "gcloud") || strings.Contains(resourceType, "gke") || strings.Contains(resourceType, "cloudrun") || strings.Contains(resourceType, "artifact") || strings.Contains(resourceType, "pubsub"):
		return "gcp"
	case strings.Contains(resourceType, "k8s") || strings.Contains(resourceType, "kubernetes") || strings.Contains(resourceType, "caddy"):
		return "kubernetes"
	case strings.Contains(resourceType, "mongodb") || strings.Contains(resourceType, "atlas"):
		return "mongodb"
	case strings.Contains(resourceType, "cloudflare"):
		return "cloudflare"
	case strings.Contains(resourceType, "fs") || strings.Contains(resourceType, "passphrase"):
		return "fs"
	case strings.Contains(resourceType, "compose"):
		return "compose"
	case strings.Contains(resourceType, "docker"):
		return "docker"
	case strings.Contains(resourceType, "github"):
		return "github"
	case strings.Contains(resourceType, "discord"):
		return "discord"
	case strings.Contains(resourceType, "slack"):
		return "slack"
	case strings.Contains(resourceType, "telegram"):
		return "telegram"
	default:
		return "unknown"
	}
}

// guessResourceType attempts to determine if this is a resource, template, or auth type
func (sg *SchemaGenerator) guessResourceType(resourceType string) string {
	switch {
	case strings.Contains(resourceType, "template") || strings.Contains(resourceType, "fargate") || strings.Contains(resourceType, "lambda") || strings.Contains(resourceType, "cloudrun") || strings.Contains(resourceType, "static"):
		return "template"
	case strings.Contains(resourceType, "auth") || strings.Contains(resourceType, "token") || strings.Contains(resourceType, "kubeconfig"):
		return "auth"
	case strings.Contains(resourceType, "secrets"):
		return "secrets"
	default:
		return "resource"
	}
}

// generateDescription creates a description based on resource type and struct name
func (sg *SchemaGenerator) generateDescription(resourceType, structName string) string {
	provider := sg.guessProviderFromResourceType(resourceType)
	resourceKind := sg.guessResourceType(resourceType)

	providerName := strings.ToUpper(provider)
	if provider == "gcp" {
		providerName = "Google Cloud Platform"
	} else if provider == "kubernetes" {
		providerName = "Kubernetes"
	} else if provider == "mongodb" {
		providerName = "MongoDB Atlas"
	} else if provider == "cloudflare" {
		providerName = "Cloudflare"
	}

	switch resourceKind {
	case "template":
		return fmt.Sprintf("%s deployment template configuration", providerName)
	case "auth":
		return fmt.Sprintf("%s authentication configuration", providerName)
	case "secrets":
		return fmt.Sprintf("%s secrets management configuration", providerName)
	default:
		return fmt.Sprintf("%s %s configuration", providerName, strings.ToLower(strings.Replace(structName, "Config", "", -1)))
	}
}

func (sg *SchemaGenerator) generateSchemaFile(resource ResourceDefinition) error {
	// Generate JSON schema using reflection
	schema := sg.generateJSONSchema(resource.Schema.(reflect.Type))

	// Create the complete resource definition
	resourceDef := ResourceDefinition{
		Name:         resource.Name,
		Type:         resource.Type,
		Provider:     resource.Provider,
		Description:  resource.Description,
		GoPackage:    resource.GoPackage,
		GoStruct:     resource.GoStruct,
		ResourceType: resource.ResourceType,
		TemplateType: resource.TemplateType,
		Schema:       schema,
	}

	// Marshal to JSON with deterministic ordering
	jsonData, err := marshalJSONDeterministic(resourceDef)
	if err != nil {
		return err
	}

	// Write to file
	filename := fmt.Sprintf("%s.json", strings.ToLower(resource.Name))
	filePath := filepath.Join(sg.outputDir, resource.Provider, filename)

	return os.WriteFile(filePath, jsonData, 0o644)
}

func (sg *SchemaGenerator) generateJSONSchema(t reflect.Type) map[string]interface{} {
	schema := map[string]interface{}{
		"$schema":    "https://json-schema.org/draft/2020-12/schema",
		"type":       "object",
		"properties": make(map[string]interface{}),
		"required":   []string{},
	}

	properties := schema["properties"].(map[string]interface{})

	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Process struct fields
	if t.Kind() == reflect.Struct {
		// Create a slice to sort fields by name for consistent ordering
		type fieldInfo struct {
			field reflect.StructField
			index int
		}
		var fields []fieldInfo
		for i := 0; i < t.NumField(); i++ {
			fields = append(fields, fieldInfo{field: t.Field(i), index: i})
		}
		// Sort fields by name for consistent ordering
		sort.Slice(fields, func(i, j int) bool {
			return fields[i].field.Name < fields[j].field.Name
		})

		for _, fieldInfo := range fields {
			field := fieldInfo.field

			// Skip unexported fields
			if !field.IsExported() {
				continue
			}

			// Get JSON tag
			jsonTag := field.Tag.Get("json")
			yamlTag := field.Tag.Get("yaml")

			// Determine field name (prefer json tag, fallback to yaml, then field name)
			fieldName := field.Name
			if jsonTag != "" && jsonTag != "-" {
				fieldName = strings.Split(jsonTag, ",")[0]
			} else if yamlTag != "" && yamlTag != "-" {
				fieldName = strings.Split(yamlTag, ",")[0]
			}

			// Skip if marked as ignored
			if jsonTag == "-" || yamlTag == "-" {
				continue
			}

			// Generate property schema
			propSchema := sg.generatePropertySchema(field.Type, field.Tag)
			properties[fieldName] = propSchema

			// Check if field is required (not omitempty)
			if !strings.Contains(jsonTag, "omitempty") && !strings.Contains(yamlTag, "omitempty") {
				required := schema["required"].([]string)
				schema["required"] = append(required, fieldName)
			}
		}
	}

	// Sort the required fields for consistent ordering
	if required, ok := schema["required"].([]string); ok {
		sort.Strings(required)
		schema["required"] = required
	}

	return schema
}

func (sg *SchemaGenerator) generatePropertySchema(t reflect.Type, tag reflect.StructTag) map[string]interface{} {
	schema := make(map[string]interface{})

	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.String:
		schema["type"] = "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		schema["type"] = "integer"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		schema["type"] = "integer"
		schema["minimum"] = 0
	case reflect.Float32, reflect.Float64:
		schema["type"] = "number"
	case reflect.Bool:
		schema["type"] = "boolean"
	case reflect.Slice, reflect.Array:
		schema["type"] = "array"
		schema["items"] = sg.generatePropertySchema(t.Elem(), "")
	case reflect.Map:
		schema["type"] = "object"
		schema["additionalProperties"] = sg.generatePropertySchema(t.Elem(), "")
	case reflect.Struct:
		// For nested structs, generate a nested object schema
		schema = sg.generateJSONSchema(t)
	case reflect.Interface:
		// For interfaces, allow any type
		schema["type"] = []string{"string", "number", "boolean", "object", "array", "null"}
	default:
		schema["type"] = "string" // fallback
	}

	return schema
}

func (sg *SchemaGenerator) generateIndexFiles(resources []ResourceDefinition) error {
	// Group resources by provider
	byProvider := make(map[string][]ResourceDefinition)
	for _, resource := range resources {
		byProvider[resource.Provider] = append(byProvider[resource.Provider], resource)
	}

	// Generate index for each provider (sort providers for consistent ordering)
	var providers []string
	for provider := range byProvider {
		providers = append(providers, provider)
	}
	sort.Strings(providers)

	for _, provider := range providers {
		providerResources := byProvider[provider]

		// Sort resources within provider for consistent ordering
		sort.Slice(providerResources, func(i, j int) bool {
			return providerResources[i].Name < providerResources[j].Name
		})
		index := map[string]interface{}{
			"provider":    provider,
			"description": fmt.Sprintf("JSON Schema definitions for %s resources and templates", cases.Title(language.English).String(provider)),
			"resources":   providerResources,
		}

		jsonData, err := marshalJSONDeterministic(index)
		if err != nil {
			return err
		}

		indexPath := filepath.Join(sg.outputDir, provider, "index.json")
		if err := os.WriteFile(indexPath, jsonData, 0o644); err != nil {
			return err
		}
	}

	// Generate global index
	globalIndex := map[string]interface{}{
		"title":       "Simple Container Resource Schemas",
		"description": "JSON Schema definitions for all Simple Container resources and templates",
		"providers":   make(map[string]interface{}),
	}

	providersMap := globalIndex["providers"].(map[string]interface{})
	for _, provider := range providers {
		providerResources := byProvider[provider]
		var description string
		if provider == "core" {
			description = "Simple Container configuration file schemas (client.yaml, server.yaml, etc.)"
		} else {
			description = fmt.Sprintf("%s cloud provider resources and templates", cases.Title(language.English).String(provider))
		}

		providersMap[provider] = map[string]interface{}{
			"count":       len(providerResources),
			"description": description,
		}
	}

	jsonData, err := marshalJSONDeterministic(globalIndex)
	if err != nil {
		return err
	}

	globalIndexPath := filepath.Join(sg.outputDir, "index.json")
	return os.WriteFile(globalIndexPath, jsonData, 0o644)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <output-directory>\n", os.Args[0])
		os.Exit(1)
	}

	outputDir := os.Args[1]
	generator := NewSchemaGenerator(outputDir)

	if err := generator.Generate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating schemas: %v\n", err)
		os.Exit(1)
	}
}
