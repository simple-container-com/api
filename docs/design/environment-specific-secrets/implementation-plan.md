# Environment-Specific Secrets - Implementation Plan

## Overview

This document provides a detailed, phase-by-phase implementation plan for adding environment-specific secrets support to parent stacks.

**Total Estimated Effort**: 29-39 hours

**Implementation Approach**: Sequential phases with clear acceptance criteria for each phase.

## Phase 1: Data Structures and Schema

**Estimated Time**: 2-3 hours

**Objective**: Define the core data structures needed for environment-specific secrets.

### Tasks

1. **Create `pkg/api/secrets_config.go`** (new file)
   - Add `SecretsConfigMap` type
   - Add `EnvironmentSecretsConfig` struct
   - Add `SecretReferenceType` enum and constants
   - Add documentation for all types

2. **Modify `pkg/api/server.go`**
   - Add `SecretsConfig *SecretsConfigMap` field to `ServerDescriptor`
   - Ensure field is optional with `omitempty` tags
   - Update struct documentation

### Code Changes

**New file: `pkg/api/secrets_config.go`**

```go
package api

// SecretsConfigMap holds per-environment secret configurations
type SecretsConfigMap map[string]EnvironmentSecretsConfig

// EnvironmentSecretsConfig configures secrets for a specific environment
//
// The configuration supports three modes:
// - Include mode (inheritAll: false): Only secrets in "include" are available
// - Exclude mode (inheritAll: true): All secrets except those in "exclude" are available
// - Override mode (secrets map): Literal values override or add to secrets from secrets.yaml
//
// Secret references in "include" can be:
// - Direct: "DATABASE_URL" fetches DATABASE_URL from secrets.yaml
// - Mapped: "${secret:DB_HOST_STAGING}" fetches DB_HOST_STAGING, available as the key name
type EnvironmentSecretsConfig struct {
    // InheritAll when true includes all secrets except those in Exclude
    // When false, only secrets in Include are available
    InheritAll bool `json:"inheritAll" yaml:"inheritAll"`

    // Include lists secret names to explicitly allow (when inheritAll: false)
    // Each entry can be a direct secret name or a mapped reference like "${secret:KEY}"
    Include []string `json:"include,omitempty" yaml:"include,omitempty"`

    // Exclude lists secret names to block (when inheritAll: true)
    Exclude []string `json:"exclude,omitempty" yaml:"exclude,omitempty"`

    // Secrets defines literal secret values (not fetched from secrets.yaml)
    // These override any values from secrets.yaml
    Secrets map[string]string `json:"secrets,omitempty" yaml:"secrets,omitempty"`
}

// SecretReferenceType indicates how a secret reference should be resolved
type SecretReferenceType string

const (
    // SecretReferenceDirect is a direct reference (e.g., "DATABASE_PASSWORD")
    // Secret is fetched using the same name from secrets.yaml
    SecretReferenceDirect SecretReferenceType = "direct"

    // SecretReferenceMapped is a mapped reference (e.g., "${secret:DB_PASSWORD_STAGING}")
    // Secret is fetched using the mapped key from secrets.yaml
    SecretReferenceMapped SecretReferenceType = "mapped"

    // SecretReferenceLiteral is a literal value (not a reference)
    SecretReferenceLiteral SecretReferenceType = "literal"
)
```

**Modified: `pkg/api/server.go`**

```go
// ServerDescriptor describes the server schema
type ServerDescriptor struct {
    SchemaVersion string                        `json:"schemaVersion" yaml:"schemaVersion"`
    Provisioner   ProvisionerDescriptor         `json:"provisioner" yaml:"provisioner"`
    Secrets       SecretsConfigDescriptor       `json:"secrets" yaml:"secrets"`
    SecretsConfig *SecretsConfigMap             `json:"secretsConfig,omitempty" yaml:"secretsConfig,omitempty"` // NEW
    CiCd          CiCdDescriptor                `json:"cicd" yaml:"cicd"`
    Templates     map[string]StackDescriptor    `json:"templates" yaml:"templates"`
    Resources     PerStackResourcesDescriptor   `json:"resources" yaml:"resources"`
    Variables     map[string]VariableDescriptor `json:"variables" yaml:"variables"`
}
```

