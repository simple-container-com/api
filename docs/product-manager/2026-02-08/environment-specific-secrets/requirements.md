# Product Requirements: Environment-Specific Secrets in Parent Stacks

**Issue ID:** #60
**Feature Request:** Environment-Specific Secrets in Parent Stacks
**Date:** 2026-02-08
**Status:** Requirements Definition

## Executive Summary

This document defines the requirements for implementing environment-specific secrets management in parent stacks within the Simple Container API. Currently, the secrets management system does not support differentiating secrets based on deployment environments (production, staging, development) when using parent/child stack architectures.

## Problem Statement

### Current Limitations

1. **No Environment Awareness**: Secrets are stored in a single `secrets.yaml` file per stack without environment differentiation
2. **Shared Secrets Across Environments**: Production and development environments must use the same secrets or require separate stack definitions
3. **Security Risk**: Development environments may accidentally use production secrets
4. **Operational Complexity**: Teams must maintain separate stack definitions for different environments or manually manage secret rotation
5. **No Context-Based Resolution**: The placeholder resolution system (`${secret:name}`) cannot automatically provide environment-specific secrets

### Current Implementation Analysis

Based on codebase analysis:

- **Secret Storage**: Encrypted secrets stored in `.sc/secrets.yaml` using RSA/Ed25519 encryption
- **Secret Resolution**: Handled by `pkg/provisioner/placeholders/placeholders.go` via `${secret:name}` placeholder
- **Parent/Child Stack Inheritance**: Child stacks can inherit secrets from parent stacks using `inherit.inherit` configuration
- **Secret Files**: Individual secret files tracked in registry and encrypted for multiple public keys

**Example Current Structure:**
```yaml
# .sc/stacks/parent-infrastructure/secrets.yaml
schemaVersion: 1.0
values:
  database-password: "shared-password-for-all-envs"
  api-key: "shared-api-key"
```

## User Stories

### Primary User Stories

1. **As a DevOps engineer**, I want to define different secrets for production, staging, and development environments in a single parent stack, so that I can maintain separation of concerns while avoiding stack duplication.

2. **As a security conscious developer**, I want to ensure that development environments can never access production secrets, so that I can prevent accidental data exposure.

3. **As a platform operator**, I want to centrally manage environment-specific secrets in parent stacks, so that child stacks can inherit appropriate secrets based on their deployment environment.

4. **As a CI/CD pipeline designer**, I want to specify the target environment during deployment, so that the correct secrets are automatically resolved and used.

### Secondary User Stories

5. **As a developer**, I want to use default secrets for local development while maintaining environment-specific secrets for deployed environments, so that I can work efficiently without managing multiple configurations.

6. **As a team lead**, I want to audit which environment's secrets are being accessed, so that I can maintain compliance and security standards.

## Functional Requirements

### FR-1: Environment-Aware Secret Storage

**Requirement:** The secrets storage structure MUST support environment-specific secret organization.

**Details:**
- Extend the `secrets.yaml` schema to support environment grouping
- Maintain backward compatibility with existing single-environment secret files
- Support for arbitrary environment names (production, staging, development, etc.)

**Proposed Schema:**
```yaml
# .sc/stacks/parent-infrastructure/secrets.yaml
schemaVersion: 2.0  # New schema version
defaultEnvironment: development
environments:
  production:
    values:
      database-password: "prod-secure-password"
      api-key: "prod-api-key"
      database-host: "prod-db.example.com"
  staging:
    values:
      database-password: "staging-secure-password"
      api-key: "staging-api-key"
      database-host: "staging-db.example.com"
  development:
    values:
      database-password: "dev-secure-password"
      api-key: "dev-api-key"
      database-host: "localhost"
# Optional: shared secrets across all environments
shared:
  values:
    company-name: "Example Corp"
```

**Acceptance Criteria:**
- [ ] Schema v2.0 is parsed correctly by the secrets management system
- [ ] Schema v1.0 (current format) continues to work without modification
- [ ] Environment names are validated to be valid identifiers
- [ ] Default environment can be specified in the schema
- [ ] Shared secrets can be defined and merged with environment-specific secrets

### FR-2: Environment Context Specification

**Requirement:** The system MUST provide mechanisms to specify the target environment for secret resolution.

**Details:**
- Support environment specification via command-line flag
- Support environment specification via configuration file
- Support environment detection from stack metadata
- Provide clear error messages when environment is not specified

**Implementation Options:**

1. **Command-Line Flag:**
   ```bash
   sc apply --environment production
   ```

