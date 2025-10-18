# Resource Inventory: ACME Corp Infrastructure

## Overview

This document provides a comprehensive inventory of all cloud resources, services, and configurations detected in the ACME Corp Infrastructure analysis. This inventory serves as the foundation for the migration strategy to Simple Container.

## Infrastructure Scope

### **Multi-Project GCP Architecture**
- **Geographic Distribution**: 3 regions across Middle East, Asia, and Europe
- **Environment Separation**: Complete project isolation for staging/production
- **Service Count**: 10+ microservices with centralized infrastructure management

### **Project Breakdown**
| Environment | GCP Project ID | Region | Zone | Registry |
|------------|----------------|---------|------|----------|
| **Staging** | `acme-staging` | `me-central1` | `me-central1-a` | `asia-east1-docker.pkg.dev/acme-staging/docker-registry-staging` |
| **Production** | `acme-production` | `asia-east1` | `asia-east1-a` | `asia-east1-docker.pkg.dev/acme-production/docker-registry-prod` |
| **Prod-EU** | `acme-prod-eu` | `europe-west1` | `europe-west1-b` | `europe-central2-docker.pkg.dev/acme-prod-eu/docker-registry-prod-eu` |

## Core Infrastructure Resources

### **1. Compute Resources**

#### **Google Kubernetes Engine (GKE)**
- **Cluster Count**: 3 (one per environment)
- **Node Configuration**: 
  - Machine Type: `e2-standard-4` (staging), `n1-standard-4` (production)
  - Node Count: Variable scaling based on workload
- **Features**:
  - Regional persistent disks for high availability
  - Workload Identity for secure service communication
  - Horizontal Pod Autoscaling (HPA)
  - Cluster Autoscaling

#### **Container Registry (GCR/Artifact Registry)**
- **Registry Type**: Google Artifact Registry
- **Total Registries**: 3 environment-specific registries
- **Image Management**: 
  - CalVer versioning: `YYYY.MM.DD-{commit}`
  - Multi-arch support (AMD64, ARM64)
  - Vulnerability scanning enabled
- **Access Control**: IAM-based with service account authentication

#### **Load Balancing**
- **Global Load Balancer**: HTTP(S) load balancing across regions
- **SSL Termination**: Google-managed SSL certificates
- **CDN Integration**: Cloud CDN for static content optimization
- **Health Checks**: Application-level health monitoring

### **2. Data Layer Resources**

#### **Cloud SQL (PostgreSQL)**
- **Instance Configuration**:
  - **Staging**: `db-g1-small` (1 vCPU, 1.7GB RAM)
  - **Production**: `db-n1-standard-2` (2 vCPUs, 7.5GB RAM)
  - **Prod-EU**: `db-n1-standard-2` (2 vCPUs, 7.5GB RAM)
- **Features**:
  - Automated backups with point-in-time recovery
  - High availability with regional persistent disks
  - Read replicas for production workloads
  - Encryption at rest and in transit

#### **MongoDB Atlas**
- **Cluster Configuration**:
  - **Staging**: M10 (2GB RAM, 10GB storage)
  - **Production**: M30 (8GB RAM, 40GB storage) 
  - **Prod-EU**: M30 (8GB RAM, 40GB storage)
- **Regional Distribution**:
  - **Staging**: `EUROPE_CENTRAL_1`
  - **Production**: `ASIA_PACIFIC_NORTHEAST_1`
  - **Prod-EU**: `EUROPE_WEST_1`
- **Features**:
  - Cross-region replication
  - Automated backups with configurable retention
  - Database-level and collection-level access controls

#### **Redis (Memorystore)**
- **Instance Configuration**:
  - **Staging**: 1GB memory, single zone
  - **Production**: 4GB memory, high availability
  - **Prod-EU**: 4GB memory, high availability
- **Features**:
  - Redis 6.x with AUTH token security
  - Automatic failover in production
  - VPC peering for secure access

### **3. Storage Resources**

#### **Cloud Storage Buckets**
- **Storage Classes**: Standard, Nearline, Coldline, Archive
- **Bucket Inventory**:
  - `acme-storage-staging`: Application data and uploads
  - `acme-storage-production`: Production data with versioning
  - `acme-backup-*`: Automated backup storage
  - `acme-logs-*`: Application and system logs
