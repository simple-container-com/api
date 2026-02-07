# Environment-Specific Secrets - Validation and Migration Guide

## Validation Requirements

### Pre-Deployment Validation

The `sc validate` command MUST perform the following checks:

#### V-1: Configuration Validation

**Validates:** server.yaml structure

**Checks:**
1. `include` and `exclude` sections are not used together in the same environment
2. `exclude` section is only used with `inheritAll: true`
3. Secret reference syntax is valid (`${secret:KEY}` format)
4. No empty secret references (`${secret:}`)

**Error Messages:**
```
Error: environment "staging": cannot use both 'include' and 'exclude' sections

Error: environment "staging": 'exclude' section requires 'inheritAll: true'

Error: environment "production": invalid secret reference for "API_KEY": "${secret:}"
```

#### V-2: Secret Reference Validation

**Validates:** References in secretsConfig point to existing secrets

**Checks:**
1. All `${secret:KEY}` references in `include`/`override` sections exist in secrets.yaml
2. All `~` (null) references have corresponding keys in secrets.yaml (when `inheritAll: false`)

**Error Messages:**
```
Error: environment "staging": mapped secret "DATABASE_PASSWORD_STAGING" not found in secrets.yaml

Error: environment "production": secret reference "STRIPE_API_KEY" (via ~) not found in secrets.yaml
```

#### V-3: Client Secret Availability Validation

**Validates:** Secrets referenced in client.yaml are available in target environment

**Checks:**
1. All `${secret:NAME}` references in client.yaml are available for the target environment
2. Consider `inheritAll`, `include`, `exclude`, and `override` rules

**Error Messages:**
```
Warning: secret "PRODUCTION_API_KEY" is not available in environment "staging"
Warning: secret "DATADOG_API_KEY" is excluded in environment "staging"

Error: secret "DATABASE_URL" referenced in client.yaml is not available in environment "production"
```

#### V-4: Unused Secrets Warning

**Validates:** Secrets in secrets.yaml that are never used

**Checks:**
1. Secrets defined in secrets.yaml that are not referenced by any environment
2. Secrets in `include`/`override` sections that are never used by clients

**Warning Messages:**
```
Warning: secret "OLD_API_KEY" in secrets.yaml is not used in any environment
Warning: secret "TEST_SECRET" configured for environment "staging" is not referenced by any client
```

### Validation Commands

```bash
# Validate all stacks
sc validate

# Validate specific stack
sc validate -s my-stack

# Validate specific environment
sc validate -s my-stack -e staging

# Detailed validation with explanations
sc validate --verbose
```

## Migration Guide

### Migration Strategy

This feature is **fully backwards compatible**. No migration is required for existing stacks.

**Key Principles:**
1. **Opt-in:** Existing stacks continue working without modification
2. **Incremental adoption:** Adopt environment-specific secrets gradually
3. **Safe defaults:** `inheritAll: true` when section is absent (current behavior)

### Phase 1: Assessment (No Changes)

**Goal:** Understand current secret usage

**Steps:**

1. Audit current secrets:
```bash
# List all secrets in parent stack
cat .sc/stacks/devops/secrets.yaml | grep -E "^  [A-Z_]+:"
```

2. Check which environments reference which secrets:
```bash
# Search for secret references in client configs
grep -r "\${secret:" .sc/stacks/*/client.yaml
```

3. Identify secrets that should be environment-specific:
- Production API keys
- Database passwords
- Third-party service credentials

**Deliverable:** Inventory of secrets and their current usage

### Phase 2: Safe Exclusion (Low Risk)

**Goal:** Block production secrets from lower environments

**Approach:** Use `inheritAll: true` with `exclude` list

**Before (current state):**
```yaml
# .sc/stacks/devops/secrets.yaml
values:
  SHARED_SECRET: "shared-value"
  PROD_API_KEY: "prod-key-secret"
  TEST_API_KEY: "test-key"
```

