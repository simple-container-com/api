# Acceptance Criteria and Test Scenarios: Environment-Specific Secrets

**Feature Request:** Environment-Specific Secrets in Parent Stacks
**Issue ID:** #60
**Date:** 2026-02-08

## Overview

This document provides detailed acceptance criteria for each functional requirement and comprehensive test scenarios to validate the implementation.

## Acceptance Criteria by Functional Requirement

### FR-1: Environment-Aware Secret Storage

#### AC-1.1: Schema Version 2.0 Support

**Given:** A new `secrets.yaml` file using schema version 2.0
**When:** The file is parsed by the secrets management system
**Then:**
- The file is parsed without errors
- Environment-specific secrets are loaded correctly
- Shared secrets are loaded correctly
- Default environment is recognized

**Test Case 1.1.1: Valid Schema v2.0**
```yaml
# .sc/stacks/infrastructure/secrets.yaml
schemaVersion: 2.0
defaultEnvironment: development
environments:
  production:
    values:
      api-key: "prod-key"
  development:
    values:
      api-key: "dev-key"
shared:
  values:
    company: "Example Corp"
```
**Expected Result:** File loads successfully, all secrets accessible

**Test Case 1.1.2: Missing Default Environment**
```yaml
schemaVersion: 2.0
environments:
  production:
    values:
      api-key: "prod-key"
```
**Expected Result:** File loads successfully, but error when environment not specified

#### AC-1.2: Backward Compatibility

**Given:** An existing `secrets.yaml` file using schema version 1.0 (or no version)
**When:** The file is parsed by the secrets management system
**Then:**
- The file is parsed without errors
- Secrets are accessible using existing syntax
- No migration is required

**Test Case 1.2.1: Schema v1.0 File**
```yaml
# .sc/stacks/infrastructure/secrets.yaml
schemaVersion: 1.0
values:
  api-key: "shared-key"
```
**Expected Result:** File loads successfully, `${secret:api-key}` works

**Test Case 1.2.2: No Schema Version**
```yaml
# .sc/stacks/infrastructure/secrets.yaml
values:
  api-key: "shared-key"
```
**Expected Result:** File loads successfully, treated as v1.0

#### AC-1.3: Environment Name Validation

**Given:** A schema v2.0 file with invalid environment names
**When:** The file is parsed
**Then:** A clear error message is displayed indicating the invalid environment name

**Test Case 1.3.1: Invalid Characters**
```yaml
schemaVersion: 2.0
environments:
  prod@uction:
    values:
      api-key: "key"
```
**Expected Result:** Error message "Invalid environment name 'prod@uction'. Environment names must contain only alphanumeric characters, hyphens, and underscores"

### FR-2: Environment Context Specification

#### AC-2.1: CLI Flag Support

**Given:** A stack configuration with environment-specific secrets
**When:** User runs `sc apply --environment production`
**Then:** Secrets from the production environment are used

**Test Case 2.1.1: Valid Environment Flag**
```bash
sc apply --environment production
```
**Expected Result:** Production secrets are resolved and applied

**Test Case 2.1.2: Invalid Environment Flag**
```bash
sc apply --environment nonexistent
```
**Expected Result:** Error message "Environment 'nonexistent' not found. Available environments: production, development"

#### AC-2.2: Stack Configuration Environment

**Given:** A child stack with `environment: production` in its client.yaml
**When:** The stack is applied without CLI flag
**Then:** Production secrets are used

**Test Case 2.2.1: Stack-Level Environment**
```yaml
# .sc/stacks/prod-app/client.yaml
schemaVersion: 1.0
stack:
  parent: infrastructure
  environment: production
```
**Expected Result:** Production secrets are resolved

**Test Case 2.2.2: CLI Flag Overrides Stack Config**
```yaml
# Same stack as above
```
```bash
sc apply --environment development
```
**Expected Result:** Development secrets are used (CLI flag takes precedence)

#### AC-2.3: Environment Variable Support

**Given:** The `SC_ENVIRONMENT` variable is set
**When:** A command is run without CLI flag
**Then:** The environment from the variable is used