**Modified: `pkg/api/copy.go`**

```go
// Add copy method for SecretsConfigMap
func (s *SecretsConfigMap) Copy() *SecretsConfigMap {
    if s == nil {
        return nil
    }
    result := make(SecretsConfigMap, len(*s))
    for k, v := range *s {
        result[k] = v
    }
    return &result
}

// Update ServerDescriptor.Copy() to include SecretsConfig
func (sd *ServerDescriptor) Copy() ServerDescriptor {
    return ServerDescriptor{
        SchemaVersion: sd.SchemaVersion,
        Provisioner:   sd.Provisioner.Copy(),
        Secrets:       sd.Secrets.Copy(),
        SecretsConfig: sd.SecretsConfig.Copy(), // NEW
        CiCd:          sd.CiCd.Copy(),
        Templates: lo.MapValues(sd.Templates, func(value StackDescriptor, key string) StackDescriptor {
            return value.Copy()
        }),
        Resources: sd.Resources.Copy(),
        Variables: lo.MapValues(sd.Variables, func(value VariableDescriptor, key string) VariableDescriptor {
            return value.Copy()
        }),
    }
}
```

### Acceptance Criteria

- [ ] `SecretsConfigMap` type defined in `secrets_config.go`
- [ ] `EnvironmentSecretsConfig` struct with all fields defined
- [ ] `SecretReferenceType` enum with all three constants
- [ ] `ServerDescriptor` has `SecretsConfig *SecretsConfigMap` field
- [ ] Field is optional (omitempty tags)
- [ ] Copy methods updated
- [ ] Code compiles without errors

---

## Phase 2: Configuration Reading and Detection

**Estimated Time**: 3-4 hours

**Objective**: Add detection and validation logic for secretsConfig during configuration reading.

### Tasks

1. **Add `DetectSecretsConfigType()` function to `pkg/api/read.go`**
   - Validate secretsConfig structure
   - Check for conflicting configurations
   - Return meaningful error messages

2. **Integrate into `ReadServerConfigs()`**
   - Call `DetectSecretsConfigType()` after other detection functions
   - Ensure validation happens early in the pipeline

### Code Changes

**Modified: `pkg/api/read.go`**

```go
// Add new function after DetectSecretsType()

// DetectSecretsConfigType validates the secretsConfig structure
func DetectSecretsConfigType(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
    if descriptor.SecretsConfig == nil {
        return descriptor, nil // No secretsConfig, use default behavior
    }

    // Validate each environment configuration
    for env, config := range *descriptor.SecretsConfig {
        // Validate: can't have both include and inheritAll: true
        if config.InheritAll && len(config.Include) > 0 {
            return nil, errors.Errorf("secretsConfig.%s: cannot use 'include' with 'inheritAll: true'", env)
        }

        // Validate: can't have both include and exclude
        if len(config.Include) > 0 && len(config.Exclude) > 0 {
            return nil, errors.Errorf("secretsConfig.%s: cannot use both 'include' and 'exclude'", env)
        }

        // Validate: must specify include, exclude, or secrets when inheritAll is false
        if !config.InheritAll && len(config.Include) == 0 && len(config.Exclude) == 0 && len(config.Secrets) == 0 {
            return nil, errors.Errorf("secretsConfig.%s: must specify 'include', 'exclude', or 'secrets' when inheritAll is false", env)
        }

        // Validate: each include entry is a valid reference
        for i, ref := range config.Include {
            if ref == "" {
                return nil, errors.Errorf("secretsConfig.%s.include[%d]: reference cannot be empty", env, i)
            }
            // Validate mapped reference format
            if strings.HasPrefix(ref, "${secret:") {
                if !strings.HasSuffix(ref, "}") {
                    return nil, errors.Errorf("secretsConfig.%s.include[%d]: invalid mapped reference format '%s', expected '${secret:KEY}'", env, i, ref)
                }
            }
        }
    }

    return descriptor, nil
}

// Modify ReadServerConfigs() to include secretsConfig detection

func ReadServerConfigs(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
    if descriptor == nil {
        return nil, errors.Errorf("failed to read descriptor: reference is nil")
    }
    res := *descriptor

    if withProvisioner, err := DetectProvisionerType(&res); err != nil {
        return nil, err
    } else {
        res = *withProvisioner
    }

    if withSecrets, err := DetectSecretsType(&res); err != nil {
        return nil, err
    } else {
        res = *withSecrets
    }

    // NEW: Detect and validate secretsConfig
    if withSecretsConfig, err := DetectSecretsConfigType(&res); err != nil {
        return nil, err
    } else {
        res = *withSecretsConfig
    }

    // ... rest of function continues unchanged ...
}
```