**After (Phase 2):**
```yaml
# .sc/stacks/devops/server.yaml
schemaVersion: 1.0

secretsConfig:
  inheritAll: true  # Keep existing behavior

  environments:
    development:
      exclude:
        - PROD_API_KEY  # Block production key

      override:
        API_KEY: "${secret:TEST_API_KEY}"  # Use test key

    staging:
      exclude:
        - PROD_API_KEY
      override:
        API_KEY: "${secret:TEST_API_KEY}"

    production:
      # No exclusions - all secrets available
```

**Benefits:**
- Production secrets protected from dev/staging
- Minimal changes to existing behavior
- Easy to rollback (remove section)

### Phase 3: Explicit Mappings (Medium Risk)

**Goal:** Explicitly define which secrets go where

**Approach:** Use `override` to map secret names to environment-specific values

**Configuration:**
```yaml
# .sc/stacks/devops/server.yaml
secretsConfig:
  inheritAll: true

  environments:
    development:
      exclude:
        - PROD_DATABASE_PASSWORD
        - PROD_STRIPE_KEY

      override:
        DATABASE_PASSWORD: "${secret:DEV_DATABASE_PASSWORD}"
        STRIPE_API_KEY: "${secret:DEV_STRIPE_KEY}"

    staging:
      exclude:
        - PROD_DATABASE_PASSWORD
        - DEV_DATABASE_PASSWORD
        - PROD_STRIPE_KEY

      override:
        DATABASE_PASSWORD: "${secret:STAGING_DATABASE_PASSWORD}"
        STRIPE_API_KEY: "${secret:STAGING_STRIPE_KEY}"

    production:
      exclude:
        - DEV_DATABASE_PASSWORD
        - STAGING_DATABASE_PASSWORD
        - DEV_STRIPE_KEY

      override:
        DATABASE_PASSWORD: "${secret:PROD_DATABASE_PASSWORD}"
        STRIPE_API_KEY: "${secret:PROD_STRIPE_KEY}"
```

**Benefits:**
- Clear mapping between secret names and values
- Consistent secret names across environments
- Easy to audit which secrets are used where

### Phase 4: Strict Mode (Advanced)

**Goal:** Only allow explicitly defined secrets per environment

**Approach:** Use `inheritAll: false` with `include` lists

**Configuration:**
```yaml
# .sc/stacks/devops/server.yaml
secretsConfig:
  inheritAll: false  # Whitelist mode

  environments:
    development:
      include:
        # Sensitive values from secrets.yaml
        DATABASE_PASSWORD: "${secret:DEV_DATABASE_PASSWORD}"
        STRIPE_API_KEY: "${secret:DEV_STRIPE_KEY}"

        # Literal values
        LOG_LEVEL: "debug"
        ENVIRONMENT: "development"

    staging:
      include:
        DATABASE_PASSWORD: "${secret:STAGING_DATABASE_PASSWORD}"
        STRIPE_API_KEY: "${secret:STAGING_STRIPE_KEY}"
        LOG_LEVEL: "info"
        ENVIRONMENT: "staging"

    production:
      include:
        DATABASE_PASSWORD: "${secret:PROD_DATABASE_PASSWORD}"
        STRIPE_API_KEY: "${secret:PROD_STRIPE_KEY}"
        LOG_LEVEL: "warn"
        ENVIRONMENT: "production"
```

**Benefits:**
- Maximum security: only defined secrets available
- Clear audit trail of secrets per environment
- No accidental secret exposure

**Trade-offs:**
- More verbose configuration
- Must explicitly add new secrets to each environment

## Rollback Strategy

If issues arise after adopting environment-specific secrets:

### Immediate Rollback

1. **Remove the `secretsConfig` section** from server.yaml:
```bash
# Edit server.yaml and remove the secretsConfig section
git restore .sc/stacks/devops/server.yaml
```

2. **Redeploy:**
```bash
sc provision -s devops --update
sc deploy -s my-app -e staging
```

### Partial Rollback

