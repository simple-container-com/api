# Migration Strategy: Pulumi to Simple Container

## Executive Summary

This document outlines a comprehensive migration strategy for converting the ACME Corp infrastructure from its current hybrid Pulumi/Simple Container setup to a pure Simple Container architecture. The strategy emphasizes gradual transition to minimize risk while preserving all existing functionality.

## 🔑 Critical Architectural Decision: Fresh Pulumi State

**Pulumi State Approach**:
- ✅ **Create COMPLETELY NEW Pulumi state** (fresh state backend configuration in server.yaml)
- ✅ **Adopt existing cloud resources** (reference without recreating using `adopt: true`)
- ✅ **No import of old Pulumi state files** (clean slate for Simple Container)

**This Means**:
1. Configure new Pulumi state backend (GCS bucket, S3, etc.) in server.yaml provisioner section
2. Mark existing cloud resources with `adopt: true` in resource definitions
3. Run `sc provision` - Pulumi adds adopted resources to NEW state without touching infrastructure
4. Old Pulumi state remains untouched (can be archived or kept as backup)
5. SC manages all future infrastructure changes through new state

**Benefits**:
- 🎯 **Clean separation** from legacy Pulumi infrastructure
- 🎯 **No state migration complexity** - fresh start with SC
- 🎯 **Zero risk to existing resources** - only metadata tracking changes
- 🎯 **Easier rollback** - old Pulumi state remains available if needed

## Current State Assessment

### **Existing Infrastructure**
- **Parent Stack**: Pulumi TypeScript with 422-line GitHub Actions workflow
- **Client Stacks**: Simple service deployments calling parent workflows
- **Environments**: Multi-project GCP setup (acme-staging, acme-production, acme-prod-eu)
- **Resources**: Comprehensive GCP services (GKE, databases, storage, networking)
- **Simple Container Presence**: Experimental `.sc` directory already exists

### **Migration Readiness Indicators**
✅ **Existing SC Experimentation**: `.sc/` directory with 113KB secrets.yaml  
✅ **Centralized Architecture**: Parent-child pattern already established  
✅ **Well-Defined Environments**: Clear staging/prod/prod-eu separation  
✅ **Comprehensive Resource Inventory**: All GCP services documented  
✅ **Advanced Notification Requirements**: Multi-channel alerting defined  

## Migration Phases

### **Phase 0: Resource Adoption (Critical Prerequisites)**
**Duration**: 2-3 weeks  
**Risk Level**: Low  
**Priority**: **MUST COMPLETE FIRST**

#### **0.1 Existing Resource Identification and Import**

**Problem Statement**: ACME Corp infrastructure has production resources that cannot be reprovisioned:
- **Production Databases**: MongoDB Atlas clusters with live data
- **Redis Clusters**: Memorystore instances with cached data  
- **Storage Buckets**: GCS buckets with existing content
- **Networking**: VPC configurations and firewall rules
- **Security**: KMS keys and existing secrets

**Solution**: Resource Adoption Pattern

