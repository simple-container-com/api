# Simple Container GitHub Actions - Usage Examples

This document provides real-world usage examples for the Simple Container GitHub Actions, showing how customers would implement them in their repositories.

## Action Repository Structure

The actions are published from the main Simple Container repository:
- **Repository**: `https://github.com/simple-container-com/api`  
- **Actions Path**: `.github/actions/` within the repository
- **Usage**: `simple-container-com/api/.github/actions/<action-name>@v1`

## Available Actions

| Action | Purpose | Usage |
|--------|---------|--------|
| **deploy-client-stack** | Deploy application stacks | `simple-container-com/api/.github/actions/deploy-client-stack@v1` |
| **provision-parent-stack** | Provision infrastructure | `simple-container-com/api/.github/actions/provision-parent-stack@v1` |
| **destroy-client-stack** | Destroy application stacks | `simple-container-com/api/.github/actions/destroy-client-stack@v1` |
| **destroy-parent-stack** | Destroy infrastructure | `simple-container-com/api/.github/actions/destroy-parent-stack@v1` |

## Shared Actions

| Action | Purpose | Usage |
|--------|---------|--------|
| **setup-sc** | Install and configure SC CLI | `simple-container-com/api/.github/actions/setup-sc@v1` |
| **notify** | Send notifications | `simple-container-com/api/.github/actions/notify@v1` |

## Complete Implementation Examples

### 1. Basic Application Deployment

**File**: `.github/workflows/deploy.yml`

```yaml
name: Deploy Application
on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

jobs:
  deploy-staging:
    if: github.ref_name != 'main'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Deploy to Staging
        uses: simple-container-com/api/.github/actions/deploy-client-stack@v1
        with:
          stack-name: "my-app"
          environment: "staging"
          sc-config: ${{ secrets.SC_CONFIG }}
          
      - name: Notify Team
        if: always()
        uses: simple-container-com/api/.github/actions/notify@v1
        with:
          status: ${{ job.status }}
          operation: "deploy"
          stack-name: "my-app"
          environment: "staging"
          slack-webhook-url: ${{ secrets.SLACK_WEBHOOK }}

  deploy-production:
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    environment: production
    steps:
      - uses: actions/checkout@v4
      
      - name: Deploy to Production
        uses: simple-container-com/api/.github/actions/deploy-client-stack@v1
        with:
          stack-name: "my-app"
          environment: "production"
          sc-config: ${{ secrets.SC_CONFIG }}
          validation-command: |
            sleep 30
            curl -f https://api.mycompany.com/health
```

### 2. PR Preview Deployments

**File**: `.github/workflows/pr-preview.yml`

```yaml
name: PR Preview
on:
  pull_request:
    types: [opened, synchronize, reopened]

jobs:
  deploy-preview:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        
      - name: Deploy PR Preview
        uses: simple-container-com/api/.github/actions/deploy-client-stack@v1
        with:
          stack-name: "webapp"
          environment: "staging"
          sc-config: ${{ secrets.SC_CONFIG }}
          pr-preview: true
          preview-domain-base: "preview.mycompany.com"
        
      - name: Comment PR with Preview Link
        uses: actions/github-script@v7
        with:
          script: |
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: '🚀 Preview deployed: https://pr${{ github.event.pull_request.number }}-preview.mycompany.com'
            })
```

### 3. PR Preview Cleanup

**File**: `.github/workflows/pr-cleanup.yml`

```yaml
name: PR Cleanup
on:
  pull_request:
    types: [closed]

jobs:
  cleanup-preview:
    runs-on: ubuntu-latest
    steps:
      - name: Cleanup PR Preview
        uses: simple-container-com/api/.github/actions/destroy-client-stack@v1
        with:
          stack-name: "webapp"
          environment: "staging"
          sc-config: ${{ secrets.SC_CONFIG }}
          pr-preview: true
          preview-domain-base: "preview.mycompany.com"
          auto-confirm: true
          skip-backup: true
```

### 4. Infrastructure Management

**File**: `.github/workflows/infrastructure.yml`

```yaml
name: Infrastructure Management
on:
  push:
    branches: [main]
    paths: 
      - 'infrastructure/**'
      - '.sc/stacks/*/server.yaml'
  workflow_dispatch:
    inputs:
      action:
        description: 'Action to perform'
        required: true
        type: choice
        options:
          - provision
          - destroy-dev
          - destroy-staging

jobs:
  provision:
    if: github.event_name == 'push' || github.event.inputs.action == 'provision'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        
      - name: Provision Infrastructure
        uses: simple-container-com/api/.github/actions/provision-parent-stack@v1
        with:
          sc-config: ${{ secrets.SC_CONFIG }}

  destroy-development:
    if: github.event.inputs.action == 'destroy-dev'
    runs-on: ubuntu-latest
    environment: destroy-infrastructure
    steps:
      - uses: actions/checkout@v4
        
      - name: Destroy Development Infrastructure
        uses: simple-container-com/api/.github/actions/destroy-parent-stack@v1
        with:
          sc-config: ${{ secrets.SC_CONFIG }}
          confirmation: "DESTROY-INFRASTRUCTURE"
          target-environment: "development"
          destroy-scope: "environment-only"

  destroy-staging:
    if: github.event.inputs.action == 'destroy-staging'  
    runs-on: ubuntu-latest
    environment: destroy-infrastructure
    steps:
      - uses: actions/checkout@v4
        
      - name: Destroy Staging Infrastructure
        uses: simple-container-com/api/.github/actions/destroy-parent-stack@v1
        with:
          sc-config: ${{ secrets.SC_CONFIG }}
          confirmation: "DESTROY-INFRASTRUCTURE"
          target-environment: "staging"
          destroy-scope: "environment-only"
          backup-before-destroy: true
```