1. **Revert to previous phase** (e.g., from Phase 4 back to Phase 3)
2. **Test in lower environment first**

### Debugging Issues

If secrets aren't resolving correctly:

1. **Validate configuration:**
```bash
sc validate -s devops -e staging --verbose
```

2. **Check secret availability:**
```bash
# Show which secrets are available for environment
sc secret list -s devops -e staging
```

3. **Verify secret references:**
```bash
# Check what secrets client is trying to use
grep "\${secret:" .sc/stacks/my-app/client.yaml
```

## Testing Strategy

### Pre-Migration Testing

1. **Create test stack** with environment-specific secrets
2. **Deploy to development** and verify secret resolution
3. **Test validation** with invalid configurations
4. **Verify rollback** by removing `secretsConfig` section

### Testing Checklist

- [ ] Development deployment resolves secrets correctly
- [ ] Staging deployment resolves secrets correctly
- [ ] Production deployment resolves secrets correctly
- [ ] Production secrets NOT accessible in staging/development
- [ ] Validation errors for missing secrets
- [ ] Validation errors for invalid mappings
- [ ] Validation warnings for unused secrets
- [ ] Client configurations work without changes
- [ ] Rollback restores previous behavior

## Common Migration Issues

### Issue 1: Secret Not Found

**Symptom:**
```
Error: secret "DATABASE_URL" is not available in environment "staging"
```

**Solution:**
1. Check if secret is in `include` list for staging
2. Check if secret is in `exclude` list for staging
3. Verify `inheritAll` setting

### Issue 2: Invalid Mapping

**Symptom:**
```
Error: mapped secret "DB_PASSWORD_STAGING" not found in secrets.yaml
```

**Solution:**
1. Verify secret exists in secrets.yaml
2. Check for typos in secret name
3. Ensure secrets.yaml is decrypted

### Issue 3: Both Include and Exclude

**Symptom:**
```
Error: environment "staging": cannot use both 'include' and 'exclude' sections
```

**Solution:**
Choose one approach:
- Use `include` for whitelist mode (`inheritAll: false`)
- Use `exclude` for blacklist mode (`inheritAll: true`)

### Issue 4: Client Configuration Not Working

**Symptom:**
Services can't access secrets after migration

**Solution:**
1. Verify client.yaml hasn't changed
2. Check secret names match between client and server
3. Run validation: `sc validate -s my-stack -e staging`

## Best Practices

### DO:

1. **Start with exclusion mode** (Phase 2) for safer migration
2. **Test in development** before production
3. **Use validation** before deploying
4. **Document secret mappings** in code comments
5. **Audit secrets regularly** for unused secrets
6. **Use literal values** for non-sensitive config
7. **Keep secret names consistent** across environments

### DON'T:

1. **Don't use both include and exclude** in same environment
2. **Don't use exclude without inheritAll: true**
3. **Don't skip validation** before deployment
4. **Don't commit actual secret values** to server.yaml (use references)
5. **Don't use empty secret references** (`${secret:}`)
6. **Don't migrate all stacks at once** - start with one

## Support and Troubleshooting

### Getting Help

1. **Run validation first:**
```bash
sc validate --verbose
```

2. **Check documentation:**
- `docs/product-manager/environment-specific-secrets/requirements.md`
- `docs/product-manager/environment-specific-secrets/examples.md`

3. **Enable debug logging:**
```bash
sc provision -s my-stack --debug
```

### Reporting Issues

When reporting issues, include:

1. **server.yaml** (without secret values)
2. **secrets.yaml** (without secret values, just keys)
3. **client.yaml** configuration
4. **Validation output:**
```bash
sc validate --verbose > validation-output.txt
```
5. **Error message** with full stack trace

## Additional Resources

- **Main Requirements:** `requirements.md`
- **Technical Specification:** `technical-specification.md`
- **Configuration Examples:** `examples.md`
- **Schema Definition:** `docs/schemas/core/serverdescriptor.json`
