# Architecture: Environment-Specific Secrets in Parent Stacks

## Overview

This document describes the architecture for implementing environment-specific secrets configuration in Simple Container parent stacks. The feature enables fine-grained control over which secrets are available in each environment (production, staging, development), addressing security concerns and providing better isolation.

## Current State Analysis

### Existing Secrets Structure

The current secrets system has these key components:

1. **`SecretsConfigDescriptor`** (`pkg/api/server.go:133-137`):
   ```go
   type SecretsConfigDescriptor struct {
       Type    string `json:"type" yaml:"type"`
       Config  `json:",inline" yaml:",inline"`
       Inherit `json:",inline" yaml:",inline"`
   }
   ```

2. **`SecretsDescriptor`** (`pkg/api/secrets.go:8-12`):
   ```go
   type SecretsDescriptor struct {
       SchemaVersion string                    `json:"schemaVersion" yaml:"schemaVersion"`
       Auth          map[string]AuthDescriptor `json:"auth" yaml:"auth"`
       Values        map[string]string         `json:"values" yaml:"values"`
   }
   ```

3. **Secret Resolution Flow**:
   - `ReadServerConfigs()` → `DetectSecretsType()` → provider config mapping
   - Client stacks reference secrets via `${secret:NAME}` syntax
   - All secrets in `secrets.yaml` are globally available to all environments

### Problem Statement

**Security Risk**: Production secrets (API keys, passwords) are accessible in dev/staging environments because there's no environment-level filtering.

**Lack of Isolation**: The same secret name cannot have different values per environment without naming conflicts.

**Current Limitations**:
- No per-environment secret allow/block lists
- All secrets in `secrets.yaml` are universally available
- No mechanism to override secret values per environment

## Proposed Architecture

### 1. New Data Structures

#### 1.1 Environment-Specific Secrets Configuration

Add to `pkg/api/server.go`:

```go
// EnvironmentSecretsConfig defines which secrets are available for a specific environment
type EnvironmentSecretsConfig struct {
    // Mode: "include" (allow list), "exclude" (block list), "override" (replace)
    Mode string `json:"mode" yaml:"mode"`

    // Secrets: Map of secret references (key: client-facing name, value: source)
    // Three patterns supported:
    //   1. Direct reference: "~" -> use same name from secrets.yaml
    //   2. Mapped reference: "${secret:SOURCE_KEY}" -> use different source key
    //   3. Literal value: "actual-value" -> use literal string (not from secrets.yaml)
    Secrets map[string]string `json:"secrets" yaml:"secrets"`
}

// SecretsConfigDescriptor extends existing structure
type SecretsConfigDescriptor struct {
    Type    string                                   `json:"type" yaml:"type"`
    Config  `json:",inline" yaml:",inline"`
    Inherit `json:",inline" yaml:",inline"`

    // NEW: Environment-specific configuration
    SecretsConfig *SecretsConfigMap `json:"secretsConfig,omitempty" yaml:"secretsConfig,omitempty"`
}

// SecretsConfigMap contains per-environment secret configurations
type SecretsConfigMap struct {
    // InheritAll: When true, all secrets are available by default (legacy behavior)
    // Individual environments can still exclude specific secrets
    InheritAll bool `json:"inheritAll" yaml:"inheritAll"`

    // Environments: Map of environment name to configuration
    // Examples: "production", "staging", "development"
    Environments map[string]EnvironmentSecretsConfig `json:"environments" yaml:"environments"`
}
```

#### 1.2 Secret Resolution Context

Add to `pkg/api/secrets.go`:

```go
// SecretResolutionContext holds context for resolving secret references
type SecretResolutionContext struct {
    Environment       string
    StackName         string
    ParentStackName   string
    AvailableSecrets  map[string]string // Resolved secrets for this context
}

// SecretResolver handles environment-aware secret resolution
type SecretResolver struct {
    globalSecrets   *SecretsDescriptor
    secretsConfig   *SecretsConfigMap
}
```