### Acceptance Criteria

- [ ] `DetectSecretsConfigType()` function implemented
- [ ] Validates include/inheritAll conflict
- [ ] Validates include/exclude conflict
- [ ] Validates empty configuration
- [ ] Validates reference format
- [ ] Integrated into `ReadServerConfigs()`
- [ ] All validation tests pass
- [ ] Invalid configurations return clear error messages

---

## Phase 3: Secret Resolution Logic

**Estimated Time**: 6-8 hours

**Objective**: Implement the core secret resolution logic with support for all three modes.

### Tasks

1. **Create `SecretResolver` type in `pkg/api/secrets.go`**
   - Add `Resolve()` method
   - Add `resolveIncludeMode()` method
   - Add `resolveExcludeMode()` method
   - Add `parseSecretReference()` method
   - Add `fetchSecret()` method

2. **Add `ResolveSecretsForEnvironment()` function**
   - Public API for resolving secrets
   - Handles nil secretsConfig for backwards compatibility

### Code Changes

**Modified: `pkg/api/secrets.go`**

```go
package api

import (
    "strings"
    "github.com/pkg/errors"
)

// Existing code remains...

// ResolveSecretsForEnvironment resolves secrets for a specific environment
// based on the parent stack's secretsConfig. If secretsConfig is nil,
// returns all secrets (backwards compatible behavior).
func ResolveSecretsForEnvironment(
    serverDesc *ServerDescriptor,
    secretsDesc *SecretsDescriptor,
    environment string,
) (map[string]string, error) {
    // Backwards compatibility: no secretsConfig means all secrets available
    if serverDesc.SecretsConfig == nil {
        return secretsDesc.Values, nil
    }

    resolver := &SecretResolver{
        secretsConfig: serverDesc.SecretsConfig,
        allSecrets:    secretsDesc.Values,
        environment:   environment,
    }

    return resolver.Resolve()
}

// SecretResolver resolves secret references based on environment configuration
type SecretResolver struct {
    secretsConfig *SecretsConfigMap
    allSecrets    map[string]string
    environment   string
}

// Resolve returns the resolved secrets map for the configured environment
func (r *SecretResolver) Resolve() (map[string]string, error) {
    // Backwards compatibility
    if r.secretsConfig == nil {
        return r.allSecrets, nil
    }

    // Validate environment configuration exists
    envConfig, ok := (*r.secretsConfig)[r.environment]
    if !ok {
        return nil, errors.Errorf("environment %q not configured in secretsConfig", r.environment)
    }

    // Route to appropriate mode handler
    if envConfig.InheritAll {
        return r.resolveExcludeMode(envConfig)
    }
    return r.resolveIncludeMode(envConfig)
}

// resolveIncludeMode handles include mode (explicit allow list)
func (r *SecretResolver) resolveIncludeMode(envConfig EnvironmentSecretsConfig) (map[string]string, error) {
    resolved := make(map[string]string)

    // Process each include entry
    for _, ref := range envConfig.Include {
        refType, key, err := r.parseSecretReference(ref)
        if err != nil {
            return nil, errors.Wrapf(err, "failed to parse include reference %q", ref)
        }

        if refType == SecretReferenceDirect {
            // Direct reference: use the same key name
            value, err := r.fetchSecret(key)
            if err != nil {
                return nil, errors.Wrapf(err, "failed to fetch secret %q", key)
            }
            resolved[key] = value
        } else if refType == SecretReferenceMapped {
            // Mapped reference: extract target name from include entry
            // Format in include: "TARGET_NAME: ${secret:SOURCE_KEY}"
            // or just "${secret:SOURCE_KEY}" where target is determined from context
            targetName := key // For now, use the same name
            value, err := r.fetchSecret(key)
            if err != nil {
                return nil, errors.Wrapf(err, "failed to fetch mapped secret %q", key)
            }
            resolved[targetName] = value
        }
    }

    // Add literal values (these override any fetched values)
    for name, value := range envConfig.Secrets {
        resolved[name] = value
    }

    return resolved, nil
}

// resolveExcludeMode handles exclude mode (block list)
func (r *SecretResolver) resolveExcludeMode(envConfig EnvironmentSecretsConfig) (map[string]string, error) {
    // Start with all secrets
    resolved := make(map[string]string, len(r.allSecrets))
    for k, v := range r.allSecrets {
        resolved[k] = v
    }

    // Remove excluded secrets
    for _, secretName := range envConfig.Exclude {
        delete(resolved, secretName)
    }

    // Add/override with literal values
    for name, value := range envConfig.Secrets {
        resolved[name] = value
    }

    return resolved, nil
}

// parseSecretReference parses a secret reference string
func (r *SecretResolver) parseSecretReference(ref string) (SecretReferenceType, string, error) {
    // Pattern 1: Mapped reference "${secret:KEY_NAME}"
    if strings.HasPrefix(ref, "${secret:") && strings.HasSuffix(ref, "}") {
        key := strings.TrimPrefix(ref, "${secret:")
        key = strings.TrimSuffix(key, "}")
        if key == "" {
            return SecretReferenceMapped, "", errors.Errorf("empty key in mapped reference")
        }
        return SecretReferenceMapped, key, nil
    }

    // Pattern 2: Direct reference - a valid secret name (not a template string)
    if !strings.HasPrefix(ref, "${") {
        return SecretReferenceDirect, ref, nil
    }

    // Pattern 3: Invalid or unhandled template format
    return SecretReferenceLiteral, ref, errors.Errorf("invalid reference format: %q", ref)
}

// fetchSecret fetches a secret value from allSecrets
func (r *SecretResolver) fetchSecret(key string) (string, error) {
    value, ok := r.allSecrets[key]
    if !ok {
        return "", errors.Errorf("secret %q does not exist in secrets.yaml", key)
    }
    return value, nil
}
```

