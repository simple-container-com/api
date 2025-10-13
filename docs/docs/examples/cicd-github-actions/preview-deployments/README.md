# Preview Deployments Example

This example demonstrates setting up PR-based preview environments that automatically deploy changes for testing and cleanup after PR closure.

## Overview

This setup provides:
- **Automatic preview deployment** when PRs are opened or updated
- **Temporary environment creation** with unique URLs
- **Resource cleanup** when PRs are closed or merged
- **Integration testing** in isolated environments
- **Cost optimization** through automatic cleanup

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Pull Request  │    │ Preview Deploy  │    │ Preview Env     │
│     #123        │    │    Workflow     │    │  pr-123-app     │
│                 │    │                 │    │                 │
│ feat/new-ui ────┼───▶│ Deploy PR-123   │───▶│ https://        │
│                 │    │                 │    │ pr-123.app.com  │
│                 │    │                 │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   PR Closed     │    │ Cleanup         │    │  Resources      │
│   or Merged     │    │  Workflow       │    │   Destroyed     │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Project Structure

```
preview-app/
├── .sc/
│   └── stacks/
│       └── preview-app/
│           ├── server.yaml      # Preview-enabled infrastructure
│           ├── secrets.yaml     # Encrypted secrets
│           └── client.yaml      # Application configuration
├── .github/
│   └── workflows/
│       ├── preview-deploy.yml   # PR preview deployment
│       ├── preview-cleanup.yml  # PR cleanup workflow
│       └── main-deploy.yml      # Main branch deployment
├── src/                         # Application source code
├── tests/                       # Test suites
└── README.md
```

## Configuration Files

### server.yaml

```yaml
schemaVersion: 1.0

# Infrastructure configuration with preview support
provisioner:
  type: pulumi
  config:
    state-storage:
      type: pulumi-cloud
      config:
        url: https://api.pulumi.com
        access-token: ${secret:pulumi-access-token}

# CI/CD configuration with preview environments
cicd:
  type: github-actions
  config:
    organization: "my-company"
    
    # Standard environments
    environments:
      staging:
        type: staging
        protection: false
        auto-deploy: true
        runners: ["ubuntu-latest"]
        deploy-flags: ["--skip-preview"]
        variables:  # Non-sensitive environment variables for GitHub Actions workflows
          ENVIRONMENT: "staging"
          DOMAIN_SUFFIX: "staging.myapp.com"
      
      production:
        type: production
        protection: true
        reviewers: ["senior-dev", "devops-team"]
        auto-deploy: false
        runners: ["ubuntu-latest"]
        deploy-flags: ["--skip-preview"]
        variables:  # Non-sensitive environment variables for GitHub Actions workflows
          ENVIRONMENT: "production"
          DOMAIN_SUFFIX: "myapp.com"
      
      # Preview environment template
      preview:
        type: preview
        protection: false
        auto-deploy: true
        runners: ["ubuntu-latest"]
        deploy-flags: ["--skip-preview"]
        variables:  # Non-sensitive environment variables for GitHub Actions workflows
          ENVIRONMENT: "preview"
          DOMAIN_SUFFIX: "preview.myapp.com"
          CLEANUP_AFTER: "7d"  # Auto-cleanup after 7 days
    
    # Enhanced notifications for previews
    notifications:
      slack: "${secret:slack-webhook-url}"
      discord: "${secret:discord-webhook-url}"
    
    # Preview-specific workflow settings
    workflow-generation:
      enabled: true
      templates: ["deploy", "destroy", "preview"]
      auto-update: true
      custom-actions:
        preview-comment: "actions/comment@v1"
        url-check: "actions/url-check@v1"
      sc-version: "latest"

# ECS Fargate template for preview deployments
templates:
  preview-app:
    type: ecs-fargate
    config:
      credentials: "${auth:aws}"
      account: "${auth:aws.projectId}"

# DNS and domain management
resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: "${secret:CLOUDFLARE_ACCOUNT_ID}"
      zoneName: preview.myapp.com
      
  resources:
    staging:
      template: preview-app
      resources: &staging-resources
        database:
          type: mongodb-atlas
          config:
            instanceSize: "M10"
            region: "US_EAST_1"
            cloudProvider: AWS
            privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
            publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
    production:
      template: preview-app
      resources:
        <<: *staging-resources
        database:
          type: mongodb-atlas
          config:
            instanceSize: "M30"
            region: "US_EAST_1"
            cloudProvider: AWS
            privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
            publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
    preview:
      template: preview-app
      resources:
        <<: *staging-resources
```

