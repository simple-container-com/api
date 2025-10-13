# Multi-Stack Deployment Example

This example demonstrates a complex CI/CD setup managing multiple related stacks with proper dependency ordering and cross-stack resource sharing.

## Overview

This setup manages:
- **Infrastructure Stack** - Shared resources (VPC, databases, load balancers)
- **API Stack** - Backend services with database dependencies
- **Frontend Stack** - Web application with API dependencies
- **Dependency Management** - Proper deployment order and resource sharing

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│ Infrastructure  │    │   API Stack     │    │ Frontend Stack  │
│     Stack       │    │                 │    │                 │
│                 │    │                 │    │                 │
│  ┌───────────┐  │    │  ┌───────────┐  │    │  ┌───────────┐  │
│  │    VPC    │  │    │  │  Backend  │  │    │  │   React   │  │
│  │ Database  │  │────┼─▶│  Service  │  │────┼─▶│    App    │  │
│  │    ALB    │  │    │  │           │  │    │  │           │  │
│  └───────────┘  │    │  └───────────┘  │    │  └───────────┘  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│     Staging     │    │     Staging     │    │     Staging     │
│  Environment    │    │  Environment    │    │  Environment    │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Production    │    │   Production    │    │   Production    │
│  Environment    │    │  Environment    │    │  Environment    │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Project Structure

```
multi-stack-app/
├── .sc/
│   └── stacks/
│       ├── infrastructure/
│       │   ├── server.yaml      # Shared infrastructure
│       │   └── secrets.yaml     # Infrastructure credentials
│       ├── api/
│       │   ├── server.yaml      # API stack configuration
│       │   ├── secrets.yaml     # API secrets
│       │   └── client.yaml      # API deployment config
│       └── frontend/
│           ├── server.yaml      # Frontend stack configuration
│           ├── secrets.yaml     # Frontend secrets
│           └── client.yaml      # Frontend deployment config
├── .github/
│   └── workflows/
│       ├── deploy-infrastructure.yml  # Generated infrastructure workflow
│       ├── deploy-api.yml             # Generated API workflow
│       ├── deploy-frontend.yml        # Generated frontend workflow
│       └── deploy-full-stack.yml      # Orchestrated full deployment
├── api/                               # Backend service source
├── frontend/                          # Frontend application source
└── README.md
```

## Configuration Files

### Infrastructure Stack (`infrastructure/server.yaml`)

```yaml
schemaVersion: 1.0

# Infrastructure provisioning
provisioner:
  type: pulumi
  config:
    state-storage:
      type: pulumi-cloud
      config:
        url: https://api.pulumi.com
        access-token: ${secret:pulumi-access-token}

# CI/CD configuration for infrastructure
cicd:
  type: github-actions
  config:
    organization: "my-company"
    
    environments:
      staging:
        type: staging
        protection: false
        auto-deploy: true
        runners: ["ubuntu-latest"]
        deploy-flags: ["--skip-preview"]
        variables:
          ENVIRONMENT: "staging"
      
      production:
        type: production
        protection: true
        reviewers: ["infrastructure-team", "senior-dev"]
        auto-deploy: false
        runners: ["ubuntu-latest"]
        deploy-flags: ["--timeout", "30m"]
        variables:
          ENVIRONMENT: "production"
    
    notifications:
      slack: "${secret:infrastructure-slack-webhook}"
    
    workflow-generation:
      enabled: true
      templates: ["deploy", "destroy"]
      sc-version: "latest"

# DNS and domain management
resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: "${secret:CLOUDFLARE_ACCOUNT_ID}"
      zoneName: mycompany.com
      
  resources:
    staging:
      template: main-infrastructure
      resources: &staging-resources
        database:
          type: mongodb-atlas
          config:
            instanceSize: "M10"
            region: "US_EAST_1"
            cloudProvider: AWS
            privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
            publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
        media-storage:
          type: s3-bucket
          config:
            credentials: "${auth:aws}"
    production:
      template: main-infrastructure
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

### API Stack (`api/server.yaml`)

```yaml
schemaVersion: 1.0

# API stack depends on infrastructure
parent: my-company/infrastructure