### Acceptance Criteria

- [ ] `SecretResolver` type implemented
- [ ] `Resolve()` method routes to correct mode
- [ ] `resolveIncludeMode()` correctly resolves include list
- [ ] `resolveExcludeMode()` correctly excludes secrets
- [ ] `parseSecretReference()` handles all three patterns
- [ ] `fetchSecret()` returns error for missing secrets
- [ ] `ResolveSecretsForEnvironment()` handles nil secretsConfig
- [ ] Unit tests for all methods pass

---

## Phase 4: Stack Reconciliation Integration

**Estimated Time**: 4-6 hours

**Objective**: Integrate secret resolution into the stack reconciliation process.

### Tasks

1. **Modify `ReconcileForDeploy()` in `pkg/api/models.go`**
   - Call `ResolveSecretsForEnvironment()` after copying parent stack
   - Use child stack's ParentEnv as the environment
   - Update child stack's Secrets.Values with resolved secrets

2. **Handle errors gracefully**
   - Return clear error messages on resolution failure
   - Ensure partial deployments don't occur on error

### Code Changes

**Modified: `pkg/api/models.go`**

```go
func (m *StacksMap) ReconcileForDeploy(params StackParams) (*StacksMap, error) {
    current := *m
    iterMap := lo.Assign(current)
    for stackName, stack := range iterMap {
        if len(stack.Client.Stacks) == 0 {
            // skip server-only stack
            continue
        }
        clientDesc, ok := stack.Client.Stacks[params.Environment]
        if !ok && stackName != params.StackName {
            // skip non-target stacks if they are not configured for env
            continue
        }
        if !ok {
            return nil, errors.Errorf("client stack %q is not configured for %q", stackName, params.Environment)
        }
        parentStackParts := strings.SplitN(clientDesc.ParentStack, "/", 3)
        parentStackName := parentStackParts[len(parentStackParts)-1]
        if parentStack, ok := current[parentStackName]; ok {
            stack.Server = parentStack.Server.Copy()
            stack.Secrets = parentStack.Secrets.Copy()

            // NEW: Resolve secrets based on environment configuration
            if parentStack.Server.SecretsConfig != nil {
                // Determine the environment to use for resolution
                // Prefer ParentEnv if specified, otherwise use the stack's environment
                resolveEnv := clientDesc.ParentEnv
                if resolveEnv == "" {
                    resolveEnv = params.Environment
                }

                resolved, err := ResolveSecretsForEnvironment(
                    &parentStack.Server,
                    &stack.Secrets,
                    resolveEnv,
                )
                if err != nil {
                    return nil, errors.Wrapf(err, "failed to resolve secrets for stack %q in environment %q", stackName, resolveEnv)
                }
                stack.Secrets.Values = resolved
            }
        } else {
            return nil, errors.Errorf("parent stack %q is not configured for %q in %q", clientDesc.ParentStack, stackName, params.Environment)
        }
        current[stackName] = stack
    }
    return &current, nil
}
```

