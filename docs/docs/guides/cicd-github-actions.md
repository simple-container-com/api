# CI/CD with GitHub Actions

This comprehensive guide covers how to set up continuous integration and deployment (CI/CD) pipelines using Simple Container's built-in GitHub Actions integration. Simple Container automatically generates optimized workflow files from your infrastructure configuration, providing seamless deployment automation.

## Overview

Simple Container's CI/CD integration provides:

- **Automatic workflow generation** from server.yaml configuration  
- **Multi-environment deployment** with staging and production pipelines
- **Built-in secret management** integration with GitHub Secrets
- **Infrastructure provisioning** and application deployment workflows
- **Notification support** for Slack, Discord, and Telegram
- **Preview deployments** for pull requests
- **Rollback capabilities** for failed deployments

## How It Works

Simple Container generates GitHub Actions workflows based on your infrastructure configuration:

1. **Configuration is read** from `server.yaml` in your `.sc/stacks/<stack-name>/` directory
2. **Workflows are generated** automatically using the `sc cicd` command
3. **GitHub Actions execute** provisioning and deployment steps
4. **Notifications are sent** to your configured channels on success/failure
5. **Environments are managed** with proper protection rules and approvals

## Prerequisites

Before setting up CI/CD, ensure you have:

- Simple Container CLI installed
- GitHub repository with Actions enabled
- Appropriate cloud provider credentials (AWS, GCP, etc.)
- Simple Container project with server.yaml configuration

## Server Configuration

### Basic CI/CD Configuration

Add CI/CD configuration to your `server.yaml` file:

```yaml
schemaVersion: 1.0
cicd:
  type: github-actions
  config:
    organization: "your-org-name"
    
    # Environment configurations
    environments:
      staging:
        type: staging
        protection: false           # No approval required for staging
        auto-deploy: true          # Deploy automatically on main branch push
        runner: "ubuntu-latest"
        deploy-flags: ["--skip-preview"]  # Skip preview for automated deployment
        secrets: ["DATABASE_URL", "API_KEY"]  # Which secrets from secrets.yaml are available
        variables:                 # Non-sensitive environment variables for workflows
          NODE_ENV: "staging"
          LOG_LEVEL: "debug"
      
      production:
        type: production
        protection: true           # Require approval for production
        reviewers: ["senior-dev", "devops-team"]
        auto-deploy: false         # Manual deployment only
        runner: "ubuntu-latest"
        deploy-flags: ["--skip-preview"]
        secrets: ["DATABASE_URL", "API_KEY"]  # Which secrets from secrets.yaml are available
        variables:                 # Non-sensitive environment variables for workflows
          NODE_ENV: "production"
          LOG_LEVEL: "warn"
    
    # Notification settings
    notifications:
      slack:
        webhook-url: "${secret:slack-webhook-url}"
        enabled: true
      discord:
        webhook-url: "${secret:discord-webhook-url}"
        enabled: true
      telegram:
        bot-token: "${secret:telegram-bot-token}"
        chat-id: "${secret:telegram-chat-id}"
        enabled: false
    
    # Workflow generation settings
    workflow-generation:
      enabled: true
      templates: ["deploy", "destroy"]
      auto-update: true
      sc-version: "latest"
```

### Advanced Configuration

For more complex setups, you can configure additional options:

```yaml
cicd:
  type: github-actions
  config:
    organization: "your-org-name"
    
    environments:
      staging:
        type: staging
        protection: false
        auto-deploy: true
        runner: "ubuntu-latest"
        secrets: ["STAGING_DATABASE_URL", "STAGING_API_KEY"]
        variables:
          NODE_ENV: "staging"
          LOG_LEVEL: "debug"
        deploy-flags: ["--skip-preview", "--timeout", "15m"]
      
      production:
        type: production
        protection: true
        reviewers: ["senior-dev", "devops-team"]
        auto-deploy: false
        runner: "self-hosted"
        secrets: ["PRODUCTION_DATABASE_URL", "PRODUCTION_API_KEY"]
        variables:
          NODE_ENV: "production"
          LOG_LEVEL: "warn"
        deploy-flags: ["--timeout", "30m"]
    
    notifications:
      slack:
        webhook-url: "${secret:slack-webhook-url}"
        enabled: true
      discord:
        webhook-url: "${secret:discord-webhook-url}"
        enabled: true
      telegram:
        bot-token: "${secret:telegram-bot-token}"
        chat-id: "${secret:telegram-chat-id}"
        enabled: true
    
    workflow-generation:
      enabled: true
      output-path: ".github/workflows/"
      templates: ["deploy", "destroy", "preview"]
      auto-update: true
      custom-actions:
        security-scan: "security/scan@v1"
        performance-test: "perf/test@v2"
      sc-version: "v1.2.0"
```

