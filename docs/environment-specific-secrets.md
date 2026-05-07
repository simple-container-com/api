# Environment-Specific Secrets in Parent Stacks

## Overview

This feature implements environment-specific secrets management for the Simple Container API, allowing different secret values for different deployment environments (production, staging, development) while maintaining full backward compatibility with existing v1.0 configurations.

## Schema Version 2.0

The secrets schema has been upgraded to version 2.0 to support environment-specific values:

```yaml
schemaVersion: "2.0"

# Shared secrets (backward compatible with v1.0)
values:
  SHARED_API_KEY: "shared-value"

# Environment-specific secrets
environments:
  production:
    values:
      API_KEY: "production-api-key"
      DATABASE_URL: "postgres://prod-db.example.com:5432/mydb"

  staging:
    values:
      API_KEY: "staging-api-key"
      DATABASE_URL: "postgres://staging-db.example.com:5432/mydb"
```

## Features

### 1. Environment-Aware Secret Resolution

Secrets are automatically resolved based on the deployment environment:

- **Implicit resolution**: `${secret:API_KEY}` uses the current environment
- **Explicit override**: `${secret:API_KEY:production}` forces a specific environment

### 2. Fallback Mechanism

When looking up a secret:
1. First, check environment-specific values for the current environment
2. If not found, fall back to shared values
3. If still not found, return an error

This ensures backward compatibility with existing configurations.

### 3. Parent Stack Inheritance

Child stacks inherit secrets from parent stacks with environment context:

```yaml
# Parent stack (.sc/stacks/parent/server.yaml)
schemaVersion: "1.0"
secrets:
  type: repository

# Child stack (.sc/stacks/child/client.yaml)
stacks:
  production:
    parentStack: "parent"
    # Inherits parent's production secrets automatically
```

The child stack's environment determines which secrets are inherited from the parent.

### 4. CLI Commands

#### List Secrets

```bash
# List all secrets
sc secrets list

# List secrets for a specific environment
sc secrets list --environment production
```

#### Add Environment-Specific Secret

```bash
# Add encrypted file (original behavior)
sc secrets add path/to/secret/file

# Add environment-specific secret reference
sc secrets add API_KEY --environment production
```

#### Delete Environment-Specific Secret

```bash
# Delete from encrypted files (original behavior)
sc secrets delete path/to/secret/file

# Delete environment-specific secret
sc secrets delete API_KEY --environment production
```

## Usage Examples

### Example 1: Basic Environment-Specific Secrets

```yaml
# .sc/secrets.yaml
schemaVersion: "2.0"
values:
  SHARED_CONFIG: "config-for-all-envs"

environments:
  production:
    values:
      API_KEY: "prod-key"
      DB_PASSWORD: "prod-password"

  staging:
    values:
      API_KEY: "staging-key"
      DB_PASSWORD: "staging-password"
```

Usage in stack configuration:

```yaml
# .sc/stacks/myapp/server.yaml
resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:API_KEY}"  # Automatically resolves based on deployment environment
```

### Example 2: Explicit Environment Override

```yaml
resources:
  database:
    type: postgres
    config:
      # Force production credentials even in staging
      credentials: "${secret:DB_PASSWORD:production}"
```

### Example 3: Parent Stack with Multiple Environments

```yaml
# Parent: .sc/stacks/base/server.yaml
schemaVersion: "1.0"
secrets:
  type: repository

# .sc/secrets.yaml
schemaVersion: "2.0"
environments:
  production:
    values:
      CLOUDFLARE_API_TOKEN: "prod-token"
  staging:
    values:
      CLOUDFLARE_API_TOKEN: "staging-token"

# Child: .sc/stacks/website/client.yaml
stacks:
  production:
    parentStack: "base"
    # Inherits CLOUDFLARE_API_TOKEN from base stack's production environment

  staging:
    parentStack: "base"
    # Inherits CLOUDFLARE_API_TOKEN from base stack's staging environment
```

## Migration Guide

### From v1.0 to v2.0

**Step 1**: Update schema version in `.sc/secrets.yaml`:

```yaml
schemaVersion: "2.0"
```

**Step 2**: Keep existing shared secrets under `values`:

```yaml
values:
  API_KEY: "existing-api-key"
```

