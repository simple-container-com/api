# Resource Adoption Requirements

## Overview

Resource Adoption is a **critical enterprise feature** that allows Simple Container to reference and manage existing cloud resources without reprovisioning them. This enables brownfield migrations where production databases, storage, and other resources contain live data that cannot be recreated.

**Key Architecture**:
- ✅ **Pulumi State**: COMPLETELY NEW (fresh provisioner configuration in server.yaml)
- ✅ **Cloud Resources**: EXISTING/ADOPTED (referenced via `adopt: true`, not recreated)
- ✅ **State Management**: Pulumi tracks adopted resources in new state without modifying them

**This Means**:
1. Start with fresh Pulumi state backend (new GCS bucket, S3, or local state)
2. Configure provisioner in server.yaml as normal
3. Mark existing cloud resources with `adopt: true`
4. Pulumi imports resources into new state without touching actual infrastructure
5. SC can now manage service deployments using adopted resources

## Problem Statement

### **Current SC Limitation**
Simple Container assumes all resources are greenfield (new provisioning):
1. `sc provision` creates all resources from scratch
2. No way to reference existing production databases with live data
3. Cannot adopt existing storage buckets with uploaded content
4. Missing support for existing KMS keys protecting encrypted data
5. No mechanism to preserve complex network configurations

### **Enterprise Migration Blocker**
Without resource adoption, enterprises cannot migrate because:
- **Data Loss Risk**: Cannot recreate production databases
- **Operational Disruption**: Cannot recreate storage with existing content
- **Security Risk**: Cannot recreate KMS keys without losing encrypted data
- **Network Complexity**: Cannot recreate complex firewall rules and VPC configs

## Technical Requirements

### **1. Fresh Provisioner Configuration**

#### **Server.yaml - New Pulumi State Backend**

Since you're starting with completely fresh Pulumi state, configure the provisioner as normal:

```yaml
# server.yaml - Fresh provisioner configuration
schemaVersion: 1.0

provisioner:
  type: pulumi
  config:
    # NEW Pulumi state backend (not importing existing state)
    state-storage:
      type: gcp-bucket
      config:
        credentials: "${auth:gcloud}"
        provision: false  # Don't create state bucket (may already exist)
        projectId: "acme-staging"
        bucketName: "acme-sc-pulumi-state"  # NEW state bucket for SC
    
    # Optional: Secrets provider
    secrets-provider:
      type: gcp-kms
      config:
        credentials: "${auth:gcloud}"
        provision: true
        keyName: "acme-sc-secrets-key"
```

**Key Points**:
- ✅ This creates a **NEW Pulumi state** - separate from any existing Pulumi infrastructure
- ✅ State bucket can be new or existing (set `provision: false` if it exists)
- ✅ Pulumi stack names will be fresh (e.g., `acme-corp-infrastructure-staging`)
- ✅ No import of existing Pulumi state files needed

### **2. Configuration Schema Extensions**

#### **Server.yaml Schema Changes - Adopted Resources**
```yaml
resources:
  resources:
    production:
      resources:
        database-main:
          type: gcp-cloudsql-postgres
          config:
            # NEW: adopt flag tells SC not to provision
            adopt: true
            
            # Required for adopted resources: identification
            instanceName: "existing-postgres-prod"
            connectionName: "project:region:instance"
            
            # Optional: Override default properties
            host: "10.0.1.5"
            port: 5432
            database: "app_production"
```