## Secret Configuration

Create a `secrets.yaml` file in your stack directory for CI/CD secrets:

```yaml
schemaVersion: 1.0

# Cloud provider authentication for infrastructure provisioning
auth:
  aws:
    type: aws-token
    config:
      account: "123456789012"
      accessKey: "${secret:aws-access-key}"
      secretAccessKey: "${secret:aws-secret-key}"
      region: us-east-1

values:
  # Cloud provider credentials
  aws-access-key: your-aws-access-key-here
  aws-secret-key: your-aws-secret-key-here
  
  # Notification webhooks managed by Simple Container
  slack-webhook-url: "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
  discord-webhook-url: "https://discord.com/api/webhooks/YOUR/WEBHOOK/URL"
  telegram-bot-token: your-telegram-bot-token-here
  telegram-chat-id: your-telegram-chat-id-here
  
  # Application secrets for deployment
  staging-database-url: your-staging-database-connection-string
  production-database-url: your-production-database-connection-string
  staging-api-key: your-staging-api-key
  production-api-key: your-production-api-key
```

## Command Usage

### Generate Workflows

Generate GitHub Actions workflows from your configuration:

```bash
# Generate workflows for a specific stack
sc cicd generate --stack myorg/infrastructure --output .github/workflows/

# Generate with custom configuration file
sc cicd generate --config .sc/stacks/myapp/server.yaml --output .github/workflows/

# Force overwrite existing workflows
sc cicd generate --stack myorg/infrastructure --force
```

### Validate Configuration

Validate your CI/CD configuration and existing workflows:

```bash
# Validate CI/CD configuration for a stack
sc cicd validate --stack myorg/infrastructure

# Validate with specific configuration file
sc cicd validate --stack myorg/infrastructure --config .sc/stacks/myorg-infrastructure/server.yaml

# Show differences between configuration and existing workflows
sc cicd validate --stack myorg/infrastructure --show-diff
```

### Sync Workflows

Synchronize existing workflows with updated configuration:

```bash
# Sync workflows for a specific stack
sc cicd sync --stack myorg/infrastructure

# Sync with dry-run to see what would change
sc cicd sync --stack myorg/infrastructure --dry-run

# Force sync without confirmation
sc cicd sync --stack myorg/infrastructure --force
```

### Preview Workflows

Preview generated workflows before writing files:

```bash
# Preview all workflow templates
sc cicd preview --stack myorg/infrastructure

# Preview with detailed output
sc cicd preview --stack myorg/infrastructure --format detailed

# Show workflow content
sc cicd preview --stack myorg/infrastructure --show-content
```

## Generated Workflows

Simple Container generates optimized GitHub Actions workflows for different deployment scenarios:

### Deploy Workflow

Generated at `.github/workflows/deploy-<stack-name>.yml`:

```yaml
name: Deploy Stack
on:
  push:
    branches: [main]
  workflow_dispatch:
    inputs:
      environment:
        description: 'Environment to deploy to'
        required: true
        default: 'staging'
        type: choice
        options:
        - staging
        - production
      verbose:
        description: 'Enable verbose logging for debugging'
        type: boolean
        default: false

jobs:
  deploy:
    runs-on: ubuntu-latest
    environment: ${{ github.event.inputs.environment || 'staging' }}
    outputs:
      stack-name: ${{ steps.deploy.outputs.stack-name }}
      status: ${{ steps.deploy.outputs.status }}
    steps:
    - name: Deploy Infrastructure
      id: deploy
      uses: simple-container-com/api/.github/actions/provision@v2025.10.4
      with:
        stack-name: myorg/infrastructure
        sc-config: ${{ secrets.SC_CONFIG }}
        verbose: ${{ github.event.inputs.verbose || 'false' }}
        # Built-in notifications automatically configured via SC secrets
```

