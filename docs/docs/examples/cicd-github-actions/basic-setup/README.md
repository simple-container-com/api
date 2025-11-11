# Basic CI/CD Setup Example

This example demonstrates a simple CI/CD setup with staging and production environments using GitHub Actions and Simple Container.

## Overview

This setup provides:
- **Automatic staging deployment** when code is pushed to the main branch
- **Manual production deployment** with approval requirement
- **Slack notifications** for deployment status
- **Basic secret management** for cloud provider credentials

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   GitHub Repo   │    │  GitHub Actions │    │ Simple Container│
│                 │    │                 │    │                 │
│ Push to main ───┼───▶│ Deploy Staging  │───▶│   AWS ECS       │
│                 │    │                 │    │   (Staging)     │
│                 │    │                 │    │                 │
│ Manual trigger ─┼───▶│ Deploy Prod     │───▶│   AWS ECS       │
│ (with approval) │    │ (with approval) │    │  (Production)   │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                                │
                                ▼
                        ┌─────────────────┐
                        │ Slack Channel   │
                        │ (Notifications) │
                        └─────────────────┘
```

## Project Structure

```
my-app/
├── .sc/
│   └── stacks/
│       └── my-app/
│           ├── server.yaml      # Infrastructure configuration
│           ├── secrets.yaml     # Encrypted secrets
│           └── client.yaml      # Application configuration
├── .github/
│   └── workflows/
│       ├── deploy-my-app.yml    # Generated deployment workflow
│       └── destroy-my-app.yml   # Generated cleanup workflow
├── src/                         # Application source code
└── README.md
```

## Configuration Files

### server.yaml

```yaml
schemaVersion: 1.0

# Infrastructure configuration
provisioner:
  type: pulumi
  config:
    state-storage:
      type: pulumi-cloud
      config:
        url: https://api.pulumi.com
        access-token: ${secret:pulumi-access-token}

# CI/CD configuration
cicd:
  type: github-actions
  config:
    organization: "my-company"
    
    # Environment configurations
    environments:
      staging:
        type: staging
        protection: false           # No approval required
        auto-deploy: true          # Deploy automatically on main branch push
        runner: "ubuntu-latest"
        deploy-flags: ["--skip-preview"]  # Skip preview for automated deployment
        secrets: ["DATABASE_URL", "API_KEY"]  # Which secrets from secrets.yaml are available to this environment
        variables:                               # Non-sensitive environment variables for GitHub Actions workflows
          NODE_ENV: "staging"
          LOG_LEVEL: "debug"
      
      production:
        type: production
        protection: true           # Require approval
        reviewers: ["senior-dev", "team-lead"]
        auto-deploy: false         # Manual deployment only
        runner: "ubuntu-latest"
        deploy-flags: ["--skip-preview"]
        secrets: ["DATABASE_URL", "API_KEY"]  # Which secrets from secrets.yaml are available to this environment
        variables:                               # Non-sensitive environment variables for GitHub Actions workflows
          NODE_ENV: "production"
          LOG_LEVEL: "warn"
    
    # Notification settings
    notifications:
      slack:
        webhook-url: "${secret:slack-webhook-url}"
        enabled: true
    
    # Workflow generation settings
    workflow-generation:
      enabled: true
      templates: ["deploy", "destroy"]
      auto-update: true
      sc-version: "latest"

# ECS Fargate template - handles VPC, load balancer, and ECS cluster automatically
templates:
  main-app:
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
      zoneName: myapp.com
      
  resources:
    staging:
      template: main-app
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
      template: main-app
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
            backup:
              every: 1h
              retention: 168h
```

### secrets.yaml

```yaml
schemaVersion: 1.0

# Cloud provider authentication
auth:
  aws:
    type: aws-token
    config:
      account: "123456789012"
      accessKey: "${secret:aws-access-key}"
      secretAccessKey: "${secret:aws-secret-key}"
      region: us-east-1
  pulumi:
    type: pulumi-token
    config:
      credentials: "${secret:pulumi-access-token}"

# Secret values (actual values, not environment variables)
values:
  # AWS credentials
  aws-access-key: your-aws-access-key-here
  aws-secret-key: your-aws-secret-key-here
  
  # Pulumi access token
  pulumi-access-token: pul-YOUR-PULUMI-ACCESS-TOKEN-HERE
  
  # MongoDB Atlas credentials
  MONGODB_ATLAS_PUBLIC_KEY: your-mongodb-public-key-here
  MONGODB_ATLAS_PRIVATE_KEY: your-mongodb-private-key-here
  
  # Cloudflare API token and account ID for DNS management
  CLOUDFLARE_API_TOKEN: your-cloudflare-api-token-here
  CLOUDFLARE_ACCOUNT_ID: 23c5ca78cfb4721d9a603ed695a2623e
  
  # Application secrets
  api-key: your-application-api-key-here
  
  # CI/CD notification webhooks
  slack-webhook-url: "https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK"
