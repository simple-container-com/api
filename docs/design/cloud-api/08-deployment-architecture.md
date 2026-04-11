# Simple Container Cloud API - Deployment Architecture

## Overview

This document outlines the deployment architecture for the Simple Container Cloud API using Simple Container's existing GitHub Actions CI/CD integration. The SC Cloud API follows SC's standard parent-client stack pattern and uses real SC CLI commands for deployment automation.

## Simple Container Configuration Structure

The SC Cloud API follows the standard SC project structure:

```
.sc/
└── stacks/
    └── sc-cloud-api/
        ├── server.yaml          # Infrastructure configuration  
        ├── client.yaml          # Application configuration
        └── secrets.yaml         # Encrypted secrets
```

### Infrastructure Stack - server.yaml

Based on existing SC resource types and patterns from the documentation:

```yaml
schemaVersion: 1.0

provisioner:
  type: pulumi
  config:
    state-storage:
      type: gcp-bucket
      config:
        credentials: "${auth:gcloud}"
        projectId: "${auth:gcloud.projectId}"
        provision: true
        bucketName: sc-cloud-api-state
        location: us-central1
    secrets-provider:
      type: gcp-kms
      config:
        provision: true
        projectId: "${auth:gcloud.projectId}"
        keyName: sc-cloud-api-kms-key
        keyLocation: global
        credentials: "${auth:gcloud}"

templates:
  cloud-api:
    type: gcp-gke-autopilot
    config:
      projectId: "${auth:gcloud.projectId}"
      credentials: "${auth:gcloud}"
      gkeClusterResource: gke-cluster
      artifactRegistryResource: artifact-registry

cicd:
  type: github-actions
  config:
    organization: "simple-container-org"
    environments:
      staging:
        type: staging
        protection: false
        auto-deploy: true
        runner: "ubuntu-latest"
        deploy-flags: ["--skip-preview"]
        secrets: ["MONGODB_CONNECTION_STRING", "REDIS_URL", "JWT_SECRET"]
        variables:
          NODE_ENV: "staging"
          LOG_LEVEL: "debug"
      production:
        type: production
        protection: true
        reviewers: ["devops-team", "senior-dev"]
        auto-deploy: false
        runner: "ubuntu-latest"
        deploy-flags: ["--skip-preview", "--timeout", "30m"]
        secrets: ["MONGODB_CONNECTION_STRING", "REDIS_URL", "JWT_SECRET"]
        variables:
          NODE_ENV: "production"
          LOG_LEVEL: "warn"
    notifications:
      slack:
        enabled: true
        webhook-url: "${secret:SLACK_WEBHOOK_URL}"
      discord:
        enabled: false
        webhook-url: ""
      telegram:
        enabled: false
        bot-token: ""
        chat-id: ""
    workflow-generation:
      enabled: true
      templates: ["deploy", "destroy"]
      auto-update: true
      custom-actions: {}
      output-path: ".github/workflows/"
      sc-version: "latest"

resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: "${secret:CLOUDFLARE_ACCOUNT_ID}"
      zoneName: simple-container.com
  resources:
    staging:
      template: cloud-api
      resources:
        mongodb:
          type: mongodb-atlas
          config:
            admins: ["admin"]
            developers: []
            instanceSize: "M2"
            orgId: "${secret:MONGODB_ATLAS_ORG_ID}"
            region: "US_CENTRAL"
            cloudProvider: GCP
            privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
            publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
        redis:
          type: gcp-redis
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            memorySizeGb: 2
            region: us-central1
            redisConfig:
              maxmemory-policy: noeviction
        gke-cluster:
          type: gcp-gke-autopilot-cluster
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            location: us-central1
            caddy:
              enable: true
              namespace: caddy
              replicas: 1
        artifact-registry:
          type: gcp-artifact-registry
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            location: us-central1
            docker:
              immutableTags: false
    production:
      template: cloud-api
      resources:
        mongodb:
          type: mongodb-atlas
          config:
            admins: ["admin"]
            developers: []
            instanceSize: "M10"
            orgId: "${secret:MONGODB_ATLAS_ORG_ID}"
            region: "US_CENTRAL"
            cloudProvider: GCP
            privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
            publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
        redis:
          type: gcp-redis
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            memorySizeGb: 5
            region: us-central1
            redisConfig:
              maxmemory-policy: noeviction
        gke-cluster:
          type: gcp-gke-autopilot-cluster
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            location: us-central1
            caddy:
              enable: true
              namespace: caddy
              replicas: 2
        artifact-registry:
          type: gcp-artifact-registry
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            location: us-central1
            docker:
              immutableTags: false

secrets:
  type: fs-passphrase
  config:
    inherit: ""

variables: {}
```

