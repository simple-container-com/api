package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/docs"
)

// Validator provides validation of generated YAML configurations against JSON schemas
type Validator struct {
	schemaFS fs.FS
}

// NewValidator creates a new validator with embedded schemas
func NewValidator() *Validator {
	return &Validator{
		schemaFS: docs.EmbeddedSchemas,
	}
}

// ValidationResult contains the result of schema validation
type ValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// ValidateClientYAML validates client.yaml content against ClientDescriptor schema
func (v *Validator) ValidateClientYAML(ctx context.Context, yamlContent string) ValidationResult {
	// Parse YAML content
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &data); err != nil {
		return ValidationResult{
			Valid:  false,
			Errors: []string{fmt.Sprintf("YAML parsing error: %v", err)},
		}
	}

	// Load ClientDescriptor schema
	schemaPath := "schemas/core/clientdescriptor.json"
	schemaContent, err := fs.ReadFile(v.schemaFS, schemaPath)
	if err != nil {
		return ValidationResult{
			Valid:    false,
			Errors:   []string{fmt.Sprintf("Failed to load schema %s: %v", schemaPath, err)},
			Warnings: []string{"Schema validation skipped - using basic structure validation"},
		}
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(schemaContent, &schema); err != nil {
		return ValidationResult{
			Valid:    false,
			Errors:   []string{fmt.Sprintf("Schema parsing error: %v", err)},
			Warnings: []string{"Schema validation skipped - using basic structure validation"},
		}
	}

	// Perform validation
	return v.validateAgainstSchema(ctx, data, schema, "client.yaml")
}

// ValidateServerYAML validates server.yaml content against ServerDescriptor schema
func (v *Validator) ValidateServerYAML(ctx context.Context, yamlContent string) ValidationResult {
	// Parse YAML content
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &data); err != nil {
		return ValidationResult{
			Valid:  false,
			Errors: []string{fmt.Sprintf("YAML parsing error: %v", err)},
		}
	}

	// Load ServerDescriptor schema
	schemaPath := "schemas/core/serverdescriptor.json"
	schemaContent, err := fs.ReadFile(v.schemaFS, schemaPath)
	if err != nil {
		return ValidationResult{
			Valid:    false,
			Errors:   []string{fmt.Sprintf("Failed to load schema %s: %v", schemaPath, err)},
			Warnings: []string{"Schema validation skipped - using basic structure validation"},
		}
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(schemaContent, &schema); err != nil {
		return ValidationResult{
			Valid:    false,
			Errors:   []string{fmt.Sprintf("Schema parsing error: %v", err)},
			Warnings: []string{"Schema validation skipped - using basic structure validation"},
		}
	}

	// Perform validation
	return v.validateAgainstSchema(ctx, data, schema, "server.yaml")
}

// validateAgainstSchema performs basic structure validation against schema
func (v *Validator) validateAgainstSchema(ctx context.Context, data map[string]interface{}, schema map[string]interface{}, configType string) ValidationResult {
	var errors []string
	var warnings []string

	// Extract schema properties
	schemaProps, ok := schema["schema"].(map[string]interface{})
	if !ok {
		return ValidationResult{
			Valid:    false,
			Errors:   []string{"Invalid schema format"},
			Warnings: []string{"Using basic validation only"},
		}
	}

	_, ok = schemaProps["properties"].(map[string]interface{})
	if !ok {
		warnings = append(warnings, "Schema properties not found, using basic validation")
	}

	required, _ := schemaProps["required"].([]interface{})

	// Check required properties
	for _, req := range required {
		reqStr, ok := req.(string)
		if !ok {
			continue
		}

		if _, exists := data[reqStr]; !exists {
			errors = append(errors, fmt.Sprintf("Missing required property: %s", reqStr))
		}
	}

	// Validate specific patterns for Simple Container
	if configType == "client.yaml" {
		errors = append(errors, v.validateClientYAMLStructure(data)...)
	} else if configType == "server.yaml" {
		errors = append(errors, v.validateServerYAMLStructure(data)...)
	}

	// Check for fictional properties
	errors = append(errors, v.checkForFictionalProperties(data, configType)...)

	// Validation completed

	return ValidationResult{
		Valid:    len(errors) == 0,
		Errors:   errors,
		Warnings: warnings,
	}
}