- **Features**:
  - Object lifecycle management
  - Cross-regional replication for production
  - IAM and ACL-based access controls

#### **Persistent Volumes**
- **Volume Types**: Regional persistent disks (SSD)
- **Backup Strategy**: Automated snapshots with retention policies
- **Performance Tiers**: SSD for databases, standard for logs

### **4. Security and Networking**

#### **Key Management Service (KMS)**
- **Key Rings**: Environment-specific key rings
  - `acme-kms-staging`
  - `acme-kms-production`
  - `acme-kms-prod-eu`
- **Use Cases**:
  - Database encryption at rest
  - Application secret encryption
  - Container image signing

#### **VPC and Networking**
- **Network Architecture**: Custom VPC per environment
- **Subnets**: Application, database, and management subnets
- **Firewall Rules**: Restrictive ingress/egress rules
- **Private Google Access**: Enabled for secure API communication

#### **Secret Manager**
- **Secret Inventory**: 50+ environment-specific secrets
- **Categories**:
  - Database credentials and connection strings
  - Third-party API keys and tokens
  - SSL certificates and private keys
  - Service account keys

#### **Cloud DNS**
- **Zone Management**: `acme-corp.com` domain with subdomains
- **Record Types**: A, AAAA, CNAME, TXT, MX records
- **Health Checks**: DNS-based health monitoring

### **5. Messaging and Integration**

#### **Cloud Pub/Sub**
- **Topics**: 15+ topics for inter-service communication
- **Subscriptions**: Push and pull subscriptions
- **Message Flow**: Event-driven architecture support
- **Dead Letter Queues**: Error handling and retry logic

#### **RabbitMQ (Self-Managed)**
- **Deployment**: Containerized on GKE
- **Configuration**: Clustered setup with persistence
- **Use Cases**: Complex routing and message patterns
- **Monitoring**: Prometheus and Grafana integration

### **6. External Services**

#### **Cloudflare Integration**
- **DNS Management**: Primary DNS provider for `acme-corp.com`
- **CDN Services**: Global content distribution
- **Security Features**:
  - DDoS protection
  - Web Application Firewall (WAF)
  - SSL/TLS encryption
- **Analytics**: Traffic analysis and performance monitoring

#### **Third-Party APIs**
- **MongoDB Atlas**: Managed database service
- **Mailgun**: Transactional email delivery
- **Notification Services**:
  - Slack webhooks for team notifications
  - Discord webhooks for community alerts
  - Telegram Bot API for deployment notifications

## Service Architecture

### **Application Services Inventory**

#### **1. sample-app** (Node.js)
- **Type**: Web application / API server
- **Dependencies**: MongoDB, Redis, Cloud Storage
- **Deployment**: Single container per environment
- **Features**: User management, content delivery, analytics

#### **2. Infrastructure Services**
- **Monitoring Stack**: Prometheus, Grafana, AlertManager
- **Logging**: Fluent Bit, Elasticsearch, Kibana
- **Service Mesh**: Istio for traffic management
- **Security**: Falco for runtime security monitoring

### **Deployment Patterns**

#### **Container Orchestration**
- **Platform**: Google Kubernetes Engine (GKE)
- **Deployment Strategy**: Rolling updates with readiness probes
- **Service Discovery**: Kubernetes native DNS
- **Load Balancing**: Kubernetes services with GCP load balancers

#### **Configuration Management**
- **Secrets**: Kubernetes secrets with GCP Secret Manager integration
- **Config Maps**: Application configuration and feature flags
- **Environment Variables**: Runtime configuration injection

## Resource Dependencies

### **Critical Path Dependencies**
1. **GCP Projects** → **VPC Networks** → **GKE Clusters**
2. **KMS Keys** → **Secret Manager** → **Application Secrets**
3. **Cloud DNS** → **Load Balancers** → **Application Ingress**
4. **Container Registry** → **GKE Deployments** → **Application Pods**

