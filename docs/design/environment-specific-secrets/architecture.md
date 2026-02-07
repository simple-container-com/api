# Environment-Specific Secrets - Architecture Design

## 1. Overview

This document describes the architecture for implementing environment-specific secrets in parent stacks. The feature enables precise control over which secrets are available to child stacks on a per-environment basis.

## 2. Current State Analysis

### 2.1 Existing Secrets Implementation

The current implementation has these key components:

**SecretsDescriptor** (`pkg/api/secrets.go`):
```go
type SecretsDescriptor struct {
    SchemaVersion string                    `json:"schemaVersion" yaml:"schemaVersion"`
    Auth          map[string]AuthDescriptor `json:"auth" yaml:"auth"`
    Values        map[string]string         `json:"values" yaml:"values"`
}
```

**SecretsConfigDescriptor** (`pkg/api/server.go`):
```go
type SecretsConfigDescriptor struct {
    Type    string `json:"type" yaml:"type"`
    Config  `json:",inline" yaml:",inline"`
    Inherit `json:",inline" yaml:",inline"`
}
```

**ServerDescriptor** (`pkg/api/server.go`):
```go
type ServerDescriptor struct {
    SchemaVersion string                        `json:"schemaVersion" yaml:"schemaVersion"`
    Provisioner   ProvisionerDescriptor         `json:"provisioner" yaml:"provisioner"`
    Secrets       SecretsConfigDescriptor       `json:"secrets" yaml:"secrets"`
    // ... other fields
}
```

### 2.2 Current Secret Resolution Flow

1. `ReadServerConfigs()` reads and processes server.yaml
2. `DetectSecretsType()` identifies the secrets provider type
3. `ReconcileForDeploy()` copies parent stack's secrets to child stacks
4. **All secrets from parent stack's secrets.yaml are copied to child stack**
5. No environment-based filtering occurs

### 2.3 Problems with Current Implementation

1. **Security Risk**: Production secrets accessible in dev/staging environments
2. **Naming Conflicts**: Same secret name needs different values per environment
3. **No Isolation**: Cannot limit which secrets are available per environment
4. **All-or-Nothing**: Either all secrets or no secrets from parent stack

## 3. New Data Structures

### 3.1 EnvironmentSecretsConfig

Configuration for secrets in a specific environment:

```go
// EnvironmentSecretsConfig configures secrets for a specific environment
type EnvironmentSecretsConfig struct {
    // InheritAll when true includes all secrets except those in Exclude
    // When false, only secrets in Include are available
    InheritAll bool `json:"inheritAll" yaml:"inheritAll"`

    // Include lists secret names to explicitly allow (when inheritAll: false)
    // Each entry can be:
    // - "~" (direct reference: same key name in secrets.yaml)
    // - "${secret:OTHER_KEY}" (mapped reference: different key name)
    Include []string `json:"include,omitempty" yaml:"include,omitempty"`

    // Exclude lists secret names to block (when inheritAll: true)
    Exclude []string `json:"exclude,omitempty" yaml:"exclude,omitempty"`

    // Secrets defines literal secret values (not fetched from secrets.yaml)
    // These override any values from secrets.yaml
    Secrets map[string]string `json:"secrets,omitempty" yaml:"secrets,omitempty"`
}
```

**Usage Examples:**

```yaml
# Include mode: explicit allow list
secretsConfig:
  staging:
    inheritAll: false
    include:
      - "~"                          # ERROR: ambiguous - must specify secret name
      - DATABASE_URL                 # Direct reference to DATABASE_URL
      - "${secret:DB_HOST_STAGING}"  # Mapped to DATABASE_URL
    secrets:
      ENVIRONMENT: "staging"         # Literal value

# Exclude mode: block specific secrets
secretsConfig:
  production:
    inheritAll: true
    exclude:
      - DEV_API_KEY
      - TEST_DATABASE_URL
```

### 3.2 SecretsConfigMap

Container for per-environment configurations:

```go
// SecretsConfigMap holds per-environment secret configurations
type SecretsConfigMap map[string]EnvironmentSecretsConfig
```

### 3.3 SecretReferenceType

Enum for reference pattern types:

```go
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

### 3.4 SecretResolver

Core component for resolving secrets based on environment configuration:

```go
// SecretResolver resolves secret references based on environment configuration
type SecretResolver struct {
    secretsConfig *SecretsConfigMap  // Per-environment configuration
    allSecrets    map[string]string  // All secrets from secrets.yaml
    environment   string             // Current environment
}

// Resolve returns the resolved secrets map for the configured environment
func (r *SecretResolver) Resolve() (map[string]string, error)

// resolveIncludeMode handles include mode (explicit allow list)
func (r *SecretResolver) resolveIncludeMode(envConfig EnvironmentSecretsConfig) (map[string]string, error)

// resolveExcludeMode handles exclude mode (block list)
func (r *SecretResolver) resolveExcludeMode(envConfig EnvironmentSecretsConfig) (map[string]string, error)

// parseSecretReference parses a secret reference string
func (r *SecretResolver) parseSecretReference(ref string) (refType SecretReferenceType, key string, err error)

// fetchSecret fetches a secret value from allSecrets
func (r *SecretResolver) fetchSecret(key string) (string, error)
```

### 3.5 Updated ServerDescriptor

Modified to include optional `secretsConfig`:

```go
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

**Note**: `SecretsConfig` is optional (omitempty) for backwards compatibility.

## 4. Component Architecture

### 4.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        server.yaml                               │
│  ┌────────────┐  ┌────────────┐  ┌──────────────────────────┐  │
│  │  secrets   │  │secretsConfig│ │     (optional)           │  │
│  │  type: aws │  │ staging:   │ │  inheritAll: false       │  │
│  │            │  │   include: │ │  include:                │  │
│  │            │  │     - ...  │ │    - DATABASE_URL        │  │
│  └────────────┘  └────────────┘ │    - API_KEY             │  │
│                                  └──────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                    ReadServerConfigs()                          │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  DetectSecretsConfigType()                               │  │
│  │    - Validates secretsConfig structure                   │  │
│  │    - Detects mode (include/exclude)                      │  │
│  │    - Returns validation errors if needed                 │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                    ReconcileForDeploy()                         │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  For each child stack:                                   │  │
│  │    1. Copy parent's ServerDescriptor                      │  │
│  │    2. Create SecretResolver for child's environment      │  │
│  │    3. Resolve secrets based on secretsConfig             │  │
│  │    4. Replace Secrets.Values with resolved secrets       │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                       SecretResolver                            │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  Resolve()                                                │  │
│  │    if secretsConfig is nil:                               │  │
│  │      return allSecrets (backwards compatibility)          │  │
│  │    if environment not in secretsConfig:                   │  │
│  │      return error (environment not configured)            │  │
│  │    envConfig := secretsConfig[environment]                │  │
│  │    if envConfig.InheritAll:                               │  │
│  │      return resolveExcludeMode(envConfig)                 │  │
│  │    else:                                                   │  │
│  │      return resolveIncludeMode(envConfig)                 │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Resolved Secrets                             │
│  Only secrets configured for the environment are available     │
└─────────────────────────────────────────────────────────────────┘
```

### 4.2 Secret Resolution Algorithm

The `SecretResolver.Resolve()` method implements a 6-step algorithm:

```
STEP 1: Check for backwards compatibility
  if secretsConfig is nil:
    return allSecrets (no filtering)

STEP 2: Validate environment configuration exists
  if environment not in secretsConfig:
    return error "environment '{env}' not configured in secretsConfig"

STEP 3: Get environment configuration
  envConfig := secretsConfig[environment]

STEP 4: Route to appropriate mode handler
  if envConfig.InheritAll == true:
    return resolveExcludeMode(envConfig)
  else:
    return resolveIncludeMode(envConfig)