2. **Stack Configuration:**
   ```yaml
   # .sc/stacks/production-app/client.yaml
   schemaVersion: 1.0
   stack:
     type: static
     parent: infrastructure
     environment: production  # Specify environment for this stack
   ```

3. **Environment Variable:**
   ```bash
   export SC_ENVIRONMENT=production
   sc apply
   ```

**Acceptance Criteria:**
- [ ] Command-line flag `--environment` is respected
- [ ] Stack-level `environment` field is respected
- [ ] Environment variable `SC_ENVIRONMENT` is respected
- [ ] Precedence order: CLI flag > stack config > environment variable > default
- [ ] Clear error message when environment is not specified and no default exists

### FR-3: Environment-Specific Secret Resolution

**Requirement:** The placeholder resolution system MUST support environment-aware secret lookups.

**Details:**
- Extend `${secret:name}` placeholder to support environment context
- Support explicit environment specification: `${secret:name:environment}`
- Support implicit environment resolution from context
- Maintain backward compatibility with existing `${secret:name}` syntax

**Placeholder Syntax:**

1. **Implicit (uses current environment context):**
   ```yaml
   # .sc/stacks/production-app/client.yaml
   config:
     password: "${secret:database-password}"  # Resolved from production environment
   ```

2. **Explicit (overrides environment context):**
   ```yaml
   config:
     # Use staging password even in production context
     test-password: "${secret:database-password:staging}"
   ```

3. **Shared secrets:**
   ```yaml
   config:
     company: "${secret:company-name}"  # Resolved from shared values
   ```

**Acceptance Criteria:**
- [ ] Implicit secret resolution uses current environment context
- [ ] Explicit environment specification overrides context
- [ ] Shared secrets are accessible from any environment
- [ ] Environment-specific secrets take precedence over shared secrets
- [ ] Existing `${secret:name}` syntax continues to work (backward compatibility)
- [ ] Clear error messages when secret is not found in specified environment

### FR-4: Parent Stack Environment Inheritance

**Requirement:** Child stacks MUST inherit environment-appropriate secrets from parent stacks.

**Details:**
- Child stacks specify their environment in configuration
- Parent stack secrets are resolved based on child's environment context
- Support for different child stacks using different environments from same parent
- Maintain backward compatibility with current inheritance mechanism

**Example:**
```yaml
# Parent stack: .sc/stacks/infrastructure/secrets.yaml
schemaVersion: 2.0
environments:
  production:
    values:
      api-key: "prod-key"
  development:
    values:
      api-key: "dev-key"

# Child stack 1: .sc/stacks/production-app/client.yaml
schemaVersion: 1.0
stack:
  parent: infrastructure
  environment: production
# ${secret:api-key} resolves to "prod-key"

# Child stack 2: .sc/stacks/dev-app/client.yaml
schemaVersion: 1.0
stack:
  parent: infrastructure
  environment: development
# ${secret:api-key} resolves to "dev-key"
```

**Acceptance Criteria:**
- [ ] Child stacks can specify environment in client configuration
- [ ] Parent stack secrets are resolved using child's environment context
- [ ] Multiple child stacks can use different environments from same parent
- [ ] Existing inheritance mechanism continues to work for schema v1.0
- [ ] Clear error messages when parent stack doesn't support requested environment

### FR-5: Secret Validation and Error Handling

**Requirement:** The system MUST provide clear validation and error messages for environment-specific secrets.

**Details:**
- Validate secret references at configuration load time
- Provide helpful error messages for missing secrets
- Warn when accessing secrets from unintended environments
- Support dry-run mode to preview secret resolution

**Validation Scenarios:**

1. **Missing Secret:**
   ```
   Error: Secret "database-password" not found in environment "production"
   Available environments: development, staging
   ```

2. **Invalid Environment:**
   ```
   Error: Environment "production" not defined in parent stack "infrastructure"
   Available environments: development, staging
   ```

3. **Security Warning:**
   ```
   Warning: Using production secrets in development environment
   Stack: dev-app
   Secret: api-key
   ```

**Acceptance Criteria:**
- [ ] Configuration validation fails fast with clear error messages
- [ ] Error messages include available alternatives (environments, secrets)
- [ ] Security warnings are displayed for inappropriate environment access
- [ ] Dry-run mode shows which secrets would be resolved without applying changes
- [ ] Validation occurs before any deployment operations

## Non-Functional Requirements

### NFR-1: Performance

- Secret resolution MUST NOT add significant latency to stack operations
- Environment context resolution MUST be cached within a single operation
- Secret file parsing MUST remain efficient with large numbers of environments