```

### client.yaml

```yaml
schemaVersion: 1.0

# Application deployment configuration
stacks:
  staging:
    type: cloud-compose
    parent: my-company/my-app
    config:
      # Domain for the staging environment
      domain: staging.myapp.com
      
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
      
      # Use resources defined in parent stack
      uses:
        - database
      
      # Environment variables
      env:
        NODE_ENV: "staging"
        PORT: "3000"
        BASE_URI: https://staging.myapp.com
      
      # Application secrets from parent resources
      secrets:
        MONGO_URL: "${resource:database.uri}"
        API_KEY: "${secret:api-key}"
  
  production:
    type: cloud-compose
    parent: my-company/my-app
    parentEnv: production
    config:
      # Domain for the production environment
      domain: myapp.com
      
      # Size configuration (higher for production)
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
      
      # Use resources defined in parent stack
      uses:
        - database
      
      # Environment variables
      env:
        NODE_ENV: "production"
        PORT: "3000"
        BASE_URI: https://myapp.com
      
      # Application secrets from parent resources
      secrets:
        MONGO_URL: "${resource:database.uri}"
        API_KEY: "${secret:api-key}"
```

## GitHub Repository Setup

### 1. Configure GitHub Secrets

Go to your repository **Settings** → **Secrets and variables** → **Actions** and add:

**Required secrets:**
- `SC_CONFIG` - Simple Container configuration with SSH key pair to decrypt repository secrets

**Note:** All cloud provider credentials, API tokens, and application secrets are managed in `.sc/stacks/my-app/secrets.yaml` and encrypted using Simple Container's secrets management. GitHub Actions only needs the `SC_CONFIG` secret to decrypt and access all other secrets.

### 2. Configure Simple Container Secrets

```bash
# Initialize secrets management
sc secrets init

# Add your public key for secrets access
sc secrets allow your-public-key

# Edit secrets file with actual values
# (Replace placeholder values in .sc/stacks/my-app/secrets.yaml with real credentials)
vim .sc/stacks/my-app/secrets.yaml

# Encrypt and hide secrets in repository
sc secrets hide

# Commit encrypted secrets
git add .sc/stacks/my-app/secrets.yaml
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

### 3. Configure Environments

Go to **Settings** → **Environments** and create:

**Staging Environment:**
- Name: `staging`
- No protection rules (allows automatic deployment)

**Production Environment:**
- Name: `production`  
- Enable **Required reviewers** and add team members
- Optionally set **Wait timer** (e.g., 10 minutes)
- Configure **Deployment branches** to restrict to `main` branch only

## Setup Instructions

### 1. Clone and Configure

```bash
# Clone your repository
git clone https://github.com/my-company/my-app.git
cd my-app

# Create Simple Container configuration
mkdir -p .sc/stacks/my-app
```

### 2. Add Configuration Files

Copy the configuration files above into your project:
- `server.yaml` → `.sc/stacks/my-app/server.yaml`
- `secrets.yaml` → `.sc/stacks/my-app/secrets.yaml` 
- `client.yaml` → `.sc/stacks/my-app/client.yaml`

### 3. Encrypt Secrets

```bash
# Initialize secrets management
sc secrets init

# Add your public key
sc secrets allow your-public-key

# Encrypt the secrets file
sc secrets hide
```

### 4. Generate Workflows

```bash
# Generate GitHub Actions workflows
sc cicd generate --stack my-app --output .github/workflows/

# Validate the generated configuration
sc cicd validate --stack my-app
```

### 5. Commit and Push

```bash
# Add all files
git add .

# Commit changes
git commit -m "Add Simple Container CI/CD configuration"

# Push to trigger first deployment
git push origin main
```

## Generated Workflows

Simple Container will generate the following workflow files:

### `.github/workflows/deploy-my-app.yml`

This workflow handles deployment to both staging and production:

```yaml
name: Deploy My App
on:
  push:
    branches: [main]
  workflow_dispatch:
    inputs:
      environment:
        description: 'Environment to deploy'
        required: true
        default: 'staging'
        type: choice
        options: ['staging', 'production']

jobs:
  deploy-staging:
    if: github.ref == 'refs/heads/main' && github.event_name == 'push'
    runs-on: ubuntu-latest
    environment: staging
    steps:
      - name: Deploy to Staging
        uses: simple-container-com/api/.github/actions/deploy@v2025.10.4
        with:
          stack-name: my-app
          environment: staging
          sc-config: ${{ secrets.SC_CONFIG }}
  
  deploy-production:
    if: github.event_name == 'workflow_dispatch' && github.event.inputs.environment == 'production'
    runs-on: ubuntu-latest
    environment: production
    steps:
      - name: Deploy to Production
        uses: simple-container-com/api/.github/actions/deploy@v2025.10.4
        with:
          stack-name: my-app
          environment: production
          sc-config: ${{ secrets.SC_CONFIG }}
```