### Acceptance Criteria

- [ ] `ReconcileForDeploy()` calls secret resolution
- [ ] Uses correct environment (ParentEnv or stack's environment)
- [ ] Resolved secrets replace Secrets.Values
- [ ] Errors are wrapped with context
- [ ] Backwards compatible (nil secretsConfig works)
- [ ] Integration tests pass

---

## Phase 5: Validation

**Estimated Time**: 4-5 hours

**Objective**: Add comprehensive validation for secret references and availability.

### Tasks

1. **Create `pkg/api/validation.go`** (new file)
   - Add `ValidateSecretsReferences()` function
   - Add `ValidateSecretsConfig()` function
   - Add helper validation functions

2. **Add validation tests**
   - Test all validation scenarios
   - Test error messages
   - Test edge cases

### Code Changes

**New file: `pkg/api/validation.go`**

```go
package api

import (
    "strings"
    "github.com/pkg/errors"
)

// ValidateSecretsReferences checks that all referenced secrets are available
func ValidateSecretsReferences(
    serverDesc *ServerDescriptor,
    secretsDesc *SecretsDescriptor,
    environment string,
) error {
    // Backwards compatibility: no secretsConfig means all secrets available
    if serverDesc.SecretsConfig == nil {
        return nil
    }

    // Check if environment is configured
    envConfig, ok := (*serverDesc.SecretsConfig)[environment]
    if !ok {
        return errors.Errorf("environment %q not configured in secretsConfig", environment)
    }

    // Validate include references
    for _, ref := range envConfig.Include {
        refType, key, err := parseSecretReference(ref)
        if err != nil {
            return errors.Wrapf(err, "invalid secret reference %q in environment %q", ref, environment)
        }

        if refType != SecretReferenceLiteral {
            if _, exists := secretsDesc.Values[key]; !exists {
                return errors.Errorf("secret %q referenced in include does not exist in secrets.yaml (environment: %q)", key, environment)
            }
        }
    }

    // Validate exclude references
    for _, name := range envConfig.Exclude {
        if _, exists := secretsDesc.Values[name]; !exists {
            return errors.Errorf("secret %q referenced in exclude does not exist in secrets.yaml (environment: %q)", name, environment)
        }
    }

    return nil
}

// ValidateSecretsConfig validates the structure and content of secretsConfig
func ValidateSecretsConfig(config SecretsConfigMap) error {
    for env, envConfig := range config {
        // Validate: can't have both include and inheritAll: true
        if envConfig.InheritAll && len(envConfig.Include) > 0 {
            return errors.Errorf("secretsConfig.%s: cannot use 'include' with 'inheritAll: true'", env)
        }

        // Validate: can't have both include and exclude
        if len(envConfig.Include) > 0 && len(envConfig.Exclude) > 0 {
            return errors.Errorf("secretsConfig.%s: cannot use both 'include' and 'exclude'", env)
        }

        // Validate: must specify something when inheritAll is false
        if !envConfig.InheritAll && len(envConfig.Include) == 0 && len(envConfig.Exclude) == 0 && len(envConfig.Secrets) == 0 {
            return errors.Errorf("secretsConfig.%s: must specify 'include', 'exclude', or 'secrets' when inheritAll is false", env)
        }

        // Validate include references
        for i, ref := range envConfig.Include {
            if ref == "" {
                return errors.Errorf("secretsConfig.%s.include[%d]: reference cannot be empty", env, i)
            }
            if strings.HasPrefix(ref, "${secret:") {
                if !strings.HasSuffix(ref, "}") {
                    return errors.Errorf("secretsConfig.%s.include[%d]: invalid mapped reference format '%s'", env, i, ref)
                }
            }
        }
    }
    return nil
}

// parseSecretReference is a helper function for validation
func parseSecretReference(ref string) (SecretReferenceType, string, error) {
    if strings.HasPrefix(ref, "${secret:") && strings.HasSuffix(ref, "}") {
        key := strings.TrimPrefix(ref, "${secret:")
        key = strings.TrimSuffix(key, "}")
        if key == "" {
            return SecretReferenceMapped, "", errors.Errorf("empty key in mapped reference")
        }
        return SecretReferenceMapped, key, nil
    }
    if !strings.HasPrefix(ref, "${") {
        return SecretReferenceDirect, ref, nil
    }
    return SecretReferenceLiteral, ref, nil
}
```

### Acceptance Criteria

- [ ] `ValidateSecretsReferences()` implemented
- [ ] `ValidateSecretsConfig()` implemented
- [ ] All validation scenarios covered
- [ ] Clear error messages for each failure case
- [ ] Unit tests for validation pass

---

## Phase 6: JSON Schema Regeneration

**Estimated Time**: 1 hour

**Objective**: Regenerate JSON schemas to include the new secretsConfig field.

### Tasks

1. **Run schema generation**
   ```bash
   make schema-gen
   ```

2. **Verify generated schema**
   - Check `docs/schemas/core/serverdescriptor.json`
   - Verify `secretsConfig` field is present
   - Verify field is not in `required` array
   - Verify schema structure is correct

3. **Test schema validation**
   - Create test configuration files
   - Validate against schema

### Verification Commands

```bash
# Generate schema
go run cmd/schema-gen/main.go docs/schemas

# Check secretsConfig field exists
cat docs/schemas/core/serverdescriptor.json | jq '.schema.properties.secretsConfig'

# Verify it's not required
cat docs/schemas/core/serverdescriptor.json | jq '.schema.required'

# Validate schema is proper JSON
cat docs/schemas/core/serverdescriptor.json | jq . > /dev/null && echo "Valid JSON" || echo "Invalid JSON"
```

### Acceptance Criteria

- [ ] Schema generation completes without errors
- [ ] `secretsConfig` property exists in schema
- [ ] `secretsConfig` is NOT in required array
- [ ] Schema has correct structure (additionalProperties)
- [ ] Schema is valid JSON
- [ ] Test configurations validate successfully

---

## Phase 7: Testing

**Estimated Time**: 6-8 hours

**Objective**: Comprehensive testing of all functionality.

### Tasks

1. **Unit Tests**
   - Test `SecretResolver.Resolve()`
   - Test `resolveIncludeMode()`
   - Test `resolveExcludeMode()`
   - Test `parseSecretReference()`
   - Test `fetchSecret()`
   - Test `ValidateSecretsConfig()`
   - Test `ValidateSecretsReferences()`
   - Test backwards compatibility

2. **Integration Tests**
   - Test `ReadServerConfigs()` with secretsConfig
   - Test `ReconcileForDeploy()` with secret resolution
   - Test error propagation through the stack

3. **End-to-End Tests**
   - Create test stack with secretsConfig
   - Deploy and verify resolved secrets
   - Test failure scenarios
   - Test multiple environments

### Test File Structure

**New file: `pkg/api/secrets_config_test.go`**

```go
package api

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestSecretResolver_Resolve_IncludeMode(t *testing.T) {
    allSecrets := map[string]string{
        "DATABASE_URL":        "postgres://prod/db",
        "API_KEY":            "prod-key",
        "REDIS_HOST":         "redis.prod",
        "DATABASE_URL_STAGING": "postgres://staging/db",
    }

    config := SecretsConfigMap{
        "staging": {
            InheritAll: false,
            Include: []string{
                "DATABASE_URL",
                "${secret:DATABASE_URL_STAGING}",
            },
            Secrets: map[string]string{
                "ENVIRONMENT": "staging",
            },
        },
    }

    resolver := &SecretResolver{
        secretsConfig: &config,
        allSecrets:    allSecrets,
        environment:   "staging",
    }

    resolved, err := resolver.Resolve()
    require.NoError(t, err)

    assert.Equal(t, "postgres://prod/db", resolved["DATABASE_URL"])
    assert.Equal(t, "postgres://staging/db", resolved["DATABASE_URL_STAGING"])
    assert.Equal(t, "staging", resolved["ENVIRONMENT"])
    assert.NotContains(t, resolved, "API_KEY")
    assert.NotContains(t, resolved, "REDIS_HOST")
}

func TestSecretResolver_Resolve_ExcludeMode(t *testing.T) {
    allSecrets := map[string]string{
        "DATABASE_URL": "postgres://prod/db",
        "API_KEY":     "prod-key",
        "REDIS_HOST":  "redis.prod",
        "DEV_KEY":     "dev-value",
    }

    config := SecretsConfigMap{
        "production": {
            InheritAll: true,
            Exclude: []string{"DEV_KEY"},
            Secrets: map[string]string{
                "ENVIRONMENT": "production",
            },
        },
    }

    resolver := &SecretResolver{
        secretsConfig: &config,
        allSecrets:    allSecrets,
        environment:   "production",
    }

    resolved, err := resolver.Resolve()
    require.NoError(t, err)

    assert.Contains(t, resolved, "DATABASE_URL")
    assert.Contains(t, resolved, "API_KEY")
    assert.Contains(t, resolved, "REDIS_HOST")
    assert.NotContains(t, resolved, "DEV_KEY")
    assert.Equal(t, "production", resolved["ENVIRONMENT"])
}

func TestSecretResolver_Resolve_BackwardsCompatible(t *testing.T) {
    allSecrets := map[string]string{
        "DATABASE_URL": "postgres://db",
        "API_KEY":     "key",
    }

    resolver := &SecretResolver{
        secretsConfig: nil,
        allSecrets:    allSecrets,
        environment:   "production",
    }

    resolved, err := resolver.Resolve()
    require.NoError(t, err)

    assert.Equal(t, allSecrets, resolved)
}

func TestSecretResolver_Resolve_EnvironmentNotConfigured(t *testing.T) {
    config := SecretsConfigMap{
        "staging": {
            InheritAll: false,
            Include:    []string{"DATABASE_URL"},
        },
    }

    resolver := &SecretResolver{
        secretsConfig: &config,
        allSecrets:    map[string]string{},
        environment:   "production", // Not configured
    }

    _, err := resolver.Resolve()
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "environment \"production\" not configured")
}

func TestSecretResolver_ParseSecretReference(t *testing.T) {
    tests := []struct {
        name          string
        ref           string
        expectedType  SecretReferenceType
        expectedKey   string
        expectError   bool
    }{
        {
            name:         "direct reference",
            ref:          "DATABASE_URL",
            expectedType: SecretReferenceDirect,
            expectedKey:  "DATABASE_URL",
            expectError:  false,
        },
        {
            name:         "mapped reference",
            ref:          "${secret:DB_HOST_STAGING}",
            expectedType: SecretReferenceMapped,
            expectedKey:  "DB_HOST_STAGING",
            expectError:  false,
        },
        {
            name:         "invalid mapped reference - missing closing brace",
            ref:          "${secret:DB_HOST",
            expectError:  true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            resolver := &SecretResolver{}
            refType, key, err := resolver.parseSecretReference(tt.ref)

            if tt.expectError {
                assert.Error(t, err)
            } else {
                require.NoError(t, err)
                assert.Equal(t, tt.expectedType, refType)
                assert.Equal(t, tt.expectedKey, key)
            }
        })
    }
}
```

### Acceptance Criteria

- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] All end-to-end tests pass
- [ ] Test coverage > 80% for new code
- [ ] Edge cases covered
- [ ] Error conditions tested