STEP 5A: Include mode resolution (when InheritAll: false)
  resolved := {}
  for ref in envConfig.Include:
    refType, key, err := parseSecretReference(ref)
    if err != return err
    if refType == Direct:
      resolved[ref] = fetchSecret(ref)
    else if refType == Mapped:
      resolved[targetName] = fetchSecret(key)
  // Add literal values
  for name, value in envConfig.Secrets:
    resolved[name] = value
  return resolved

STEP 5B: Exclude mode resolution (when InheritAll: true)
  resolved := copy(allSecrets)
  for secretName in envConfig.Exclude:
    delete(resolved, secretName)
  // Add/override with literal values
  for name, value in envConfig.Secrets:
    resolved[name] = value
  return resolved

STEP 6: Validate no unavailable secrets referenced
  (Handled in ValidateSecretsReferences() during validation phase)
```

### 4.3 Reference Pattern Parsing

The `parseSecretReference()` method handles three patterns:

```go
func (r *SecretResolver) parseSecretReference(ref string) (SecretReferenceType, string, error) {
    // Pattern 1: Mapped reference "${secret:KEY_NAME}"
    if strings.HasPrefix(ref, "${secret:") && strings.HasSuffix(ref, "}") {
        key := strings.TrimPrefix(ref, "${secret:")
        key = strings.TrimSuffix(key, "}")
        return SecretReferenceMapped, key, nil
    }

    // Pattern 2: Direct reference - must be a valid secret name
    // (not "~" - that was an error in the spec, should be actual secret name)
    if ref != "" && !strings.Contains(ref, "$") {
        return SecretReferenceDirect, ref, nil
    }

    return SecretReferenceLiteral, ref, nil
}
```

**Examples:**

| Input Reference | Type | Fetched Key | Target Name |
|----------------|------|-------------|-------------|
| `DATABASE_URL` | Direct | `DATABASE_URL` | `DATABASE_URL` |
| `${secret:DB_HOST_STAGING}` | Mapped | `DB_HOST_STAGING` | `DATABASE_URL` (from include key) |
| `"literal-value"` | Literal | N/A | N/A |

## 5. Integration Points

### 5.1 Configuration Reading (`pkg/api/read.go`)

**New Function:**

```go
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

        // Validate: inheritAll requires exclude or neither include/exclude
        if !config.InheritAll && len(config.Include) == 0 && len(config.Exclude) == 0 && len(config.Secrets) == 0 {
            return nil, errors.Errorf("secretsConfig.%s: must specify 'include', 'exclude', or 'secrets' when inheritAll is false", env)
        }

        // Validate: each include entry is a valid reference
        for i, ref := range config.Include {
            if ref == "" {
                return nil, errors.Errorf("secretsConfig.%s.include[%d]: reference cannot be empty", env, i)
            }
            if ref == "~" {
                return nil, errors.Errorf("secretsConfig.%s.include[%d]: direct reference must specify secret name (use the actual secret name, not '~')", env, i)
            }
        }
    }

    return descriptor, nil
}
```

**Integration into `ReadServerConfigs()`:**

```go
func ReadServerConfigs(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
    // ... existing detection calls ...

    // NEW: Detect and validate secretsConfig
    if withSecretsConfig, err := DetectSecretsConfigType(&res); err != nil {
        return nil, err
    } else {
        res = *withSecretsConfig
    }

    // ... rest of function ...
}
```

### 5.2 Stack Reconciliation (`pkg/api/models.go`)

**Modified `ReconcileForDeploy()` method:**

```go
func (m *StacksMap) ReconcileForDeploy(params StackParams) (*StacksMap, error) {
    current := *m
    iterMap := lo.Assign(current)
    for stackName, stack := range iterMap {
        if len(stack.Client.Stacks) == 0 {
            continue
        }
        clientDesc, ok := stack.Client.Stacks[params.Environment]
        if !ok && stackName != params.StackName {
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
                resolved, err := ResolveSecretsForEnvironment(
                    &parentStack.Server,
                    &stack.Secrets,
                    clientDesc.ParentEnv,
                )
                if err != nil {
                    return nil, errors.Wrapf(err, "failed to resolve secrets for stack %q", stackName)
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

### 5.3 Secret Resolution Function (`pkg/api/secrets.go`)

**New Function:**

```go
// ResolveSecretsForEnvironment resolves secrets for a specific environment
// based on the parent stack's secretsConfig
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
```

## 6. Three Operation Modes

### 6.1 Include Mode (Explicit Allow List)

**When**: `inheritAll: false`

**Behavior**: Only secrets listed in `include` are available

**Example:**

```yaml
secretsConfig:
  staging:
    inheritAll: false
    include:
      - DATABASE_URL           # Direct: fetch DATABASE_URL
      - "${secret:API_KEY_STAGING}"  # Mapped: fetch API_KEY_STAGING, assign to API_KEY
    secrets:
      ENVIRONMENT: "staging"   # Literal value
```

**Result**: Only 3 secrets available:
- `DATABASE_URL` (from secrets.yaml)
- `API_KEY` (mapped from `API_KEY_STAGING` in secrets.yaml)
- `ENVIRONMENT` (literal "staging")

### 6.2 Exclude Mode (Block List)

**When**: `inheritAll: true`

**Behavior**: All secrets except those in `exclude` are available

**Example:**

```yaml
secretsConfig:
  production:
    inheritAll: true
    exclude:
      - DEV_API_KEY
      - STAGING_DB_PASSWORD
    secrets:
      ENVIRONMENT: "production"  # Override/add literal value
```

**Result**: All secrets from secrets.yaml except `DEV_API_KEY` and `STAGING_DB_PASSWORD`, plus `ENVIRONMENT: "production"`.

### 6.3 Override Mode (Literal Values)

**When**: Using `secrets` map

**Behavior**: Literal values override or add secrets

**Example:**

```yaml
secretsConfig:
  development:
    inheritAll: false
    include:
      - DATABASE_URL
    secrets:
      DATABASE_URL: "localhost:5432/dev"  # Override with literal value
      DEBUG: "true"                       # Add new secret
```

**Result**: `DATABASE_URL` is "localhost:5432/dev" (not fetched from secrets.yaml), `DEBUG: "true"`.

## 7. Validation Logic

### 7.1 Configuration Validation

Validated in `DetectSecretsConfigType()`:

1. **Include/InheritAll conflict**: Can't use `include` with `inheritAll: true`
2. **Include/Exclude conflict**: Can't use both `include` and `exclude`
3. **Empty configuration**: Must specify `include`, `exclude`, or `secrets`
4. **Invalid references**: Include entries must be valid secret names or mapped references
5. **Ambiguous references**: Cannot use "~" (must specify actual secret name)

### 7.2 Secret Availability Validation

Validated during deployment/reconciliation:

```go
// ValidateSecretsReferences checks that all referenced secrets are available
func ValidateSecretsReferences(
    serverDesc *ServerDescriptor,
    secretsDesc *SecretsDescriptor,
    environment string,
) error {
    if serverDesc.SecretsConfig == nil {
        return nil // No filtering, all secrets available
    }

    envConfig, ok := (*serverDesc.SecretsConfig)[environment]
    if !ok {
        return errors.Errorf("environment %q not configured in secretsConfig", environment)
    }

    // Validate include references
    for _, ref := range envConfig.Include {
        refType, key, err := parseSecretReference(ref)
        if err != nil {
            return errors.Wrapf(err, "invalid secret reference %q", ref)
        }

        if refType != SecretReferenceLiteral {
            if _, exists := secretsDesc.Values[key]; !exists {
                return errors.Errorf("secret %q referenced in include does not exist in secrets.yaml", key)
            }
        }
    }

    // Validate exclude references
    for _, name := range envConfig.Exclude {
        if _, exists := secretsDesc.Values[name]; !exists {
            return errors.Errorf("secret %q referenced in exclude does not exist in secrets.yaml", name)
        }
    }

    return nil
}
```

## 8. Security Considerations

### 8.1 Principle of Least Privilege

- Include mode allows explicit listing of required secrets
- Exclude mode enables blocking sensitive secrets from lower environments
- Parent stack controls all child stack access

### 8.2 Fail-Safe Behavior

- Invalid configuration returns errors during validation (not silently ignored)
- Missing environment configuration is an error (not fallback to all secrets)
- Invalid secret references are caught before deployment

### 8.3 Audit Trail

- Configuration is declarative and version-controlled
- All secrets used are explicitly listed or inherited
- Easy to audit which secrets each environment has access to

## 9. Performance Considerations

### 9.1 Secret Resolution Complexity

- **Include mode**: O(n) where n = number of secrets in include list
- **Exclude mode**: O(m) where m = number of secrets in secrets.yaml
- **No caching needed**: Resolution happens once per deployment

### 9.2 Memory Impact

- `SecretsConfigMap`: Small (configuration only, not secret values)
- `SecretResolver`: Transient (created and destroyed during reconciliation)
- `Resolved secrets`: Same size as original secrets map

## 10. Error Handling

### 10.1 Validation Errors

| Error | Condition | Message |
|-------|-----------|---------|
| `IncludeWithInheritAll` | `inheritAll: true` with `include` | "cannot use 'include' with 'inheritAll: true'" |
| `IncludeAndExclude` | Both `include` and `exclude` specified | "cannot use both 'include' and 'exclude'" |
| `EmptyConfiguration` | No include/exclude/secrets | "must specify 'include', 'exclude', or 'secrets'" |
| `InvalidReference` | Malformed reference | "invalid secret reference %q" |
| `SecretNotFound` | Referenced secret doesn't exist | "secret %q does not exist in secrets.yaml" |
| `EnvironmentNotConfigured` | Environment not in secretsConfig | "environment %q not configured in secretsConfig" |

### 10.2 Runtime Errors

| Error | Condition | Recovery |
|-------|-----------|----------|
| `SecretUnavailable` | Client references unavailable secret | Fail deployment with clear error message |
| `ParentStackNotFound` | Parent stack missing during reconciliation | Fail deployment |
| `ResolutionFailure` | Error during secret resolution | Fail deployment |

## 11. Backwards Compatibility

### 11.1 No Breaking Changes

- `secretsConfig` field is optional (omitempty)
- When `secretsConfig` is nil, behavior is identical to current implementation
- All secrets are available to all environments (current behavior)

### 11.2 Migration Path

1. **Phase 1**: Add `secretsConfig` to new parent stacks
2. **Phase 2**: Gradually add `secretsConfig` to existing parent stacks
3. **Phase 3**: Eventually make `secretsConfig` required (future version)

## 12. Testing Strategy

### 12.1 Unit Tests

- Test each mode (include, exclude, override)
- Test reference pattern parsing
- Test validation logic
- Test error conditions
- Test backwards compatibility

### 12.2 Integration Tests

- Test full deployment flow with secretsConfig
- Test child stack reconciliation
- Test error propagation
- Test multiple environments

### 12.3 End-to-End Tests

- Deploy stacks with secretsConfig
- Verify only configured secrets are available
- Test failure scenarios
- Validate error messages

## 13. File Summary

### New Files to Create

1. `pkg/api/secrets_config.go` - EnvironmentSecretsConfig, SecretsConfigMap, SecretResolver
2. `pkg/api/validation.go` - ValidateSecretsReferences, ValidateSecretsConfig

### Files to Modify

1. `pkg/api/server.go` - Add SecretsConfig field to ServerDescriptor
2. `pkg/api/read.go` - Add DetectSecretsConfigType(), integrate into ReadServerConfigs()
3. `pkg/api/models.go` - Modify ReconcileForDeploy() to use secret resolution
4. `pkg/api/copy.go` - Add Copy() method for SecretsConfigMap
5. `pkg/api/secrets.go` - Add ResolveSecretsForEnvironment() function
6. `cmd/schema-gen/main.go` - No changes needed (schema auto-generated from structs)