### `.github/workflows/destroy-my-app.yml`

This workflow handles cleanup and resource destruction:

```yaml
name: Destroy My App
on:
  workflow_dispatch:
    inputs:
      environment:
        description: 'Environment to destroy'
        required: true
        type: choice
        options: ['staging', 'production']
      confirm:
        description: 'Type "destroy" to confirm'
        required: true

jobs:
  destroy:
    if: github.event.inputs.confirm == 'destroy'
    runs-on: ubuntu-latest
    environment: ${{ github.event.inputs.environment }}
    steps:
      - name: Destroy Stack
        uses: simple-container-com/api/.github/actions/destroy@v2025.10.4
        with:
          stack-name: my-app
          environment: ${{ github.event.inputs.environment }}
          sc-config: ${{ secrets.SC_CONFIG }}
```

## Usage

### Automatic Staging Deployment

Push code to the main branch to automatically deploy to staging:

```bash
git add .
git commit -m "Update application code"
git push origin main
```

The staging deployment will trigger automatically and you'll receive a Slack notification upon completion.

### Manual Production Deployment

1. Go to your repository's **Actions** tab
2. Select the **Deploy My App** workflow
3. Click **Run workflow**
4. Select **production** environment
5. Click **Run workflow**

The deployment will wait for approval from the configured reviewers before proceeding.

### Resource Cleanup

To destroy resources in an environment:

1. Go to **Actions** tab
2. Select the **Destroy My App** workflow
3. Click **Run workflow**
4. Select the environment to destroy
5. Type "destroy" in the confirmation field
6. Click **Run workflow**

## Monitoring

### Deployment Status

Monitor deployments through:
- **GitHub Actions** - View workflow runs and logs
- **Slack notifications** - Receive status updates in your channel
- **AWS Console** - Monitor ECS services and RDS databases

### Health Checks

The application includes health check endpoints:
- **Staging**: `https://staging.my-app.com/health`
- **Production**: `https://my-app.com/health`

### Logs and Metrics

Access application logs through:
- **CloudWatch Logs** - Application and infrastructure logs
- **ECS Console** - Container-level metrics
- **Application Insights** - Custom application metrics

## Customization

### Adding More Environments

To add a development environment:

```yaml
# In server.yaml
environments:
  development:
    type: development
    protection: false
    auto-deploy: true
    runner: "ubuntu-latest"
    variables:
      NODE_ENV: "development"
      LOG_LEVEL: "debug"
```

### Custom Notifications

Add Discord notifications:

```yaml
# In server.yaml
notifications:
  slack: "${secret:slack-webhook-url}"
  discord: "${secret:discord-webhook-url}"
```

### Advanced Workflow Triggers

Customize deployment triggers:

```yaml
# Custom trigger in generated workflow
on:
  push:
    branches: [main]
    paths: 
      - 'src/**'
      - '.sc/**'
      - 'Dockerfile'
  
  schedule:
    - cron: '0 2 * * 1'  # Weekly deployment Monday 2 AM UTC
```

## Troubleshooting

### Common Issues

**Deployment fails with "AWS credentials not found":**
- Verify GitHub secrets are properly configured
- Check AWS IAM permissions for the access key
- Ensure AWS region is correct in server.yaml

**Workflow doesn't trigger automatically:**
- Check branch protection rules don't block pushes
- Verify workflow file syntax is correct
- Ensure the file is in `.github/workflows/` directory

**Production deployment hangs on approval:**
- Check environment protection settings
- Ensure reviewers have repository access
- Verify reviewers are available to approve

### Debug Steps

1. **Check workflow logs** in GitHub Actions tab
2. **Validate configuration locally:**
   ```bash
   sc cicd validate --stack my-app --show-diff
   ```
3. **Test deployment locally:**
   ```bash
   sc deploy -s my-app -e staging --preview
   ```
4. **Enable debug logging** by adding `ACTIONS_STEP_DEBUG=true` to GitHub secrets

## Next Steps

After setting up basic CI/CD:

1. **Add monitoring** - Set up CloudWatch alarms and dashboards
2. **Implement rollback** - Configure automated rollback on health check failures
3. **Add testing** - Integrate automated tests before deployment
4. **Scale resources** - Configure auto-scaling based on metrics
5. **Custom domains** - Set up DNS and SSL certificates

For more advanced setups, check out:
- **[Multi-Stack Deployment](../multi-stack/)** - Deploy multiple related stacks
- **[Preview Deployments](../preview-deployments/)** - PR-based testing environments
- **[Advanced Notifications](../advanced-notifications/)** - Multi-channel alerts