# CI/CD configuration for API
cicd:
  type: github-actions
  config:
    organization: "my-company"
    
    environments:
      staging:
        type: staging
        protection: false
        auto-deploy: true
        runners: ["ubuntu-latest"]
        deploy-flags: ["--skip-preview"]
        variables:
          NODE_ENV: "staging"
          API_VERSION: "v1"
      
      production:
        type: production
        protection: true
        reviewers: ["backend-team"]
        auto-deploy: false
        runners: ["ubuntu-latest"]
        deploy-flags: ["--skip-preview"]
        variables:
          NODE_ENV: "production"
          API_VERSION: "v1"
    
    notifications:
      slack: "${secret:api-slack-webhook}"
    
    workflow-generation:
      enabled: true
      templates: ["deploy", "destroy"]
      sc-version: "latest"

# API stack inherits resources from parent infrastructure stack
# No additional resources needed - uses parent's database and media-storage
```

### Frontend Stack (`frontend/server.yaml`)

```yaml
schemaVersion: 1.0

# Frontend depends on both infrastructure and API
parent: my-company/infrastructure

# CI/CD configuration for frontend
cicd:
  type: github-actions
  config:
    organization: "my-company"
    
    environments:
      staging:
        type: staging
        protection: false
        auto-deploy: true
        runners: ["ubuntu-latest"]
        deploy-flags: ["--skip-preview"]
        variables:
          REACT_APP_ENV: "staging"
          REACT_APP_API_URL: "https://api-staging.mycompany.com"
      
      production:
        type: production
        protection: true
        reviewers: ["frontend-team"]
        auto-deploy: false
        runners: ["ubuntu-latest"]
        deploy-flags: ["--skip-preview"]
        variables:
          REACT_APP_ENV: "production"
          REACT_APP_API_URL: "https://api.mycompany.com"
    
    notifications:
      slack: "${secret:frontend-slack-webhook}"
    
    workflow-generation:
      enabled: true
      templates: ["deploy", "destroy"]
      sc-version: "latest"

# Frontend stack inherits resources from parent infrastructure stack
# Uses parent's media-storage for assets and DNS for domain management
```

### API Client Configuration (`api/client.yaml`)

```yaml
schemaVersion: 1.0

stacks:
  staging:
    type: cloud-compose
    parent: my-company/api
    config:
      # Domain for the staging API
      domain: api-staging.mycompany.com
      
      # Size configuration
      size:
        cpu: 256
        memory: 512
      
      # Scaling configuration
      scale:
        min: 1
        max: 5
        policy:
          cpu:
            max: 70
      
      # Use parent resources
      uses:
        - database
        - media-storage
      
      # Environment variables
      env:
        NODE_ENV: "staging"
        PORT: "3000"
        LOG_LEVEL: "debug"
        BASE_URI: https://api-staging.mycompany.com
      
      # Application secrets from parent stack
      secrets:
        MONGO_URL: "${resource:database.uri}"
        JWT_SECRET: "${secret:jwt-secret}"
        EXTERNAL_API_KEY: "${secret:external-api-key}"
  
  production:
    type: cloud-compose
    parent: my-company/api
    parentEnv: production
    config:
      # Domain for the production API
      domain: api.mycompany.com
      
      # Size configuration (higher for production)
      size:
        cpu: 512
        memory: 1024
      
      # Scaling configuration
      scale:
        min: 2
        max: 20
        policy:
          cpu:
            max: 70
      
      # Use parent resources
      uses:
        - database
        - media-storage
      
      # Environment variables
      env:
        NODE_ENV: "production"
        PORT: "3000"
        LOG_LEVEL: "warn"
        BASE_URI: https://api.mycompany.com
      
      # Application secrets from parent stack
      secrets:
        MONGO_URL: "${resource:database.uri}"
        JWT_SECRET: "${secret:jwt-secret}"
        EXTERNAL_API_KEY: "${secret:external-api-key}"
```

### Frontend Client Configuration (`frontend/client.yaml`)

```yaml
schemaVersion: 1.0