```yaml
# server.yaml - Resource Adoption Configuration
# ✅ Validated against actual schemas in pkg/clouds/

schemaVersion: 1.0

provisioner:
  type: pulumi
  config:
    state-storage:
      type: gcp-bucket
      config:
        credentials: "${auth:gcloud}"
        projectId: "acme-staging"
        bucketName: "acme-sc-pulumi-state"  # NEW state bucket for SC

templates:
  gke-staging:
    type: gcp-gke-autopilot
    config:
      credentials: "${auth:gcloud}"
      gkeClusterResource: "gke-autopilot-res"
      artifactRegistryResource: "artifact-registry-res"

resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: 87152c65fca76d443751a37a91a77c17
      zoneName: acme-corp.com
      
  resources:
    staging:
      template: gke-staging
      resources:
        # ========================================
        # ADOPTED RESOURCES
        # ========================================
        
        # Adopted GKE Autopilot Cluster
        # Schema: pkg/clouds/gcloud/gke_autopilot.go:GkeAutopilotResource
        gke-autopilot-res:
          type: gcp-gke-autopilot-cluster
          config:
            adopt: true
            clusterName: "acme-staging-cluster"
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            location: me-central1
            zone: me-central1-a
            caddy:
              enable: true
              namespace: caddy
              replicas: 2
              adoptionHandling:
                patchExisting: true
                deploymentName: "caddy"
        
        # Adopted MongoDB Atlas Cluster
        # Schema: pkg/clouds/mongodb/mongodb.go:AtlasConfig
        mongodb-cluster:
          type: mongodb-atlas
          config:
            adopt: true
            clusterName: "ACME-Staging"
            orgId: 5b89110a4e6581562623c59c
            projectId: "507f1f77bcf86cd799439011"
            projectName: "acme-staging-project"
            region: "WESTERN_EUROPE"
            cloudProvider: GCP
            privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
            publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
            
        # Adopted Cloud SQL Postgres
        # Schema: pkg/clouds/gcloud/postgres.go:PostgresGcpCloudsqlConfig
        postgresql-main:
          type: gcp-cloudsql-postgres
          config:
            adopt: true
            instanceName: "acme-postgres-staging"
            connectionName: "acme-staging:me-central1:acme-postgres-staging"
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            project: "acme-staging"
            version: "POSTGRES_14"
            region: "me-central1"
            usersProvisionRuntime:
              type: "gke"
              resourceName: "gke-autopilot-res"
            
        # Adopted Redis Instance
        # Schema: pkg/clouds/gcloud/redis.go:RedisConfig
        redis-cache:
          type: gcp-redis
          config:
            adopt: true
            instanceId: "acme-redis-staging"
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            region: me-central1
            
        # ========================================
        # NEW SC-MANAGED RESOURCES
        # ========================================
        
        # New Artifact Registry
        # Schema: pkg/clouds/gcloud/artifact_registry.go:ArtifactRegistryConfig
        artifact-registry-res:
          type: gcp-artifact-registry
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            location: me-central1
            docker:
              immutableTags: false
```

#### **0.2 Adoption Flow**

```bash
# No separate import command needed!
# Just run normal provision - SC detects adopt: true and handles it automatically

sc provision -s acme-corp-infrastructure -e staging

# SC automatically:
# ✅ Adopts gke-autopilot-res (imports into Pulumi state, doesn't modify cluster)
# ✅ Adopts mongodb-cluster (imports into Pulumi state, doesn't modify cluster)  
# ✅ Adopts postgresql-main (imports into Pulumi state, doesn't modify instance)
# ✅ Adopts redis-cache (imports into Pulumi state, doesn't modify instance)
# ✅ Provisions artifact-registry-res (creates new registry)
```

#### **0.3 Required Secrets Configuration**

```yaml
# secrets.yaml - Credentials for adopted resources
# ✅ Validated against actual compute processor requirements

values:
  # Cloudflare API
  CLOUDFLARE_API_TOKEN: "your-cloudflare-token"
  
  # MongoDB Atlas API credentials (for Pulumi provider to create users)
  MONGODB_ATLAS_PUBLIC_KEY: "your-api-public-key"
  MONGODB_ATLAS_PRIVATE_KEY: "your-api-private-key"
  
  # PostgreSQL root credentials (for K8s Jobs to create users/databases)
  POSTGRES_ROOT_USER: "postgres"
  POSTGRES_ROOT_PASSWORD: "existing-postgres-root-password"
  
  # Redis AUTH token
  REDIS_AUTH_TOKEN: "existing-redis-auth-token"
```

#### **0.4 Client Service Access**

Once resources are adopted, client services can reference them using standard `${resource:}` syntax:

```yaml
# client.yaml - Service using adopted resources
schemaVersion: 1.0
stacks:
  web-app:
    environment: staging
    uses:
      - mongodb-cluster       # Adopted MongoDB Atlas
      - postgresql-main       # Adopted Cloud SQL Postgres
      - redis-cache          # Adopted Redis
    secrets:
      # SC automatically injects these from adopted resources:
      MONGO_URI: ${resource:mongodb-cluster.uri}
      DATABASE_URL: ${resource:postgresql-main.uri}
      REDIS_URL: ${resource:redis-cache.uri}
```

**How It Works**:
1. Parent stack adopts resources → exports connection details
2. Compute processors create service-specific users
3. Client service receives environment variables with credentials
4. Service connects using its own isolated credentials

### **Phase 1: Infrastructure Foundation (Parent Stack)**
**Duration**: 4-6 weeks  
**Risk Level**: Medium  
**Rollback Strategy**: Keep existing Pulumi stack as backup  
**Prerequisites**: Phase 0 Resource Adoption MUST be completed first

