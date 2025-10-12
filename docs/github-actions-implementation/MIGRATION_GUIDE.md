# Migration Guide: From Hardcoded Workflows to Simple Container Actions

This guide provides step-by-step instructions for migrating from the existing hardcoded Simple Container workflows to the new reusable GitHub Actions.

## Migration Overview

### What You're Migrating From

| Current Workflow | Lines | Complexity | Maintenance Burden |
|------------------|-------|------------|-------------------|
| `build-and-deploy-service.yaml` | 467 | Very High | High |
| `provision.yaml` | 150 | Medium | Medium |
| `destroy-service.yaml` | 361 | High | High |
| **Total** | **978** | **Complex** | **High** |

### What You're Migrating To

| New Action | Usage | Complexity | Maintenance |
|------------|-------|------------|-------------|
| `deploy-client-stack@v1` | ~10 lines | Very Low | None |
| `provision-parent-stack@v1` | ~5 lines | Very Low | None |
| `destroy-client-stack@v1` | ~10 lines | Very Low | None |
| `destroy-parent-stack@v1` | ~10 lines | Very Low | None |
| **Total** | **~35 lines** | **Simple** | **None** |

## Migration Strategy

### Phase 1: Parallel Implementation (Recommended)
1. Create new workflows using actions alongside existing workflows
2. Test new workflows in development/staging environments
3. Gradually migrate production workloads after validation
4. Deprecate old workflows once fully validated

### Phase 2: Direct Replacement (Advanced)
1. Replace existing workflows directly with actions
2. Suitable for teams with comprehensive testing capabilities
3. Requires thorough validation of all input/output mappings

## Pre-Migration Checklist

### ✅ Prerequisites

- [ ] **Access to GitHub repository settings** - Required for secrets and environment configuration
- [ ] **Understanding of current workflow triggers** - Document when and how workflows are currently triggered
- [ ] **Inventory of secrets and configurations** - List all secrets currently used in workflows
- [ ] **Backup of existing workflows** - Create backup branch with current workflow files
- [ ] **Test environment setup** - Prepare development/staging environment for testing

### ✅ Dependencies Audit

- [ ] **Simple Container CLI version** - Note current version used in workflows
- [ ] **Runner requirements** - Document any special runner requirements
- [ ] **External integrations** - List Slack, Discord, or other webhook integrations
- [ ] **Custom scripts** - Identify any custom scripts referenced by workflows
- [ ] **Environment variables** - Document all environment variables used

## Step-by-Step Migration

### Step 1: Deploy Client Stack Migration

#### Current Workflow Analysis

**Existing file**: `.github/workflows/build-and-deploy-service.yaml`

**Common usage patterns**:
```yaml
name: Deploy Service
on:
  push:
    branches: [main, develop]
  workflow_dispatch:

jobs:
  deploy:
    uses: myorg/devops/.github/workflows/build-and-deploy-service.yaml@main
    secrets:
      sc-config: ${{ secrets.SC_CONFIG }}
      stack-yaml-config: ${{ secrets.STACK_YAML_CONFIG }}
    with:
      stack-name: "my-service"
      environment: "staging"
      sc-version: "2025.8.5"
```

#### Migrated Workflow

**New file**: `.github/workflows/deploy.yaml`

```yaml
name: Deploy Service
on:
  push:
    branches: [main, develop]
  workflow_dispatch:
    inputs:
      environment:
        description: 'Target environment'
        required: true
        default: 'staging'
        type: choice
        options:
          - staging
          - production

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Deploy Application Stack
        uses: simple-container/actions/deploy-client-stack@v1
        with:
          stack-name: "my-service"
          environment: ${{ github.event.inputs.environment || 'staging' }}
          sc-config: ${{ secrets.SC_CONFIG }}
          stack-yaml-config: ${{ secrets.STACK_YAML_CONFIG }}
          sc-version: "2025.8.5"
```

#### Migration Steps

1. **Create new workflow file**:
   ```bash
   mkdir -p .github/workflows
   touch .github/workflows/deploy.yaml
   ```

2. **Map inputs and secrets**:
   - `sc-config` → Direct mapping
   - `stack-yaml-config` → Direct mapping  
   - `stack-name` → Direct mapping
   - `environment` → Direct mapping
   - `sc-version` → Direct mapping

3. **Update triggers** (if needed):
   - Keep existing triggers
   - Add `workflow_dispatch` for manual deployment control

