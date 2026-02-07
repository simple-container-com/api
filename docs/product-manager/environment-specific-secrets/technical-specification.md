# Environment-Specific Secrets - Technical Specification

## Overview

This document describes the technical implementation details for environment-specific secrets in parent stacks, including data structures, API changes, and integration points.

## Data Structures

### New Types in `pkg/api/server.go`

```go
// EnvironmentSecretsConfig defines per-environment secret configuration
type EnvironmentSecretsConfig struct {
    Include  map[string]string `json:"include,omitempty" yaml:"include,omitempty"`
    Exclude  []string          `json:"exclude,omitempty" yaml:"exclude,omitempty"`
    Override map[string]string `json:"override,omitempty" yaml:"override,omitempty"`
}

// ServerSecretsConfig defines environment-specific secrets in server.yaml
type ServerSecretsConfig struct {
    InheritAll   bool                             `json:"inheritAll,omitempty" yaml:"inheritAll,omitempty"`
    Environments map[string]EnvironmentSecretsConfig `json:"environments,omitempty" yaml:"environments,omitempty"`
}
```

### Modified Types

```go
// ServerDescriptor - add SecretsConfig field
type ServerDescriptor struct {
    SchemaVersion   string                        `json:"schemaVersion" yaml:"schemaVersion"`
    Provisioner     ProvisionerDescriptor         `json:"provisioner" yaml:"provisioner"`
    Secrets         SecretsConfigDescriptor       `json:"secrets" yaml:"secrets"`
    SecretsConfig   ServerSecretsConfig           `json:"secretsConfig,omitempty" yaml:"secretsConfig,omitempty"`
    CiCd            CiCdDescriptor                `json:"cicd" yaml:"cicd"`
    Templates       map[string]StackDescriptor    `json:"templates" yaml:"templates"`
    Resources       PerStackResourcesDescriptor   `json:"resources" yaml:"resources"`
    Variables       map[string]VariableDescriptor `json:"variables" yaml:"variables"`
}
```

### Secret Resolution Context

```go
// SecretResolutionContext captures context for resolving secrets
type SecretResolutionContext struct {
    StackParams     StackParams
    ServerSecrets   ServerSecretsConfig
    AvailableSecrets map[string]string
}
```

## Secret Resolution Algorithm

### Resolution Priority

1. **Literal values** (highest priority) - Hardcoded values in server.yaml
2. **Mapped references** - `${secret:KEY}` mappings to secrets.yaml
3. **Direct references** - `~` (null) references to same-named secrets
4. **Inherited secrets** - From `inheritAll: true` (lowest priority)

### Resolution Logic

```go
func ResolveSecretForEnvironment(
    ctx SecretResolutionContext,
    secretName string,
    targetEnv string,
) (string, error) {

    // Step 1: Check if environment-specific config exists
    envConfig, hasEnvConfig := ctx.ServerSecrets.Environments[targetEnv]
    if !hasEnvConfig {
        // No config, use inheritAll default (true)
        if val, ok := ctx.AvailableSecrets[secretName]; ok {
            return val, nil
        }
        return "", ErrorSecretNotFound{Secret: secretName, Environment: targetEnv}
    }

    // Step 2: Check exclusion list
    if ctx.ServerSecrets.InheritAll {
        for _, excluded := range envConfig.Exclude {
            if excluded == secretName {
                return "", ErrorSecretExcluded{Secret: secretName, Environment: targetEnv}
            }
        }
    }

    // Step 3: Check include/override lists
    candidateLists := []map[string]string{}
    if envConfig.Override != nil {
        candidateLists = append(candidateLists, envConfig.Override)
    }
    if envConfig.Include != nil {
        candidateLists = append(candidateLists, envConfig.Include)
    }

    for _, list := range candidateLists {
        if val, ok := list[secretName]; ok {
            return resolveSecretValue(val, ctx.AvailableSecrets)
        }
    }

    // Step 4: Check inherited secrets
    if ctx.ServerSecrets.InheritAll {
        if val, ok := ctx.AvailableSecrets[secretName]; ok {
            return val, nil
        }
    }

    return "", ErrorSecretNotFound{Secret: secretName, Environment: targetEnv}
}

func resolveSecretValue(
    value string,
    availableSecrets map[string]string,
) (string, error) {
    // Case 1: Null/empty value (~ in YAML)
    if value == "" || value == "~" {
        // Return empty string (will use default behavior)
        return "", nil
    }

    // Case 2: Literal value (no ${secret:} prefix)
    if !strings.HasPrefix(value, "${secret:") {
        return value, nil
    }

    // Case 3: Mapped reference ${secret:KEY}
    secretRef := strings.TrimPrefix(value, "${secret:")
    secretRef = strings.TrimSuffix(secretRef, "}")

    if val, ok := availableSecrets[secretRef]; ok {
        return val, nil
    }

    return "", ErrorMappedSecretNotFound{Ref: secretRef}
}
```