```yaml
resources:
  production:
    template: gke-production
    resources:
      # ADOPT - Critical production data (cannot recreate)
      postgresql-main:
        type: gcp-cloudsql-postgres
        config:
          adopt: true
          instanceName: "acme-postgres-prod"
          
      mongodb-cluster:
        type: mongodb-atlas
        config:
          adopt: true
          clusterName: "ACME-Corp-Production"
          
      # PROVISION - New SC-managed resources
      new-analytics-db:
        type: gcp-cloudsql-postgres
        config:
          tier: "db-n1-standard-2"  # SC will provision this
          
      sc-managed-storage:
        type: gcp-bucket
        config:
          buckets:
            - name: "acme-sc-analytics"  # New bucket for SC services
```

#### **1.1 Parent Stack Server Configuration** 
Convert Pulumi infrastructure to Simple Container server.yaml with resource adoption:

```yaml
# server.yaml
schemaVersion: 1.0

provisioner:
  type: pulumi
  config:
    state-storage:
      type: gcp-bucket
      config:
        credentials: "${auth:gcloud}"
        provision: false
        projectId: "${auth:gcloud.projectId}"
        bucketName: acme-sc-state
    secrets-provider:
      type: gcp-kms
      config:
        credentials: "${auth:gcloud}"
        provision: true
        keyName: acme-kms-key

templates:
  # GKE Autopilot templates for multi-project setup
  gke-staging:
    type: gcp-gke-autopilot
    config:
      credentials: "${auth:gcloud-staging}"
      projectId: "${auth:gcloud-staging.projectId}"
      region: "me-central1"
      
  gke-production:
    type: gcp-gke-autopilot
    config:
      credentials: "${auth:gcloud-prod}"
      projectId: "${auth:gcloud-prod.projectId}"
      region: "asia-east1"
      
  gke-prod-eu:
    type: gcp-gke-autopilot
    config:
      credentials: "${auth:gcloud-prod-eu}"
      projectId: "${auth:gcloud-prod-eu.projectId}"
      region: "europe-west1"

resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      zoneName: acme-corp.com
      
  resources:
    staging:
      template: gke-staging
      resources:
        postgresql-main:
          type: gcp-cloudsql-postgres
          config:
            credentials: "${auth:gcloud-staging}"
            tier: "db-g1-small"
        mongodb-cluster:
          type: mongodb-atlas
          config:
            publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
            privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
            clusterTier: "M10"
            region: "EUROPE_CENTRAL_1"
        redis-cache:
          type: gcp-redis
          config:
            credentials: "${auth:gcloud-staging}"
            memorySizeGb: 1
        storage-buckets:
          type: gcp-bucket
          config:
            credentials: "${auth:gcloud-staging}"
            buckets:
              - name: "acme-storage-staging"
        
    production:
      template: gke-production
      resources:
        postgresql-main:
          type: gcp-cloudsql-postgres
          config:
            credentials: "${auth:gcloud-prod}"
            tier: "db-n1-standard-2"
        mongodb-cluster:
          type: mongodb-atlas
          config:
            publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
            privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
            clusterTier: "M30"
            region: "ASIA_PACIFIC_NORTHEAST_1"
        redis-cache:
          type: gcp-redis
          config:
            credentials: "${auth:gcloud-prod}"
            memorySizeGb: 4
            
    prod-eu:
      template: gke-prod-eu
      resources:
        postgresql-main:
          type: gcp-cloudsql-postgres
          config:
            credentials: "${auth:gcloud-prod-eu}"
            tier: "db-n1-standard-2"
        mongodb-cluster:
          type: mongodb-atlas
          config:
            publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
            privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
            clusterTier: "M30"
            region: "EUROPE_WEST_1"

secrets:
  type: simple-container
  config:
    inherit: pulumi

cicd:
  type: github-actions
  config:
    organization: "acme-org"
    environments:
      staging:
        type: staging
        auto-deploy: true
        runners: ["ubuntu-latest"]
      production:
        type: production
        protection: true
        reviewers: ["devops-team"]
        auto-deploy: false
      prod-eu:
        type: production
        protection: true
        reviewers: ["devops-team"] 
        auto-deploy: false
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
```

#### **1.2 Secrets Migration**
Leverage existing secrets.yaml (113KB indicates comprehensive setup):

