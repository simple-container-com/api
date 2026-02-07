# JSON Schema Updates: Environment-Specific Secrets

## Overview

This document describes the JSON schema changes required to support environment-specific secrets configuration in Simple Container. The schema is automatically generated from Go structs via `cmd/schema-gen/main.go`.

## Current Schema Structure

### ServerDescriptor Schema (`docs/schemas/core/serverdescriptor.json`)

The current `secrets` property in ServerDescriptor:

```json
{
  "secrets": {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "properties": {
      "": {
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "properties": {
          "inherit": {
            "type": "string"
          }
        },
        "required": ["inherit"],
        "type": "object"
      },
      "type": {
        "type": "string"
      }
    },
    "required": ["", "", "type"],
    "type": "object"
  }
}
```

## Proposed Schema Changes

### 1. Go Struct Changes

#### File: `pkg/api/server.go`

**Add new types:**

```go
// EnvironmentSecretsConfig defines which secrets are available for a specific environment
type EnvironmentSecretsConfig struct {
    // Mode: "include" (allow list), "exclude" (block list), "override" (replace)
    Mode string `json:"mode" yaml:"mode"`

    // Secrets: Map of secret references
    // Three patterns supported:
    //   1. Direct reference: "~" -> use same name from secrets.yaml
    //   2. Mapped reference: "${secret:SOURCE_KEY}" -> use different source key
    //   3. Literal value: "actual-value" -> use literal string
    Secrets map[string]string `json:"secrets" yaml:"secrets"`
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

**Modify existing type:**

```go
// BEFORE:
type SecretsConfigDescriptor struct {
    Type    string `json:"type" yaml:"type"`
    Config  `json:",inline" yaml:",inline"`
    Inherit `json:",inline" yaml:",inline"`
}

// AFTER:
type SecretsConfigDescriptor struct {
    Type    string `json:"type" yaml:"type"`
    Config  `json:",inline" yaml:",inline"`
    Inherit `json:",inline" yaml:",inline"`

    // NEW: Environment-specific configuration (optional for backwards compatibility)
    SecretsConfig *SecretsConfigMap `json:"secretsConfig,omitempty" yaml:"secretsConfig,omitempty"`
}
```

### 2. Generated JSON Schema Structure

After adding the new struct and field, the schema generator will automatically produce:

#### Updated ServerDescriptor Schema

```json
{
  "secrets": {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "properties": {
      "config": {
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "properties": {
          "config": {
            "description": "Config is a generic container for provider-specific configuration",
            "type": ["string", "number", "boolean", "object", "array", "null"]
          }
        },
        "required": ["config"],
        "type": "object"
      },
      "inherit": {
        "type": "string"
      },
      "secretsConfig": {
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "description": "Environment-specific secrets configuration",
        "properties": {
          "environments": {
            "additionalProperties": {
              "$schema": "https://json-schema.org/draft/2020-12/schema",
              "description": "Per-environment secret configuration",
              "properties": {
                "mode": {
                  "description": "Mode: 'include' (allow list), 'exclude' (block list), 'override' (replace)",
                  "enum": ["include", "exclude", "override"],
                  "type": "string"
                },
                "secrets": {
                  "additionalProperties": {
                    "type": "string"
                  },
                  "description": "Map of secret references (key: client-facing name, value: source)",
                  "type": "object"
                }
              },
              "required": ["mode", "secrets"],
              "type": "object"
            },
            "description": "Map of environment name to configuration",
            "type": "object"
          },
          "inheritAll": {
            "description": "When true, all secrets are available by default (legacy behavior)",
            "type": "boolean"
          }
        },
        "required": ["environments"],
        "type": "object"
      },
      "type": {
        "type": "string"
      }
    },
    "required": ["inherit", "type"],
    "type": "object"
  }
}
```

### 3. Schema Validation Patterns

The new schema supports these validation patterns:

#### Pattern 1: Include Mode (Allow List)

```yaml
secretsConfig:
  inheritAll: false
  environments:
    production:
      mode: include  # Must be one of: include, exclude, override
      secrets:
        DATABASE_URL: "~"           # Direct reference
        API_KEY: "${secret:PROD_KEY}"  # Mapped reference
        LITERAL: "actual-value"     # Literal value
```

**Schema Validation:**
- `inheritAll`: boolean (optional, defaults to false)
- `environments.production.mode`: must be "include", "exclude", or "override"
- `environments.production.secrets`: object with string values

#### Pattern 2: Exclude Mode (Block List)

```yaml
secretsConfig:
  inheritAll: true
  environments:
    development:
      mode: exclude
      secrets:
        PROD_ONLY_SECRET: "~"
```

**Schema Validation:**
- `inheritAll`: true (all secrets available by default)
- `mode`: "exclude" (block specific secrets)

#### Pattern 3: Override Mode

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

**Schema Validation:**
- `inheritAll`: false (only listed secrets available)
- `mode`: "override" (replace all with literal values)

### 4. Backwards Compatibility

The schema is backwards compatible because `secretsConfig` is marked `omitempty`:

```go
SecretsConfig *SecretsConfigMap `json:"secretsConfig,omitempty" yaml:"secretsConfig,omitempty"`
```

**Legacy Configuration (still valid):**
```yaml
secrets:
  type: fs-secrets
  # No secretsConfig section