### 2. Component Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                          Parent Stack                                │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  server.yaml                                                 │   │
│  │  ┌─────────────────────────────────────────────────────┐    │   │
│  │  │  secrets:                                           │    │   │
│  │  │    type: fs-secrets                                │    │   │
│  │  │  secretsConfig:                                     │    │   │
│  │  │    inheritAll: false                               │    │   │
│  │  │    environments:                                   │    │   │
│  │  │      production:                                   │    │   │
│  │  │        mode: include                               │    │   │
│  │  │        secrets:                                    │    │   │
│  │  │          API_KEY: "${secret:PROD_API_KEY}"         │    │   │
│  │  │          DB_PASSWORD: "~"                          │    │   │
│  │  │      staging:                                      │    │   │
│  │  │        mode: include                               │    │   │
│  │  │        secrets:                                    │    │   │
│  │  │          API_KEY: "${secret:STAGING_API_KEY}"      │    │   │
│  │  │          DB_PASSWORD: "~"                          │    │   │
│  │  └─────────────────────────────────────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  secrets.yaml                                                │   │
│  │  values:                                                     │   │
│  │    PROD_API_KEY: "prod-key-123"                             │   │
│  │    STAGING_API_KEY: "staging-key-456"                       │   │
│  │    DB_PASSWORD: "secure-password"                           │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              │ Inheritance
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│                          Client Stack                                │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  client.yaml                                                │   │
│  │  stacks:                                                    │   │
│  │    production:                                              │   │
│  │      parent: company/infrastructure                         │   │
│  │      config:                                                │   │
│  │        secrets:                                             │   │
│  │          DATABASE_PASSWORD: "${secret:DB_PASSWORD}"         │   │
│  │          API_KEY: "${secret:API_KEY}"                       │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

### 3. Secret Resolution Algorithm

```
┌──────────────────────────────────────────────────────────────┐
│                    RESOLVE SECRET                            │
│                   (secret: SECRET_NAME)                      │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│  1. CHECK: Is secretsConfig defined?                          │
│     NO → Use legacy behavior (all secrets available)          │
│     YES → Continue to step 2                                  │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│  2. CHECK: Is environment configured in secretsConfig?        │
│     NO → Apply inheritAll logic                               │
│     YES → Continue to step 3                                  │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│  3. DETERMINE: Available secrets based on mode               │
│                                                               │
│     Mode "include": Only listed secrets are available         │
│     Mode "exclude": All secrets except listed are available   │
│     Mode "override": Replace global secrets with mapped ones  │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│  4. RESOLVE: Secret value based on reference pattern         │
│                                                               │
│     Pattern "~" → Use same name from secrets.yaml            │
│     Pattern "${secret:KEY}" → Use KEY from secrets.yaml       │
│     Pattern "literal-value" → Use literal string              │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│  5. VALIDATE: Secret exists in secrets.yaml (if referenced)  │
│     ERROR if not found                                        │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│  6. RETURN: Resolved secret value                            │
└──────────────────────────────────────────────────────────────┘
```

### 4. Integration Points

#### 4.1 Configuration Reading (`pkg/api/read.go`)

**Modify: `ReadServerConfigs()`**
- Add new detection step: `DetectSecretsConfigType()`
- Process `secretsConfig` section if present
- Create `SecretResolver` with context

**New Function: `DetectSecretsConfigType()`**
```go
func DetectSecretsConfigType(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
    if descriptor.Secrets.SecretsConfig == nil {
        return descriptor, nil // No secretsConfig, use legacy behavior
    }

    // Validate configuration
    if err := validateSecretsConfig(descriptor.Secrets.SecretsConfig); err != nil {
        return nil, errors.Wrapf(err, "invalid secretsConfig")
    }

    return descriptor, nil
}
```

#### 4.2 Stack Reconciliation (`pkg/api/models.go`)

**Modify: `ReconcileForDeploy()`**
- Add environment context to secret resolution
- Call `SecretResolver` for each stack during reconciliation
- Filter secrets based on environment configuration

#### 4.3 Validation (`pkg/api/validation.go` - new file)

**New File: `pkg/api/validation.go`**
```go
// ValidateSecretsConfig validates the secretsConfig structure
func ValidateSecretsConfig(config *SecretsConfigMap) error {
    // Validate modes
    // Validate secret reference patterns
    // Validate secret availability in global secrets.yaml
}
```

### 5. Data Flow

```
User runs: sc deploy --environment production
                 │
                 ▼
┌─────────────────────────────────────────────────────────────┐
│  1. Read server.yaml (parent stack)                         │
│     - Parse secretsConfig section                           │
│     - Load secrets.yaml                                     │
│     - Create SecretResolver with production context         │
└─────────────────────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────┐
│  2. Read client.yaml                                        │
│     - Parse secret references (${secret:NAME})              │
│     - Identify parent stack relationship                    │
└─────────────────────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────┐
│  3. Reconcile stacks (ReconcileForDeploy)                   │
│     - Resolve secret references                             │
│     - Apply environment filtering via SecretResolver        │
│     - Validate secret availability                          │
└─────────────────────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────┐
│  4. Deploy with resolved secrets                            │
│     - Only production-configured secrets are injected       │
│     - Validation errors prevent deployment if misconfigured │
└─────────────────────────────────────────────────────────────┘
```