4. **Test the new workflow**:
   ```bash
   # Push to development branch first
   git checkout -b migrate-deploy-workflow
   git add .github/workflows/deploy.yaml
   git commit -m "Add new deploy action workflow"
   git push origin migrate-deploy-workflow
   ```

#### Advanced Migration Features

**PR Preview Support**:
```yaml
  deploy-pr-preview:
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    steps:
      - uses: simple-container/actions/deploy-client-stack@v1
        with:
          stack-name: "my-service"
          environment: "staging"
          sc-config: ${{ secrets.SC_CONFIG }}
          pr-preview: true
          preview-domain-base: "dev.mycompany.com"
```

**Production Deployment with Approval**:
```yaml
  deploy-production:
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    environment: 
      name: production
      required-reviewers: ["team-lead", "devops-team"]
    steps:
      - uses: simple-container/actions/deploy-client-stack@v1
        with:
          stack-name: "my-service"
          environment: "production"
          sc-config: ${{ secrets.SC_CONFIG }}
          validation-command: |
            sleep 30
            curl -f https://api.mycompany.com/health
```

### Step 2: Provision Parent Stack Migration

#### Current Workflow Analysis

**Existing file**: `.github/workflows/provision.yaml`

**Common usage pattern**:
```yaml
name: Provision Infrastructure
on:
  push:
    branches: [main]

jobs:
  provision:
    runs-on: ubuntu-latest
    steps:
      # ... 150 lines of complex provisioning logic
```

#### Migrated Workflow

**New file**: `.github/workflows/provision.yaml`

```yaml
name: Provision Infrastructure
on:
  push:
    branches: [main]
    paths: ['infrastructure/**', '.sc/stacks/*/server.yaml']
  workflow_dispatch:

jobs:
  provision:
    runs-on: ubuntu-latest
    steps:
      - name: Provision Parent Stack
        uses: simple-container/actions/provision-parent-stack@v1
        with:
          sc-config: ${{ secrets.SC_CONFIG }}
```

#### Migration Benefits

**Before (Complex)**:
- 3 jobs with complex dependencies
- Manual CLI installation and setup
- Complex secret handling
- Manual notification logic

**After (Simple)**:
- Single step with comprehensive functionality
- Automatic CLI installation and setup
- Built-in secret handling
- Professional notification system

### Step 3: Destroy Client Stack Migration

#### Current Workflow Analysis

**Existing file**: `.github/workflows/destroy-service.yaml`

#### Migrated Workflow

**New file**: `.github/workflows/destroy.yaml`

```yaml
name: Destroy Service Stack
on:
  workflow_dispatch:
    inputs:
      stack_name:
        description: 'Stack name to destroy'
        required: true
      environment:
        description: 'Environment to destroy from'
        required: true
        type: choice
        options:
          - development
          - staging
      confirmation:
        description: 'Type "DESTROY" to confirm'
        required: true

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - name: Validate confirmation
        if: ${{ github.event.inputs.confirmation != 'DESTROY' }}
        run: |
          echo "Invalid confirmation. Must type 'DESTROY'"
          exit 1

  destroy:
    needs: validate
    runs-on: ubuntu-latest
    steps:
      - name: Destroy Application Stack
        uses: simple-container/actions/destroy-client-stack@v1
        with:
          stack-name: ${{ github.event.inputs.stack_name }}
          environment: ${{ github.event.inputs.environment }}
          sc-config: ${{ secrets.SC_CONFIG }}
          auto-confirm: true
```

#### PR Cleanup Automation

**New file**: `.github/workflows/pr-cleanup.yaml`

```yaml
name: PR Cleanup
on:
  pull_request:
    types: [closed]

jobs:
  cleanup:
    runs-on: ubuntu-latest
    if: github.event.pull_request.head.repo.full_name == github.repository
    steps:
      - uses: simple-container/actions/destroy-client-stack@v1
        with:
          stack-name: "my-service"
          environment: "staging"
          sc-config: ${{ secrets.SC_CONFIG }}
          pr-preview: true
          auto-confirm: true
          notify-on-start: false
```

### Step 4: Parent Stack Destruction Setup

This is a **new capability** that didn't exist before. Add infrastructure lifecycle management:

**New file**: `.github/workflows/destroy-infrastructure.yaml`

