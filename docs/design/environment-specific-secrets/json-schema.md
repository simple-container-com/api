# JSON Schema Design - Environment-Specific Secrets

## 1. Overview

This document describes the JSON schema changes required to support environment-specific secrets in parent stacks.

## 2. Go Struct Modifications

### 2.1 pkg/api/server.go

**Current ServerDescriptor:**

```go
type ServerDescriptor struct {
    SchemaVersion string                        `json:"schemaVersion" yaml:"schemaVersion"`
    Provisioner   ProvisionerDescriptor         `json:"provisioner" yaml:"provisioner"`
    Secrets       SecretsConfigDescriptor       `json:"secrets" yaml:"secrets"`
    CiCd          CiCdDescriptor                `json:"cicd" yaml:"cicd"`
    Templates     map[string]StackDescriptor    `json:"templates" yaml:"templates"`
    Resources     PerStackResourcesDescriptor   `json:"resources" yaml:"resources"`
    Variables     map[string]VariableDescriptor `json:"variables" yaml:"variables"`
}
```

**Modified ServerDescriptor:**

```go
type ServerDescriptor struct {
    SchemaVersion string                        `json:"schemaVersion" yaml:"schemaVersion"`
    Provisioner   ProvisionerDescriptor         `json:"provisioner" yaml:"provisioner"`
    Secrets       SecretsConfigDescriptor       `json:"secrets" yaml:"secrets"`
    SecretsConfig *SecretsConfigMap             `json:"secretsConfig,omitempty" yaml:"secretsConfig,omitempty"` // NEW: Optional for backwards compatibility
    CiCd          CiCdDescriptor                `json:"cicd" yaml:"cicd"`
    Templates     map[string]StackDescriptor    `json:"templates" yaml:"templates"`
    Resources     PerStackResourcesDescriptor   `json:"resources" yaml:"resources"`
    Variables     map[string]VariableDescriptor `json:"variables" yaml:"variables"`
}
```

**Key Changes:**
- Added `SecretsConfig *SecretsConfigMap` field
- Field is optional (omitempty tag) for backwards compatibility
- Pointer type (*SecretsConfigMap) allows nil checking

### 2.2 New Types (pkg/api/secrets.go or new file pkg/api/secrets_config.go)

```go
// SecretsConfigMap holds per-environment secret configurations
type SecretsConfigMap map[string]EnvironmentSecretsConfig

// EnvironmentSecretsConfig configures secrets for a specific environment
type EnvironmentSecretsConfig struct {
    // InheritAll when true includes all secrets except those in Exclude
    // When false, only secrets in Include are available
    InheritAll bool `json:"inheritAll" yaml:"inheritAll"`

    // Include lists secret names to explicitly allow (when inheritAll: false)
    // Each entry can be:
    // - A direct secret name (e.g., "DATABASE_URL")
    // - A mapped reference (e.g., "${secret:DB_HOST_STAGING}")
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
    SecretReferenceDirect  SecretReferenceType = "direct"   // Direct reference to secret name
    SecretReferenceMapped  SecretReferenceType = "mapped"   // Mapped reference like ${secret:KEY}
    SecretReferenceLiteral SecretReferenceType = "literal"  // Literal value, not a reference
)
```

## 3. Generated JSON Schema Structure

After adding the new types and running `make schema-gen`, the generated schema will include:

### 3.1 Updated ServerDescriptor Schema

**Current schema** (`docs/schemas/core/serverdescriptor.json`):

```json
{
  "name": "ServerDescriptor",
  "type": "configuration",
  "provider": "core",
  "description": "Simple Container server.yaml configuration file schema",
  "goPackage": "pkg/api/server.go",
  "goStruct": "ServerDescriptor",
  "resourceType": "server-config",
  "schema": {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "properties": {
      "cicd": { ... },
      "provisioner": { ... },
      "resources": { ... },
      "schemaVersion": { ... },
      "secrets": { ... },
      "templates": { ... },
      "variables": { ... }
    },
    "required": ["cicd", "provisioner", "resources", "schemaVersion", "secrets", "templates", "variables"],
    "type": "object"
  }
}
```

**Updated schema** (after changes):

```json
{
  "name": "ServerDescriptor",
  "type": "configuration",
  "provider": "core",
  "description": "Simple Container server.yaml configuration file schema",
  "goPackage": "pkg/api/server.go",
  "goStruct": "ServerDescriptor",
  "resourceType": "server-config",
  "schema": {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "properties": {
      "cicd": { ... },
      "provisioner": { ... },
      "resources": { ... },
      "schemaVersion": { ... },
      "secrets": { ... },
      "secretsConfig": {                         // NEW
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "additionalProperties": {                // map[string]EnvironmentSecretsConfig
          "$schema": "https://json-schema.org/draft/2020-12/schema",
          "properties": {
            "exclude": {                         // []string
              "items": {
                "type": "string"
              },
              "type": "array"
            },
            "include": {                         // []string
              "items": {
                "type": "string"
              },
              "type": "array"
            },
            "inheritAll": {                      // bool
              "type": "boolean"
            },
            "secrets": {                         // map[string]string
              "additionalProperties": {
                "type": "string"
              },
              "type": "object"
            }
          },
          "type": "object"
        },
        "type": "object"
      },
      "templates": { ... },
      "variables": { ... }
    },
    "required": ["cicd", "provisioner", "resources", "schemaVersion", "secrets", "templates", "variables"],
    "type": "object"
  }
}
```