**Test Case 2.3.1: Environment Variable**
```bash
export SC_ENVIRONMENT=staging
sc apply
```
**Expected Result:** Staging secrets are resolved

**Test Case 2.3.2: Precedence Order**
```bash
export SC_ENVIRONMENT=development
sc apply --environment production
```
**Expected Result:** Production secrets are used (CLI flag overrides environment variable)

### FR-3: Environment-Specific Secret Resolution

#### AC-3.1: Implicit Environment Resolution

**Given:** A stack with environment context set to production
**When:** A placeholder `${secret:api-key}` is resolved
**Then:** The value from the production environment is returned

**Test Case 3.1.1: Implicit Resolution**
```yaml
# Stack context: environment=production
config:
  key: "${secret:api-key}"
```
**Expected Result:** Resolves to production API key

**Test Case 3.1.2: Environment Context from Parent**
```yaml
# Parent stack: infrastructure (has production, development)
# Child stack: prod-app (environment: production in client.yaml)
config:
  key: "${secret:api-key}"
```
**Expected Result:** Resolves to production API key from parent

#### AC-3.2: Explicit Environment Specification

**Given:** A placeholder with explicit environment `${secret:api-key:staging}`
**When:** The placeholder is resolved
**Then:** The value from the staging environment is returned, regardless of current context

**Test Case 3.2.1: Explicit Environment Overrides Context**
```yaml
# Stack context: environment=production
config:
  key: "${secret:api-key:staging}"
```
**Expected Result:** Resolves to staging API key (explicit overrides context)

**Test Case 3.2.2: Invalid Explicit Environment**
```yaml
config:
  key: "${secret:api-key:nonexistent}"
```
**Expected Result:** Error message "Environment 'nonexistent' not found for secret 'api-key'. Available environments: production, staging, development"

#### AC-3.3: Shared Secrets Access

**Given:** A shared secret defined in the schema
**When:** The secret is referenced from any environment
**Then:** The shared value is returned

**Test Case 3.3.1: Shared Secret Access**
```yaml
# secrets.yaml
schemaVersion: 2.0
shared:
  values:
    company: "Example Corp"
environments:
  production:
    values:
      api-key: "prod-key"
```
```yaml
# Stack context: environment=production
config:
  company: "${secret:company}"
```
**Expected Result:** Resolves to "Example Corp"

**Test Case 3.3.2: Environment-Specific Overrides Shared**
```yaml
# secrets.yaml
schemaVersion: 2.0
shared:
  values:
    region: "us-east-1"
environments:
  production:
    values:
      region: "eu-west-1"
```
**Expected Result:** Production context resolves to "eu-west-1" (environment-specific takes precedence)

#### AC-3.4: Backward Compatibility with Existing Syntax

**Given:** A schema v1.0 file with existing placeholder syntax
**When:** The placeholder `${secret:api-key}` is resolved
**Then:** The value is returned correctly

**Test Case 3.4.1: Old Syntax with v1.0 Schema**
```yaml
# v1.0 secrets.yaml
schemaVersion: 1.0
values:
  api-key: "shared-key"
```
```yaml
config:
  key: "${secret:api-key}"
```
**Expected Result:** Resolves to "shared-key"

### FR-4: Parent Stack Environment Inheritance

#### AC-4.1: Child Stack Environment Specification

**Given:** A parent stack with multiple environments
**When:** Child stacks specify different environments
**Then:** Each child gets the correct environment's secrets

**Test Case 4.1.1: Multiple Children with Different Environments**
```yaml
# Parent: .sc/stacks/infrastructure/secrets.yaml
schemaVersion: 2.0
environments:
  production:
    values:
      api-key: "prod-key"
  development:
    values:
      api-key: "dev-key"
```

```yaml
# Child 1: .sc/stacks/prod-app/client.yaml
stack:
  parent: infrastructure
  environment: production
```

```yaml
# Child 2: .sc/stacks/dev-app/client.yaml
stack:
  parent: infrastructure
  environment: development
```