```yaml
name: Destroy Infrastructure
on:
  workflow_dispatch:
    inputs:
      environment:
        description: 'Environment to destroy'
        required: true
        type: choice
        options:
          - development
          - testing
      confirmation:
        description: 'Type "DESTROY-INFRASTRUCTURE" to confirm'
        required: true

jobs:
  destroy-infrastructure:
    runs-on: ubuntu-latest
    environment: infrastructure-destroy
    steps:
      - name: Destroy Parent Stack
        uses: simple-container/actions/destroy-parent-stack@v1
        with:
          sc-config: ${{ secrets.SC_CONFIG }}
          confirmation: ${{ github.event.inputs.confirmation }}
          target-environment: ${{ github.event.inputs.environment }}
```

## Input/Output Mapping Reference

### Deploy Action Mapping

| Old Workflow Input | New Action Input | Notes |
|-------------------|------------------|-------|
| `stack-name` | `stack-name` | Direct mapping |
| `environment` | `environment` | Direct mapping |
| `sc-config` (secret) | `sc-config` | Direct mapping |
| `sc-version` | `sc-version` | Direct mapping |
| `sc-deploy-flags` | `sc-deploy-flags` | Direct mapping |
| `runner` | `runner` | Direct mapping |
| `pr-preview` | `pr-preview` | Direct mapping |
| `stack-yaml-config` (secret) | `stack-yaml-config` | Direct mapping |
| `validation-command` | `validation-command` | Direct mapping |

### Provision Action Mapping

| Old Workflow Input | New Action Input | Notes |
|-------------------|------------------|-------|
| `SC_CONFIG` (env) | `sc-config` | Environment to input |
| `SIMPLE_CONTAINER_VERSION` (env) | `sc-version` | Environment to input |

### Destroy Action Mapping

| Old Workflow Input | New Action Input | Notes |
|-------------------|------------------|-------|
| `stack-name` | `stack-name` | Direct mapping |
| `environment` | `environment` | Direct mapping |
| `sc-config` (secret) | `sc-config` | Direct mapping |
| `sc-destroy-flags` | `sc-destroy-flags` | Direct mapping |
| `pr-preview` | `pr-preview` | Direct mapping |

## Common Migration Patterns

### Pattern 1: Environment-Based Deployment

**Before**:
```yaml
strategy:
  matrix:
    environment: [staging, production]
jobs:
  deploy:
    strategy:
      matrix: ${{ strategy }}
    # ... complex deployment logic
```

**After**:
```yaml
strategy:
  matrix:
    environment: [staging, production]
jobs:
  deploy:
    runs-on: ubuntu-latest
    strategy:
      matrix: ${{ strategy }}
    steps:
      - uses: simple-container/actions/deploy-client-stack@v1
        with:
          stack-name: "my-service"
          environment: ${{ matrix.environment }}
          sc-config: ${{ secrets.SC_CONFIG }}
```

### Pattern 2: Conditional Deployment

**Before**:
```yaml
jobs:
  deploy-staging:
    if: github.ref != 'refs/heads/main'
    # ... staging deployment logic
    
  deploy-production:
    if: github.ref == 'refs/heads/main'
    # ... production deployment logic
```

**After**:
```yaml
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Deploy to Staging
        if: github.ref != 'refs/heads/main'
        uses: simple-container/actions/deploy-client-stack@v1
        with:
          stack-name: "my-service"
          environment: "staging"
          sc-config: ${{ secrets.SC_CONFIG }}
          
      - name: Deploy to Production  
        if: github.ref == 'refs/heads/main'
        uses: simple-container/actions/deploy-client-stack@v1
        with:
          stack-name: "my-service"
          environment: "production"
          sc-config: ${{ secrets.SC_CONFIG }}
```

## Testing Your Migration

### Development Environment Testing

1. **Create test branch**:
   ```bash
   git checkout -b test-new-actions
   ```

2. **Test deployment action**:
   ```bash
   # Trigger workflow manually
   gh workflow run deploy.yaml -f environment=development
   ```

3. **Verify outputs**:
   - Check deployment completed successfully
   - Verify all notifications were sent
   - Confirm stack is running correctly

### Staging Environment Validation

1. **Full workflow test**:
   ```bash
   # Test complete workflow
   git push origin test-new-actions
   ```

2. **Compare results**:
   - Same deployment outcome as old workflow
   - Same notification format and recipients
   - Same timing and resource usage