### client.yaml

```yaml
schemaVersion: 1.0

# Preview-enabled application configuration
stacks:
  staging:
    type: cloud-compose
    parent: my-company/preview-app
    config:
      # Domain for staging environment
      domain: staging.preview.myapp.com
      
      # Size configuration
      size:
        cpu: 256
        memory: 512
      
      # Scaling configuration
      scale:
        min: 1
        max: 3
        policy:
          cpu:
            max: 70
      
      # Use parent resources
      uses:
        - database
        
      env:
        NODE_ENV: "staging"
        PORT: "3000"
        API_VERSION: "v1"
        BASE_URI: https://staging.preview.myapp.com
        
      secrets:
        MONGO_URL: "${resource:database.uri}"
        JWT_SECRET: "${secret:jwt-secret}"

  production:
    type: cloud-compose
    parent: my-company/preview-app
    parentEnv: production
    config:
      # Domain for production environment
      domain: preview.myapp.com
      
      # Size configuration
      size:
        cpu: 512
        memory: 1024
      
      # Scaling configuration
      scale:
        min: 2
        max: 10
        policy:
          cpu:
            max: 70
      
      # Use parent resources
      uses:
        - database
        
      env:
        NODE_ENV: "production"
        PORT: "3000"
        API_VERSION: "v1"
        BASE_URI: https://preview.myapp.com
        
      secrets:
        MONGO_URL: "${resource:database.uri}"
        JWT_SECRET: "${secret:jwt-secret}"

  # Preview environment template
  preview:
    type: cloud-compose
    parent: my-company/preview-app
    parentEnv: preview
    config:
      # Dynamic domain for PR previews
      domain: pr-${PR_NUMBER}.preview.myapp.com
      
      # Minimal size for preview
      size:
        cpu: 128
        memory: 256
      
      # Limited scaling for cost optimization
      scale:
        min: 1
        max: 2
        policy:
          cpu:
            max: 80
      
      # Use parent resources
      uses:
        - database
        
      env:
        NODE_ENV: "preview"
        PORT: "3000"
        API_VERSION: "v1"
        PREVIEW_MODE: "true"
        BASE_URI: https://pr-${PR_NUMBER}.preview.myapp.com
        
      secrets:
        MONGO_URL: "${resource:database.uri}"
        JWT_SECRET: "${secret:jwt-secret-preview}"
      
      # Preview-specific features
      features:
        debug-mode: true
        metrics-collection: false
        error-reporting: false
```

## GitHub Workflows

### Preview Deployment Workflow

```yaml
# .github/workflows/preview-deploy.yml
name: Deploy Preview Environment
on:
  pull_request:
    types: [opened, synchronize, reopened]
    paths:
      - 'src/**'
      - '.sc/**'
      - 'Dockerfile'
      - 'package.json'

env:
  PR_NUMBER: ${{ github.event.number }}
  STACK_NAME: preview-app-pr-${{ github.event.number }}

jobs:
  deploy-preview:
    runs-on: ubuntu-latest
    environment: preview
    steps:
      - name: Deploy Preview Environment
        uses: simple-container-com/api/.github/actions/deploy@v2025.10.4
        with:
          stack-name: ${{ env.STACK_NAME }}
          environment: preview
          sc-config: ${{ secrets.SC_CONFIG }}
```

### Preview Cleanup Workflow

```yaml
# .github/workflows/preview-cleanup.yml
name: Cleanup Preview Environment
on:
  pull_request:
    types: [closed]
  schedule:
    - cron: '0 2 * * *'  # Daily cleanup at 2 AM

jobs:
  cleanup-preview:
    runs-on: ubuntu-latest
    steps:
      - name: Cleanup Preview Environment
        uses: simple-container-com/api/.github/actions/destroy@v2025.10.4
        with:
          stack-name: preview-app-pr-${{ github.event.number }}
          environment: preview
          sc-config: ${{ secrets.SC_CONFIG }}
```