### **Data Flow Architecture**
```
Internet → Cloudflare → GCP Load Balancer → GKE Ingress → Application Pods
                                                            ↓
Application Data → Cloud SQL (PostgreSQL) ← Backup → Cloud Storage
                                                            ↓
Session Data → Redis (Memorystore) ← Monitoring ← Prometheus
                                                            ↓
Document Data → MongoDB Atlas ← Analytics ← BigQuery
```

## Cost Analysis

### **Estimated Monthly Costs (USD)**

#### **Compute Resources**
| Resource | Staging | Production | Prod-EU | Total |
|----------|---------|------------|---------|-------|
| GKE Clusters | $180 | $450 | $350 | $980 |
| Container Registry | $20 | $45 | $35 | $100 |
| Load Balancers | $25 | $40 | $35 | $100 |

#### **Data Storage**
| Resource | Staging | Production | Prod-EU | Total |
|----------|---------|------------|---------|-------|
| Cloud SQL | $65 | $180 | $160 | $405 |
| MongoDB Atlas | $57 | $250 | $220 | $527 |
| Redis (Memorystore) | $45 | $120 | $110 | $275 |
| Cloud Storage | $30 | $85 | $70 | $185 |

#### **Networking & Security**
| Resource | Monthly Cost |
|----------|--------------|
| Cloud DNS | $15 |
| KMS Operations | $25 |
| Secret Manager | $10 |
| VPC Networking | $40 |

#### **External Services**
| Service | Monthly Cost |
|---------|--------------|
| Cloudflare Pro | $20 |
| MongoDB Atlas | $527 |
| Monitoring Tools | $150 |

**Total Estimated Monthly Cost: ~$3,355 USD**

## Migration Complexity Assessment

### **High Complexity Resources**
1. **Multi-Project GCP Setup**: Requires careful credential and IAM management
2. **Custom Network Architecture**: Complex VPC and subnet configurations
3. **Inter-Service Dependencies**: Tightly coupled service communications
4. **Data Migration**: Large databases requiring careful migration planning

### **Medium Complexity Resources**
1. **Container Registry Migration**: Existing images need retagging
2. **Secret Management**: Large number of environment-specific secrets
3. **Monitoring Integration**: Custom Prometheus and Grafana configurations
4. **CI/CD Pipeline**: Complex 422-line manual workflows (to be replaced with SC auto-generation)

### **Low Complexity Resources**
1. **DNS Configuration**: Standard record migration to Simple Container
2. **Load Balancer Setup**: Standard HTTP(S) load balancing patterns
3. **Storage Buckets**: Direct migration with policy preservation
4. **Basic Compute**: Standard container workloads

## Resource Adoption Strategy

### **Critical Migration Requirement: ADOPT vs PROVISION**

**Problem**: ACME Corp has production resources with live data that cannot be recreated:

| Resource Type | Production Data Risk | Adoption Strategy |
|--------------|---------------------|-------------------|
| **MongoDB Atlas** | ⚠️ **HIGH** - Live user data, cannot recreate | **ADOPT** - Reference existing clusters |
| **Cloud SQL PostgreSQL** | ⚠️ **HIGH** - Transaction data, backups exist | **ADOPT** - Import existing instances |
| **Redis Memorystore** | 🔄 **MEDIUM** - Cache data, can rebuild | **ADOPT** - Preserve existing connections |
| **GCS Buckets** | ⚠️ **HIGH** - User uploads, media assets | **ADOPT** - Reference existing buckets |
| **KMS Keys** | 🔒 **CRITICAL** - Encrypted data dependencies | **ADOPT** - Cannot recreate without data loss |
| **VPC Networks** | 🌐 **MEDIUM** - Complex firewall rules | **ADOPT** - Preserve network security |

### **Resource Adoption Implementation**