#### **Resource Type Support Matrix**
| Resource Type           | Adopt Support  | Adoption Method       | Required Identifiers                   |
|-------------------------|----------------|-----------------------|----------------------------------------|
| `gcp-gke-autopilot`     | ✅ **CRITICAL** | `adopt: true` + `sc provision` | `clusterName`, `location`, `projectId` |
| `gcp-cloudsql-postgres` | ✅ Required     | `adopt: true` + `sc provision` | `instanceName`, `connectionName`       |
| `gcp-cloudsql-mysql`    | ✅ Required     | `adopt: true` + `sc provision` | `instanceName`, `connectionName`       |
| `mongodb-atlas`         | ✅ Required     | `adopt: true` + `sc provision` | `clusterName`, `projectId`             |
| `gcp-memorystore-redis` | ✅ Required     | `adopt: true` + `sc provision` | `instanceId`, `region`                 |
| `gcp-storage`           | ✅ Required     | `adopt: true` + `sc provision` | `bucketName`                           |
| `gcp-kms`               | ✅ Critical     | `adopt: true` + `sc provision` | `keyRing`, `cryptoKeys[]`              |
| `aws-rds-postgres`      | ✅ Required     | `adopt: true` + `sc provision` | `dbInstanceIdentifier`                 |
| `aws-elasticache-redis` | ✅ Required     | `adopt: true` + `sc provision` | `cacheClusterId`                       |
| `aws-s3`                | ✅ Required     | `adopt: true` + `sc provision` | `bucketName`                           |

#### **GKE Autopilot Cluster Adoption** ⚠️ **CRITICAL REQUIREMENT**

**Why Critical**: GKE Autopilot clusters are the compute foundation where all Kubernetes workloads run. Adopting existing clusters is essential for:
- **Zero Downtime**: Services continue running in existing clusters
- **Existing Workloads**: Caddy and other infrastructure already deployed
- **Data Persistence**: PersistentVolumes with existing data
- **Network Configuration**: Existing ingress, load balancers, and firewall rules

**Configuration Schema**:
```yaml
# server.yaml - GKE Autopilot Cluster Adoption
resources:
  resources:
    staging:
      cluster:
        type: gcp-gke-autopilot
        config:
          adopt: true  # Don't create cluster - reference existing
          
          # Required: Cluster identification
          clusterName: "acme-staging-cluster"
          location: "me-central1"  # Region or zone
          projectId: "acme-staging"
          
          # Required: Service account with cluster access
          serviceAccount: "${secret:GKE_STAGING_SERVICE_ACCOUNT}"
          
          # Optional: Existing Caddy deployment handling
          caddy:
            skipDeployment: false    # Default: deploy Caddy if not exists
            patchExisting: true      # Default: patch existing Caddy deployment
            deploymentName: "caddy"  # Existing Caddy deployment name
```

**Caddy Deployment Handling**:
When adopting GKE clusters with existing Caddy deployments:

1. **Detection Logic**:
   ```go
   // Check if Caddy deployment exists in cluster
   existingCaddy := checkCaddyDeployment(cluster, "caddy")
   if existingCaddy != nil {
       if config.Caddy.PatchExisting {
           // Patch existing Caddy with new configuration
           patchCaddyDeployment(existingCaddy, newConfig)
       } else if config.Caddy.SkipDeployment {
           // Skip Caddy deployment entirely
           log.Info("Skipping Caddy deployment - using existing")
       }
   } else {
       // Deploy new Caddy instance
       deployCaddy(cluster, config)
   }
   ```

2. **Patching Strategy**:
   - Update Caddy configuration ConfigMap
   - Preserve existing TLS certificates
   - Add new route configurations
   - Trigger rolling update for Caddy pods

3. **Skip vs Patch Decision Matrix**:
   | Scenario | skipDeployment | patchExisting | Behavior |
   |----------|----------------|---------------|----------|
   | New cluster | false | false | Deploy Caddy normally |
   | Existing Caddy, need updates | false | true | Patch existing Caddy |
   | Existing Caddy, no changes | true | false | Skip Caddy entirely |
   | Migration in progress | false | false | Deploy new, migrate traffic |

**Adoption Flow**:
```bash
# 1. Configure cluster with adopt: true in server.yaml (shown above)

# 2. Run normal provision command - SC automatically adopts instead of creating
sc provision -s acme-corp-infrastructure -e staging

# SC detects adopt: true and:
# - Looks up existing GKE cluster
# - Imports it into Pulumi state (doesn't modify cluster)
# - Generates kubeconfig from service account
# - Detects and optionally patches existing Caddy deployment
# - Exports cluster connection details
```