// validateClientYAMLStructure performs Simple Container specific validation for client.yaml
func (v *Validator) validateClientYAMLStructure(data map[string]interface{}) []string {
	var errors []string

	// Check schema version
	schemaVersion, exists := data["schemaVersion"]
	if !exists {
		errors = append(errors, "Missing schemaVersion property")
	} else if schemaVersion != "1.0" && schemaVersion != 1.0 {
		errors = append(errors, "schemaVersion must be 1.0")
	}

	// Check stacks section (not environments)
	if _, hasStacks := data["stacks"]; !hasStacks {
		errors = append(errors, "Missing 'stacks' section - client.yaml must use 'stacks:' not 'environments:'")
	}

	if _, hasEnvironments := data["environments"]; hasEnvironments {
		errors = append(errors, "client.yaml should use 'stacks:' section, not 'environments:'")
	}

	// Validate stacks structure
	if stacks, ok := data["stacks"].(map[string]interface{}); ok {
		for stackName, stackData := range stacks {
			if stack, ok := stackData.(map[string]interface{}); ok {
				errors = append(errors, v.validateStackStructure(stackName, stack)...)
			}
		}
	}

	return errors
}

// validateServerYAMLStructure performs Simple Container specific validation for server.yaml
func (v *Validator) validateServerYAMLStructure(data map[string]interface{}) []string {
	var errors []string

	// Check schema version
	schemaVersion, exists := data["schemaVersion"]
	if !exists {
		errors = append(errors, "Missing schemaVersion property")
	} else if schemaVersion != "1.0" && schemaVersion != 1.0 {
		errors = append(errors, "schemaVersion must be 1.0")
	}

	// Server.yaml should NOT have stacks section
	if _, hasStacks := data["stacks"]; hasStacks {
		errors = append(errors, "server.yaml should not have 'stacks' section - use 'resources:' instead")
	}

	// Should have resources section
	if _, hasResources := data["resources"]; !hasResources {
		errors = append(errors, "server.yaml should have 'resources:' section")
	}

	return errors
}

// validateStackStructure validates individual stack configuration
func (v *Validator) validateStackStructure(stackName string, stack map[string]interface{}) []string {
	var errors []string

	// Check required properties
	requiredProps := []string{"type", "parent", "parentEnv"}
	for _, prop := range requiredProps {
		if _, exists := stack[prop]; !exists {
			errors = append(errors, fmt.Sprintf("Stack '%s' missing required property: %s", stackName, prop))
		}
	}

	// Validate config section
	if config, ok := stack["config"].(map[string]interface{}); ok {
		// Check for scaling in wrong location
		if _, hasScaling := config["scaling"]; hasScaling {
			errors = append(errors, fmt.Sprintf("Stack '%s' uses deprecated 'scaling:' section - use 'scale:' instead", stackName))
		}

		// Check environment vs env
		if _, hasEnvironment := config["environment"]; hasEnvironment {
			errors = append(errors, fmt.Sprintf("Stack '%s' uses 'environment:' - use 'env:' instead", stackName))
		}
	}

	return errors
}

