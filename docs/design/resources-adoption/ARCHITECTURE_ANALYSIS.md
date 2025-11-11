# Architecture Analysis: ACME Corp Infrastructure

## Executive Summary

This document analyzes a sophisticated production cloud infrastructure running on GCP with a hybrid Pulumi/Simple Container setup. The architecture demonstrates enterprise-grade patterns including multi-project deployments, advanced CI/CD pipelines, and comprehensive resource management.

## Infrastructure Overview

### **Parent Stack: acme-corp-infrastructure**
- **Repository**: `acme-org/acme-corp-infrastructure`
- **Technology Stack**: Pulumi (TypeScript) + Simple Container (experimental)
- **Deployment Pattern**: Centralized infrastructure management
- **Workflow Complexity**: 422-line GitHub Actions workflow

### **Client Stack Pattern: sample-app (Example)**
- **Repository**: `acme-org/sample-app`
- **Service Type**: Node.js application
- **Deployment Method**: Calls parent stack's reusable workflow
- **Current Workflow**: 17-line GitHub Actions workflow
- **SC Target**: 8-line workflow using auto-generated parent workflows

## Technical Architecture

### **Multi-Project GCP Setup**

The infrastructure spans multiple GCP projects for environment isolation:

```yaml
Environments:
  staging:
    project: acme-staging
    region: me-central1
    zone: me-central1-a
    registry: asia-east1-docker.pkg.dev/acme-staging/docker-registry-staging
    
  production:
    project: acme-production
    region: asia-east1
    zone: asia-east1-a
    registry: asia-east1-docker.pkg.dev/acme-production/docker-registry-prod
    
  prod-eu:
    project: acme-prod-eu
    region: europe-west1
    zone: europe-west1-b
    registry: europe-central2-docker.pkg.dev/acme-prod-eu/docker-registry-prod-eu
```

### **Resource Architecture**

Based on the Pulumi configuration and stack files, the infrastructure includes:

#### **Compute Resources**
- **GKE Clusters**: Kubernetes orchestration across multiple regions
- **Container Registry**: Regional Docker registries for each environment
- **Load Balancers**: Traffic distribution and ingress management

#### **Data Layer**
- **Cloud SQL**: PostgreSQL and MySQL managed databases
- **MongoDB**: Custom MongoDB deployments
- **Redis**: Caching layer
- **Storage**: GCS buckets for different purposes

#### **Security & Networking**
- **KMS**: Encryption key management
- **Secret Manager**: Centralized secrets storage
- **Cloudflare**: CDN and DNS management
- **VPC**: Network isolation and security

#### **Messaging & Integration**
- **Pub/Sub**: Asynchronous messaging
- **RabbitMQ**: Advanced message queuing

## Deployment Pipeline Analysis

### **Client-to-Parent Workflow Integration**

**Client Service Workflow (sample-app)**:
```yaml
name: build and deploy (staging)
on:
  push:
    branches: ['main']
jobs:
  deploy-staging:
    uses: acme-org/acme-corp-infrastructure/.github/workflows/deploy-stack-gs.yaml@main
    with:
      service: 'sample-app'
      env: 'staging'
      platform: 'nodejs'
      telegram-notify-bot-chat: '-985701161'
    secrets:
      gcp-credentials-json: "${{ secrets.GCP_CREDENTIALS_STAGING_JSON }}"
      pat-github: "${{ secrets.TOKEN_FOR_SUB }}"
```

**Parent Stack Workflow Features**:
- **Multi-step Build Process**: Checkout, build, push to registry
- **Dynamic Configuration**: Environment-specific project and region selection
- **Advanced Notifications**: Telegram, Discord, Slack integration
- **Version Management**: CalVer with API validation
- **Pulumi Deployment**: Infrastructure-as-code deployment
- **Error Handling**: Comprehensive failure notifications and cleanup

### **Sophisticated Features**

#### **Environment Configuration Management**
```bash
# Dynamic environment selection based on input
if [[ "${{ inputs.env }}" == "prod" ]]; then
  echo "gcp-project=acme-production"
  echo "registry-url=asia-east1-docker.pkg.dev/acme-production/docker-registry-prod"
elif [[ "${{ inputs.env }}" == "prod-eu" ]]; then
  echo "gcp-project=acme-prod-eu"  
  echo "registry-url=europe-central2-docker.pkg.dev/acme-prod-eu/docker-registry-prod-eu"
else 
  echo "gcp-project=acme-staging"
  echo "registry-url=asia-east1-docker.pkg.dev/acme-staging/docker-registry-staging"
fi
```

#### **Multi-Channel Notification System**
The workflow includes sophisticated notifications:
- **Success Notifications**: Slack, Discord, Telegram
- **Failure Notifications**: Different channels with escalation
- **Build Metadata**: Branch, author, commit message extraction
- **Duration Tracking**: Performance monitoring

#### **Security Integration**
- **Secret Management**: GCP Secret Manager integration
- **Service Account**: Dedicated deployment credentials
- **SSH Key Management**: Private repository access
- **Container Security**: Registry authentication and scanning

## Simple Container Integration Points

The infrastructure already has a `.sc` directory with:
- **Configuration**: `cfg.default.yaml` with project settings
- **Secrets Management**: Encrypted `secrets.yaml` (113KB - comprehensive)
- **Template Structure**: Ready for Simple Container integration

This suggests they're already experimenting with Simple Container alongside Pulumi, making migration more feasible.

## Architecture Strengths

1. **Enterprise-Grade Separation**: Multi-project isolation
2. **Comprehensive Resource Management**: Full GCP service integration
3. **Advanced CI/CD**: Sophisticated build and deployment pipeline
4. **Multi-Region Support**: Geographic distribution capability
5. **Notification Integration**: Multi-channel alerting system
6. **Security Best Practices**: Proper secret and credential management
7. **Service Scalability**: Easy addition of new services via reusable workflows

## Migration Readiness Assessment

**Ready for Migration**:
- ✅ Existing Simple Container experimentation (`.sc` directory)
- ✅ Well-structured environment separation
- ✅ Comprehensive resource inventory
- ✅ Advanced notification requirements clearly defined
- ✅ Centralized deployment pattern already established

**Migration Complexity**: **Medium-High**
- Complex multi-project setup requires careful environment mapping
- Advanced Pulumi configurations need equivalent Simple Container templates
- Sophisticated CI/CD pipeline needs modernization strategy

This architecture represents an ideal candidate for Simple Container migration, demonstrating both the complexity that needs to be supported and the benefits that would be gained from simplification.