**Step 3**: Add environment-specific sections as needed:

```yaml
environments:
  production:
    values:
      API_KEY: "production-api-key"
  staging:
    values:
      API_KEY: "staging-api-key"
```

**Step 4**: Test your deployments to ensure secrets resolve correctly.

### Backward Compatibility

- All existing v1.0 configurations continue to work without modification
- Shared secrets (`values`) work as before
- Existing deployments are not affected
- No breaking changes to the API

## Implementation Details

### Secret Resolution Algorithm

```
1. Parse placeholder: ${secret:name[:environment]}
2. If environment explicitly provided, use it
3. Otherwise, use stack's environment context
4. Look up secret in environment-specific values
5. If not found, fall back to shared values
6. If still not found, return error
```

### Environment Context Sources

Environment context is determined in the following order:

1. Explicit environment in placeholder: `${secret:name:env}`
2. Stack's `environment` field in server.yaml
3. Deployment environment from CLI flag: `--env production`
4. Environment variable: `SC_ENVIRONMENT`

### Files Modified

- `pkg/api/secrets.go`: Added environment support to schema
- `pkg/api/server.go`: Added `Environment` field to `ServerDescriptor`
- `pkg/api/copy.go`: Added copy methods for new types
- `pkg/api/models.go`: Updated `ReconcileForDeploy` to pass environment context
- `pkg/provisioner/placeholders/placeholders.go`: Updated `tplSecrets` for environment-aware resolution
- `pkg/cmd/cmd_secrets/cmd_list.go`: Enhanced to show environment-specific secrets
- `pkg/cmd/cmd_secrets/cmd_add.go`: Added `--environment` flag
- `pkg/cmd/cmd_secrets/cmd_delete.go`: Added `--environment` flag

## Testing

Unit tests have been added to verify:

- Environment-specific secret lookup
- Fallback to shared values
- Parent stack inheritance with environment context
- Backward compatibility with v1.0 schema
- Copy/deep copy operations

Run tests:

```bash
go test ./pkg/api -run TestSecretsDescriptor
go test ./pkg/provisioner/placeholders -run TestTemplateSecrets
```

## Best Practices

1. **Use shared secrets for common values**: Store secrets that are the same across all environments in the `values` section

2. **Override in environments**: Only specify environment-specific values in `environments` sections

3. **Explicit overrides for cross-env access**: Use `${secret:name:env}` when you need to access a different environment's secret

4. **Test in non-production first**: Always test environment-specific secrets in staging before deploying to production

5. **Document your secrets structure**: Use comments to explain which secrets are shared vs. environment-specific

6. **Use environment variables for automation**: Set `SC_ENVIRONMENT` in your CI/CD pipeline

## Troubleshooting

### Secret not found error

**Problem**: `secret "API_KEY" not found in stack "mystack" (environment: "production")`

**Solutions**:
1. Check if the secret exists in `environments.production.values`
2. Check if the secret exists in shared `values` (fallback)
3. Verify the correct environment is being deployed: `sc deploy --env production`

### Wrong secret value

**Problem**: Secret resolves to unexpected value

**Solutions**:
1. Check if there's an environment-specific value overriding the shared value
2. Verify the environment context: `sc secrets list --environment production`
3. Check for explicit environment override in the placeholder: `${secret:name:env}`

### Parent stack secrets not inherited

**Problem**: Child stack doesn't get parent's secrets

**Solutions**:
1. Verify parent stack has the secret in the child's environment
2. Check `ReconcileForDeploy` is setting environment context
3. Ensure `parentStack` is correctly configured in client.yaml

## Future Enhancements

Potential future improvements:

1. **Secret validation**: Validate secret presence before deployment
2. **Dry-run mode**: Preview which secrets will be used without deploying
3. **Secret templates**: Support for templated secret values
4. **Cross-stack references**: Reference secrets from other stacks
5. **Secret rotation**: Built-in support for rotating secrets
6. **Secret versioning**: Track history of secret changes

## References

- Original issue: #60 - Feature Request: Environment-Specific Secrets in Parent Stacks
- Schema design: `.sc/secrets.v2.example.yaml`
- Unit tests: `pkg/api/secrets_test.go`, `pkg/provisioner/placeholders/environment_test.go`