stacks:
  staging:
    type: static
    parent: my-company/frontend
    config:
      # Domain for staging frontend
      domain: staging.mycompany.com
      
      # Build configuration
      buildCommand: "npm run build"
      buildDir: "frontend/dist/"
      
      # Use parent resources for media assets
      uses:
        - media-storage
      
      # Environment variables for build
      env:
        REACT_APP_ENV: "staging"
        REACT_APP_API_URL: "https://api-staging.mycompany.com"
        REACT_APP_VERSION: "${GIT_SHA}"
      
  production:
    type: static
    parent: my-company/frontend
    parentEnv: production
    config:
      # Domain for production frontend
      domain: mycompany.com
      
      # Build configuration
      buildCommand: "npm run build"
      buildDir: "frontend/dist/"
      
      # Use parent resources for media assets
      uses:
        - media-storage
      
      # Environment variables for build
      env:
        REACT_APP_ENV: "production"
        REACT_APP_API_URL: "https://api.mycompany.com"
        REACT_APP_VERSION: "${GIT_SHA}"
```

## GitHub Actions Orchestration

### Master Deployment Workflow

Create a custom workflow for orchestrated deployment:

```yaml
# .github/workflows/deploy-full-stack.yml
name: Deploy Full Stack
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
  # Step 1: Deploy infrastructure first
  deploy-infrastructure:
    runs-on: ubuntu-latest
    environment: ${{ github.event.inputs.environment || 'staging' }}
    outputs:
      stack-name: ${{ steps.infra-deploy.outputs.stack-name }}
      status: ${{ steps.infra-deploy.outputs.status }}
    steps:
      - name: Deploy Infrastructure
        id: infra-deploy
        uses: simple-container-com/api/.github/actions/provision@v2025.10.4
        with:
          stack-name: infrastructure
          sc-config: ${{ secrets.SC_CONFIG }}
  
  # Step 2: Deploy API services (depends on infrastructure)
  deploy-api:
    needs: deploy-infrastructure
    runs-on: ubuntu-latest
    environment: ${{ github.event.inputs.environment || 'staging' }}
    outputs:
      version: ${{ steps.api-deploy.outputs.version }}
      environment: ${{ steps.api-deploy.outputs.environment }}
    steps:
      - name: Deploy API Stack
        id: api-deploy
        uses: simple-container-com/api/.github/actions/deploy@v2025.10.4
        with:
          stack-name: api
          environment: ${{ github.event.inputs.environment || 'staging' }}
          sc-config: ${{ secrets.SC_CONFIG }}
  
  # Step 3: Deploy frontend (depends on API)
  deploy-frontend:
    needs: [deploy-infrastructure, deploy-api]
    runs-on: ubuntu-latest
    environment: ${{ github.event.inputs.environment || 'staging' }}
    steps:
      - name: Deploy Frontend Application
        uses: simple-container-com/api/.github/actions/deploy@v2025.10.4
        with:
          stack-name: frontend
          environment: ${{ github.event.inputs.environment || 'staging' }}
          sc-config: ${{ secrets.SC_CONFIG }}
  
  # Step 4: Run integration tests
  integration-tests:
    needs: [deploy-infrastructure, deploy-api, deploy-frontend]
    runs-on: ubuntu-latest
    if: github.event.inputs.environment == 'staging' || github.ref == 'refs/heads/main'
    steps:
      - uses: actions/checkout@v4
      
      - name: Run Integration Tests
        run: |
          npm install
          npm run test:integration
        env:
          ENVIRONMENT: ${{ needs.deploy-api.outputs.environment }}
          API_VERSION: ${{ needs.deploy-api.outputs.version }}
          FRONTEND_URL: https://${{ github.event.inputs.environment || 'staging' }}.mycompany.com
```

**Note**: All Simple Container actions (`provision@v2025.10.4` and `deploy@v2025.10.4`) include built-in notification support. Configure notification webhooks in your secrets to receive automatic notifications on deployment success or failure.
```

## Setup Instructions

### 1. Repository Structure

```bash
# Create the multi-stack structure
mkdir -p .sc/stacks/{infrastructure,api,frontend}
mkdir -p api frontend
mkdir -p .github/workflows
```

### 2. Generate Individual Workflows

```bash
# Generate workflows for each stack
sc cicd generate --stack infrastructure --output .github/workflows/
sc cicd generate --stack api --output .github/workflows/
sc cicd generate --stack frontend --output .github/workflows/
```

### 3. Configure Simple Container Secrets

**Required GitHub Secret:**
- `SC_CONFIG` - Simple Container configuration with SSH key pair to decrypt repository secrets