### 6. Modes of Operation

#### 6.1 Include Mode (Allow List)

**YAML Configuration:**
```yaml
secretsConfig:
  inheritAll: false
  environments:
    production:
      mode: include
      secrets:
        DATABASE_URL: "~"
        API_KEY: "${secret:PROD_API_KEY}"
        API_SECRET: "literal-secret-value"
```

**Behavior:**
- Only the 3 listed secrets are available in production
- `DATABASE_URL` resolves to `DATABASE_URL` from secrets.yaml
- `API_KEY` resolves to `PROD_API_KEY` from secrets.yaml
- `API_SECRET` uses the literal value "literal-secret-value"
- Any other secret reference results in validation error

#### 6.2 Exclude Mode (Block List)

**YAML Configuration:**
```yaml
secretsConfig:
  inheritAll: true
  environments:
    development:
      mode: exclude
      secrets:
        PROD_CREDENTIALS: "~"
        PROD_API_KEY: "~"
```

**Behavior:**
- All secrets from secrets.yaml are available EXCEPT those listed
- Useful for preventing production secrets from leaking to dev

#### 6.3 Override Mode

**YAML Configuration:**
```yaml
secretsConfig:
  inheritAll: false
  environments:
    staging:
      mode: override
      secrets:
        API_ENDPOINT: "https://staging.example.com"
        DEBUG: "true"
```

**Behavior:**
- Only listed secrets are available
- All values are literal (no references to secrets.yaml)
- Useful for environment-specific configuration values

### 7. Backwards Compatibility

**Legacy Behavior (no secretsConfig section):**
```yaml
# server.yaml (existing format)
secrets:
  type: fs-secrets
# No secretsConfig section → all secrets available to all environments
```

**New Behavior (opt-in):**
```yaml
# server.yaml (new format with secretsConfig)
secrets:
  type: fs-secrets
secretsConfig:
  inheritAll: false
  environments:
    production:
      mode: include
      secrets: ...
```

**Migration Path:**
1. Existing parent stacks without `secretsConfig` work unchanged
2. Adding `secretsConfig` is opt-in
3. `inheritAll: true` mimics legacy behavior while allowing exclusions

### 8. Error Handling

**Validation Errors:**
```
Error: secret "PROD_API_KEY" is not available in environment "staging"
  → Secret is not in staging's include list

Error: secret "${secret:NONEXISTENT}" references undefined secret in secrets.yaml
  → Referenced secret doesn't exist in global secrets.yaml

Error: invalid mode "invalid-mode" for environment "production"
  → Must be one of: include, exclude, override
```

**Validation Points:**
1. YAML parsing (detect malformed configuration)
2. Mode validation (only include/exclude/override allowed)
3. Secret reference validation (referenced secrets must exist)
4. Environment-specific validation (requested secrets must be available)

## Security Considerations

1. **Secret Isolation**: Production secrets cannot leak to lower environments
2. **Validation**: Fail-fast when invalid secret references are detected
3. **Audit Trail**: Clear configuration makes it easy to audit which secrets are available where
4. **Least Privilege**: Default to `inheritAll: false` (explicit allow list)

## Performance Considerations

1. **One-Time Resolution**: Secrets resolved once during configuration reading
2. **Caching**: Resolved secrets cached in `SecretResolutionContext`
3. **Minimal Overhead**: Only impacts stacks with `secretsConfig` defined
4. **Lazy Validation**: Validation happens during `sc validate`, not during every operation

## Testing Strategy

1. **Unit Tests**: Test each mode (include/exclude/override) independently
2. **Integration Tests**: Test full stack reconciliation with various configurations
3. **Backwards Compatibility Tests**: Ensure existing stacks work unchanged
4. **Validation Tests**: Test error conditions and edge cases
5. **Performance Tests**: Measure overhead of secret resolution

## Future Enhancements (Out of Scope)

1. Secret rotation mechanisms
2. Secret versioning
3. Integration with external secret managers (AWS Secrets Manager, Vault)
4. Dynamic secret generation
5. UI/CLI for managing secrets configuration