**Service Account Requirements**:
The service account must have the following GCP IAM permissions:
```yaml
# Required GCP IAM roles for adopted GKE cluster
required_roles:
  - roles/container.viewer                  # View cluster details
  - roles/container.clusterViewer          # View cluster resources
  - roles/iam.serviceAccountUser           # Use service account
  
# Additional permissions for Caddy management
  - roles/container.developer              # Deploy/update workloads
```

**Secrets Configuration**:
```yaml
# secrets.yaml - GKE Service Account
values:
  # Service account key for cluster access
  GKE_STAGING_SERVICE_ACCOUNT: |
    {
      "type": "service_account",
      "project_id": "acme-staging",
      "private_key_id": "...",
      "private_key": "...",
      "client_email": "gke-deployer@acme-staging.iam.gserviceaccount.com"
    }
```

**Kubeconfig Access**:
SC generates kubeconfig from adopted cluster configuration:
```go
// Generate kubeconfig for adopted cluster
func generateKubeconfig(cluster *AdoptedGKECluster) (*kubernetes.Provider, error) {
    // Authenticate with service account
    credentials := loadServiceAccount(cluster.ServiceAccount)
    
    // Get cluster endpoint and CA certificate
    clusterInfo := gcp.GetClusterInfo(cluster.ProjectId, cluster.Location, cluster.ClusterName)
    
    // Create Kubernetes provider for cluster
    return kubernetes.NewProvider(ctx, "adopted-gke", &kubernetes.ProviderArgs{
        Kubeconfig: generateKubeconfigFromCluster(clusterInfo, credentials),
    })
}
```

### **2. Natural Adoption via `sc provision`**

#### **How Adoption Works**

**Key Principle**: Resource adoption happens automatically during normal `sc provision` flow when `adopt: true` is detected.

**Workflow**:
```bash
# 1. Configure resources with adopt: true in server.yaml
# 2. Run normal provision command
sc provision -s acme-corp-infrastructure -e staging

# SC automatically:
# - Detects adopt: true flag for each resource
# - Routes to adoption logic instead of creation logic
# - Looks up existing cloud resource
# - Imports into Pulumi state (via sdk.Import())
# - Exports connection details (same format as provisioned resources)
# - Continues to next resource
```

**What This Does**:
1. ✅ Adds cloud resource to Pulumi state without creating it
2. ✅ Records resource metadata (connection details, identifiers)
3. ✅ Enables `${resource:}` syntax for client services
4. ❌ Does NOT modify the actual cloud resource
5. ❌ Does NOT require separate import command
6. ❌ Does NOT create any new infrastructure

**Example Output**:
```bash
$ sc provision -s acme-corp-infrastructure -e staging

Provisioning parent stack...
  ✅ Adopting GKE cluster gke-staging-cluster (not creating)
  ✅ Adopting MongoDB Atlas cluster ACME-Staging (not creating)
  ✅ Adopting Cloud SQL instance acme-postgres-staging (not creating)
  ✅ Creating new GCS bucket media-storage (provisioning)
  
All resources ready. Adopted: 3, Provisioned: 1
```

#### **Optional: Resource Status Command**
```bash
sc resource status --stack <stack-name>
# Output:
# Resource                Type                    Status      Management
# gke-staging            gcp-gke-autopilot       ADOPTED     External
# postgresql-main        gcp-cloudsql-postgres   ADOPTED     External
# mongodb-cluster        mongodb-atlas           ADOPTED     External  
# media-storage          gcp-storage             PROVISIONED SC-Managed
```

### **3. State Management Requirements**

#### **Resource State Schema**
```yaml
# .sc/stacks/<stack>/state.yaml - Extended with adoption tracking
resources:
  postgresql-main:
    type: gcp-cloudsql-postgres
    status: adopted
    management: external
    identifiers:
      instanceName: "postgres-prod"
      connectionName: "acme-staging:me-central1:postgres-prod" 
      cloudResourceId: "projects/acme-staging/instances/postgres-prod"
    properties:
      host: "10.0.1.5"
      port: 5432
      database: "app_production"
    credentials:
      secretName: "POSTGRES_PROD_PASSWORD"
    lastUpdated: "2025-01-15T10:30:00Z"
    adoptedAt: "2025-01-15T10:30:00Z"
```