### Application Stack - client.yaml

```yaml
schemaVersion: 1.0

stacks:
  production:
    type: cloud-compose
    parent: simple-container-org/sc-cloud-infra
    parentEnv: production
    template: cloud-api
    config:
      uses: [mongodb, redis, gke-cluster, artifact-registry]
      domain: api.simple-container.com
      runs: [cloud-api]
      scale:
        min: 3
        max: 10
      env:
        NODE_ENV: "production"
        LOG_LEVEL: "warn"
      
  staging:
    type: cloud-compose  
    parent: simple-container-org/sc-cloud-infra
    parentEnv: staging
    template: cloud-api
    config:
      uses: [mongodb, redis, gke-cluster, artifact-registry]
      domain: api-staging.simple-container.com
      runs: [cloud-api]
      scale:
        min: 1
        max: 3
      env:
        NODE_ENV: "staging"
        LOG_LEVEL: "debug"
```

### Secrets Configuration - secrets.yaml

Based on SC's real secrets management pattern:

```yaml
schemaVersion: 1.0

# Cloud provider authentication (as shown in SC documentation)
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
  
  # Application secrets for deployment
  MONGODB_CONNECTION_STRING: mongodb+srv://user:pass@cluster.mongodb.net/db
  REDIS_URL: redis://redis-cluster:6379
  JWT_SECRET: your-jwt-secret-key
  
  # GitHub App credentials for API functionality
  GITHUB_APP_ID: your-github-app-id
  GITHUB_APP_PRIVATE_KEY: your-github-app-private-key
  GITHUB_WEBHOOK_SECRET: your-github-webhook-secret
  
  # Google OAuth credentials
  GOOGLE_CLIENT_ID: your-google-client-id
  GOOGLE_CLIENT_SECRET: your-google-client-secret
```

### Docker Compose Configuration

```yaml
# docker-compose.yaml for SC Cloud API
version: '3.8'

services:
  cloud-api:
    build: .
    ports:
      - "8080:8080"
      - "9090:9090"  # metrics
    environment:
      - DATABASE_URL=${MONGODB_CONNECTION_STRING}
      - REDIS_URL=${REDIS_URL}
      - JWT_SECRET=${JWT_SECRET}
      - GITHUB_APP_ID=${GITHUB_APP_ID}
      - GITHUB_APP_PRIVATE_KEY=${GITHUB_APP_PRIVATE_KEY}
      - GITHUB_WEBHOOK_SECRET=${GITHUB_WEBHOOK_SECRET}
      - GOOGLE_CLIENT_ID=${GOOGLE_CLIENT_ID}
      - GOOGLE_CLIENT_SECRET=${GOOGLE_CLIENT_SECRET}
```

## Real SC CI/CD Deployment Workflow

### Setup and Configuration

Following the actual SC GitHub Actions workflow:

```bash
# 1. Clone and setup project structure
git clone https://github.com/simple-container-org/sc-cloud-api.git
cd sc-cloud-api

# Create SC project structure
mkdir -p .sc/stacks/sc-cloud-api
```

### Secrets Management (Real SC Commands)