### Production Migration

1. **Create migration PR**:
   ```yaml
   name: Migrate to Simple Container Actions
   description: |
     This PR migrates our deployment workflows from hardcoded workflows 
     to reusable Simple Container GitHub Actions.
     
     **Changes:**
     - Replace build-and-deploy-service.yaml with deploy.yaml
     - Replace provision.yaml with simplified provision.yaml
     - Add destroy.yaml for stack cleanup
     - Add infrastructure destruction capability
     
     **Benefits:**
     - 95% reduction in workflow complexity
     - Centralized maintenance and updates
     - Enhanced safety and error handling
     - Professional notification system
   ```

2. **Gradual rollout**:
   ```bash
   # Start with non-production environments
   # Monitor for 1-2 weeks
   # Then migrate production
   ```

## Rollback Procedures

### Emergency Rollback

If issues are discovered after migration:

1. **Immediate rollback**:
   ```bash
   # Revert to previous workflow files
   git checkout main -- .github/workflows/
   git commit -m "Emergency rollback to old workflows"
   git push origin main
   ```

2. **Keep new workflows for testing**:
   ```bash
   # Rename new workflows for future testing
   mv .github/workflows/deploy.yaml .github/workflows/deploy-new.yaml.disabled
   ```

### Planned Rollback

For planned rollback during testing:

1. **Document issues found**:
   ```markdown
   ## Migration Issues Found
   - [ ] Issue 1: Description and impact
   - [ ] Issue 2: Description and impact
   ```

2. **Schedule fixes**:
   ```yaml
   # Add workflow_dispatch trigger for testing
   on:
     workflow_dispatch: # Enable manual testing
     # push: # Disable automatic triggers
   ```

## Troubleshooting

### Common Issues and Solutions

#### Issue 1: Secret Not Found

**Error**: `Secret SC_CONFIG not found`

**Solution**:
```yaml
# Ensure secrets are properly configured
with:
  sc-config: ${{ secrets.SC_CONFIG }} # Not ${{ env.SC_CONFIG }}
```

#### Issue 2: Wrong Environment

**Error**: `Stack not found in environment`

**Solution**:
```yaml
# Verify environment names match exactly
environment: "staging" # Not "Staging" or "STAGING"
```

#### Issue 3: Permission Denied

**Error**: `Permission denied for production deployment`

**Solution**:
```yaml
# Add environment protection rules
environment: 
  name: production
  required-reviewers: ["team-lead"]
```

#### Issue 4: Action Not Found

**Error**: `Action simple-container/actions/deploy-client-stack@v1 not found`

**Solution**:
```yaml
# Use correct action reference when available
uses: simple-container/actions/deploy-client-stack@v1
# Or use local actions during development
uses: ./.github/actions/deploy-client-stack
```

### Debug Mode

Enable debug mode for troubleshooting:

```yaml
steps:
  - uses: simple-container/actions/deploy-client-stack@v1
    with:
      stack-name: "my-service"
      environment: "staging"
      sc-config: ${{ secrets.SC_CONFIG }}
      sc-deploy-flags: "--verbose --debug" # Enable debug mode
```

## Migration Checklist

### Pre-Migration
- [ ] Backup existing workflows
- [ ] Document current workflow behavior
- [ ] Identify all secrets and configurations
- [ ] Set up test environments
- [ ] Review action documentation

### During Migration
- [ ] Create new workflow files
- [ ] Map all inputs and outputs
- [ ] Test in development environment
- [ ] Validate in staging environment
- [ ] Update documentation
- [ ] Train team members

### Post-Migration
- [ ] Monitor production workflows
- [ ] Verify notifications work correctly
- [ ] Confirm all integrations functional
- [ ] Clean up old workflow files
- [ ] Update runbooks and documentation

## Support and Resources

### Getting Help

1. **Documentation**: Review action-specific documentation
2. **Issues**: Create issues in the actions repository
3. **Team Support**: Consult with DevOps team for complex migrations
4. **Testing**: Use development environments extensively

### Additional Resources

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Simple Container CLI Documentation](https://simple-container.com/docs)
- [Workflow Syntax Reference](https://docs.github.com/en/actions/reference/workflow-syntax-for-github-actions)

This migration guide transforms your complex, hardcoded workflows into simple, maintainable, and reliable GitHub Actions while preserving all existing functionality and adding new capabilities.
