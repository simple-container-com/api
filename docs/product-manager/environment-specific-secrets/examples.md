# Environment-Specific Secrets - Configuration Examples

## Table of Contents

1. [Basic Environment Isolation](#basic-environment-isolation)
2. [Secret Mapping](#secret-mapping)
3. [Literal Values](#literal-values)
4. [Exclusion Mode](#exclusion-mode)
5. [Mixed Configuration](#mixed-configuration)
6. [Real-World Use Cases](#real-world-use-cases)

## Basic Environment Isolation

### Scenario

You want different database passwords for staging and production, with no cross-environment access.

### Configuration

**server.yaml:**
```yaml
schemaVersion: 1.0

secretsConfig:
  inheritAll: false

  environments:
    staging:
      include:
        DATABASE_PASSWORD: "${secret:DB_PASSWORD_STAGING}"
        API_KEY: "${secret:API_KEY_STAGING}"

    production:
      include:
        DATABASE_PASSWORD: "${secret:DB_PASSWORD_PRODUCTION}"
        API_KEY: "${secret:API_KEY_PRODUCTION}"
```

**secrets.yaml:**
```yaml
schemaVersion: 1.0
values:
  DB_PASSWORD_STAGING: "staging-pass-123"
  DB_PASSWORD_PRODUCTION: "prod-pass-secure-456"
  API_KEY_STAGING: "sk_test_abc123"
  API_KEY_PRODUCTION: "sk_live_xyz789"
```

**client.yaml (staging):**
```yaml
schemaVersion: 1.0
stacks:
  staging:
    parent: company/devops
    secrets:
      DB_PASSWORD: "${secret:DATABASE_PASSWORD}"  # Resolves to "staging-pass-123"
      API_KEY: "${secret:API_KEY}"                # Resolves to "sk_test_abc123"
```

**client.yaml (production):**
```yaml
schemaVersion: 1.0
stacks:
  production:
    parent: company/devops
    secrets:
      DB_PASSWORD: "${secret:DATABASE_PASSWORD}"  # Resolves to "prod-pass-secure-456"
      API_KEY: "${secret:API_KEY}"                # Resolves to "sk_live_xyz789"
```

### Result

- Staging deployments only have access to `DB_PASSWORD_STAGING` and `API_KEY_STAGING`
- Production deployments only have access to `DB_PASSWORD_PRODUCTION` and `API_KEY_PRODUCTION`
- Same secret names in client.yaml resolve to different values per environment

## Secret Mapping

### Scenario

You have environment-specific secret names in secrets.yaml but want to expose them with consistent names to services.

### Configuration

**server.yaml:**
```yaml
schemaVersion: 1.0

secretsConfig:
  inheritAll: false

  environments:
    staging:
      include:
        # Map to staging-specific Stripe keys
        STRIPE_API_KEY: "${secret:STRIPE_TEST_KEY}"
        STRIPE_WEBHOOK_SECRET: "${secret:STRIPE_WEBHOOK_SIGNING_SECRET_TEST}"

    production:
      include:
        # Map to production-specific Stripe keys
        STRIPE_API_KEY: "${secret:STRIPE_LIVE_KEY}"
        STRIPE_WEBHOOK_SECRET: "${secret:STRIPE_WEBHOOK_SIGNING_SECRET_LIVE}"
```

**secrets.yaml:**
```yaml
schemaVersion: 1.0
values:
  STRIPE_TEST_KEY: "sk_test_51Mabc..."
  STRIPE_LIVE_KEY: "sk_live_51Mxyz..."
  STRIPE_WEBHOOK_SIGNING_SECRET_TEST: "whsec_test_abc..."
  STRIPE_WEBHOOK_SIGNING_SECRET_LIVE: "whsec_live_xyz..."
```

**client.yaml:**
```yaml
# Same configuration works for both environments
schemaVersion: 1.0
stacks:
  staging:
    parent: company/devops
    secrets:
      STRIPE_KEY: "${secret:STRIPE_API_KEY}"
      WEBHOOK_SECRET: "${secret:STRIPE_WEBHOOK_SECRET}"

  production:
    parent: company/devops
    secrets:
      STRIPE_KEY: "${secret:STRIPE_API_KEY}"
      WEBHOOK_SECRET: "${secret:STRIPE_WEBHOOK_SECRET}"
```

### Result

- Services use consistent secret names (`STRIPE_API_KEY`, `STRIPE_WEBHOOK_SECRET`)
- Backend maps to environment-specific values in secrets.yaml
- No need to change service configs between environments

## Literal Values

### Scenario

You have configuration values that differ by environment but aren't sensitive enough to encrypt.

### Configuration

**server.yaml:**
```yaml
schemaVersion: 1.0

secretsConfig:
  inheritAll: false

  environments:
    development:
      include:
        # Sensitive values from secrets.yaml
        DATABASE_URL: "${secret:DEV_DATABASE_URL}"
        API_KEY: "${secret:DEV_API_KEY}"

        # Non-sensitive literal values
        LOG_LEVEL: "debug"
        ENABLE_PROFILING: "true"
        FEATURE_FLAGS: "new-feature-beta,experimental-ui"

    staging:
      include:
        DATABASE_URL: "${secret:STAGING_DATABASE_URL}"
        API_KEY: "${secret:STAGING_API_KEY}"
        LOG_LEVEL: "info"
        ENABLE_PROFILING: "false"
        FEATURE_FLAGS: "new-feature-beta"

    production:
      include:
        DATABASE_URL: "${secret:PROD_DATABASE_URL}"
        API_KEY: "${secret:PROD_API_KEY}"
        LOG_LEVEL: "warn"
        ENABLE_PROFILING: "false"
        FEATURE_FLAGS: ""
```

**secrets.yaml:**
```yaml
schemaVersion: 1.0
values:
  DEV_DATABASE_URL: "postgresql://dev-user:dev-pass@localhost:5432/devdb"
  STAGING_DATABASE_URL: "postgresql://stage-user:stage-pass@staging-db.example.com:5432/stagedb"
  PROD_DATABASE_URL: "postgresql://prod-user:prod-pass@prod-db.example.com:5432/proddb"
  DEV_API_KEY: "dev-key-123"
  STAGING_API_KEY: "staging-key-456"
  PROD_API_KEY: "prod-key-789"
```

### Result

- Sensitive values stored encrypted in secrets.yaml
- Non-sensitive config values visible in server.yaml for version control
- Clean separation between secrets and configuration

## Exclusion Mode

### Scenario

You have many shared secrets but want to block specific production secrets from lower environments.

### Configuration

**server.yaml:**
```yaml
schemaVersion: 1.0

secretsConfig:
  inheritAll: true  # All secrets available by default

  environments:
    development:
      exclude:
        - PRODUCTION_API_KEY
        - STRIPE_LIVE_KEY
        - DATADOG_API_KEY

      override:
        # Override with test keys
        STRIPE_API_KEY: "${secret:STRIPE_TEST_KEY}"

    staging:
      exclude:
        - PRODUCTION_API_KEY
        - STRIPE_LIVE_KEY

      override:
        STRIPE_API_KEY: "${secret:STRIPE_TEST_KEY}"

    production:
      # No exclusions or overrides - all secrets available
```

**secrets.yaml:**
```yaml
schemaVersion: 1.0
values:
  # Shared secrets (available everywhere unless excluded)
  SLACK_WEBHOOK: "https://hooks.slack.com/services/..."
  NOTIFICATION_EMAIL: "alerts@example.com"
  DOMAIN_NAME: "example.com"

  # Production-only secrets
  PRODUCTION_API_KEY: "prod-api-key-secure"
  STRIPE_LIVE_KEY: "sk_live_..."
  DATADOG_API_KEY: "dd-api-..."

  # Test keys
  STRIPE_TEST_KEY: "sk_test_..."
```

### Result

- Most secrets shared across environments
- Production-specific secrets blocked from dev/staging
- Override mechanism provides test variants where needed

## Mixed Configuration

### Scenario

You have a mix of shared, environment-specific, and literal values.

### Configuration

**server.yaml:**
```yaml
schemaVersion: 1.0

secretsConfig:
  inheritAll: true  # Start with shared secrets

  environments:
    development:
      include:
        # Add development-specific values
        DEV_DATABASE_URL: "${secret:DEV_DB_URL}"

        # Literal overrides
        LOG_LEVEL: "debug"
        ENABLE_MOCK_SERVICES: "true"

    staging:
      exclude:
        - PRODUCTION_DATABASE_URL
        - PRODUCTION_API_KEY

      include:
        STAGING_DATABASE_URL: "${secret:STAGE_DB_URL}"
        LOG_LEVEL: "info"

    production:
      # Inherit all, add production-specific
      include:
        PRODUCTION_DATABASE_URL: "${secret:PROD_DB_URL}"
        LOG_LEVEL: "warn"
```

**secrets.yaml:**
```yaml
schemaVersion: 1.0
values:
  # Shared across all environments
  DOMAIN: "example.com"
  SLACK_WEBHOOK: "https://hooks.slack.com/..."

  # Environment-specific
  DEV_DB_URL: "postgresql://localhost/dev"
  STAGE_DB_URL: "postgresql://staging-db/stage"
  PROD_DB_URL: "postgresql://prod-db/prod"
  PRODUCTION_API_KEY: "prod-key-secret"
```

### Result

- Shared secrets available everywhere
- Environment-specific additions per environment
- Production secrets protected from lower environments
- Literal values provide environment-specific configuration

## Real-World Use Cases

### Use Case 1: SaaS Application with Multiple Environments

**Scenario:** A SaaS application with development, staging, and production environments using different Stripe accounts and database instances.

**server.yaml:**
```yaml
schemaVersion: 1.0

provisioner:
  type: pulumi
  config:
    # ... provisioner config ...

secretsConfig:
  inheritAll: false

  environments:
    development:
      include:
        # Database
        DATABASE_URL: "${secret:DEV_DATABASE_URL}"
        DATABASE_PASSWORD: "${secret:DEV_DATABASE_PASSWORD}"

        # Stripe (test mode)
        STRIPE_API_KEY: "${secret:STRIPE_TEST_KEY}"
        STRIPE_WEBHOOK_SECRET: "${secret:STRIPE_TEST_WEBHOOK_SECRET}"

        # Email (sandbox)
        SENDGRID_API_KEY: "${secret:SENDGRID_SANDBOX_KEY}"

        # Configuration
        ENVIRONMENT: "development"
        DEBUG: "true"

    staging:
      include:
        DATABASE_URL: "${secret:STAGING_DATABASE_URL}"
        DATABASE_PASSWORD: "${secret:STAGING_DATABASE_PASSWORD}"
        STRIPE_API_KEY: "${secret:STRIPE_TEST_KEY}"
        STRIPE_WEBHOOK_SECRET: "${secret:STRIPE_TEST_WEBHOOK_SECRET}"
        SENDGRID_API_KEY: "${secret:SENDGRID_STAGING_KEY}"
        ENVIRONMENT: "staging"
        DEBUG: "false"

    production:
      include:
        DATABASE_URL: "${secret:PRODUCTION_DATABASE_URL}"
        DATABASE_PASSWORD: "${secret:PRODUCTION_DATABASE_PASSWORD}"
        STRIPE_API_KEY: "${secret:STRIPE_LIVE_KEY}"
        STRIPE_WEBHOOK_SECRET: "${secret:STRIPE_LIVE_WEBHOOK_SECRET}"
        SENDGRID_API_KEY: "${secret:SENDGRID_PRODUCTION_KEY}"
        ENVIRONMENT: "production"
        DEBUG: "false"
```

### Use Case 2: Multi-Tenant Platform

**Scenario:** Platform with shared infrastructure but tenant-specific configuration.

**server.yaml:**
```yaml
schemaVersion: 1.0

secretsConfig:
  inheritAll: false

  environments:
    # Tenant A environments
    tenant-a-staging:
      include:
        TENANT_ID: "tenant-a"
        DATABASE_SCHEMA: "${secret:TENANT_A_STAGING_SCHEMA}"
        API_KEY: "${secret:TENANT_A_STAGING_KEY}"

    tenant-a-production:
      include:
        TENANT_ID: "tenant-a"
        DATABASE_SCHEMA: "${secret:TENANT_A_PROD_SCHEMA}"
        API_KEY: "${secret:TENANT_A_PROD_KEY}"

    # Tenant B environments
    tenant-b-staging:
      include:
        TENANT_ID: "tenant-b"
        DATABASE_SCHEMA: "${secret:TENANT_B_STAGING_SCHEMA}"
        API_KEY: "${secret:TENANT_B_STAGING_KEY}"

    tenant-b-production:
      include:
        TENANT_ID: "tenant-b"
        DATABASE_SCHEMA: "${secret:TENANT_B_PROD_SCHEMA}"
        API_KEY: "${secret:TENANT_B_PROD_KEY}"
```

### Use Case 3: Gradual Migration

**Scenario:** Existing application migrating to environment-specific secrets incrementally.

**Phase 1: Start with exclusion mode (safe)**
```yaml
schemaVersion: 1.0

secretsConfig:
  inheritAll: true  # Keep existing behavior

  environments:
    staging:
      exclude:
        - PRODUCTION_ONLY_SECRET

    production:
      # No changes - all secrets available
```

**Phase 2: Add explicit mappings**
```yaml
schemaVersion: 1.0

secretsConfig:
  inheritAll: true

  environments:
    staging:
      exclude:
        - PRODUCTION_ONLY_SECRET
        - STRIPE_LIVE_KEY

      override:
        STRIPE_API_KEY: "${secret:STRIPE_TEST_KEY}"

    production:
      # Still unchanged
```

**Phase 3: Move to include mode (strict)**
```yaml
schemaVersion: 1.0

secretsConfig:
  inheritAll: false

  environments:
    staging:
      include:
        SHARED_SECRET: ~  # Reference by same name
        STRIPE_API_KEY: "${secret:STRIPE_TEST_KEY}"
        DATABASE_URL: "${secret:STAGING_DATABASE_URL}"

    production:
      include:
        SHARED_SECRET: ~
        STRIPE_API_KEY: "${secret:STRIPE_LIVE_KEY}"
        PRODUCTION_ONLY_SECRET: ~
        DATABASE_URL: "${secret:PRODUCTION_DATABASE_URL}"
```

### Use Case 4: Feature Flag Configuration

**Scenario:** Environment-specific feature flags stored as literal values.

**server.yaml:**
```yaml
schemaVersion: 1.0

secretsConfig:
  inheritAll: false

  environments:
    development:
      include:
        # Sensitive values from secrets.yaml
        DATABASE_URL: "${secret:DEV_DATABASE_URL}"

        # Feature flags (literal values)
        FEATURE_NEW_UI: "true"
        FEATURE_EXPERIMENTAL_API: "true"
        FEATURE_BILLING_V2: "false"

    staging:
      include:
        DATABASE_URL: "${secret:STAGING_DATABASE_URL}"
        FEATURE_NEW_UI: "true"
        FEATURE_EXPERIMENTAL_API: "true"
        FEATURE_BILLING_V2: "true"  # Testing new billing

    production:
      include:
        DATABASE_URL: "${secret:PRODUCTION_DATABASE_URL}"
        FEATURE_NEW_UI: "false"     # Disabled for production
        FEATURE_EXPERIMENTAL_API: "false"
        FEATURE_BILLING_V2: "false"
```

## Common Patterns

### Pattern 1: Direct Reference (Same Key)

```yaml
include:
  SLACK_WEBHOOK: ~  # References SLACK_WEBHOOK in secrets.yaml
```

### Pattern 2: Mapped Reference

```yaml
include:
  DATABASE_URL: "${secret:PROD_DATABASE_URL}"  # Maps to different key
```

### Pattern 3: Literal Value

```yaml
include:
  ENVIRONMENT: "production"
  DEBUG: "false"
```

### Pattern 4: Exclusion

```yaml
exclude:
  - PRODUCTION_SECRET_1
  - PRODUCTION_SECRET_2
```

### Pattern 5: Override

```yaml
override:
  API_KEY: "${secret:TEST_API_KEY}"  # Override inherited value
```

## Validation Examples

### Valid Configuration

```yaml
secretsConfig:
  inheritAll: false
  environments:
    staging:
      include:
        SECRET_1: ~
        SECRET_2: "${secret:OTHER_SECRET}"
        SECRET_3: "literal-value"
```

### Invalid Configuration

```yaml
# ERROR: Cannot use both include and exclude
secretsConfig:
  inheritAll: false
  environments:
    staging:
      include:
        SECRET_1: ~
      exclude:
        - SECRET_2
```

```yaml
# ERROR: exclude requires inheritAll: true
secretsConfig:
  inheritAll: false
  environments:
    staging:
      exclude:
        - SECRET_1
```

```yaml
# ERROR: Invalid secret reference format
secretsConfig:
  inheritAll: false
  environments:
    staging:
      include:
        SECRET_1: "${secret:}"  # Empty reference
```