**Performance Targets:**
- Secret resolution: < 10ms per placeholder
- Configuration parsing: < 100ms for files with up to 50 environments
- No performance degradation for existing single-environment configurations

### NFR-2: Backward Compatibility

- Existing `secrets.yaml` files (schema v1.0) MUST continue to work without modification
- Existing placeholder syntax `${secret:name}` MUST continue to work
- Existing stack inheritance MUST continue to work
- No breaking changes to existing APIs or command-line interfaces

**Migration Path:**
- Schema v1.0 files are treated as having a single "default" environment
- Optional migration tool to convert v1.0 to v2.0 format
- Clear documentation on migration process

### NFR-3: Security

- Production secrets MUST NOT be accessible in development environments by default
- Environment context MUST be validated before secret access
- Audit logging MUST track which environment's secrets are accessed
- No secret values appear in error messages or logs

**Security Considerations:**
- Environment specification should be explicit, not inferred from network/location
- Secrets remain encrypted at rest regardless of environment structure
- Access control mechanisms should prevent cross-environment secret access

### NFR-4: Usability

- Learning curve for existing users should be minimal
- Clear documentation and examples for environment-specific secrets
- Intuitive command-line interface for environment specification
- Helpful error messages guide users to correct configuration

**Usability Targets:**
- Existing users can adopt new features with < 30 minutes of learning
- Error messages provide actionable guidance
- CLI auto-completion for environment names
- Interactive help for secret resolution debugging

## Technical Constraints

### TC-1: Existing Codebase Structure

Based on codebase analysis:

- **Secret Storage**: `pkg/api/secrets/` package handles encryption/decryption
- **Secret Resolution**: `pkg/provisioner/placeholders/placeholders.go` handles placeholder resolution
- **Stack Configuration**: Server and client YAML files define stack structure
- **Inheritance Mechanism**: Existing `inherit.inherit` field for parent stack references

**Constraint:** New implementation must work within existing package structure without major refactoring.

### TC-2: Encryption Mechanism

- Current encryption uses RSA/Ed25519 keys
- Each secret file is encrypted for multiple public keys
- Secrets are encrypted at rest and decrypted on-demand

**Constraint:** Environment-specific secrets must use existing encryption mechanisms without changes to core cryptographic operations.

### TC-3: Placeholder Resolution

- Current placeholder system supports `${secret:name}`, `${auth:name}`, `${var:name}`, etc.
- Placeholders are resolved recursively through stack inheritance
- Resolution happens during stack configuration processing

**Constraint:** New environment context must integrate with existing placeholder resolution system.

### TC-4: Configuration Schema

- Existing schema uses `schemaVersion` field for versioning
- Current secret schema: `values` map with string keys/values

**Constraint:** New schema must use new `schemaVersion` (e.g., 2.0) to enable backward compatibility.

## Dependencies

### External Dependencies

None - this feature is fully contained within the Simple Container API codebase.

### Internal Dependencies

1. **Secrets Management Package** (`pkg/api/secrets/`)
   - Must extend to support environment-aware secret storage
   - Must maintain backward compatibility with existing secret files

2. **Placeholder Resolution** (`pkg/provisioner/placeholders/`)
   - Must extend `tplSecrets` function to support environment context
   - Must maintain backward compatibility with existing placeholder syntax

3. **Stack Configuration** (`pkg/api/models.go`)
   - May need to add `environment` field to stack configuration
   - Must handle environment specification in client/server configs

4. **CLI Commands** (`cmd/sc/main.go`)
   - Must add `--environment` flag to relevant commands
   - Must handle environment context propagation through command execution

## Out of Scope

The following features are explicitly out of scope for this implementation:

1. **External Secrets Managers**: Integration with HashiCorp Vault, AWS Secrets Manager, Azure Key Vault, etc.
   - These are planned for future phases
   - Current implementation focuses on repository-based secrets only

2. **Secret Rotation**: Automated rotation of secrets based on expiry or schedule
   - Secrets must still be manually updated and re-encrypted
   - No automatic renewal or rotation logic

3. **Dynamic Secret Generation**: On-demand generation of secrets (e.g., database credentials)
   - All secrets must be statically defined in configuration files
   - No integration with dynamic secret generation systems

4. **Cross-Environment Secret References**: Ability to reference secrets from different environments
   - Each stack operates within a single environment context
   - No cross-environment secret access or references

5. **Environment Promotion**: Copying secrets between environments (e.g., staging → production)
   - No built-in secret promotion or copy functionality
   - Manual secret management between environments

