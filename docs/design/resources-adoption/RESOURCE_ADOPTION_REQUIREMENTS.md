# Resource Adoption Requirements

## Overview

Resource Adoption is a **critical enterprise feature** that allows Simple Container to reference and manage existing cloud resources without reprovisioning them. This enables brownfield migrations where production databases, storage, and other resources contain live data that cannot be recreated.

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

### **1. Configuration Schema Extensions**

#### **Server.yaml Schema Changes**
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
| Resource Type           | Adopt Support | Import Command       | Required Identifiers             |
|-------------------------|---------------|----------------------|----------------------------------|
| `gcp-cloudsql-postgres` | ✅ Required    | `sc resource import` | `instanceName`, `connectionName` |
| `gcp-cloudsql-mysql`    | ✅ Required    | `sc resource import` | `instanceName`, `connectionName` |
| `mongodb-atlas`         | ✅ Required    | `sc resource import` | `clusterName`, `projectId`       |
| `gcp-memorystore-redis` | ✅ Required    | `sc resource import` | `instanceId`, `region`           |
| `gcp-storage`           | ✅ Required    | `sc resource import` | `bucketName`                     |
| `gcp-kms`               | ✅ Critical    | `sc resource import` | `keyRing`, `cryptoKeys[]`        |
| `aws-rds-postgres`      | ✅ Required    | `sc resource import` | `dbInstanceIdentifier`           |
| `aws-elasticache-redis` | ✅ Required    | `sc resource import` | `cacheClusterId`                 |
| `aws-s3`                | ✅ Required    | `sc resource import` | `bucketName`                     |

### **2. CLI Command Extensions**

#### **Resource Import Command**
```bash
sc resource import --stack <stack-name> \
  --resource <resource-name> \
  --type <resource-type> \
  --identifier <cloud-resource-id> \
  [--properties key=value]

# Examples:
sc resource import --stack acme-corp-infrastructure \
  --resource postgresql-main \
  --type gcp-cloudsql-postgres \
  --identifier "projects/acme-staging/instances/postgres-prod" \
  --properties connectionName=acme-staging:me-central1:postgres-prod

sc resource import --stack acme-corp-infrastructure \
  --resource mongodb-cluster \
  --type mongodb-atlas \
  --identifier "cluster-id-12345" \
  --properties projectId=507f1f77bcf86cd799439011
```

#### **Resource Status Command**
```bash
sc resource status --stack <stack-name>
# Output:
# Resource                Type                    Status      Management
# postgresql-main        gcp-cloudsql-postgres   ADOPTED     External
# mongodb-cluster        mongodb-atlas           ADOPTED     External  
# analytics-db           gcp-cloudsql-postgres   PROVISIONED SC-Managed
# storage-new            gcp-storage             PROVISIONED SC-Managed
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