// checkForFictionalProperties checks for properties that were eliminated during validation
func (v *Validator) checkForFictionalProperties(data map[string]interface{}, configType string) []string {
	var errors []string

	// Common fictional properties
	fictionalProps := map[string]string{
		"version":          "use 'schemaVersion' instead",
		"environments":     "use 'stacks' section instead",
		"account":          "belongs in server.yaml, not client.yaml",
		"bucketName":       "use 'name' in resource definitions",
		"connectionString": "fictional property - use auto-injected environment variables",
	}

	errors = append(errors, v.checkForFictionalPropsRecursive(data, fictionalProps, "")...)

	// Check for fictional scaling structure
	if stacks, ok := data["stacks"].(map[string]interface{}); ok {
		for stackName, stackData := range stacks {
			if stack, ok := stackData.(map[string]interface{}); ok {
				if config, ok := stack["config"].(map[string]interface{}); ok {
					// Check for minCapacity/maxCapacity
					if _, hasMinCapacity := config["minCapacity"]; hasMinCapacity {
						errors = append(errors, fmt.Sprintf("Stack '%s' uses fictional 'minCapacity' - use 'scale: {min: 1, max: 3}' instead", stackName))
					}
					if _, hasMaxCapacity := config["maxCapacity"]; hasMaxCapacity {
						errors = append(errors, fmt.Sprintf("Stack '%s' uses fictional 'maxCapacity' - use 'scale: {min: 1, max: 3}' instead", stackName))
					}

					// Check for scaling section instead of scale
					if _, hasScaling := config["scaling"]; hasScaling {
						errors = append(errors, fmt.Sprintf("Stack '%s' uses fictional 'scaling:' section - use 'scale:' instead", stackName))
					}
				}
			}
		}
	}

	return errors
}

// checkForFictionalPropsRecursive recursively checks for fictional properties
func (v *Validator) checkForFictionalPropsRecursive(data map[string]interface{}, fictionalProps map[string]string, path string) []string {
	var errors []string

	for key, value := range data {
		currentPath := key
		if path != "" {
			currentPath = path + "." + key
		}

		// Check if this key is fictional
		if reason, isFictional := fictionalProps[key]; isFictional {
			errors = append(errors, fmt.Sprintf("Fictional property '%s' found at %s - %s", key, currentPath, reason))
		}

		// Recursively check nested objects
		if nestedMap, ok := value.(map[string]interface{}); ok {
			errors = append(errors, v.checkForFictionalPropsRecursive(nestedMap, fictionalProps, currentPath)...)
		}
	}

	return errors
}

// GetAvailableSchemas returns a list of available schemas for validation
func (v *Validator) GetAvailableSchemas(ctx context.Context) []string {
	var schemas []string

	err := fs.WalkDir(v.schemaFS, "schemas", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && strings.HasSuffix(path, ".json") && !strings.HasSuffix(path, "index.json") {
			schemas = append(schemas, path)
		}
		return nil
	})
	// Ignore walk errors - return collected schemas regardless
	_ = err
	return schemas
}

// GetClientYAMLSchema returns the client.yaml schema for prompt enrichment
func (v *Validator) GetClientYAMLSchema(ctx context.Context) (map[string]interface{}, error) {
	schemaPath := "schemas/core/clientdescriptor.json"
	schemaContent, err := fs.ReadFile(v.schemaFS, schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load schema %s: %w", schemaPath, err)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(schemaContent, &schema); err != nil {
		return nil, fmt.Errorf("schema parsing error: %w", err)
	}

	return schema, nil
}

// GetStackConfigComposeSchema returns the stack config schema for prompt enrichment
func (v *Validator) GetStackConfigComposeSchema(ctx context.Context) (map[string]interface{}, error) {
	schemaPath := "schemas/core/stackconfigcompose.json"
	schemaContent, err := fs.ReadFile(v.schemaFS, schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load schema %s: %w", schemaPath, err)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(schemaContent, &schema); err != nil {
		return nil, fmt.Errorf("schema parsing error: %w", err)
	}

	return schema, nil
}

// GetServerYAMLSchema returns the server.yaml schema for prompt enrichment
func (v *Validator) GetServerYAMLSchema(ctx context.Context) (map[string]interface{}, error) {
	schemaPath := "schemas/core/serverdescriptor.json"
	schemaContent, err := fs.ReadFile(v.schemaFS, schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load schema %s: %w", schemaPath, err)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(schemaContent, &schema); err != nil {
		return nil, fmt.Errorf("schema parsing error: %w", err)
	}

	return schema, nil
}