```yaml
# secrets.yaml (migration from existing)
values:
  # GCP Authentication
  gcp-staging-credentials: "${GCP_STAGING_CREDENTIALS}"
  gcp-prod-credentials: "${GCP_PROD_CREDENTIALS}"  
  gcp-prod-eu-credentials: "${GCP_PROD_EU_CREDENTIALS}"
  
  # Notification Webhooks
  slack-webhook-url: "${SLACK_WEBHOOK_URL}"
  discord-webhook-url: "${DISCORD_WEBHOOK_URL}"
  telegram-bot-token: "${TELEGRAM_BOT_TOKEN}"
  telegram-chat-id: "${TELEGRAM_CHAT_ID}"
  
  # Database Credentials
  mongodb-atlas-public-key: "${MONGODB_ATLAS_PUBLIC_KEY}"
  mongodb-atlas-private-key: "${MONGODB_ATLAS_PRIVATE_KEY}"
  postgres-password: "${POSTGRES_PASSWORD}"
  redis-auth-token: "${REDIS_AUTH_TOKEN}"
```

#### **1.3 GitHub Actions Generation** 
Replace 422-line manual workflow with Simple Container's automatic workflow generation:

```bash
# Generate GitHub Actions workflows from cicd configuration
sc cicd generate --stack acme-corp-infrastructure --output .github/workflows/

# This automatically generates:
# - .github/workflows/provision-staging.yml
# - .github/workflows/provision-production.yml 
# - .github/workflows/provision-prod-eu.yml
# - .github/workflows/deploy-staging.yml
# - .github/workflows/deploy-production.yml
# - .github/workflows/deploy-prod-eu.yml

# Validate the generated configuration
sc cicd validate --stack acme-corp-infrastructure
```

**Generated Provision Workflow Example**:
```yaml
# .github/workflows/provision-staging.yml (auto-generated)
name: Provision Infrastructure - Staging
on:
  workflow_dispatch:
  push:
    branches: [main]
    paths: ['.sc/stacks/acme-corp-infrastructure/**']

jobs:
  provision:
    runs-on: ubuntu-latest
    steps:
      - uses: simple-container-com/api/.github/actions/provision@v1
        with:
          stack-name: acme-corp-infrastructure
          environment: staging
        env:
          SC_CONFIG: ${{ secrets.SC_CONFIG }}
```

**Generated Deploy Workflow Example**:
```yaml
# .github/workflows/deploy-staging.yml (auto-generated)
name: Deploy Client - Staging
on:
  workflow_call:
    inputs:
      service:
        required: true
        type: string

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: simple-container-com/api/.github/actions/deploy@v1
        with:
          stack-name: ${{ inputs.service }}
          environment: staging
        env:
          SC_CONFIG: ${{ secrets.SC_CONFIG }}
```

### **Phase 2: Client Stack Standardization**
**Duration**: 2-3 weeks per service  
**Risk Level**: Low  
**Rollback Strategy**: Individual service rollback capability  

#### **2.1 Client Service Conversion**
Convert each service (starting with sample-app) to Simple Container client.yaml:

```yaml
# client.yaml (sample-app)
schemaVersion: 1.0
stacks:
  staging:
    type: single-image
    parent: acme-org/acme-corp-infrastructure
    parentEnv: staging
    config:
      image: node:18-alpine
      port: 3000
      env:
        NODE_ENV: staging
        STAGE: staging
      secrets:
        DATABASE_URL: ${resource:postgresql-main.uri}
        REDIS_URL: ${resource:redis-cache.uri}
        API_KEY: ${secret:staging-api-key}
      healthCheck: "/health"
      
  production:
    type: single-image  
    parent: acme-org/acme-corp-infrastructure
    parentEnv: production
    config:
      image: node:18-alpine
      port: 3000
      scale:
        min: 2
        max: 10
      env:
        NODE_ENV: production
        STAGE: production
      secrets:
        DATABASE_URL: ${resource:postgresql-main.uri}
        REDIS_URL: ${resource:redis-cache.uri}
        API_KEY: ${secret:production-api-key}
      healthCheck: "/health"
```

#### **2.2 Workflow Integration**
Use the parent stack's generated workflows for client service deployment:

```yaml
# .github/workflows/deploy-staging.yml (client service)
name: Deploy sample-app to staging
on:
  push:
    branches: ['main']

jobs:
  deploy-staging:
    uses: acme-org/acme-corp-infrastructure/.github/workflows/deploy-staging.yml@main
    with:
      service: sample-app
    secrets:
      SC_CONFIG: ${{ secrets.SC_CONFIG }}
```