## Setup Instructions

### 1. Repository Configuration

```bash
# Create preview-enabled project structure
mkdir -p .sc/stacks/preview-app
mkdir -p src tests
mkdir -p .github/workflows
```

### 2. Generate Base Workflows

```bash
# Generate standard workflows
sc cicd generate --stack preview-app --output .github/workflows/

# Add custom preview workflows (copy from examples above)
```

### 3. GitHub Repository Settings

**Environment Configuration:**
- Create `preview` environment with no protection rules
- Enable automatic deployment for preview environment

**Required Secrets:**
- `SC_CONFIG` - Simple Container configuration with SSH key pair to decrypt repository secrets

**Branch Protection:**
- Require status checks from preview deployment
- Require branches to be up to date

## Cost Optimization

**Automatic Cleanup:**
- Preview environments are automatically cleaned up when PRs are closed
- Daily scheduled cleanup removes stale environments older than 7 days
- Built-in notifications keep teams informed of cleanup activities

**Resource Management:**
- Use smaller instance sizes for preview environments
- Configure shorter TTL for preview resources
- Consider using spot instances where applicable

## Setup Instructions

### 1. Repository Configuration

```bash
# Create preview-enabled project structure
mkdir -p .sc/stacks/preview-app
mkdir -p src tests
mkdir -p .github/workflows
```

### 2. Generate Base Workflows

```bash
# Generate standard workflows
sc cicd generate --stack preview-app --output .github/workflows/

# Add custom preview workflows (copy from examples above)
```

### 3. GitHub Repository Settings

**Environment Configuration:**
- Create `preview` environment with no protection rules
- Enable automatic deployment for preview environment

**Required Secrets:**
- `SC_CONFIG` - Simple Container configuration with SSH key pair to decrypt repository secrets

**Note:** All cloud provider credentials, API tokens, and application secrets are managed in `.sc/stacks/preview-app/secrets.yaml` and encrypted using Simple Container's secrets management. GitHub Actions only needs the `SC_CONFIG` secret to decrypt and access all other secrets.

**Branch Protection:**
- Require status checks from preview deployment
- Require branches to be up to date

### 4. Configure Simple Container Secrets

```bash
# Initialize secrets management
sc secrets init

# Add your public key for secrets access
sc secrets allow your-public-key

# Edit secrets file with actual values
# (Replace placeholder values in .sc/stacks/preview-app/secrets.yaml with real credentials)
vim .sc/stacks/preview-app/secrets.yaml

# Encrypt and hide secrets in repository
sc secrets hide

# Commit encrypted secrets
git add .sc/stacks/preview-app/secrets.yaml
git commit -m "Add encrypted secrets configuration"
```

**Create GitHub Secret:**
```bash
# Generate SC_CONFIG for GitHub Actions
# SC_CONFIG contains your SSH private key and Simple Container configuration
# Get your SSH private key (used for decrypting repository secrets):
cat ~/.ssh/id_rsa

# Copy the private key content and add as SC_CONFIG secret in GitHub repository
# Go to: Settings → Secrets and variables → Actions → New repository secret
# Name: SC_CONFIG
# Value: <paste your SSH private key here>
```

### 5. DNS Configuration

DNS records are automatically provisioned by Simple Container based on the `domain` property in your stack configuration when a Cloudflare registrar is configured in server.yaml. No manual DNS setup required.

## Testing Strategy

### Smoke Tests

Basic functionality tests for previews:
```javascript
// tests/smoke/preview.test.js
describe('Preview Environment Smoke Tests', () => {
  const baseUrl = process.env.PREVIEW_URL || 'http://localhost:3000';
  
  test('Health endpoint responds', async () => {
    const response = await fetch(`${baseUrl}/health?preview=true`);
    expect(response.status).toBe(200);
    
    const health = await response.json();
    expect(health.environment).toBe('preview');
    expect(health.preview).toBe(true);
  });
  
  test('API endpoints accessible', async () => {
    const response = await fetch(`${baseUrl}/api/users`);
    expect(response.status).toBe(200);
  });
  
  test('Database connectivity', async () => {
    const response = await fetch(`${baseUrl}/api/health/db`);
    expect(response.status).toBe(200);
  });
});
```

