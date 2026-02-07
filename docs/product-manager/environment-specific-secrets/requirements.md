# Environment-Specific Secrets in Parent Stacks - Requirements

## Feature Overview

Add environment-specific secret configuration to parent stack `server.yaml` files to control which secrets are available in which environments, with support for secret mapping, filtering, and literal values.

## Problem Statement

Currently, secrets defined in `.sc/stacks/<parent-stack>/secrets.yaml` are globally available to all environments. This creates:

1. **Security Risk**: Production secrets (API keys, passwords) are accessible in dev/staging environments
2. **Lack of Isolation**: Some secrets should only exist in specific environments
3. **Naming Conflicts**: The same secret name (e.g., `DATABASE_PASSWORD`) needs different values per environment while services expect consistent naming

### Current Behavior Example

```yaml
# .sc/stacks/devops/secrets.yaml (current)
values:
  DATABASE_PASSWORD: "prod-password-123"      # Available everywhere - WRONG for dev/staging
  STRIPE_API_KEY: "sk_live_xxx"               # Production key exposed to all envs
  SLACK_WEBHOOK: "https://hooks.slack.com/..." # Maybe OK to share
```

A developer deploying to `staging` inadvertently gets access to production secrets.

## Proposed Solution

Add a `secrets` section to `server.yaml` that controls per-environment secret availability and mapping.

## Requirements

### Functional Requirements

#### FR-1: Environment-Specific Secret Declaration

- The `server.yaml` file MUST support a new `secrets` top-level section
- The `secrets` section MUST support an `inheritAll` boolean flag (default: `true`)
- The `secrets` section MUST support an `environments` map keyed by environment name
- Each environment MUST support either `include` or `exclude` secret lists

#### FR-2: Secret Inclusion Mode

When `inheritAll: false`:
- Only explicitly listed secrets in the `include` section are available
- The `include` section supports three patterns:
  1. `SECRET_NAME: ~` - Use value of `SECRET_NAME` from secrets.yaml (same key reference)
  2. `NAME: "${secret:KEY}"` - Expose as `NAME`, fetch value from `KEY` in secrets.yaml (mapped reference)
  3. `NAME: "value"` - Expose as `NAME` with hardcoded literal value

#### FR-3: Secret Exclusion Mode

When `inheritAll: true` (default):
- All secrets from secrets.yaml are available by default
- The `exclude` section lists secrets to block
- The `override` section allows redefining specific secrets to different values or mappings

#### FR-4: Backwards Compatibility

- Existing configurations without the `secrets` section MUST continue to work
- The default behavior when `secrets` section is absent MUST be `inheritAll: true` (all secrets available everywhere)
- No changes to client.yaml or service configurations are required

#### FR-5: Secret Resolution

- Secret references in client.yaml MUST resolve to environment-specific values based on the target environment
- The secret resolution MUST occur at deploy/provision time
- Secrets MUST remain encrypted in secrets.yaml at rest

### Non-Functional Requirements

#### NFR-1: Security

- Secret values MUST remain encrypted in secrets.yaml
- Mapping logic MUST be in version-controlled server.yaml (unencrypted)
- Production secrets MUST NOT be accessible to lower environments when configured

#### NFR-2: Validation

- `sc validate` MUST warn if a secret referenced in client.yaml isn't available for the target environment
- `sc validate` MUST error if `${secret:KEY}` references a non-existent key in secrets.yaml
- `sc validate` SHOULD warn about unused secrets in secrets.yaml

#### NFR-3: Migration

- The feature MUST be opt-in - teams adopt incrementally
- Existing parent stacks without the `secrets` section MUST work without modification
- Client configurations MUST work without changes

## Acceptance Criteria

### AC-1: Basic Environment Isolation

Given a parent stack with environment-specific secrets configured
When deploying to staging
Then only staging-configured secrets are available
And production secrets are not accessible

### AC-2: Secret Mapping

Given a parent stack with secret mapping configured
When a client references `DATABASE_PASSWORD`
And the environment is staging
Then the value resolves to `DATABASE_PASSWORD_STAGING` from secrets.yaml

### AC-3: Literal Values

Given a parent stack with literal secret values configured
When a client references the secret
Then the literal value is used (not fetched from secrets.yaml)

### AC-4: Exclusion Mode

Given a parent stack with `inheritAll: true` and exclusions configured
When deploying to staging
Then all secrets except excluded ones are available
And excluded secrets return validation errors if referenced