#### **State File Structure**
```
.sc/stacks/acme-corp-infrastructure/
├── state.yaml              # Extended with adopted resources
├── adopted-resources.yaml  # Detailed adoption metadata
├── provisioned-resources.yaml  # SC-managed resources
└── secrets.yaml            # Credentials for both adopted and provisioned
```

### **4. Resource Processor Extensions**

#### **Unified Resource Interface**
Both adopted and provisioned resources must provide the same `${resource:name.property}` interface:

```bash
# Resource processor output for adopted PostgreSQL
export DATABASE_URL="postgresql://app_user:${POSTGRES_PROD_PASSWORD}@10.0.1.5:5432/app_production"
export POSTGRES_HOST="10.0.1.5"
export POSTGRES_PORT="5432"
export POSTGRES_USER="app_user"
export POSTGRES_PASSWORD="${POSTGRES_PROD_PASSWORD}"
export POSTGRES_DATABASE="app_production"

# Same interface as if SC provisioned it
```

**🔗 See [COMPUTE_PROCESSORS_ADOPTION.md](COMPUTE_PROCESSORS_ADOPTION.md) for detailed technical implementation of unified resource processing.**

#### **Credential Resolution**
```yaml
# secrets.yaml - Mapping existing credentials
values:
  # Adopted resource credentials
  POSTGRES_PROD_PASSWORD: "${POSTGRES_PROD_PASSWORD}"
  MONGODB_ATLAS_URI: "mongodb+srv://user:${MONGO_PASSWORD}@prod-cluster.mongodb.net/app"
  REDIS_PROD_URL: "redis://:${REDIS_AUTH_TOKEN}@10.0.1.6:6379"
  
  # New SC-managed resource credentials (auto-generated)
  ANALYTICS_DB_PASSWORD: "${generated:analytics-db-password}"
```

### **5. Validation and Safety**

#### **Pre-Import Validation**
```bash
sc resource validate --type gcp-cloudsql-postgres \
  --identifier "projects/acme-staging/instances/postgres-prod"
# Checks:
# ✅ Resource exists and is accessible
# ✅ Required permissions available
# ✅ Resource is not already managed by another SC stack
# ✅ Credentials are valid and have necessary access
# ⚠️  Resource is production (requires --confirm-production flag)
```

#### **Import Safety Checks**
1. **Resource Existence**: Verify resource exists and is accessible
2. **Permission Validation**: Confirm SC has read access to resource metadata
3. **State Conflict**: Ensure resource isn't already managed by SC
4. **Production Warning**: Require explicit confirmation for production resources
5. **Credential Testing**: Validate provided credentials work with resource

#### **Rollback Safety**
```bash
sc resource unadopt --stack <stack-name> --resource <resource-name>
# Removes resource from SC state but leaves cloud resource untouched
# Allows reverting adoption if issues arise
```

### **6. Implementation Architecture**

#### **Resource Adoption Flow**
```mermaid
graph TD
    A[sc resource import] --> B{Resource Exists?}
    B -->|No| C[Error: Resource not found]
    B -->|Yes| D{Validate Permissions}
    D -->|Fail| E[Error: Insufficient permissions]
    D -->|Success| F{Check Conflicts}
    F -->|Conflict| G[Error: Resource already managed]
    F -->|Clear| H[Import to SC State]
    H --> I[Update server.yaml with adopt: true]
    I --> J[Generate Credential Templates]
    J --> K[Resource Available via ${resource:}]
```

