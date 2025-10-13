# GitHub Actions Versioning Strategy

## Overview

Simple Container supports flexible versioning for GitHub Actions references, allowing you to choose between:

1. **Latest version** (`@main`) - Always use the newest features
2. **CalVer tags** (`@v2025.10.4`) - Pin to specific Simple Container releases
3. **Custom actions** - Use your own forked actions

## Configuration Options

### 1. Using Latest Version (Default)

```yaml
# server.yaml
cicd:
  type: github-actions
  config:
    organization: "your-org"
    workflow-generation:
      sc-version: "latest" # Uses @main branch (default)
```

**Generated action reference:**
```yaml
uses: simple-container-com/api/.github/actions/deploy@main
```

### 2. Using CalVer Tags (Recommended for Production)

```yaml
# server.yaml
cicd:
  type: github-actions
  config:
    organization: "your-org"
    workflow-generation:
      sc-version: "v2025.10.4" # Pin to specific release
```

**Generated action reference:**
```yaml
uses: simple-container-com/api/.github/actions/deploy@v2025.10.4
```

### 3. Using Custom Actions

```yaml
# server.yaml
cicd:
  type: github-actions
  config:
    organization: "your-org"
    workflow-generation:
      custom-actions:
        deploy: "your-org/custom-deploy-action@v1.0.0"
        destroy: "your-org/custom-destroy-action@main"
        provision: "your-org/custom-provision-action@v2.1.0"
```

## Versioning Recommendations

### For Development/Testing
- Use `sc-version: "latest"` to get the newest features
- Actions will reference `@main` branch

### For Production
- Use `sc-version: "v2025.10.4"` (or current SC release)
- This pins workflows to tested, stable action versions
- Update `sc-version` when upgrading Simple Container

### For Enterprise/Custom Deployments
- Fork Simple Container actions to your organization
- Use `custom-actions` to reference your forks
- This gives you full control over action versions and modifications

## Action Types

Simple Container provides these GitHub Actions:

- **`deploy`** - Deploy stacks to environments
- **`destroy`** - Destroy stacks and clean up resources  
- **`provision`** - Provision parent infrastructure
- **`destroy-parent`** - Destroy parent infrastructure

## Examples

### Basic Production Setup
```yaml
cicd:
  type: github-actions
  config:
    organization: "mycompany"
    environments:
      production:
        type: production
        protection: true
        auto-deploy: false
    workflow-generation:
      enabled: true
      sc-version: "v2025.10.4"  # Pin to SC release
```

### Development Setup
```yaml
cicd:
  type: github-actions  
  config:
    organization: "mycompany"
    environments:
      staging:
        type: staging
        auto-deploy: true
    workflow-generation:
      enabled: true
      sc-version: "latest"  # Use latest features
```

### Custom Actions Setup
```yaml
cicd:
  type: github-actions
  config:
    organization: "mycompany" 
    workflow-generation:
      enabled: true
      custom-actions:
        deploy: "mycompany/deploy-with-slack@v1.0"
        destroy: "mycompany/destroy-with-approval@v1.0"
```

## Migration from v1 Tags

If you were previously using hardcoded `@v1` references:

1. **Update your server.yaml** to include `sc-version`
2. **Choose your versioning strategy** (latest, CalVer, or custom)
3. **Regenerate workflows** with `sc cicd generate --stack yourstack --force`
4. **Test the updated workflows** in a staging environment first

This approach eliminates the need to maintain `v1` tags and aligns with Simple Container's CalVer release strategy.