### AC-5: Backwards Compatibility

Given an existing parent stack without the `secrets` section
When deploying
Then all secrets behave as before (globally available)
And no configuration changes are required

### AC-6: Validation Errors

Given a parent stack with environment-specific secrets
And a client configuration referencing an unavailable secret
When running `sc validate`
Then a validation error is returned indicating the secret is not available

## Configuration Examples

### Example 1: Include Mode (Explicit Allow List)

```yaml
# .sc/stacks/devops/server.yaml
schemaVersion: 1.0

secrets:
  inheritAll: false

  environments:
    staging:
      include:
        # Use same key from secrets.yaml
        SLACK_WEBHOOK: ~

        # Map to different key in secrets.yaml
        DATABASE_PASSWORD: "${secret:DATABASE_PASSWORD_STAGING}"
        STRIPE_API_KEY: "${secret:STRIPE_TEST_KEY}"

        # Literal value (not in secrets.yaml)
        LOG_LEVEL: "debug"
        ENABLE_PROFILING: "true"

    production:
      include:
        SLACK_WEBHOOK: ~
        DATABASE_PASSWORD: "${secret:DATABASE_PASSWORD_PROD}"
        STRIPE_API_KEY: "${secret:STRIPE_LIVE_KEY}"
        DATADOG_API_KEY: ~
        LOG_LEVEL: "warn"
```

```yaml
# .sc/stacks/devops/secrets.yaml
values:
  DATABASE_PASSWORD_PROD: "super-secret-prod-password"
  DATABASE_PASSWORD_STAGING: "staging-password-123"
  STRIPE_LIVE_KEY: "sk_live_xxx"
  STRIPE_TEST_KEY: "sk_test_yyy"
  SLACK_WEBHOOK: "https://hooks.slack.com/..."
  DATADOG_API_KEY: "dd-api-xxx"
```

### Example 2: Exclude Mode (Block Specific Secrets)

```yaml
# .sc/stacks/devops/server.yaml
schemaVersion: 1.0

secrets:
  inheritAll: true  # Start with all secrets

  environments:
    staging:
      exclude:
        - DATADOG_API_KEY      # Not needed in staging
        - STRIPE_LIVE_KEY      # Security: no prod keys in staging
      override:
        STRIPE_API_KEY: "${secret:STRIPE_TEST_KEY}"

    production:
      # Gets everything, no overrides needed
```

## Service Usage (No Changes)

Client configurations remain unchanged:

```yaml
# client.yaml - deploying to staging
stacks:
  staging:
    parent: company/devops
    secrets:
      DB_PASS: "${secret:DATABASE_PASSWORD}"  # Resolves to staging value
      STRIPE: "${secret:STRIPE_API_KEY}"      # Resolves to test key
```

```yaml
# Same client.yaml - deploying to production
stacks:
  production:
    parent: company/devops
    secrets:
      DB_PASS: "${secret:DATABASE_PASSWORD}"  # Resolves to prod value
      STRIPE: "${secret:STRIPE_API_KEY}"      # Resolves to live key
```

## Implementation Scope

### In Scope

1. Add `secrets` section to `server.yaml` schema
2. Implement secret resolution logic based on environment
3. Add validation for environment-specific secret availability
4. Update schema validation to support new structure
5. Documentation and examples

### Out of Scope

1. Changes to client.yaml schema or syntax
2. Runtime secret rotation
3. Secret versioning or history
4. Dynamic secret generation
5. UI/CLI changes (beyond validation messages)

## Dependencies

- Server descriptor schema (`pkg/api/server.go`)
- Secret resolution logic (`pkg/api/secrets.go`)
- Configuration validation (`pkg/api/read.go`)
- JSON schema generation (`cmd/schema-gen/main.go`)

## Success Metrics

1. All acceptance criteria pass
2. Backwards compatibility is maintained (existing tests pass)
3. New tests for environment-specific secret resolution pass
4. Validation correctly identifies unavailable secrets
5. Documentation is complete with examples

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Breaking existing deployments | High | Default `inheritAll: true` when section is absent |
| Complex configuration errors | Medium | Clear validation error messages |
| Performance overhead | Low | Secret resolution happens once at deploy time |
| Migration confusion | Medium | Comprehensive documentation and examples |

## Open Questions

None at this time.