**Key Points:**
- `secretsConfig` is NOT in the `required` array (backwards compatible)
- Schema allows any environment name as a key (`additionalProperties`)
- All fields in `EnvironmentSecretsConfig` are optional

### 3.2 EnvironmentSecretsConfig Schema

The schema for `EnvironmentSecretsConfig` will be auto-generated by `cmd/schema-gen/main.go`:

```json
{
  "name": "EnvironmentSecretsConfig",
  "type": "object",
  "properties": {
    "inheritAll": {
      "type": "boolean",
      "description": "When true, all secrets are included except those in exclude. When false, only secrets in include are available."
    },
    "include": {
      "type": "array",
      "items": {
        "type": "string",
        "description": "Secret name to include. Can be a direct name (e.g., 'DATABASE_URL') or a mapped reference (e.g., '${secret:DB_HOST_STAGING}')"
      }
    },
    "exclude": {
      "type": "array",
      "items": {
        "type": "string",
        "description": "Secret name to exclude from inherited secrets"
      }
    },
    "secrets": {
      "type": "object",
      "additionalProperties": {
        "type": "string"
      },
      "description": "Literal secret values that override or add to secrets from secrets.yaml"
    }
  }
}
```

## 4. Validation Patterns

### 4.1 Include Mode Pattern

```yaml
secretsConfig:
  staging:
    inheritAll: false
    include:
      - DATABASE_URL
      - API_KEY
      - "${secret:REDIS_HOST_STAGING}"
    secrets:
      ENVIRONMENT: "staging"
```

**Schema Validation:**
- `inheritAll` is `false` (boolean)
- `include` is an array of strings
- `secrets` is an object with string values
- `exclude` is not present (or empty)

### 4.2 Exclude Mode Pattern

```yaml
secretsConfig:
  production:
    inheritAll: true
    exclude:
      - DEV_API_KEY
      - STAGING_DATABASE_URL
    secrets:
      ENVIRONMENT: "production"
```

**Schema Validation:**
- `inheritAll` is `true` (boolean)
- `exclude` is an array of strings
- `secrets` is an object with string values
- `include` is not present (or empty)

### 4.3 Override Mode Pattern

```yaml
secretsConfig:
  development:
    inheritAll: false
    include:
      - DATABASE_URL
    secrets:
      DATABASE_URL: "localhost:5432/dev"
      DEBUG: "true"
```

**Schema Validation:**
- `inheritAll` is `false`
- `include` is an array with `DATABASE_URL`
- `secrets` includes `DATABASE_URL` (overrides included secret) and `DEBUG` (new secret)

## 5. Backwards Compatibility Approach

### 5.1 Optional Field

The `secretsConfig` field uses:
- Pointer type: `*SecretsConfigMap`
- JSON tag: `json:"secretsConfig,omitempty"`
- YAML tag: `yaml:"secretsConfig,omitempty"`

This ensures:
1. Old configurations without `secretsConfig` work unchanged
2. New field is not required in schema
3. Nil pointer can be checked at runtime

### 5.2 Behavior with nil SecretsConfig

```go
// In ReconcileForDeploy or during secret resolution
if serverDesc.SecretsConfig == nil {
    // Backwards compatible: all secrets available
    return secretsDesc.Values, nil
}
```

### 5.3 Migration Path

**Phase 1: Optional** (This implementation)
- `secretsConfig` is optional
- Missing `secretsConfig` = all secrets available (current behavior)

**Phase 2: Recommended** (Future)
- Documentation recommends using `secretsConfig`
- Warnings in logs when not configured

**Phase 3: Required** (Future major version)
- `secretsConfig` becomes required
- Breaking change with migration guide

## 6. IDE Integration

### 6.1 VS Code

With the JSON schema, VS Code will provide:
- Auto-completion for `secretsConfig` field
- Validation of configuration structure
- Error markers for invalid patterns

**Configuration in `.vscode/settings.json`:**

```json
{
  "yaml.schemas": {
    "https://example.com/schemas/core/serverdescriptor.json": "*/server.yaml"
  }
}
```

### 6.2 JetBrains IDEs

**Configuration in IDE settings:**
1. Settings → Languages & Frameworks → Schemas and DTDs
2. Add JSON schema mapping for `server.yaml` files

### 6.3 CLI Validation

The `sc validate` command will use the schema:

```bash
# Validate configuration
sc validate --stack my-stack

# Output with invalid configuration:
# Error: secretsConfig.staging: cannot use 'include' with 'inheritAll: true'
```

## 7. Schema Testing

### 7.1 Validation Tests

Create tests in `pkg/api/secrets_config_test.go`:

```go
func TestSecretsConfigValidation(t *testing.T) {
    tests := []struct {
        name        string
        config      SecretsConfigMap
        expectError bool
        errorMsg    string
    }{
        {
            name: "valid include mode",
            config: SecretsConfigMap{
                "staging": {
                    InheritAll: false,
                    Include:    []string{"DATABASE_URL", "API_KEY"},
                    Secrets:    map[string]string{"ENV": "staging"},
                },
            },
            expectError: false,
        },
        {
            name: "invalid: include with inheritAll true",
            config: SecretsConfigMap{
                "production": {
                    InheritAll: true,
                    Include:    []string{"DATABASE_URL"},
                },
            },
            expectError: true,
            errorMsg:    "cannot use 'include' with 'inheritAll: true'",
        },
        {
            name: "valid exclude mode",
            config: SecretsConfigMap{
                "production": {
                    InheritAll: true,
                    Exclude:    []string{"DEV_KEY"},
                },
            },
            expectError: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateSecretsConfig(tt.config)
            if tt.expectError {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.errorMsg)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### 7.2 Schema Generation Tests

Verify schema generation works correctly:

```bash
# Run schema generation
make schema-gen

# Verify schema file exists and is valid JSON
cat docs/schemas/core/serverdescriptor.json | jq .

# Check for secretsConfig field
cat docs/schemas/core/serverdescriptor.json | jq '.schema.properties.secretsConfig'
```

Expected output:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "additionalProperties": {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "properties": {
      "exclude": {
        "items": {
          "type": "string"
        },
        "type": "array"
      },
      "include": {
        "items": {
          "type": "string"
        },
        "type": "array"
      },
      "inheritAll": {
        "type": "boolean"
      },
      "secrets": {
        "additionalProperties": {
          "type": "string"
        },
        "type": "object"
      }
    },
    "type": "object"
  },
  "type": "object"
}
```

## 8. Schema Regeneration Process

### 8.1 When to Regenerate

Regenerate the JSON schema when:
1. After modifying `ServerDescriptor` struct
2. After adding new types to `secrets_config.go`
3. After changing field tags (json/yaml)
4. Before release (to ensure docs are up to date)

### 8.2 Regeneration Command

```bash
# From repository root
make schema-gen

# Or directly
go run cmd/schema-gen/main.go docs/schemas
```

### 8.3 Verification Checklist

After regeneration:
- [ ] `docs/schemas/core/serverdescriptor.json` contains `secretsConfig` property
- [ ] `secretsConfig` is NOT in the `required` array
- [ ] `secretsConfig` has correct structure (map of EnvironmentSecretsConfig)
- [ ] All fields of `EnvironmentSecretsConfig` are present
- [ ] Schema is valid JSON (`jq .` succeeds)

## 9. Examples for Documentation

### 9.1 Example 1: Include Mode (Development)

**server.yaml:**

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
      - "${secret:API_KEY_DEV}"  # Maps to API_KEY in resolved secrets
    secrets:
      ENVIRONMENT: "development"
      DEBUG: "true"
```

**Expected result:**
- Available secrets: `DATABASE_URL`, `REDIS_HOST`, `API_KEY`, `ENVIRONMENT`, `DEBUG`
- All other secrets from secrets.yaml are NOT available

### 9.2 Example 2: Exclude Mode (Production)

**server.yaml:**

```yaml
schemaVersion: "1.0"
secrets:
  type: aws-secrets
secretsConfig:
  production:
    inheritAll: true
    exclude:
      - DEV_API_KEY
      - STAGING_DATABASE_URL
      - TEST_REDIS_HOST
    secrets:
      ENVIRONMENT: "production"
      DEBUG: "false"
```

**Expected result:**
- All secrets from secrets.yaml EXCEPT `DEV_API_KEY`, `STAGING_DATABASE_URL`, `TEST_REDIS_HOST`
- `ENVIRONMENT` and `DEBUG` added/overridden with literal values

### 9.3 Example 3: Mixed Environments

**server.yaml:**

```yaml
schemaVersion: "1.0"
secrets:
  type: aws-secrets
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

## 10. Error Messages in Schema Validation

| Validation Error | Schema Violation | User-Facing Message |
|-----------------|------------------|---------------------|
| Include with inheritAll | `inheritAll: true` + `include: [...]` | "cannot use 'include' with 'inheritAll: true' in environment '{env}'" |
| Include and Exclude | Both `include` and `exclude` present | "cannot use both 'include' and 'exclude' in environment '{env}'" |
| Empty configuration | No `include`, `exclude`, or `secrets` | "must specify 'include', 'exclude', or 'secrets' in environment '{env}' when inheritAll is false" |
| Invalid reference | Malformed reference string | "invalid secret reference '{ref}': expected secret name or '${secret:KEY}' format" |
| Secret not found | Referenced secret missing | "secret '{key}' does not exist in secrets.yaml" |
| Environment not configured | Environment missing from secretsConfig | "environment '{env}' not configured in secretsConfig" |