**Expected Result:**
- prod-app resolves to "prod-key"
- dev-app resolves to "dev-key"

#### AC-4.2: Parent Stack Validation

**Given:** A child stack references a parent stack
**When:** The parent stack doesn't support the requested environment
**Then:** A clear error message is displayed

**Test Case 4.2.1: Parent Without Requested Environment**
```yaml
# Parent: .sc/stacks/infrastructure/secrets.yaml
schemaVersion: 2.0
environments:
  development:
    values:
      api-key: "dev-key"
```

```yaml
# Child: .sc/stacks/prod-app/client.yaml
stack:
  parent: infrastructure
  environment: production
```

**Expected Result:** Error message "Environment 'production' not found in parent stack 'infrastructure'. Available environments: development"

### FR-5: Secret Validation and Error Handling

#### AC-5.1: Missing Secret Error Messages

**Given:** A placeholder references a non-existent secret
**When:** The placeholder is resolved
**Then:** A clear error message indicates the missing secret and available alternatives

**Test Case 5.1.1: Missing Secret in Environment**
```yaml
config:
  key: "${secret:nonexistent-secret}"
```
**Expected Result:** Error message "Secret 'nonexistent-secret' not found in environment 'production'. Available secrets: api-key, database-password"

**Test Case 5.1.2: Missing Secret with Explicit Environment**
```yaml
config:
  key: "${secret:nonexistent-secret:staging}"
```
**Expected Result:** Error message "Secret 'nonexistent-secret' not found in environment 'staging'. Available secrets: api-key, database-password"

#### AC-5.2: Security Warnings

**Given:** A development stack attempts to use production secrets
**When:** The secret is resolved
**Then:** A security warning is displayed

**Test Case 5.2.1: Production Secret in Development**
```yaml
# Stack context: environment=development
config:
  key: "${secret:api-key:production}"
```
**Expected Result:** Warning message "Security Warning: Using production secrets in development environment. Stack: dev-app, Secret: api-key"

#### AC-5.3: Dry-Run Mode

**Given:** A stack configuration with secret placeholders
**When:** Running `sc apply --dry-run --environment production`
**Then:** The command shows which secrets would be resolved without applying changes

**Test Case 5.3.1: Dry-Run Shows Resolution**
```bash
sc apply --dry-run --environment production
```
**Expected Output:**
```
Dry-run mode: No changes will be applied
Secret resolution preview:
  ${secret:api-key} → [PROD-KEY-VALUE]
  ${secret:database-password} → [PROD-PASSWORD-VALUE]
```

## Integration Test Scenarios

### Scenario 1: Multi-Environment Deployment

**Description:** Deploy the same application to production, staging, and development environments using a single parent stack.

**Setup:**
```yaml
# .sc/stacks/infrastructure/secrets.yaml
schemaVersion: 2.0
defaultEnvironment: development
environments:
  production:
    values:
      database-host: "prod-db.example.com"
      database-password: "prod-secure-password"
      api-key: "prod-api-key-12345"
  staging:
    values:
      database-host: "staging-db.example.com"
      database-password: "staging-secure-password"
      api-key: "staging-api-key-67890"
  development:
    values:
      database-host: "localhost"
      database-password: "dev-password"
      api-key: "dev-api-key-11111"
shared:
  values:
    app-name: "My Application"
```

**Test Steps:**
1. Deploy to production: `sc apply stack prod-app --environment production`
2. Verify production secrets are used
3. Deploy to staging: `sc apply stack staging-app --environment staging`
4. Verify staging secrets are used
5. Deploy to development: `sc apply stack dev-app --environment development`
6. Verify development secrets are used

**Expected Results:**
- Each environment uses correct database host
- Each environment uses correct password
- All environments share the same app-name

### Scenario 2: Parent-Child Stack Inheritance

**Description:** Child stacks inherit environment-specific secrets from parent stack.

**Setup:**
```yaml
# Parent: .sc/stacks/common/secrets.yaml
schemaVersion: 2.0
environments:
  production:
    values:
      cloud-api-key: "prod-cloud-key"
      cdn-url: "cdn.example.com"
  development:
    values:
      cloud-api-key: "dev-cloud-key"
      cdn-url: "dev-cdn.example.com"
```