**Available Inputs:**

All Simple Container GitHub Actions support these common inputs:

- **`stack-name`** *(required)* - Name of the stack to operate on
- **`sc-config`** *(required)* - Simple Container configuration (use `${{ secrets.SC_CONFIG }}`)
- **`environment`** - Target environment (e.g., `staging`, `production`)
- **`verbose`** - Enable verbose logging for detailed debugging information (`true`/`false`, default: `false`)
- **`dry-run`** - Run in preview mode without making actual changes (`true`/`false`, default: `false`)
- **`notify-on-completion`** - Send notifications when operation completes (`true`/`false`, default: `true`)

**Available Outputs:**
- **`stack-name`** - Name of the deployed stack
- **`status`** - Deployment status ("success")

For `deploy@v2025.10.4` action:
- **`version`** - Deployed application version
- **`environment`** - Target environment name

### Destroy Workflow  

Generated at `.github/workflows/destroy-<stack-name>.yml` for cleanup operations:

```yaml
name: Destroy Stack
on:
  workflow_dispatch:
    inputs:
      environment:
        description: 'Environment to destroy'
        required: true
        type: choice
        options:
        - staging
        - production
      confirm:
        description: 'Type "destroy" to confirm'
        required: true

jobs:
  destroy:
    runs-on: ubuntu-latest
    if: github.event.inputs.confirm == 'destroy'
    environment: ${{ github.event.inputs.environment }}
    steps:
    # Similar steps to deploy workflow but with destroy action
```

## GitHub Repository Setup

### Required Secrets

Configure these secrets in your GitHub repository settings:

**Only ONE GitHub secret required:**
- `SC_CONFIG` - Simple Container configuration with SSH key pair to decrypt repository secrets

**All other secrets are managed in Simple Container's encrypted secrets.yaml files:**
- **Cloud provider credentials** - AWS, GCP, Azure authentication
- **Notification webhooks** - Slack, Discord, Telegram configurations  
- **Application secrets** - Database URLs, API keys, environment-specific values
- **Infrastructure secrets** - Service accounts, certificates, access tokens

**Simple Container handles ALL secret management through its encrypted secrets system - no individual GitHub Actions secrets needed.**

### Environment Protection

Configure environment protection rules in GitHub:

1. Go to **Settings** → **Environments** in your repository
2. Create environments for `staging` and `production`
3. For **production environment**:
   - Enable **Required reviewers** and add team members
   - Set **Wait timer** if needed (e.g., 10 minutes)
   - Configure **Deployment branches** to restrict to main/master
4. For **staging environment**:
   - No protection rules needed for automatic deployment

## Workflow Triggers

### Automatic Deployment

Configure automatic deployment triggers:

```yaml
# In your workflow file
on:
  push:
    branches: [main]           # Deploy staging on main branch push
    paths: ['.sc/**', 'src/**'] # Only deploy on relevant file changes
  
  pull_request:
    types: [opened, synchronize]  # Preview deployments on PRs
    paths: ['.sc/**', 'src/**']
```

### Manual Deployment

Enable manual deployment with workflow_dispatch:

```yaml
on:
  workflow_dispatch:
    inputs:
      environment:
        description: 'Environment to deploy to'
        required: true
        default: 'staging'
        type: choice
        options: [staging, production]
      
      dry_run:
        description: 'Run in preview mode'
        type: boolean
        default: false
      
      verbose:
        description: 'Enable verbose logging for detailed debugging'
        type: boolean
        default: false
```

## Best Practices

### Security

1. **Use environment-specific secrets** - Never share production secrets with staging
2. **Enable branch protection** - Require PRs and reviews for main branch
3. **Configure environment protection** - Require approvals for production deployments
4. **Rotate secrets regularly** - Update cloud provider and application credentials
5. **Use least privilege access** - Grant minimal required permissions to GitHub Actions