**Note:** All cloud provider credentials, API tokens, and application secrets are managed in each stack's `secrets.yaml` files:
- `.sc/stacks/infrastructure/secrets.yaml` - AWS, MongoDB Atlas, Cloudflare credentials
- `.sc/stacks/api/secrets.yaml` - JWT secrets, external API keys
- `.sc/stacks/frontend/secrets.yaml` - Any frontend-specific secrets

**Configure secrets for each stack:**
```bash
# Initialize secrets management (once per repository)
sc secrets init
sc secrets allow your-public-key

# Configure infrastructure secrets
vim .sc/stacks/infrastructure/secrets.yaml
sc secrets hide

# Configure API secrets
vim .sc/stacks/api/secrets.yaml  
sc secrets hide

# Configure frontend secrets
vim .sc/stacks/frontend/secrets.yaml
sc secrets hide

# Commit encrypted secrets
git add .sc/stacks/*/secrets.yaml
git commit -m "Add encrypted multi-stack secrets"
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

### 4. Environment Protection

Configure environment protection for each stack:
- **Staging**: No protection, automatic deployment
- **Production**: Require reviews from appropriate teams

## Deployment Strategies

### Sequential Deployment (Default)

Deploy stacks in dependency order:
1. Infrastructure → 2. API → 3. Frontend

### Parallel Deployment (Advanced)

For independent changes, deploy stacks in parallel:
```yaml
# In master workflow
deploy-api-and-frontend:
  needs: deploy-infrastructure
  strategy:
    matrix:
      stack: [api, frontend]
  runs-on: ubuntu-latest
  steps:
    # Deploy both API and frontend in parallel
```

### Rolling Updates

Update services with zero downtime:
```yaml
# In API client.yaml
config:
  deployment:
    strategy: rolling
    maxUnavailable: 25%
    maxSurge: 25%
```

## Monitoring and Health Checks

### Stack Health Endpoints

Each stack exposes health endpoints:
- **Infrastructure**: `/infra/health` - Database and cache connectivity
- **API**: `/api/health` - Service health and dependencies
- **Frontend**: `/health` - Application availability

### Integration Testing

Test cross-stack functionality:
```javascript
// tests/integration/full-stack.test.js
describe('Full Stack Integration', () => {
  test('API can connect to database', async () => {
    const response = await fetch(`${API_URL}/api/health`);
    expect(response.status).toBe(200);
  });
  
  test('Frontend can reach API', async () => {
    const response = await fetch(`${FRONTEND_URL}/api/users`);
    expect(response.status).toBe(200);
  });
});
```

## Troubleshooting

### Deployment Order Issues

**Problem**: API deployment fails because database doesn't exist
**Solution**: Ensure infrastructure deploys first in workflow dependencies

### Cross-Stack Resource References

**Problem**: Frontend can't access deployed resources
**Solution**: Use output values from previous deployment steps:
```yaml
needs: deploy-api
env:
  ENVIRONMENT: ${{ needs.deploy-api.outputs.environment }}
  API_VERSION: ${{ needs.deploy-api.outputs.version }}
```

### Environment Consistency

**Problem**: Different environments have different resource names
**Solution**: Use consistent naming with environment prefixes:
```yaml
resources:
  database:
    config:
      db-name: "${var:environment}-multistack"
```

## Advanced Features

### Blue-Green Deployment

Deploy new version alongside existing:
```yaml
# In client.yaml
config:
  deployment:
    strategy: blue-green
    testTrafficPercent: 10
```

### Rolling Updates

Simple Container automatically handles zero-downtime rolling deployments:
```yaml
# No configuration needed - rolling deployments are automatic
# Simple Container ensures:
# - Zero downtime deployments
# - Gradual traffic shifting  
# - Automatic health checks
# - Rollback on failure
```

### Deployment Monitoring

Monitor deployments using Simple Container's built-in health checks:
```yaml
# docker-compose.yaml - Health checks are configured here, not in stack config
services:
  app:
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:3000/health"]
      interval: 30s
      timeout: 10s
      retries: 3
    labels:
      "simple-container.com/healthcheck/path": "/health"
      "simple-container.com/healthcheck/port": "3000"
```

## Next Steps

- **[Preview Deployments](../preview-deployments/)** - Add PR-based testing
- **[Advanced Notifications](../advanced-notifications/)** - Multi-channel alerts
- **[Basic Setup](../basic-setup/)** - Simpler single-stack pattern