```yaml
# Child: .sc/stacks/frontend-app/client.yaml
schemaVersion: 1.0
stack:
  parent: common
  environment: production
config:
  cloud-key: "${secret:cloud-api-key}"
  cdn: "${secret:cdn-url}"
```

**Test Steps:**
1. Apply child stack with `environment: production`
2. Verify production secrets from parent are used
3. Change child stack to `environment: development`
4. Verify development secrets from parent are used

**Expected Results:**
- Child stack correctly inherits environment-specific secrets
- Changing child environment changes which secrets are inherited

### Scenario 3: Explicit Environment Override

**Description:** Use explicit environment specification to override context.

**Setup:**
```yaml
# .sc/stacks/app/secrets.yaml
schemaVersion: 2.0
environments:
  production:
    values:
      test-key: "prod-value"
  staging:
    values:
      test-key: "staging-value"
```

**Test Steps:**
1. Create stack with `environment: production`
2. Use placeholder `${secret:test-key}` → should resolve to "prod-value"
3. Use placeholder `${secret:test-key:staging}` → should resolve to "staging-value"

**Expected Results:**
- Implicit placeholder uses context environment
- Explicit placeholder overrides context

### Scenario 4: Migration from v1.0 to v2.0

**Description:** Migrate existing v1.0 configuration to v2.0 format.

**Setup:**
```yaml
# Existing .sc/stacks/app/secrets.yaml (v1.0)
schemaVersion: 1.0
values:
  database-host: "db.example.com"
  database-password: "secure-password"
  api-key: "api-key-123"
```

**Test Steps:**
1. Run migration tool: `sc migrate-secrets --environment production`
2. Review proposed v2.0 structure
3. Confirm migration
4. Verify original file is backed up
5. Verify new v2.0 file works correctly

**Expected Results:**
- Migration tool creates valid v2.0 schema
- All existing secrets are preserved
- Application continues to work with new schema

### Scenario 5: Shared Secrets Override

**Description:** Verify that environment-specific secrets override shared secrets.

**Setup:**
```yaml
# .sc/stacks/app/secrets.yaml
schemaVersion: 2.0
shared:
  values:
    region: "us-east-1"
    timeout: "30s"
environments:
  production:
    values:
      region: "eu-west-1"
  development:
    values:
      timeout: "60s"
```

**Test Steps:**
1. Resolve `${secret:region}` in production context
2. Resolve `${secret:region}` in development context
3. Resolve `${secret:timeout}` in production context
4. Resolve `${secret:timeout}` in development context

**Expected Results:**
- Production region: "eu-west-1" (environment-specific override)
- Development region: "us-east-1" (shared value)
- Production timeout: "30s" (shared value)
- Development timeout: "60s" (environment-specific override)

## Edge Cases and Negative Tests

### Edge Case 1: Empty Environment

**Scenario:** Environment with no secrets defined
```yaml
environments:
  production:
    values:
      api-key: "prod-key"
  staging:
    values: {}
```
**Expected Result:** Environment is valid, but has no secrets

### Edge Case 2: Secret with Empty Value

**Scenario:** Secret defined with empty string value
```yaml
environments:
  production:
    values:
      optional-secret: ""
```
**Expected Result:** Empty string is returned (not an error)

### Edge Case 3: Case-Sensitive Environment Names

**Scenario:** Reference environment with different case
```yaml
environments:
  Production:
    values:
      key: "value"
```
**Reference:** `${secret:key:production}`
**Expected Result:** Error "Environment 'production' not found" (case-sensitive)

### Edge Case 4: Special Characters in Secret Names

**Scenario:** Secret names with special characters
```yaml
environments:
  production:
    values:
      "my-secret_key.123": "value"
```
**Reference:** `${secret:my-secret_key.123}`
**Expected Result:** Should work if properly quoted

### Edge Case 5: Circular Dependencies

**Scenario:** Attempt to create circular environment references
**Note:** This should be prevented by design since environments are not hierarchical