**Key Benefits**:
- **No Custom Logic**: Uses parent stack's generated workflows  
- **Automatic Updates**: Parent stack updates benefit all services
- **Consistent Environment**: Same deployment pattern across all services
- **Built-in Features**: Notifications, environment protection, rollback all included

**Generated vs Manual Comparison**:
```bash
# ❌ Current approach 
# Parent: 422-line custom workflow with complex logic
# Each client: 17-line service workflow calling parent
# Manual maintenance and duplication required

# ✅ Simple Container approach  
# Parent: sc cicd generate (auto-generates 6 workflows)
# Each client: 8-line workflow calling generated parent workflows
# Zero maintenance, automatic updates, feature inheritance
```

### **Phase 3: Advanced Feature Preservation**
**Duration**: 2-4 weeks  
**Risk Level**: Low  
**Focus**: Ensure all sophisticated features are maintained  

#### **3.1 Notification System Enhancement**
Validate multi-channel notifications work with Simple Container:
- Telegram integration with chat ID `-985701161`
- Discord webhook notifications
- Slack channel routing

#### **3.2 Environment Protection Rules**
Configure GitHub environment protection equivalent to current setup:
```yaml
# GitHub repository settings (via SC CLI or manual)
environments:
  production:
    protection_rules:
      required_reviewers: ["devops-team"]
      wait_timer: 0
      prevent_self_review: true
  prod-eu:  
    protection_rules:
      required_reviewers: ["devops-team"]
      wait_timer: 0
      prevent_self_review: true
```

#### **3.3 Advanced Versioning**
Ensure CalVer versioning continues to work with Simple Container actions.

### **Phase 4: Optimization & Cleanup**
**Duration**: 1-2 weeks  
**Risk Level**: Minimal  
**Focus**: Remove legacy components and optimize

#### **4.1 Legacy Removal**
- Archive Pulumi TypeScript code
- Remove complex GitHub Actions workflows
- Clean up obsolete secrets and configurations

#### **4.2 Documentation Update**
- Update internal documentation
- Create Simple Container usage guides
- Document new deployment procedures

## Risk Mitigation

### **Parallel Running Period**
- Run both systems in parallel for 2 weeks minimum
- Gradual traffic migration with canary deployments
- Comprehensive monitoring during transition

### **Rollback Procedures**
- Keep Pulumi stack deployable for 30 days post-migration
- Document rollback procedures for each phase
- Maintain backup of all configuration files

### **Testing Strategy**
- Stage-by-stage validation in staging environment
- Automated smoke tests for all services
- Load testing to ensure performance parity
- Security audit of new configuration

## Success Metrics

### **Operational Improvements**
- **Workflow Complexity**: 422 lines → 8 lines + auto-generated (98% reduction)  
- **Deployment Time**: Target 30% improvement with optimized SC actions
- **Configuration Maintainability**: Single YAML with `sc cicd generate` vs. TypeScript complexity
- **Developer Onboarding**: Zero-touch service addition with generated workflows

### **Feature Parity Validation**
- ✅ All current environments deployable
- ✅ All notification channels functional
- ✅ All security measures maintained
- ✅ All performance characteristics preserved
- ✅ All monitoring and alerting operational

## Timeline Summary

| Phase     | Duration        | Deliverables                                |
|-----------|-----------------|---------------------------------------------|
| Phase 1   | 4-6 weeks       | Parent stack migration, core infrastructure |  
| Phase 2   | 6-9 weeks       | All client services migrated                |
| Phase 3   | 2-4 weeks       | Advanced features validated                 |
| Phase 4   | 1-2 weeks       | Cleanup and optimization                    |
| **Total** | **13-21 weeks** | Complete Simple Container migration         |

## Post-Migration Benefits

1. **Simplified Operations**: Dramatically reduced workflow complexity
2. **Unified Configuration**: Single source of truth for all infrastructure
3. **Enhanced Developer Experience**: Standardized patterns across all services  
4. **Improved Scalability**: Easier service addition and environment management
5. **Better Security**: Built-in Simple Container security best practices
6. **Reduced Maintenance**: Less custom code to maintain
7. **Future-Proof Architecture**: Aligned with Simple Container roadmap

This migration strategy preserves all existing sophisticated features while dramatically simplifying the operational complexity, making the infrastructure more maintainable and scalable for future growth.