### Integration Tests

Full feature tests in preview environment:
```bash
# Run in GitHub Actions
npm run test:integration -- --baseUrl="https://pr-${PR_NUMBER}.preview.myapp.com"
```

## Cost Optimization

### Resource Sizing

Preview environments use minimal resources:
- **CPU**: 128 (vs 512 for production)
- **Memory**: 256MB (vs 1GB for production)
- **Instance Count**: 1 (vs 2-10 for production)
- **Database**: t3.micro (vs t3.medium for production)

### Auto-Cleanup Policies

Multiple cleanup triggers:
1. **PR Closure** - Immediate cleanup when PR is closed/merged
2. **Scheduled Cleanup** - Daily cleanup of stale environments
3. **Age-based Cleanup** - Auto-destroy after 7 days
4. **Manual Cleanup** - On-demand cleanup via workflow dispatch

### Cost Monitoring

Track preview environment costs:
```yaml
# In server.yaml
resources:
  cost-alert:
    type: aws-budget
    config:
      budget-name: "preview-environments"
      limit-amount: 50
      limit-unit: "USD"
      time-unit: "MONTHLY"
      notification-email: "${secret:cost-alert-email}"
```

## Advanced Features

### Visual Regression Testing

Add visual diff testing to preview workflow:
```yaml
- name: Visual Regression Tests
  run: |
    npm run test:visual -- --baseUrl="https://pr-${{ env.PR_NUMBER }}.preview.myapp.com"
  
- name: Upload Visual Diffs
  if: failure()
  uses: actions/upload-artifact@v4
  with:
    name: visual-diffs
    path: tests/visual/diffs/
```

### Performance Testing

Automated performance testing in preview:
```yaml
- name: Performance Tests
  run: |
    npm run test:performance -- --url="https://pr-${{ env.PR_NUMBER }}.preview.myapp.com"
    
- name: Performance Report
  uses: actions/github-script@v7
  with:
    script: |
      const fs = require('fs');
      const report = fs.readFileSync('performance-report.json', 'utf8');
      const data = JSON.parse(report);
      
      const comment = `## ⚡ Performance Report
      
      **Load Time**: ${data.loadTime}ms
      **Memory Usage**: ${data.memoryUsage}MB
      **API Response Time**: ${data.apiResponseTime}ms
      
      ${data.score >= 90 ? '✅ Performance looks good!' : '⚠️ Performance needs attention'}`;
      
      github.rest.issues.createComment({
        issue_number: context.issue.number,
        owner: context.repo.owner,
        repo: context.repo.repo,
        body: comment
      });
```

### Security Scanning

Add security scanning for preview deployments:
```yaml
- name: Security Scan
  run: |
    docker run --rm \
      -v $(pwd):/src \
      securecodewarrior/docker-security-scan \
      --url "https://pr-${{ env.PR_NUMBER }}.preview.myapp.com"
```

## Troubleshooting

### Common Issues

**Preview deployment fails:**
- Check AWS permissions for temporary resource creation
- Ensure Cloudflare registrar is properly configured in server.yaml for automatic DNS provisioning
- Ensure cleanup jobs aren't interfering with active deployments

**Cleanup not working:**
- Check GitHub token permissions for PR access
- Verify AWS credentials for resource destruction
- Review scheduled cleanup job timing

**High costs:**
- Monitor preview environment resource usage
- Implement stricter cleanup policies
- Set up cost alerts and budgets

### Debugging

Enable debug mode for preview deployments:
```yaml
env:
  ACTIONS_STEP_DEBUG: true
  SC_DEBUG: true
```

## Next Steps

After setting up preview deployments:
- **[Advanced Notifications](../advanced-notifications/)** - Enhanced PR notifications
- **[Multi-Stack Deployment](../multi-stack/)** - Complex preview environments
- **[Basic Setup](../basic-setup/)** - Simpler deployment patterns