### Negative Test 1: Invalid Schema Version

**Scenario:**
```yaml
schemaVersion: 3.0
```
**Expected Result:** Error "Unsupported schema version '3.0'. Supported versions: 1.0, 2.0"

### Negative Test 2: Malformed YAML

**Scenario:** Invalid YAML syntax in secrets.yaml
**Expected Result:** Clear YAML parsing error with line number

### Negative Test 3: Missing Required Field

**Scenario:** v2.0 schema without `environments` field
**Expected Result:** Error "Schema version 2.0 requires 'environments' field"

## Performance Test Scenarios

### Performance Test 1: Large Number of Environments

**Setup:** 100 environments with 50 secrets each
**Metric:** Configuration parsing time
**Target:** < 100ms

### Performance Test 2: Secret Resolution Speed

**Setup:** Resolve 1000 secret placeholders
**Metric:** Average resolution time per placeholder
**Target:** < 10ms per placeholder

### Performance Test 3: Deep Inheritance Chain

**Setup:** 5 levels of parent-child stack inheritance
**Metric:** Secret resolution time
**Target:** < 50ms per secret

## Security Test Scenarios

### Security Test 1: Cross-Environment Secret Access

**Attempt:** Try to access production secrets from development context using implicit reference
**Expected:** Production secrets should NOT be accessible via implicit reference
**Workaround:** Explicit reference `${secret:key:production}` should work but generate warning

### Security Test 2: Secret Exposure in Error Messages

**Scenario:** Trigger an error with secret values
**Expected:** Error messages should NOT contain secret values

### Security Test 3: Environment Context Bypass

**Attempt:** Try to bypass environment context validation
**Expected:** System should validate environment context before secret access

## Regression Tests

### Regression Test 1: Existing v1.0 Deployments

**Setup:** Use existing v1.0 configuration without any changes
**Expected:** All existing functionality continues to work

### Regression Test 2: Existing Placeholder Syntax

**Setup:** Use existing `${secret:name}` syntax
**Expected:** Continues to work as before

### Regression Test 3: Existing Inheritance

**Setup:** Use existing parent-child stack inheritance
**Expected:** Continues to work as before

## Test Data Requirements

### Test Configuration Files

The following test configurations should be created in `pkg/api/secrets/testdata/environments/`:

1. `v2-basic.yaml` - Simple v2.0 schema
2. `v2-multiple-envs.yaml` - Multiple environments
3. `v2-shared-secrets.yaml` - With shared secrets
4. `v2-overrides.yaml` - Environment-specific overrides
5. `v1-basic.yaml` - v1.0 schema for backward compatibility tests
6. `parent-stack.yaml` - Parent stack with multiple environments
7. `child-stack-prod.yaml` - Child stack using production
8. `child-stack-dev.yaml` - Child stack using development

### Mock Secrets

For testing, use these mock secret values:
- Production: `prod-api-key-12345`, `prod-password-abcde`
- Staging: `staging-api-key-67890`, `staging-password-fghij`
- Development: `dev-api-key-11111`, `dev-password-klmno`
- Shared: `shared-value-xyz`, `common-setting-999`

## Automated Testing Requirements

### Unit Tests

- Schema parsing (v1.0 and v2.0)
- Environment context management
- Secret resolution logic
- Error handling and validation
- Placeholder parsing

### Integration Tests

- End-to-end stack deployment with environment-specific secrets
- Parent-child stack inheritance
- CLI flag functionality
- Migration tool functionality

### Performance Tests

- Benchmark secret resolution
- Benchmark configuration parsing
- Compare v1.0 vs v2.0 performance

## Manual Testing Checklist

- [ ] Deploy to production environment
- [ ] Deploy to staging environment
- [ ] Deploy to development environment
- [ ] Test explicit environment override
- [ ] Test shared secrets
- [ ] Test parent stack inheritance
- [ ] Test error messages
- [ ] Test security warnings
- [ ] Test dry-run mode
- [ ] Test migration tool
- [ ] Verify backward compatibility with existing deployments
