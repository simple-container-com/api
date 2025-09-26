package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	// Core API package for registration maps and configuration structs
	"github.com/simple-container-com/api/pkg/api"
	// Import all cloud provider packages to trigger init() functions
	_ "github.com/simple-container-com/api/pkg/clouds/aws"
	_ "github.com/simple-container-com/api/pkg/clouds/cloudflare"
	_ "github.com/simple-container-com/api/pkg/clouds/compose"
	_ "github.com/simple-container-com/api/pkg/clouds/discord"
	_ "github.com/simple-container-com/api/pkg/clouds/docker"
	_ "github.com/simple-container-com/api/pkg/clouds/fs"
	_ "github.com/simple-container-com/api/pkg/clouds/gcloud"
	_ "github.com/simple-container-com/api/pkg/clouds/github"
	_ "github.com/simple-container-com/api/pkg/clouds/k8s"
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

	// Combine resources and configuration schemas for directory creation
	allSchemas := append(resources, configSchemas...)

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

	// Generate index files
	if err := sg.generateIndexFiles(allSchemas); err != nil {
		return fmt.Errorf("failed to generate index files: %w", err)
	}

	fmt.Printf("Successfully generated JSON Schema files for %d resources in %s\n", len(resources), sg.outputDir)
	return nil
}

// discoverRegisteredResources dynamically discovers all registered resources from the DI framework
func (sg *SchemaGenerator) discoverRegisteredResources() ([]ResourceDefinition, error) {
	var resources []ResourceDefinition

	// Get all registered provider configurations (resources, templates, auth)
	providerConfigs := api.GetRegisteredProviderConfigs()
	for resourceType, readerFunc := range providerConfigs {
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
	for resourceType, readerFunc := range provisionerConfigs {
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

	// Marshal to JSON with pretty printing
	jsonData, err := json.MarshalIndent(resourceDef, "", "  ")
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
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)

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

	// Generate index for each provider
	for provider, providerResources := range byProvider {
		index := map[string]interface{}{
			"provider":    provider,
			"description": fmt.Sprintf("JSON Schema definitions for %s resources and templates", cases.Title(language.English).String(provider)),
			"resources":   providerResources,
		}

		jsonData, err := json.MarshalIndent(index, "", "  ")
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

	providers := globalIndex["providers"].(map[string]interface{})
	for provider, providerResources := range byProvider {
		var description string
		if provider == "core" {
			description = "Simple Container configuration file schemas (client.yaml, server.yaml, etc.)"
		} else {
			description = fmt.Sprintf("%s cloud provider resources and templates", cases.Title(language.English).String(provider))
		}

		providers[provider] = map[string]interface{}{
			"count":       len(providerResources),
			"description": description,
		}
	}

	jsonData, err := json.MarshalIndent(globalIndex, "", "  ")
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
