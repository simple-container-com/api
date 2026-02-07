# Environment-Specific Secrets Design

This directory contains the architectural design and implementation plan for adding environment-specific secrets configuration to Simple Container parent stacks.

## Overview

This feature addresses a critical security gap where secrets defined in parent stack `secrets.yaml` files are globally available to all environments. The new `secretsConfig` section enables fine-grained control over which secrets are available in each environment (production, staging, development).

## Problem Statement

**Security Risk**: Production secrets (API keys, passwords) are accessible in dev/staging environments.

**Lack of Isolation**: The same secret name cannot have different values per environment.

**Current Limitations**:
- No per-environment secret allow/block lists
- All secrets in `secrets.yaml` are universally available
- No mechanism to override secret values per environment

## Solution

Add a `secretsConfig` section to `server.yaml` with three modes of operation:

1. **Include Mode**: Explicit allow list of available secrets
2. **Exclude Mode**: Block specific secrets (all others available)
3. **Override Mode**: Replace with literal values

## Documents

### [architecture.md](./architecture.md)
Comprehensive system architecture including:
- Current state analysis
- New data structures
- Component architecture
- Secret resolution algorithm
- Integration points
- Data flow diagrams
- Security considerations

### [json-schema.md](./json-schema.md)
JSON schema changes including:
- Go struct modifications
- Generated schema structure
- Validation patterns
- IDE integration
- Schema testing

### [implementation-plan.md](./implementation-plan.md)
Detailed 8-phase implementation plan:
- Phase 1: Data Structures and Schema
- Phase 2: Configuration Reading and Detection
- Phase 3: Secret Resolution Logic
- Phase 4: Stack Reconciliation Integration
- Phase 5: Validation
- Phase 6: JSON Schema Regeneration
- Phase 7: Testing
- Phase 8: Documentation

## Quick Start Example

```yaml
# server.yaml (parent stack)
secrets:
  type: fs-secrets

secretsConfig:
  inheritAll: false
  environments:
    production:
      mode: include
      secrets:
        DATABASE_URL: "~"                    # Direct reference
        API_KEY: "${secret:PROD_API_KEY}"    # Mapped reference
        WEBHOOK_URL: "https://prod.webhook"  # Literal value

    staging:
      mode: include
      secrets:
        DATABASE_URL: "~"
        API_KEY: "${secret:STAGING_API_KEY}"
        WEBHOOK_URL: "https://staging.webhook"

    development:
      mode: exclude
      secrets:
        PROD_API_KEY: "~"  # Block production secrets
```

```yaml
# secrets.yaml
values:
  PROD_API_KEY: "prod-key-123"
  STAGING_API_KEY: "staging-key-456"
  DATABASE_URL: "postgres://db.example.com/mydb"
```

```yaml
# client.yaml (child stack)
stacks:
  production:
    parent: company/infrastructure
    config:
      secrets:
        DATABASE_PASSWORD: "${secret:DATABASE_URL}"
        API_KEY: "${secret:API_KEY}"
```

## Acceptance Criteria

- [ ] **AC-1**: Basic Environment Isolation - When deploying to staging, only staging-configured secrets are available
- [ ] **AC-2**: Secret Mapping - References resolve using mapped keys from secrets.yaml
- [ ] **AC-3**: Literal Values - Literal values are used directly (not fetched from secrets.yaml)
- [ ] **AC-4**: Exclusion Mode - `inheritAll: true` with exclusions blocks specific secrets
- [ ] **AC-5**: Backwards Compatibility - Existing parent stacks work without modification
- [ ] **AC-6**: Validation Errors - `sc validate` returns errors for unavailable secrets

## Key Implementation Files

| File | Purpose |
|------|---------|
| `pkg/api/server.go` | Add `EnvironmentSecretsConfig`, `SecretsConfigMap` types |
| `pkg/api/secrets.go` | Add `SecretResolver`, `SecretResolutionContext` types |
| `pkg/api/read.go` | Add `DetectSecretsConfigType()`, modify `ReadServerConfigs()` |
| `pkg/api/models.go` | Modify `ReconcileForDeploy()` for environment filtering |
| `pkg/api/validation.go` | New file for validation logic |
| `cmd/schema-gen/main.go` | Regenerate JSON schema |

## Estimated Effort

**Total: 29-39 hours**

| Phase | Effort |
|-------|--------|
| Data Structures | 2-3 hours |
| Configuration Reading | 3-4 hours |
| Secret Resolution Logic | 6-8 hours |
| Stack Reconciliation | 4-6 hours |
| Validation | 4-5 hours |
| JSON Schema | 1 hour |
| Testing | 6-8 hours |
| Documentation | 3-4 hours |

## Related Issues

- Parent Issue: #60
- Feature Request: Environment-Specific Secrets in Parent Stacks

## Next Steps

1. Review architecture design with team
2. Approve implementation plan
3. Begin Phase 1 implementation