#### **Adoptable Resources (Critical Path)**
```yaml
# server.yaml - Resource Adoption Pattern
resources:
  resources:
    staging:
      resources:
        # ADOPT EXISTING - Production data preservation
        postgresql-main:
          type: gcp-cloudsql-postgres
          config:
            adopt: true  # Don't provision - reference existing
            instanceName: "acme-postgres-staging"
            connectionName: "acme-staging:me-central1:acme-postgres-staging"
            
        mongodb-atlas-main:
          type: mongodb-atlas
          config:
            adopt: true
            clusterName: "ACME-Corp-Staging"
            projectId: "507f1f77bcf86cd799439011"
            
        redis-primary:
          type: gcp-redis
          config:
            adopt: true
            instanceId: "acme-redis-staging"
            region: "me-central1"
            
        media-storage:
          type: gcp-bucket
          config:
            adopt: true
            buckets:
              - name: "acme-storage-staging"  # Existing bucket with uploads
              
        encryption-keys:
          type: gcp-kms
          config:
            adopt: true
            keyRing: "acme-kms-staging"
            cryptoKeys:
              - "app-encryption"
              - "database-encryption"
```

#### **Resource Import Commands**
```bash
# Import existing resources into Simple Container state management
sc resource import --stack acme-corp-infrastructure \
  --resource postgresql-main \
  --type gcp-cloudsql-postgres \
  --instance-id "projects/acme-staging/instances/acme-postgres-staging" \
  --connection-name "acme-staging:me-central1:acme-postgres-staging"

sc resource import --stack acme-corp-infrastructure \
  --resource mongodb-atlas-main \
  --type mongodb-atlas \
  --cluster-id "507f1f77bcf86cd799439011" \
  --project-id "507f1f77bcf86cd799439011"

sc resource import --stack acme-corp-infrastructure \
  --resource redis-primary \
  --type gcp-redis \
  --instance-id "projects/acme-staging/locations/me-central1/instances/acme-redis-staging"

sc resource import --stack acme-corp-infrastructure \
  --resource media-storage \
  --type gcp-bucket \
  --bucket-name "acme-storage-staging"
```

#### **Client Service Integration**
```yaml
# client.yaml - sample-app accessing adopted resources
stacks:
  staging:
    config:
      secrets:
        # Seamless access to adopted resources through ${resource:} syntax
        DATABASE_URL: ${resource:postgresql-main.uri}     # Adopted PostgreSQL
        MONGODB_URI: ${resource:mongodb-atlas-main.uri}   # Adopted MongoDB Atlas
        REDIS_URL: ${resource:redis-primary.uri}          # Adopted Redis
        UPLOAD_BUCKET: ${resource:media-storage.bucket}   # Adopted GCS bucket
        
        # Mixed strategy - some adopted, some new
        ANALYTICS_DB: ${resource:new-analytics.uri}       # New SC-managed resource
```

## Simple Container Migration Readiness

### **✅ Ready for Migration**
- **Container Workloads**: All applications are containerized
- **Cloud-Native Architecture**: Kubernetes-based deployments
- **External Service Integration**: Well-defined API boundaries
- **Configuration Management**: Clear separation of config and secrets
- **Resource Identification**: Complete inventory of adoptable resources

### **⚠️ Requires Planning**
- **Resource Adoption**: Import existing production resources before migration
- **Mixed Resource Strategy**: Adopt critical resources, provision new ones
- **Database Migration**: Large datasets require careful planning (adoption mitigates risk)
- **Network Reconfiguration**: Custom VPC setup needs redesign
- **Monitoring Integration**: Custom stack requires reconfiguration

### **🔧 Enhancement Opportunities**  
- **Secret Management**: Simplify with Simple Container unified approach
- **CI/CD Simplification**: 98% workflow reduction with auto-generation (`sc cicd generate`)
- **Cost Optimization**: Potential 15-25% cost reduction through efficiency
- **Operational Overhead**: Significant reduction in maintenance complexity
- **Resource Governance**: Better tracking of adopted vs provisioned resources

## Migration Priority Matrix

| Resource Category | Business Impact | Migration Complexity | Priority |
|------------------|-----------------|---------------------|----------|
| Application Services | High | Medium | **1 - Critical** |
| Database Layer | High | High | **2 - Critical** |
| Security & Secrets | High | Medium | **3 - High** |
| CI/CD Pipelines | Medium | Low | **4 - High** |
| Monitoring Stack | Medium | Medium | **5 - Medium** |
| Storage & Backup | Medium | Low | **6 - Medium** |
| DNS & Networking | Low | Low | **7 - Low** |

This comprehensive resource inventory provides the foundation for detailed migration planning and ensures no critical components are overlooked during the transition to Simple Container.