6. **Secret Versioning**: History or versioning of secret values
   - Only current secret values are stored
   - No audit trail of secret value changes

## Risks and Mitigations

### Risk 1: Backward Compatibility Breaking

**Risk Level:** High

**Description:** Changes to secret storage schema or resolution logic could break existing deployments.

**Mitigation:**
- Use schema versioning (v1.0 vs v2.0) to distinguish old and new formats
- Extensive testing with existing secret files
- Feature flag to enable new functionality opt-in
- Comprehensive migration guide and tools

### Risk 2: Performance Degradation

**Risk Level:** Medium

**Description:** Additional environment context resolution could slow down stack operations.

**Mitigation:**
- Benchmark performance before and after implementation
- Cache environment context within operations
- Optimize secret resolution algorithms
- Lazy loading of environment-specific data

### Risk 3: Security Misconfiguration

**Risk Level:** High

**Description:** Users might accidentally configure wrong environments, leading to secret exposure.

**Mitigation:**
- Clear error messages and warnings for environment mismatches
- Explicit environment specification (no implicit defaults)
- Audit logging of environment access
- Documentation security best practices
- Optional safety checks before production deployments

### Risk 4: Complex Inheritance Scenarios

**Risk Level:** Medium

**Description:** Complex parent/child stack relationships with different environments could cause confusion.

**Mitigation:**
- Clear error messages for inheritance conflicts
- Validation rules to prevent ambiguous configurations
- Visualization tools to show environment inheritance
- Comprehensive documentation with examples

## Success Metrics

### Primary Metrics

1. **Adoption Rate**: Percentage of deployments using environment-specific secrets within 6 months
   - Target: > 40% of deployments

2. **Security Incidents**: Reduction in accidental production secret usage in development
   - Target: > 80% reduction

3. **User Satisfaction**: Feedback from users on feature usefulness and usability
   - Target: > 4.0/5.0 in user surveys

### Secondary Metrics

4. **Configuration Simplification**: Reduction in number of stack definitions needed
   - Target: 30% reduction in duplicate stack configurations

5. **Error Reduction**: Reduction in secret-related configuration errors
   - Target: 50% reduction in support tickets related to secrets

6. **Performance Impact**: Latency added to stack operations
   - Target: < 5% performance degradation

## Open Questions

1. **Default Environment Behavior**: Should there be a mandatory default environment, or should it be optional?
   - **Recommendation**: Make default environment optional but require explicit specification in production deployments

2. **Environment Validation**: Should environment names be restricted to a predefined set (production, staging, development) or allow arbitrary names?
   - **Recommendation**: Allow arbitrary environment names for flexibility, but provide best practices documentation

3. **Shared Secrets Semantics**: How should shared secrets merge with environment-specific secrets when there are conflicts?
   - **Recommendation**: Environment-specific secrets always take precedence over shared secrets

4. **Migration Strategy**: Should there be an automated migration tool for converting v1.0 to v2.0 secret files?
   - **Recommendation**: Provide optional migration tool but require manual review and confirmation

5. **Audit Logging Implementation**: How detailed should audit logging be for environment-specific secret access?
   - **Recommendation**: Log environment access at stack level, not individual secret access level for performance

## Appendix

### A. Current Implementation Details

**Key Files:**
- `pkg/api/secrets/cryptor.go`: Core encryption/decryption logic
- `pkg/api/secrets/management.go`: Secret file management operations
- `pkg/provisioner/placeholders/placeholders.go`: Placeholder resolution system
- `pkg/api/models.go`: Data models for stack configuration

**Current Secret Resolution Flow:**
1. Stack configuration is loaded from YAML files
2. Placeholder resolution processes `${secret:name}` references
3. `tplSecrets` function looks up secret in stack's secret values or parent's secret values
4. Secret value is returned and substituted into configuration

### B. Glossary

- **Parent Stack**: A stack that provides shared configuration and secrets to child stacks
- **Child Stack**: A stack that inherits configuration and secrets from a parent stack
- **Environment**: A deployment context (e.g., production, staging, development) with its own set of secrets
- **Secret**: Sensitive configuration data (passwords, API keys, etc.) encrypted at rest
- **Placeholder**: A template variable reference like `${secret:name}` that gets resolved at runtime
- **Schema Version**: Version identifier in configuration files to enable backward compatibility

### C. References

- GitHub Issue: #60 - Feature Request: Environment-Specific Secrets in Parent Stacks
- Current Secrets Documentation: [Link to existing secrets management docs]
- Stack Inheritance Documentation: [Link to stack inheritance docs]
