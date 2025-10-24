# GitHub Actions Versioning Strategy

## Overview

Simple Container's GitHub Actions use a CalVer versioning strategy that aligns with SC's release cycle, eliminating the need for hardcoded `@v1` tags and SC_VERSION environment variables.

## Key Architectural Decisions

### 1. Pre-built SC Binaries
- **Each GitHub Action image includes a pre-built Simple Container binary**
- **No `SC_VERSION` environment variables needed**  
- **No CLI installation steps required**
- **Consistent SC version across all actions in a workflow**

### 2. CalVer Action References
- **Latest:** `@main` branch (development/testing)
- **Stable:** `@v2025.10.4` tags (production)
- **Custom:** User-defined action references

### 3. Simplified Configuration
- Only `SC_CONFIG` secret required
- No individual webhook secrets needed
- Pre-built binaries reduce complexity

## Configuration Options

### 1. Using Latest Version (Development)

```yaml
# server.yaml
cicd:
  type: github-actions
  config:
    organization: "your-org"
    workflow-generation:
      sc-version: "latest" # Uses @main branch
```

**Generated workflow:**
```yaml
steps:
  - name: Deploy stack
    uses: simple-container-com/api/.github/actions/deploy@main
    with:
      stack-name: "mystack"
      sc-config: ${{ secrets.SC_CONFIG }}
```

### 2. Using CalVer Tags (Production)

```yaml
# server.yaml
cicd:
  type: github-actions
  config:
    organization: "your-org"
    workflow-generation:
      sc-version: "v2025.10.4" # Pin to specific SC release
```

**Generated workflow:**
```yaml
steps:
  - name: Deploy stack
    uses: simple-container-com/api/.github/actions/deploy@v2025.10.4
    with:
      stack-name: "mystack"
      sc-config: ${{ secrets.SC_CONFIG }}
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
        deploy: "myorg/custom-deploy@v1.2.3"
        destroy: "myorg/custom-destroy@v1.2.3"
```

## Available Actions

### Core Deployment Actions

| Action | Purpose | Pre-built SC Version | Usage |
|--------|---------|---------------------|-------|
| **`deploy`** | Deploy client stacks to environments | v2025.10.x+ | Production deployments |
| **`destroy`** | Clean up stack resources | v2025.10.x+ | Environment cleanup |
| **`provision`** | Create parent infrastructure | v2025.10.x+ | Infrastructure setup |

### Action Inputs

All actions use consistent inputs:

```yaml
with:
  stack-name: "${{ env.STACK_NAME }}"
  environment: "production"
  sc-config: ${{ secrets.SC_CONFIG }}
  # Optional flags
  sc-deploy-flags: "--verbose"
```

**Key Points:**
- **No `sc-version` input needed** - SC binary is pre-built in action image
- **No `SC_VERSION` environment variable** - version is embedded in action
- **Only `SC_CONFIG` secret required** - unified secrets management

## Versioning Strategy by Environment

### Development/Testing
```yaml
workflow-generation:
  sc-version: "latest"
```
- Uses `@main` branch
- Latest features and improvements
- May include breaking changes

### Staging
```yaml
workflow-generation:
  sc-version: "v2025.10.4"
```
- Uses stable CalVer tags
- Tested and verified releases
- Recommended for pre-production testing

### Production
```yaml
workflow-generation:
  sc-version: "v2025.10.4"
```
- Uses stable CalVer tags only
- Pin to tested SC releases
- Update `sc-version` when upgrading SC

## Migration Guide

### From Hardcoded @v1 Tags

**Before:**
```yaml
uses: simple-container-com/api/.github/actions/deploy@v1
env:
  SC_VERSION: "2025.8.5"
```

**After:**
```yaml
# In server.yaml
workflow-generation:
  sc-version: "v2025.10.4"

# Generated workflow (no SC_VERSION needed)
uses: simple-container-com/api/.github/actions/deploy@v2025.10.4
```

### From SC_VERSION Environment Variables

**Before:**
```yaml
env:
  STACK_NAME: "mystack"
  SC_VERSION: "2025.8.5"
steps:
  - name: Install SC
    run: curl -s https://dist.simple-container.com/sc.sh | bash
```

**After:**
```yaml
env:
  STACK_NAME: "mystack"
  # No SC_VERSION needed - pre-built in action
steps:
  - name: Deploy
    uses: simple-container-com/api/.github/actions/deploy@v2025.10.4
```

## Benefits

### 1. Simplified Workflow Files
- **No CLI installation steps**
- **No version management logic**
- **Shorter, cleaner workflows**

### 2. Consistent Versioning
- **Aligns with SC CalVer releases**
- **No artificial @v1 tags needed**
- **Clear upgrade path**

### 3. Better Performance
- **Pre-built binaries start faster**
- **No download/install overhead**
- **Consistent execution environment**

### 4. Easier Maintenance
- **Single action version per workflow**
- **No version conflicts**
- **Predictable behavior**

## Best Practices

### 1. Environment-Specific Versioning
```yaml
# Development
environments:
  dev:
    workflow-generation:
      sc-version: "latest"

# Production  
environments:
  prod:
    workflow-generation:
      sc-version: "v2025.10.4"
```

### 2. Upgrade Strategy
1. Test new SC version with `sc-version: "latest"`
2. When stable, pin to CalVer: `sc-version: "v2025.11.1"`
3. Regenerate workflows: `sc cicd generate --force`
4. Test in staging before production

### 3. Custom Action Development
```yaml
workflow-generation:
  custom-actions:
    deploy: "myorg/enhanced-deploy@v2025.10.4"
    # Use same CalVer versioning for consistency
```

## Troubleshooting

### Action Not Found
**Error:** `simple-container-com/api/.github/actions/deploy@v2025.10.4 not found`

**Solution:** Use a valid CalVer tag from [SC releases](https://github.com/simple-container-com/api/releases) or `@main`.

### Workflow Outdated
**Problem:** Using old `@v1` references

**Solution:** 
1. Update `server.yaml` with `sc-version`
2. Regenerate: `sc cicd generate --force`
3. Commit updated workflow files

This versioning strategy eliminates complexity while providing flexible, production-ready GitHub Actions that align with Simple Container's release cycle.