## Configuration Reading Changes

### Modified: `pkg/api/read.go`

```go
// ReadServerConfigs - add secrets config parsing
func ReadServerConfigs(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
    if descriptor == nil {
        return nil, errors.Errorf("failed to read descriptor: reference is nil")
    }
    res := *descriptor

    // ... existing provisioner, templates, resources, cicd parsing ...

    // NEW: Parse secrets config if present
    if withSecretsConfig, err := ParseSecretsConfig(&res); err != nil {
        return nil, err
    } else {
        res = *withSecretsConfig
    }

    return &res, nil
}

func ParseSecretsConfig(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
    // Validate that include and exclude are not used together
    for envName, envConfig := range descriptor.SecretsConfig.Environments {
        if len(envConfig.Include) > 0 && len(envConfig.Exclude) > 0 {
            return nil, errors.Errorf(
                "environment %q: cannot use both 'include' and 'exclude' sections",
                envName,
            )
        }

        // Validate that exclude is only used with inheritAll: true
        if len(envConfig.Exclude) > 0 && !descriptor.SecretsConfig.InheritAll {
            return nil, errors.Errorf(
                "environment %q: 'exclude' section requires 'inheritAll: true'",
                envName,
            )
        }

        // Validate mapped references
        for secretName, value := range envConfig.Include {
            if strings.HasPrefix(value, "${secret:") {
                ref := strings.TrimPrefix(value, "${secret:")
                ref = strings.TrimSuffix(ref, "}")
                if ref == "" {
                    return nil, errors.Errorf(
                        "environment %q: invalid secret reference for %q",
                        envName, secretName,
                    )
                }
            }
        }
    }

    return descriptor, nil
}
```

## Validation Changes

### Modified: `pkg/api/secrets.go` or new validation file

```go
// ValidateSecretAvailability checks if referenced secrets are available
func ValidateSecretAvailability(
    serverSecrets ServerSecretsConfig,
    allSecrets map[string]string,
    referencedSecrets map[string]string,
    targetEnv string,
) error {

    availableSecrets := buildAvailableSecretsMap(
        serverSecrets,
        allSecrets,
        targetEnv,
    )

    var errs []error
    for refName := range referencedSecrets {
        if _, ok := availableSecrets[refName]; !ok {
            errs = append(errs, errors.Errorf(
                "secret %q is not available in environment %q",
                refName, targetEnv,
            ))
        }
    }

    if len(errs) > 0 {
        return errors.Errorf("validation failed: %v", errs)
    }

    return nil
}

func buildAvailableSecretsMap(
    serverSecrets ServerSecretsConfig,
    allSecrets map[string]string,
    targetEnv string,
) map[string]bool {

    available := make(map[string]bool)

    envConfig, hasConfig := serverSecrets.Environments[targetEnv]

    if !hasConfig {
        // Default: all secrets available
        for name := range allSecrets {
            available[name] = true
        }
        return available
    }

    if serverSecrets.InheritAll {
        // Start with all secrets
        for name := range allSecrets {
            available[name] = true
        }

        // Remove excluded
        for _, excluded := range envConfig.Exclude {
            delete(available, excluded)
        }
    }

    // Add included secrets (including mapped/literal)
    for name := range envConfig.Include {
        available[name] = true
    }

    // Add overridden secrets
    if envConfig.Override != nil {
        for name := range envConfig.Override {
            available[name] = true
        }
    }

    return available
}
```

## JSON Schema Updates

### Modified: `docs/schemas/core/serverdescriptor.json`

Add to the `properties` section:

```json
{
  "secretsConfig": {
    "type": "object",
    "description": "Environment-specific secrets configuration",
    "properties": {
      "inheritAll": {
        "type": "boolean",
        "description": "When true, all secrets from secrets.yaml are available by default. When false, only explicitly included secrets are available.",
        "default": true
      },
      "environments": {
        "type": "object",
        "description": "Per-environment secret configuration",
        "additionalProperties": {
          "type": "object",
          "properties": {
            "include": {
              "type": "object",
              "description": "Secrets to explicitly include (for inheritAll: false) or override. Values can be: null (~) to reference same-named secret, '${secret:KEY}' to reference different secret, or literal string.",
              "additionalProperties": {
                "oneOf": [
                  { "type": "string" },
                  { "type": "null" }
                ]
              }
            },
            "exclude": {
              "type": "array",
              "description": "Secrets to exclude (only with inheritAll: true)",
              "items": {
                "type": "string"
              }
            },
            "override": {
              "type": "object",
              "description": "Secrets to override with different values (only with inheritAll: true)",
              "additionalProperties": {
                "oneOf": [
                  { "type": "string" },
                  { "type": "null" }
                ]
              }
            }
          },
          "oneOf": [
            {
              "required": ["include"],
              "not": { "required": ["exclude"] }
            },
            {
              "required": ["exclude"],
              "not": { "required": ["include"] }
            },
            {
              "required": ["override"]
            }
          ]
        }
      }
    }
  }
}
```

## Integration Points

### 1. Stack Reconciliation (`pkg/api/models.go`)

The `ReconcileForDeploy` function already handles secrets copying. No changes needed - secret resolution happens later during deployment.

### 2. Provisioner Interface (`pkg/api/mapping.go`)

The provisioner's `DeployStack` and `ProvisionStack` methods receive the `Stack` which includes both `Server` and `Secrets`. The secret resolution should happen:

1. After parent stack reconciliation
2. Before resource provisioning
3. In the context of the target environment

### 3. Client Configuration (`pkg/api/client.go`)

No changes needed to client configuration structures. Secret references in client.yaml use `${secret:NAME}` syntax which is already supported.

## Error Handling

### New Error Types

```go
type ErrorSecretNotFound struct {
    Secret      string
    Environment string
}

func (e ErrorSecretNotFound) Error() string {
    return fmt.Sprintf("secret %q not available in environment %q", e.Secret, e.Environment)
}

type ErrorSecretExcluded struct {
    Secret      string
    Environment string
}

func (e ErrorSecretExcluded) Error() string {
    return fmt.Sprintf("secret %q is excluded in environment %q", e.Secret, e.Environment)
}

type ErrorMappedSecretNotFound struct {
    Ref string
}

func (e ErrorMappedSecretNotFound) Error() string {
    return fmt.Sprintf("mapped secret reference %q not found in secrets.yaml", e.Ref)
}
```

## Backwards Compatibility Strategy

1. **Default Behavior**: When `secretsConfig` section is absent, default to `inheritAll: true` with no environment-specific configuration
2. **Opt-In**: Only when `secretsConfig` is present does the new logic apply
3. **Graceful Degradation**: Invalid configurations fail at validation time, not runtime
4. **No Client Changes**: Client configurations continue to work without modification

## Testing Requirements

### Unit Tests

1. **Secret Resolution**
   - Test include mode with direct references
   - Test include mode with mapped references
   - Test include mode with literal values
   - Test exclude mode
   - Test override mode
   - Test inheritAll true/false variations

2. **Validation**
   - Test validation errors for missing secrets
   - Test validation errors for invalid mappings
   - Test validation errors for combined include/exclude
   - Test validation warnings for unused secrets

3. **Backwards Compatibility**
   - Test existing configs without secretsConfig
   - Test default inheritAll behavior
   - Test existing client configs

### Integration Tests

1. Deploy stack to staging with staging-specific secrets
2. Deploy stack to production with production-specific secrets
3. Verify production secrets not accessible in staging
4. Test with multi-region parent stacks

## Performance Considerations

1. **Secret Resolution**: O(n) where n is number of secrets in environment config
2. **Caching**: Resolved secrets can be cached per environment within a single deployment
3. **No Disk I/O**: Secrets are already loaded in memory from secrets.yaml

## Security Considerations

1. **Secrets.yaml**: Remains encrypted at rest
2. **Server.yaml**: Mapping logic is unencrypted but doesn't contain secret values
3. **Runtime**: Secrets exist in memory only during deployment
4. **Audit Trail**: Secret mappings in server.yaml provide audit trail of which secrets go where

## Migration Path

No explicit migration needed. Feature is fully backwards compatible:

1. Existing stacks work without modification
2. Teams adopt incrementally per stack
3. Can enable for specific environments first
4. Can start with exclude mode (safer) before moving to include mode

## Future Enhancements (Out of Scope)

1. Secret versioning
2. Secret rotation
3. Dynamic secret generation
4. Secret inheritance between environments
5. Secret templates with variable interpolation