---

## Phase 8: Documentation

**Estimated Time**: 3-4 hours

**Objective**: Update user documentation and examples.

### Tasks

1. **Update main documentation**
   - Add section on environment-specific secrets
   - Include configuration examples
   - Explain all three modes

2. **Add example configurations**
   - Include mode example
   - Exclude mode example
   - Override mode example
   - Mixed environment example

3. **Update migration guide**
   - How to add secretsConfig to existing stacks
   - Backwards compatibility notes

### Documentation Files to Update

1. **`docs/docs/examples/secrets/README.md`**
   - Add section on environment-specific secrets
   - Include examples

2. **Create `docs/docs/examples/secrets/environment-specific/`**
   - `server.yaml` with examples
   - `secrets.yaml` with sample secrets
   - `README.md` with explanation

### Example Documentation

**New file: `docs/docs/examples/secrets/environment-specific/README.md`**

```markdown
# Environment-Specific Secrets Example

This example demonstrates how to configure environment-specific secrets in parent stacks.

## Configuration

### server.yaml

```yaml
schemaVersion: "1.0"
secrets:
  type: fs-secrets

secretsConfig:
  development:
    inheritAll: false
    include:
      - DATABASE_URL
      - REDIS_HOST
    secrets:
      ENVIRONMENT: "development"
      DEBUG: "true"

  staging:
    inheritAll: false
    include:
      - DATABASE_URL
      - REDIS_HOST
      - API_KEY
      - "${secret:SMTP_HOST_STAGING}"
    secrets:
      ENVIRONMENT: "staging"
      DEBUG: "false"

  production:
    inheritAll: true
    exclude:
      - DEV_API_KEY
      - STAGING_DATABASE_URL
    secrets:
      ENVIRONMENT: "production"
      DEBUG: "false"