### 5. Multi-Environment Deployment Matrix

**File**: `.github/workflows/multi-env-deploy.yml`

```yaml
name: Multi-Environment Deploy
on:
  workflow_dispatch:
    inputs:
      environments:
        description: 'Environments to deploy to'
        required: true
        default: '["staging"]'
        type: string
      stack-name:
        description: 'Stack name'
        required: true
        default: 'my-service'

jobs:
  deploy:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        environment: ${{ fromJSON(github.event.inputs.environments) }}
      fail-fast: false
    steps:
      - uses: actions/checkout@v4
        
      - name: Deploy to ${{ matrix.environment }}
        uses: simple-container-com/api/.github/actions/deploy-client-stack@v1
        with:
          stack-name: ${{ github.event.inputs.stack-name }}
          environment: ${{ matrix.environment }}
          sc-config: ${{ secrets.SC_CONFIG }}
```

### 6. Scheduled Infrastructure Cleanup

**File**: `.github/workflows/scheduled-cleanup.yml`

```yaml
name: Scheduled Cleanup
on:
  schedule:
    # Every Sunday at 2 AM UTC
    - cron: '0 2 * * 0'
  workflow_dispatch:

jobs:
  cleanup-old-stacks:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        stack: [temp-feature-1, temp-feature-2, old-test-stack]
      fail-fast: false
    steps:
      - name: Cleanup Old Stack
        uses: simple-container-com/api/.github/actions/destroy-client-stack@v1
        continue-on-error: true
        with:
          stack-name: ${{ matrix.stack }}
          environment: "development"
          sc-config: ${{ secrets.SC_CONFIG }}
          auto-confirm: true
          skip-backup: true
```

### 7. Advanced Deployment with Notifications

**File**: `.github/workflows/advanced-deploy.yml`

```yaml
name: Advanced Deployment
on:
  push:
    tags: [v*]

jobs:
  deploy-with-notifications:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Notify Start
        uses: simple-container-com/api/.github/actions/notify@v1
        with:
          status: "started"
          operation: "deploy"
          stack-name: "production-app"
          environment: "production"
          slack-webhook-url: ${{ secrets.SLACK_WEBHOOK }}
          
      - name: Deploy Production
        id: deploy
        uses: simple-container-com/api/.github/actions/deploy-client-stack@v1
        with:
          stack-name: "production-app"
          environment: "production"
          sc-config: ${{ secrets.SC_CONFIG }}
          sc-version: "2025.8.5"
          validation-command: |
            # Wait for deployment
            sleep 60
            
            # Run comprehensive health checks
            ./scripts/health-check.sh
            
            # Run integration tests
            npm run test:integration
            
      - name: Notify Success
        if: success()
        uses: simple-container-com/api/.github/actions/notify@v1
        with:
          status: "success"
          operation: "deploy"
          stack-name: "production-app"
          environment: "production"
          version: ${{ steps.deploy.outputs.version }}
          duration: ${{ steps.deploy.outputs.duration }}
          slack-webhook-url: ${{ secrets.SLACK_WEBHOOK }}
          custom-message: "All health checks passed ✅"
          
      - name: Notify Failure
        if: failure()
        uses: simple-container-com/api/.github/actions/notify@v1
        with:
          status: "failure"
          operation: "deploy"
          stack-name: "production-app"
          environment: "production"
          slack-webhook-url: ${{ secrets.SLACK_WEBHOOK }}
          custom-message: "Deployment failed - check logs"
```

### 8. Custom SC CLI Setup

**File**: `.github/workflows/custom-setup.yml`

```yaml
name: Custom SC Setup
on: [push]

jobs:
  custom-deployment:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      # Use shared setup action independently
      - name: Setup Simple Container
        uses: simple-container-com/api/.github/actions/setup-sc@v1
        with:
          sc-config: ${{ secrets.SC_CONFIG }}
          sc-version: "2025.8.5"
          setup-devops-repo: true
          devops-repo: "myorg/infrastructure"
          
      # Run custom SC commands
      - name: Custom SC Operations
        run: |
          # Your custom Simple Container operations
          sc status --all
          sc stack list
          sc deploy -s my-app -e staging --dry-run
```

## Repository Setup Requirements

### Required Secrets

Add these secrets to your GitHub repository:

```bash
# Required
SC_CONFIG           # Your Simple Container configuration

# Optional (for notifications)  
SLACK_WEBHOOK       # Slack webhook URL
DISCORD_WEBHOOK     # Discord webhook URL
```

### Required Permissions

Ensure your repository has these permissions:
- `actions: write` - For workflow management
- `contents: write` - For tagging releases
- `pull-requests: write` - For PR comments

## Migration from Hardcoded Workflows

To migrate from existing hardcoded workflows:

1. **Replace workflow calls**:
   ```yaml
   # Old
   uses: myorg/devops/.github/workflows/build-and-deploy-service.yaml@main
   
   # New  
   uses: simple-container-com/api/.github/actions/deploy-client-stack@v1
   ```

2. **Update input parameters**:
   ```yaml
   # Most inputs map directly
   with:
     stack-name: "my-app"
     environment: "staging"
     sc-config: ${{ secrets.SC_CONFIG }}
   ```

3. **Add environment protection** (if needed):
   ```yaml
   environment: 
     name: production
     required-reviewers: ["devops-team"]
   ```

These examples provide a complete foundation for implementing Simple Container operations using GitHub Actions, with proper error handling, notifications, and safety measures.