```bash
# Initialize secrets management  
sc secrets init

# Add your public key
sc secrets allow your-public-key

# Encrypt the secrets file
sc secrets hide
```

### Generate GitHub Actions Workflows (Real SC Command)

```bash
# Generate workflows using actual SC CLI command
sc cicd generate --stack sc-cloud-api --output .github/workflows/

# Validate the generated configuration
sc cicd validate --stack sc-cloud-api

# Preview workflows before committing
sc cicd preview --stack sc-cloud-api --show-content
```

### Generated Workflows

SC automatically generates these workflow files based on the actual patterns from SC documentation:

#### `.github/workflows/deploy-sc-cloud-api.yml`

```yaml
name: Deploy SC Cloud API
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
          stack-name: sc-cloud-api
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
          stack-name: sc-cloud-api
          environment: production
          sc-config: ${{ secrets.SC_CONFIG }}
```

#### `.github/workflows/destroy-sc-cloud-api.yml`

```yaml
name: Destroy SC Cloud API
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
          stack-name: sc-cloud-api
          environment: ${{ github.event.inputs.environment }}
          sc-config: ${{ secrets.SC_CONFIG }}
```

### Deployment Process

```bash
# 1. Commit and push configuration
git add .
git commit -m "Add SC Cloud API configuration"
git push origin main

# 2. Automatic staging deployment triggers on main branch push
# 3. Manual production deployment via GitHub UI workflow_dispatch
# 4. Monitor via GitHub Actions UI
```

## Monitoring & Operations

### Built-in SC Features

Simple Container provides these operational capabilities automatically:

- **Resource monitoring** via cloud provider dashboards (GCP Console, AWS CloudWatch)
- **Application logs** aggregated to cloud logging services
- **Basic health checks** defined in docker-compose.yaml
- **GitHub Actions workflow monitoring** in the Actions tab
- **Stack status** via existing SC CLI commands

### Operational Commands

```bash
# Check deployment status (if available in SC CLI)
sc status --stack sc-cloud-api

# View logs (if available in SC CLI) 
sc logs --stack sc-cloud-api --follow

# Update configuration and sync workflows
sc cicd sync --stack sc-cloud-api --dry-run
sc cicd sync --stack sc-cloud-api
```

### Application Health Monitoring

Health check endpoint in the Go application:

```go
// /health endpoint for basic health checking
func healthHandler(w http.ResponseWriter, r *http.Request) {
    // Basic health check logic
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}
```

## Operational Workflows

### GitHub Actions Deployment Workflow

The actual deployment process using SC's GitHub Actions integration:

```bash
# 1. Development workflow
git add .
git commit -m "Update SC Cloud API configuration"

# 2. Push to main triggers automatic staging deployment
git push origin main

# 3. Production deployment via GitHub UI
# Navigate to Actions -> Deploy SC Cloud API -> Run workflow
# Select "production" environment -> Run workflow

# 4. Monitor via GitHub Actions dashboard
# View logs and status in GitHub Actions UI
```

## Summary

The SC Cloud API deployment architecture uses Simple Container's existing features:

### **Real SC Components Used**
- **CI/CD Integration**: `sc cicd generate` command for GitHub Actions workflows
- **Stack Structure**: Parent-client pattern with server.yaml and client.yaml  
- **Secrets Management**: `sc secrets` commands for encrypted secrets.yaml
- **Resource Types**: Actual SC resource types like `gcp-gke-autopilot-cluster`, `mongodb-atlas`
- **GitHub Actions**: SC's built-in actions for deployment automation

### **Key Benefits**
- **Simplified Operations**: SC abstracts away Kubernetes complexity
- **Automated Workflows**: Generated GitHub Actions handle CI/CD
- **Secure Secrets**: Built-in secrets encryption and management
- **Multi-Environment**: Staging and production environments with approval workflows
- **Cloud Provider Integration**: Native support for GCP, AWS resources

This approach leverages SC's philosophy of simplifying cloud deployments while providing the orchestration capabilities needed for the SC Cloud API service.