```

### secrets.yaml

```yaml
schemaVersion: "1.0"
values:
  DATABASE_URL: "postgres://default/db"
  REDIS_HOST: "redis.default"
  API_KEY: "default-api-key"
  SMTP_HOST_STAGING: "smtp.staging.example.com"
  DEV_API_KEY: "dev-only-key"
  STAGING_DATABASE_URL: "postgres://staging/db"
```

## Modes Explained

### Include Mode (development, staging)
Only explicitly listed secrets are available. Useful for limiting access to sensitive production secrets.

### Exclude Mode (production)
All secrets are available except those explicitly excluded. Useful for blocking development-only secrets.

### Literal Values
The `secrets` map provides literal values that override or add to secrets fetched from secrets.yaml.

## Result

When deploying to **staging**:
- Available: `DATABASE_URL`, `REDIS_HOST`, `API_KEY`, `SMTP_HOST_STAGING`, `ENVIRONMENT`, `DEBUG`
- Not available: `DEV_API_KEY`, `STAGING_DATABASE_URL`
```

### Acceptance Criteria

- [ ] Main documentation updated
- [ ] Example configurations created
- [ ] Migration guide updated
- [ ] All examples are valid and tested
- [ ] Documentation is clear and comprehensive

---

## Summary

### Total Effort