### Deployment Strategy

1. **Deploy to staging first** - Always test changes in staging environment
2. **Use preview deployments** - Review infrastructure changes before applying
3. **Implement rollback procedures** - Maintain previous deployment artifacts
4. **Monitor deployment health** - Set up alerts and health checks
5. **Gradual production rollout** - Use blue-green or canary deployment patterns

### Workflow Organization

1. **Separate workflows by purpose** - Deploy, destroy, and maintenance workflows
2. **Use meaningful names** - Clear workflow and job names for easy identification
3. **Add comprehensive logging** - Debug deployment issues with detailed logs
4. **Implement notification strategy** - Alert on failures, summarize on success
5. **Version your workflows** - Track changes to CI/CD configuration

## Troubleshooting

### Common Issues

**Workflow fails with "Stack not found":**
```bash
# Ensure your stack exists and configuration is valid
sc cicd validate --stack myorg/infrastructure

# Check if server.yaml exists in the correct location
ls -la .sc/stacks/myorg-infrastructure/server.yaml
```

**Authentication errors:**
```bash
# Verify cloud provider credentials
aws sts get-caller-identity

# Check GitHub secrets are properly configured
# Go to Settings → Secrets and variables → Actions
```

**Configuration validation errors:**
```bash
# Validate your server.yaml configuration
sc cicd validate --stack myorg/infrastructure --show-diff

# Check the generated workflows
sc cicd preview --stack myorg/infrastructure --show-content
```

### Debugging Workflows

1. **Enable verbose logging** in Simple Container GitHub Actions:
   ```yaml
   - name: Deploy with verbose logging
     uses: simple-container-com/api/.github/actions/deploy-client-stack@main
     with:
       stack-name: "myapp"
       environment: "staging"
       sc-config: ${{ secrets.SC_CONFIG }}
       verbose: 'true'  # Enable detailed debugging information
   ```
   
   **Verbose logging provides:**
   - Detailed environment variable information
   - Step-by-step execution progress
   - Parent repository cloning details
   - Secret revelation process information
   - Provisioner parameter debugging
   - Git repository initialization details

2. **Enable GitHub Actions debug logging** (additional system-level debugging):
   - Go to repository **Settings** → **Secrets**
   - Add secret `ACTIONS_STEP_DEBUG` with value `true`

3. **Check workflow logs** in the Actions tab of your repository

3. **Use workflow artifacts** to debug generated files:
   ```yaml
   - name: Upload deployment logs
     if: failure()
     uses: actions/upload-artifact@v4
     with:
       name: deployment-logs
       path: logs/
   ```

4. **Test locally** before pushing to GitHub:
   ```bash
   # Test deployment locally
   sc deploy -s myorg/infrastructure -e staging --preview
   
   # Validate configuration
   sc cicd validate --stack myorg/infrastructure --show-diff
   ```

## Example Workflows

Check out complete examples in the [examples/cicd-github-actions/](../examples/cicd-github-actions/README.md) directory:

- **[Basic Setup](../examples/cicd-github-actions/basic-setup/)** - Simple staging/production pipeline
- **[Multi-Stack Deployment](../examples/cicd-github-actions/multi-stack/)** - Deploy multiple related stacks
- **[Preview Deployments](../examples/cicd-github-actions/preview-deployments/)** - PR-based preview environments
- **[Advanced Notifications](../examples/cicd-github-actions/advanced-notifications/)** - Multi-channel notification setup

## Next Steps

After setting up CI/CD:

1. Explore **[Advanced Deployment Patterns](../advanced/deployment-patterns.md)** for complex scenarios
2. Review **[Secrets Management](secrets-management.md)** for secure credential handling
3. Check **[DNS Management](dns-management.md)** for custom domain configuration
4. Set up **[Monitoring and Alerting](../advanced/monitoring.md)** for deployment health

## Need Help?

- Review **[Core Concepts](../concepts/main-concepts.md)** for fundamental understanding
- Check the **[GitHub Actions Examples](../examples/cicd-github-actions/README.md)** for real-world configurations
- Contact [support@simple-container.com](mailto:support@simple-container.com) for assistance