```

**New Configuration (opt-in):**
```yaml
secrets:
  type: fs-secrets
secretsConfig:
  inheritAll: false
  environments:
    production:
      mode: include
      secrets: ...
```

### 5. Schema Regeneration Process

After modifying the Go structs, regenerate the JSON schema:

```bash
cd cmd/schema-gen
go run main.go ../../docs/schemas
```

This will:
1. Reflect on the updated `ServerDescriptor` struct
2. Generate new schema with `secretsConfig` property
3. Update `/home/runner/_work/api/api/docs/schemas/core/serverdescriptor.json`
4. Update `/home/runner/_work/api/api/docs/schemas/core/index.json`

### 6. IDE Integration (VS Code)

Users can add the schema to their project for validation:

**`.vscode/settings.json` (project level):**
```json
{
  "yaml.schemas": {
    "https://example.com/schemas/core/serverdescriptor.json": [
      "server.yaml",
      "**/server.yaml"
    ]
  }
}
```

**Or workspace level:**
```json
{
  "yaml.schemas": {
    "./docs/schemas/core/serverdescriptor.json": "server.yaml"
  }
}
```

### 7. Validation Examples

#### Valid Configuration

```yaml
# ✅ Valid: Include mode with all three reference patterns
secretsConfig:
  inheritAll: false
  environments:
    production:
      mode: include
      secrets:
        DIRECT_REF: "~"
        MAPPED_REF: "${secret:OTHER_KEY}"
        LITERAL_VALUE: "plain-text-value"
```

```yaml
# ✅ Valid: Exclude mode with block list
secretsConfig:
  inheritAll: true
  environments:
    development:
      mode: exclude
      secrets:
        PROD_API_KEY: "~"
        PROD_DB_PASSWORD: "~"
```

```yaml
# ✅ Valid: Override mode with literal values
secretsConfig:
  inheritAll: false
  environments:
    staging:
      mode: override
      secrets:
        ENDPOINT: "https://staging.api.example.com"
        DEBUG: "true"
```

#### Invalid Configuration

```yaml
# ❌ Invalid: Mode not in enum
secretsConfig:
  environments:
    production:
      mode: invalid-mode  # Must be: include, exclude, override
      secrets: {}
```

```yaml
# ❌ Invalid: Missing required fields
secretsConfig:
  inheritAll: false
  environments:
    production:
      # Missing "mode" field
      # Missing "secrets" field
```

```yaml
# ❌ Invalid: secrets must be object
secretsConfig:
  inheritAll: false
  environments:
    production:
      mode: include
      secrets: "not-an-object"  # Must be object/map
```

### 8. Schema Testing

After schema generation, validate it:

```bash
# Install ajv-cli (JSON Schema validator)
npm install -g ajv-cli

# Test valid configuration
ajv validate -s docs/schemas/core/serverdescriptor.json -d tests/fixtures/valid-secrets-config.yaml

# Test invalid configuration (should fail)
ajv validate -s docs/schemas/core/serverdescriptor.json -d tests/fixtures/invalid-secrets-config.yaml
```

### 9. Migration Guide for Schema Users

**For Users:**
1. No action required for existing configurations (backwards compatible)
2. Add `secretsConfig` section to enable environment-specific secrets
3. Use YAML validation in your IDE to catch errors early

**For Developers:**
1. After modifying Go structs, run `go run cmd/schema-gen/main.go docs/schemas`
2. Commit the updated schema files
3. Update documentation with new schema examples

### 10. Schema Versioning

The schema follows semantic versioning:

- **Current Version:** 1.0 (implicit in ServerDescriptor)
- **Proposed Version:** 1.1 (new optional field)
- **Breaking Change:** No (backwards compatible)

When a breaking change is introduced in the future:
1. Increment `ServerSchemaVersion` constant
2. Add migration logic for old schemas
3. Update documentation

## Summary of Changes

| File | Change | Type |
|------|--------|------|
| `pkg/api/server.go` | Add `EnvironmentSecretsConfig` struct | New |
| `pkg/api/server.go` | Add `SecretsConfigMap` struct | New |
| `pkg/api/server.go` | Add `SecretsConfig` field to `SecretsConfigDescriptor` | Modified |
| `docs/schemas/core/serverdescriptor.json` | Auto-generated from Go structs | Regenerated |
| `docs/schemas/core/index.json` | Auto-generated | Regenerated |
| `docs/schemas/index.json` | Auto-generated | Regenerated |

## Validation Checklist

- [ ] Schema validates all three modes (include, exclude, override)
- [ ] Schema validates all three reference patterns (~, ${secret:KEY}, literal)
- [ ] Schema is backwards compatible (secretsConfig is optional)
- [ ] Schema includes proper descriptions for documentation
- [ ] Schema can be used in IDE for real-time validation
- [ ] Schema regeneration works correctly with `cmd/schema-gen/main.go`