| Phase | Description | Time |
|-------|-------------|------|
| 1 | Data Structures and Schema | 2-3 hours |
| 2 | Configuration Reading and Detection | 3-4 hours |
| 3 | Secret Resolution Logic | 6-8 hours |
| 4 | Stack Reconciliation Integration | 4-6 hours |
| 5 | Validation | 4-5 hours |
| 6 | JSON Schema Regeneration | 1 hour |
| 7 | Testing | 6-8 hours |
| 8 | Documentation | 3-4 hours |
| **Total** | | **29-39 hours** |

### Files Modified

1. `pkg/api/server.go` - Add SecretsConfig field
2. `pkg/api/read.go` - Add DetectSecretsConfigType()
3. `pkg/api/secrets.go` - Add resolution logic
4. `pkg/api/models.go` - Modify ReconcileForDeploy()
5. `pkg/api/copy.go` - Add Copy() methods
6. `docs/schemas/core/serverdescriptor.json` - Auto-generated

### Files Created

1. `pkg/api/secrets_config.go` - New types
2. `pkg/api/validation.go` - Validation functions
3. `pkg/api/secrets_config_test.go` - Unit tests
4. `docs/docs/examples/secrets/environment-specific/` - Examples

### Implementation Order

**IMPORTANT**: Implement phases in sequential order. Each phase builds on the previous one.

1. Start with Phase 1 (data structures)
2. Complete Phase 2 (detection)
3. Implement Phase 3 (resolution logic)
4. Integrate in Phase 4 (reconciliation)
5. Add validation in Phase 5
6. Regenerate schema in Phase 6
7. Test thoroughly in Phase 7
8. Document in Phase 8
