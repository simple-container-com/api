# Environment-Specific Secrets Design Documentation

This directory contains the architecture design for implementing environment-specific secrets in parent stacks.

## Overview

This feature enables parent stacks to control which secrets are available to child stacks on a per-environment basis, improving security and reducing naming conflicts.

## Design Documents

### [architecture.md](./architecture.md)
Comprehensive system architecture including:
- Current state analysis of existing secrets implementation
- New data structures and types
- Component architecture with data flow diagrams
- Secret resolution algorithm
- Integration points with existing code
- Three operation modes (include, exclude, override)
- Security and performance considerations

### [json-schema.md](./json-schema.md)
JSON schema changes including:
- Go struct modifications for `pkg/api/server.go`
- Generated schema structure
- Validation patterns for all three modes
- Backwards compatibility approach
- IDE integration instructions
- Schema testing procedures

### [implementation-plan.md](./implementation-plan.md)
Detailed 8-phase implementation plan:
1. Phase 1: Data Structures and Schema (2-3 hours)
2. Phase 2: Configuration Reading and Detection (3-4 hours)
3. Phase 3: Secret Resolution Logic (6-8 hours)
4. Phase 4: Stack Reconciliation Integration (4-6 hours)
5. Phase 5: Validation (4-5 hours)
6. Phase 6: JSON Schema Regeneration (1 hour)
7. Phase 7: Testing (6-8 hours)
8. Phase 8: Documentation (3-4 hours)

**Total Effort: 29-39 hours**

## Quick Reference

### Key Data Structures

```go
// EnvironmentSecretsConfig configures secrets for a specific environment
type EnvironmentSecretsConfig struct {
    InheritAll    bool              `json:"inheritAll" yaml:"inheritAll"`
    Include       []string          `json:"include,omitempty" yaml:"include,omitempty"`
    Exclude       []string          `json:"exclude,omitempty" yaml:"exclude,omitempty"`
    Secrets       map[string]string `json:"secrets,omitempty" yaml:"secrets,omitempty"`
}

// SecretsConfigMap holds per-environment secret configurations
type SecretsConfigMap map[string]EnvironmentSecretsConfig

// SecretResolver resolves secret references based on environment configuration
type SecretResolver struct {
    secretsConfig *SecretsConfigMap
    allSecrets    map[string]string
    environment   string
}
```

### Configuration Example

```yaml
# server.yaml in parent stack
secrets:
  type: aws-secrets
  secretsConfig:
    staging:
      inheritAll: false
      include:
        - DATABASE_URL
        - API_ENDPOINT
      secrets:
        ENVIRONMENT: "staging"
    production:
      inheritAll: false
      include:
        - DATABASE_URL
        - API_KEY
      secrets:
        ENVIRONMENT: "production"
```

## Design Decisions

1. **Optional `secretsConfig` field** - Ensures backwards compatibility
2. **Three modes of operation** - Include (allow list), Exclude (block list), Override (replace)
3. **Three reference patterns** - Direct (`~`), Mapped (`${secret:KEY}`), Literal values
4. **Fail-fast validation** - Errors caught at configuration read time
5. **No external dependencies** - Uses existing secrets.yaml infrastructure

## Related Documentation

- Product Manager Requirements: `docs/product-manager/environment-specific-secrets/`
- JSON Schemas: `docs/schemas/core/serverdescriptor.json`
- Implementation Files: `pkg/api/server.go`, `pkg/api/secrets.go`, `pkg/api/read.go`