#### **Resource Provider Extensions**
```go
// Enhanced Resource Provider Interface
type ResourceProvider interface {
    // Existing methods
    Provision(ctx context.Context, config ResourceConfig) (*Resource, error)
    Destroy(ctx context.Context, resource *Resource) error
    
    // NEW: Adoption methods
    Adopt(ctx context.Context, config AdoptionConfig) (*Resource, error)
    Validate(ctx context.Context, identifier string) (*ValidationResult, error)
    GetProperties(ctx context.Context, identifier string) (map[string]interface{}, error)
}

type AdoptionConfig struct {
    ResourceType string
    Identifier   string
    Properties   map[string]interface{}
    Credentials  map[string]string
}
```

### **7. Security and Access Control**

#### **IAM Requirements for Adoption**
```yaml
# GCP IAM permissions required for resource adoption
required_permissions:
  gcp-cloudsql-postgres:
    - "cloudsql.instances.get"
    - "cloudsql.databases.list"
    - "cloudsql.users.list"
  gcp-storage:
    - "storage.buckets.get"
    - "storage.buckets.getIamPolicy"
  gcp-kms:
    - "cloudkms.keyRings.get"
    - "cloudkms.cryptoKeys.get"
    - "cloudkms.cryptoKeys.getIamPolicy"
```

#### **Credential Management**
1. **Existing Credentials**: Map to SC secrets without modification
2. **Access Verification**: Test credentials during adoption
3. **Least Privilege**: Only request minimum permissions needed
4. **Rotation Support**: Support credential rotation for adopted resources

### **8. Error Handling and Recovery**

#### **Common Adoption Errors**
| Error Type | Cause | Resolution |
|-----------|--------|------------|
| `ResourceNotFound` | Identifier incorrect | Verify resource exists in cloud console |
| `InsufficientPermissions` | Missing IAM permissions | Add required permissions to service account |
| `CredentialFailure` | Invalid credentials | Update credentials in secrets.yaml |
| `StateConflict` | Resource already managed | Use different resource name or unadopt first |
| `ProductionWarning` | Production resource | Add `--confirm-production` flag |

#### **Recovery Mechanisms**
```bash
# If adoption fails, rollback is safe
sc resource unadopt --stack acme-corp-infrastructure --resource postgresql-main
# Removes from SC state but leaves cloud resource untouched

# Re-attempt with different configuration
sc resource import --stack acme-corp-infrastructure \
  --resource postgresql-main-v2 \
  --type gcp-cloudsql-postgres \
  --identifier "projects/acme-staging/instances/postgres-prod" \
  --properties connectionName=acme-staging:me-central1:postgres-prod
```

## Success Criteria

### **Functional Requirements**
- ✅ **Resource Import**: Successfully import existing cloud resources into SC state
- ✅ **Unified Interface**: Adopted resources provide same `${resource:}` syntax as provisioned
- ✅ **Credential Management**: Seamless access to adopted resources via SC secrets
- ✅ **State Management**: Track adopted vs provisioned resources separately
- ✅ **Validation**: Comprehensive pre-import validation and safety checks

### **Non-Functional Requirements**
- ✅ **Zero Data Risk**: Adoption never modifies existing resources
- ✅ **Rollback Safety**: Can unadopt resources without affecting cloud resources
- ✅ **Performance**: Import operations complete within 30 seconds
- ✅ **Security**: Follow principle of least privilege for adopted resource access
- ✅ **Documentation**: Complete examples for all supported resource types

### **Enterprise Readiness**
- ✅ **Production Safety**: Explicit confirmation required for production resources
- ✅ **Audit Trail**: Full logging of adoption activities
- ✅ **Multi-Environment**: Support adoption across staging/production environments
- ✅ **Team Collaboration**: Clear separation of adopted vs SC-managed resources

## Migration Impact

With Resource Adoption, the ACME Corp migration becomes:
- **Risk-Free**: No risk of data loss from reprovisioning production resources
- **Gradual**: Can adopt critical resources first, then migrate services
- **Flexible**: Mix adopted existing resources with new SC-managed ones
- **Realistic**: Addresses real-world enterprise migration constraints

This feature transforms Simple Container from a greenfield-only tool into an enterprise-ready platform capable of modernizing complex existing infrastructures safely.
