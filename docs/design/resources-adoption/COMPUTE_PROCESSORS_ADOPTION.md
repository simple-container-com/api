# Compute Processors for Resource Adoption

## Overview

Compute processors are the core mechanism that transforms resource configurations into environment variables and connection details for applications. For resource adoption to work seamlessly, compute processors must provide **identical output** whether a resource was provisioned by Simple Container or adopted from existing infrastructure.

## Current Compute Processor Architecture

### **Provisioned Resource Flow**
```mermaid
graph TD
    A[server.yaml Resource Config] --> B[Resource Provisioner]
    B --> C[Cloud Resource Created] 
    C --> D[Resource State Stored]
    D --> E[Compute Processor]
    E --> F[Environment Variables Generated]
    F --> G[Client Service Access via ${resource:}]
```

### **Current Output Example** (Provisioned PostgreSQL)
```bash
# What SC currently generates for provisioned PostgreSQL
export DATABASE_URL="postgresql://sc_user:auto_generated_password@10.1.2.3:5432/app_db"
export POSTGRES_HOST="10.1.2.3"
export POSTGRES_PORT="5432"
export POSTGRES_USER="sc_user"
export POSTGRES_PASSWORD="auto_generated_password"
export POSTGRES_DATABASE="app_db"
export POSTGRES_SSL_MODE="require"
```

## Enhanced Architecture for Adopted Resources

### **Enhanced Adopted Resource Flow**
```mermaid
graph TD
    A[server.yaml with adopt: true] --> B[Resource Import Command]
    B --> C[Existing Cloud Resource Referenced]
    C --> D[Adoption State Stored]
    D --> E[Enhanced Compute Processor - Adoption Mode]
    E --> F[Uses Existing Credentials/Connection Details]
    F --> G[Same Environment Variables Generated]
    G --> H[Client Service Access via ${resource:} - IDENTICAL]
    
    I[server.yaml with adopt: false] --> J[Pulumi Provisioner]
    J --> K[New Cloud Resource Created]
    K --> L[Enhanced Compute Processor - Provision Mode]
    L --> M[Creates New Users/Databases]
    M --> N[Same Environment Variables Generated]
    N --> O[Client Service Access via ${resource:} - IDENTICAL]
```

### **Key Requirement**: **Identical Output**
The compute processor must generate **identical environment variables** whether the resource is provisioned or adopted, ensuring client services work without modification.

## ⚠️ **CRITICAL REQUIREMENT: Active Database User Creation**

### **The Core Challenge**

**Problem Statement**: Resource adoption is NOT just reading existing connection details—compute processors must **actively create new users** in adopted databases for each service deployment.

**Why This Is Critical**:
- Each service needs its own isolated database user
- Adopted databases don't have service-specific users pre-created
- SC's security model requires separate credentials per service
- Multi-tenant deployments need user isolation

### **Current SC Behavior** (For Provisioned Databases)

When SC provisions a database and a service uses it:

1. **Parent Stack**: Provisions database with root/admin user
2. **Service Deployment**: Compute processor runs:
   ```go
   // Get root credentials from parent stack
   rootUser := parentStack.Outputs["postgres-root-user"]
   rootPassword := parentStack.Outputs["postgres-root-password"]
   
   // Generate service-specific credentials
   serviceUser := serviceName       // e.g., "web-app"
   servicePassword := generateRandom(20)
   
   // Deploy Kubernetes Job to CREATE USER in database
   NewPostgresInitDbUserJob(ctx, serviceUser, InitDbUserJobArgs{
       Namespace: kubernetesNamespace,
       User: DatabaseUser{
           Database: serviceUser,
           Username: serviceUser,
           Password: servicePassword,
       },
       RootUser: rootUser,           // Use root to create user
       RootPassword: rootPassword,
       Host: databaseHost,
       Port: "5432",
   })
   
   // Return connection details with NEW user credentials
   return ConnectionDetails{
       User: serviceUser,
       Password: servicePassword,
       Database: serviceUser,
   }
   ```

### **Required Behavior for Adopted Resources**

**Adopted databases MUST support the exact same user creation flow**:

```yaml
# server.yaml - Adopted PostgreSQL Configuration
postgresql-main:
  type: gcp-cloudsql-postgres
  config:
    adopt: true
    instanceName: "acme-postgres-prod"
    connectionName: "acme-prod:asia-east1:postgres-prod"
    
    # CRITICAL: Root credentials for user creation
    # These come from secrets.yaml, NOT from Pulumi provisioning
    rootCredentials:
      user: "${secret:POSTGRES_ROOT_USER}"
      password: "${secret:POSTGRES_ROOT_PASSWORD}"
```

**Compute Processor Implementation for Adopted Postgres**:
```go
func handleAdoptedPostgres(ctx *sdk.Context, config *AdoptedPostgresConfig, serviceName string) error {
    // Get root credentials from secrets.yaml (not from provisioning)
    rootUser := secrets["POSTGRES_ROOT_USER"]
    rootPassword := secrets["POSTGRES_ROOT_PASSWORD"]
    
    // Generate service-specific credentials (SAME as provisioned)
    serviceUser := serviceName
    servicePassword := generateRandom(20)
    
    // Deploy Kubernetes Job to CREATE USER (IDENTICAL to provisioned flow)
    NewPostgresInitDbUserJob(ctx, serviceUser, InitDbUserJobArgs{
        Namespace: kubernetesNamespace,
        User: DatabaseUser{
            Database: serviceUser,
            Username: serviceUser,
            Password: servicePassword,
        },
        RootUser: rootUser,              // From secrets.yaml
        RootPassword: rootPassword,       // From secrets.yaml
        Host: config.Host,                // From adoption config
        Port: "5432",
        KubeProvider: adoptedClusterProvider,  // Provider for adopted GKE cluster
        InstanceName: config.InstanceName,
    })
    
    // Return IDENTICAL connection details format
    return ConnectionDetails{
        User: serviceUser,
        Password: servicePassword,
        Database: serviceUser,
    }
}
```

### **MongoDB Atlas: Pulumi Provider Pattern**

**Adopted MongoDB Configuration**:
```yaml
mongodb-cluster:
  type: mongodb-atlas
  config:
    adopt: true
    clusterName: "ACME-Production"
    projectId: "507f1f77bcf86cd799439011"
    
    # MongoDB Atlas API credentials for Pulumi provider
    publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
    privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
```

**Compute Processor Implementation**:
```go
func handleAdoptedMongoDB(ctx *sdk.Context, config *AdoptedMongoConfig, serviceName string) error {
    // Get MongoDB Atlas API credentials from secrets.yaml
    publicKey := secrets["MONGODB_ATLAS_PUBLIC_KEY"]
    privateKey := secrets["MONGODB_ATLAS_PRIVATE_KEY"]
    
    // Generate service-specific credentials
    serviceUser := serviceName
    servicePassword := generateRandom(20)
    
    // Create MongoDB Atlas database user via Pulumi provider
    mongoUser, err := mongodbatlas.NewDatabaseUser(ctx, fmt.Sprintf("%s-user", serviceName), 
        &mongodbatlas.DatabaseUserArgs{
            ProjectId:        pulumi.String(config.ProjectId),
            AuthDatabaseName: pulumi.String("admin"),
            Username:         pulumi.String(serviceUser),
            Password:         servicePassword.Result,
            DatabaseName:     pulumi.String(serviceUser),
            Roles: mongodbatlas.DatabaseUserRoleArray{
                &mongodbatlas.DatabaseUserRoleArgs{
                    DatabaseName: pulumi.String(serviceUser),
                    RoleName:     pulumi.String("readWrite"),
                },
                &mongodbatlas.DatabaseUserRoleArgs{
                    DatabaseName: pulumi.String(serviceUser),
                    RoleName:     pulumi.String("dbAdmin"),
                },
            },
        })
    
    if err != nil {
        return errors.Wrapf(err, "failed to create MongoDB user for %s", serviceName)
    }
    
    return ConnectionDetails{
        User: serviceUser,
        Password: servicePassword,
        Database: serviceUser,
    }
}
```

**Why Pulumi Provider Instead of K8s Jobs**:
- ✅ MongoDB Atlas has native Pulumi provider with full API support
- ✅ No need for Kubernetes Jobs or mongosh containers
- ✅ Cleaner architecture - user creation happens during Pulumi apply
- ✅ Better error handling and retry logic via Pulumi

### **GCP Cloud SQL: On-Cluster Job Requirement**

**Critical Architecture Constraint**: For adopted GCP Cloud SQL Postgres instances, user creation **MUST** happen via Kubernetes Jobs running in the adopted GKE cluster.

**Why Kubernetes Jobs Are Required**:
1. **Cloud SQL Proxy**: Jobs use Cloud SQL Proxy sidecar for secure connection
2. **Network Access**: Jobs run in same VPC as Cloud SQL instance
3. **Service Account**: Jobs use GKE workload identity for authentication
4. **Firewall Rules**: Existing firewall rules allow GKE → Cloud SQL traffic

**Job Architecture**:
```go
func NewPostgresInitDbUserJob(ctx *sdk.Context, userName string, args InitDbUserJobArgs) (*InitUserJob, error) {
    // Create Kubernetes Job in ADOPTED GKE cluster
    job, err := batchv1.NewJob(ctx, jobName, &batchv1.JobArgs{
        Metadata: &v1.ObjectMetaArgs{
            Name: sdk.String(jobName),
            Namespace: sdk.String(args.Namespace),
        },
        Spec: &batchv1.JobSpecArgs{
            Template: &corev1.PodTemplateSpecArgs{
                Spec: &corev1.PodSpecArgs{
                    Containers: corev1.ContainerArray{
                        // Container that creates database user
                        &corev1.ContainerArgs{
                            Image: sdk.String("postgres:15-alpine"),
                            Command: sdk.StringArray{
                                sdk.String("psql"),
                                sdk.String(fmt.Sprintf("postgresql://%s:%s@%s:%s/postgres",
                                    args.RootUser, args.RootPassword, args.Host, args.Port)),
                                sdk.String("-c"),
                                sdk.String(fmt.Sprintf(
                                    "CREATE DATABASE %s; CREATE USER %s WITH PASSWORD '%s'; GRANT ALL ON DATABASE %s TO %s;",
                                    args.User.Database, args.User.Username, args.User.Password,
                                    args.User.Database, args.User.Username,
                                )),
                            },
                        },
                    },
                    RestartPolicy: sdk.String("Never"),
                },
            },
        },
    }, sdk.Provider(args.KubeProvider))  // Uses adopted GKE cluster provider
    
    return &InitUserJob{Job: job}, nil
}
```

### **Success Criteria for User Creation**

For adopted resources to work identically to provisioned resources:

1. ✅ **Root Credentials Available**: Compute processor can access root/admin credentials from secrets.yaml
2. ✅ **Kubernetes Job Deployment**: Jobs can be deployed to adopted GKE clusters
3. ✅ **Network Connectivity**: Jobs can connect to adopted databases (Cloud SQL Proxy, VPC peering, etc.)
4. ✅ **User Creation Success**: Jobs successfully create users with proper permissions
5. ✅ **Credential Return**: Service receives connection details with new user credentials
6. ✅ **Identical Interface**: Service code uses `${resource:postgres-main.uri}` identically

### **Validation Checklist**

Before resource adoption is production-ready, verify:

- [ ] Deploy service to adopted GKE cluster
- [ ] Compute processor reads root credentials from secrets.yaml
- [ ] Kubernetes Job deploys to adopted cluster successfully
- [ ] Job connects to adopted database (Postgres/MongoDB)
- [ ] Job creates database and user with correct permissions
- [ ] Service receives environment variables with new credentials
- [ ] Service successfully connects to database with new credentials
- [ ] Multiple services can each get their own isolated users
- [ ] User creation failures are properly handled and reported

## Technical Implementation Required

### **1. Enhanced Compute Processor Interface**

#### **Current Processor Signature (Needs Enhancement)**
```go
func PostgresComputeProcessor(
    ctx *sdk.Context, 
    stack api.Stack, 
    input api.ResourceInput, 
    collector pApi.ComputeContextCollector, 
    params pApi.ProvisionParams
) (*api.ResourceOutput, error)
```

#### **Enhanced Processor Logic Required**
```go
func EnhancedPostgresComputeProcessor(
    ctx *sdk.Context, 
    stack api.Stack, 
    input api.ResourceInput, 
    collector pApi.ComputeContextCollector, 
    params pApi.ProvisionParams
) (*api.ResourceOutput, error) {
    
    // NEW: Check if resource is adopted
    config := input.Descriptor.Config.Config
    if isAdoptedResource(config) {
        return handleAdoptedPostgres(ctx, stack, input, collector, params)
    } else {
        return handleProvisionedPostgres(ctx, stack, input, collector, params)
    }
}

func isAdoptedResource(config interface{}) bool {
    // Check for adopt: true flag in resource configuration
    if adoptable, ok := config.(AdoptableResource); ok {
        return adoptable.GetAdopt()
    }
    return false
}
```

### **2. Resource Configuration Schema Enhancement**

#### **Pulumi-Based State Management (Actual SC Architecture)**
```go
// Simple Container uses Pulumi's standard state system, NOT custom state files
// State is stored in Pulumi backend (GCP bucket, S3, filesystem, or Pulumi Cloud)

// During resource adoption, connection details are exported to Pulumi stack outputs
func adoptPostgresResource(ctx *sdk.Context, cfg *PostgresConfig) error {
    // For adopted resources, export connection details to Pulumi stack
    ctx.Export("postgresql-main-host", sdk.String("10.1.0.5"))
    ctx.Export("postgresql-main-port", sdk.String("5432"))
    ctx.Export("postgresql-main-database", sdk.String("acme_production"))
    ctx.Export("postgresql-main-username", sdk.String("acme_app"))
    ctx.Export("postgresql-main-connection-name", sdk.String("acme-staging:me-central1:postgres-prod"))
    ctx.Export("postgresql-main-ssl-mode", sdk.String("require"))
    
    // Mark as adopted resource with metadata
    ctx.Export("postgresql-main-management-type", sdk.String("adopted"))
    ctx.Export("postgresql-main-cloud-resource-id", sdk.String("projects/acme-staging/instances/postgres-prod"))
    ctx.Export("postgresql-main-adopted-at", sdk.String("2025-01-15T10:30:00Z"))
    
    // Password comes from secrets.yaml, not exported (security)
    return nil
}
```

#### **Provisioned Resource State** (For Comparison)
```go
// For SC-provisioned resources, Pulumi manages the actual cloud resource
func provisionPostgresResource(ctx *sdk.Context, cfg *PostgresConfig) error {
    // Create actual PostgreSQL instance via Pulumi provider
    instance, err := sql.NewDatabaseInstance(ctx, "analytics-db", &sql.DatabaseInstanceArgs{
        Project:        sdk.String(cfg.ProjectId),
        DatabaseVersion: sdk.String("POSTGRES_14"),
        Tier:           sdk.String("db-n1-standard-2"),
        Region:         sdk.String(cfg.Region),
        Settings: &sql.DatabaseInstanceSettingsArgs{
            IpConfiguration: &sql.DatabaseInstanceSettingsIpConfigurationArgs{
                Ipv4Enabled: sdk.Bool(true),
            },
        },
    })
    if err != nil {
        return err
    }

    // Export connection details from actual resource
    ctx.Export("analytics-db-host", instance.IpAddresses.Index(sdk.Int(0)).IpAddress())
    ctx.Export("analytics-db-port", sdk.String("5432"))
    ctx.Export("analytics-db-connection-name", instance.ConnectionName)
    
    // Mark as SC-provisioned
    ctx.Export("analytics-db-management-type", sdk.String("provisioned"))
    ctx.Export("analytics-db-pulumi-resource-urn", instance.URN())
    
    return nil
}
```

### **2. Enhanced Compute Processor Logic**

#### **Unified Processing Interface**
```go
// Enhanced compute processor interface
type ComputeProcessor interface {
    // Existing method enhanced to handle both adopted and provisioned
    GenerateEnvironment(ctx context.Context, resource *Resource, secrets map[string]string) (map[string]string, error)
    
    // New methods for adoption support
    ValidateAdoptedResource(ctx context.Context, config AdoptionConfig) error
    MapAdoptedResourceProperties(ctx context.Context, config AdoptionConfig) (*ResourceConnection, error)
}

type ResourceConnection struct {
    Host        string
    Port        int
    Database    string
    Username    string
    SSLMode     string
    ExtraParams map[string]string
}
```

#### **PostgreSQL Compute Processor Implementation**
```go
func (p *PostgreSQLProcessor) GenerateEnvironment(ctx context.Context, resource *Resource, secrets map[string]string) (map[string]string, error) {
    env := make(map[string]string)
    
    // Get connection details (same logic for adopted and provisioned)
    conn := resource.Connection
    password := secrets[resource.Credentials.PasswordSecretName]
    
    // Generate identical environment variables regardless of management type
    env["DATABASE_URL"] = fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=%s",
        conn.Username, password, conn.Host, conn.Port, conn.Database, conn.SSLMode)
    env["POSTGRES_HOST"] = conn.Host
    env["POSTGRES_PORT"] = strconv.Itoa(conn.Port)
    env["POSTGRES_USER"] = conn.Username
    env["POSTGRES_PASSWORD"] = password
    env["POSTGRES_DATABASE"] = conn.Database
    env["POSTGRES_SSL_MODE"] = conn.SSLMode
    
    // Add SSL cert if provided (common for adopted resources)
    if caCert, exists := secrets[resource.Credentials.CaCertSecretName]; exists && caCert != "" {
        env["POSTGRES_CA_CERT"] = caCert
        env["POSTGRES_SSL_CERT"] = caCert  // Compatibility alias
    }
    
    return env, nil
}
```

### **3. Resource Import and Pulumi Export Process**

#### **Import Command Processing**
```bash
# Import command adds adoption metadata to Pulumi stack
sc resource import --stack acme-corp-infrastructure \
  --resource postgresql-main \
  --type gcp-cloudsql-postgres \
  --identifier "projects/acme-staging/instances/postgres-prod" \
  --connection-details database=acme_production,username=acme_app
```

#### **Import Implementation (New)**
```go
// pkg/clouds/pulumi/import.go - NEW FILE NEEDED
func ImportResource(ctx *sdk.Context, resourceName string, config ResourceImportConfig) error {
    // Query cloud provider for resource details
    instance, err := getCloudResourceDetails(ctx, config.Identifier)
    if err != nil {
        return fmt.Errorf("failed to get resource details: %w", err)
    }
    
    // Export adoption metadata to Pulumi stack - CRITICAL for compute processors
    ctx.Export(fmt.Sprintf("%s-host", resourceName), sdk.String(instance.Host))
    ctx.Export(fmt.Sprintf("%s-port", resourceName), sdk.String(instance.Port))
    ctx.Export(fmt.Sprintf("%s-database", resourceName), sdk.String(config.Database))
    ctx.Export(fmt.Sprintf("%s-username", resourceName), sdk.String(config.Username))
    ctx.Export(fmt.Sprintf("%s-connection-name", resourceName), sdk.String(instance.ConnectionName))
    
    // Adoption metadata
    ctx.Export(fmt.Sprintf("%s-management-type", resourceName), sdk.String("adopted"))
    ctx.Export(fmt.Sprintf("%s-cloud-resource-id", resourceName), sdk.String(config.Identifier))
    ctx.Export(fmt.Sprintf("%s-adopted-at", resourceName), sdk.String(time.Now().Format(time.RFC3339)))
    
    // Password is NOT exported - comes from secrets.yaml for security
    
    return nil
}
```

#### **Auto-Detection Logic**
```go
func (p *PostgreSQLProcessor) MapAdoptedResourceProperties(ctx context.Context, config AdoptionConfig) (*ResourceConnection, error) {
    // Query cloud provider for resource details
    instance, err := p.gcpClient.GetSQLInstance(ctx, config.Identifier)
    if err != nil {
        return nil, fmt.Errorf("failed to get instance details: %w", err)
    }
    
    // Map cloud resource properties to SC resource connection
    return &ResourceConnection{
        Host:        instance.IPAddresses[0].IPAddress,  // Primary IP
        Port:        5432,  // PostgreSQL default
        Database:    config.Properties["database"],      // From user input
        Username:    config.Properties["username"],      // From user input
        SSLMode:     "require",  // Default for Cloud SQL
        ConnectionName: instance.ConnectionName,
    }, nil
}
```

#### **Credential Discovery and Mapping**
```yaml
# secrets.yaml - Manual mapping for adopted resources
values:
  # Adopted PostgreSQL credentials (existing)
  POSTGRES_PROD_PASSWORD: "${POSTGRES_PROD_PASSWORD}"  # Existing env var
  POSTGRES_PROD_CA_CERT: |
    -----BEGIN CERTIFICATE-----
    MIIDbTCCAlWgAwIBAgIJAJ... (existing cert)
    -----END CERTIFICATE-----
  
  # New SC-managed resource credentials (auto-generated)
  ANALYTICS_DB_PASSWORD: "${generated:analytics-db-password}"
```

### **4. Resource Resolution During Deployment**

#### **Client.yaml Processing**
```yaml
# client.yaml - sample-app service
stacks:
  staging:
    config:
      secrets:
        DATABASE_URL: ${resource:postgresql-main.uri}      # Adopted resource
        ANALYTICS_DB: ${resource:analytics-db.uri}         # Provisioned resource
        REDIS_URL: ${resource:redis-primary.uri}           # Adopted resource
```

#### **Resolution Process**
```go
func (r *ResourceResolver) ResolveResourceReference(resourceRef string) (string, error) {
    // Parse reference: "postgresql-main.uri"
    parts := strings.Split(resourceRef, ".")
    resourceName, property := parts[0], parts[1]
    
    // Get resource from state (adopted or provisioned)
    resource := r.state.Resources[resourceName]
    
    // Generate environment using compute processor (same logic for both)
    env, err := r.computeProcessor.GenerateEnvironment(ctx, resource, r.secrets)
    if err != nil {
        return "", err
    }
    
    // Return requested property
    switch property {
    case "uri":
        return env["DATABASE_URL"], nil
    case "host":
        return env["POSTGRES_HOST"], nil
    case "port":
        return env["POSTGRES_PORT"], nil
    case "user":
        return env["POSTGRES_USER"], nil
    // ... other properties
    }
}
```

### **5. Output Comparison: Adopted vs Provisioned**

#### **Adopted PostgreSQL Output**
```bash
# Generated for adopted PostgreSQL (existing production database)
export DATABASE_URL="postgresql://acme_app:existing_password@10.1.0.5:5432/acme_production?sslmode=require"
export POSTGRES_HOST="10.1.0.5"
export POSTGRES_PORT="5432"
export POSTGRES_USER="acme_app"
export POSTGRES_PASSWORD="existing_password"
export POSTGRES_DATABASE="acme_production"
export POSTGRES_SSL_MODE="require"
export POSTGRES_CA_CERT="-----BEGIN CERTIFICATE-----\nMIIDbT..."
```

#### **Provisioned PostgreSQL Output**
```bash
# Generated for SC-provisioned PostgreSQL (new analytics database)
export DATABASE_URL="postgresql://sc_analytics_user:auto_generated_pass@10.1.0.8:5432/analytics?sslmode=require"
export POSTGRES_HOST="10.1.0.8"
export POSTGRES_PORT="5432"  
export POSTGRES_USER="sc_analytics_user"
export POSTGRES_PASSWORD="auto_generated_pass"
export POSTGRES_DATABASE="analytics"
export POSTGRES_SSL_MODE="require"
```

#### **Key Observation**: **Identical Structure, Different Values**
Both outputs have the same environment variable names and structure, ensuring client applications work identically regardless of resource origin.

### **6. Multi-Provider Support**

#### **MongoDB Atlas Adopted Resource**
```yaml
# server.yaml
mongodb-cluster:
  type: mongodb-atlas
  config:
    adopt: true
    clusterName: "ACME-Corp-Production"
    projectId: "507f1f77bcf86cd799439011"
```

#### **MongoDB Compute Processor Output**
```bash
# Generated environment variables (adopted MongoDB Atlas)
export MONGODB_URI="mongodb+srv://acme_user:existing_pass@acme-production.mongodb.net/acme_db?retryWrites=true&w=majority"
export MONGO_HOST="acme-production.mongodb.net"
export MONGO_PORT="27017"
export MONGO_USER="acme_user"
export MONGO_PASSWORD="existing_pass"
export MONGO_DATABASE="acme_db"
export MONGO_REPLICA_SET="acme-production"
```

#### **Redis Adopted Resource**
```bash
# Generated environment variables (adopted GCP Memorystore Redis)
export REDIS_URL="redis://:existing_auth_token@10.1.0.6:6379"
export REDIS_HOST="10.1.0.6"
export REDIS_PORT="6379"
export REDIS_AUTH_TOKEN="existing_auth_token"
export REDIS_SSL="false"
```

### **7. Error Handling and Validation**

#### **Adoption Validation Checks**
```go
func (p *PostgreSQLProcessor) ValidateAdoptedResource(ctx context.Context, config AdoptionConfig) error {
    // 1. Verify resource exists and is accessible
    instance, err := p.gcpClient.GetSQLInstance(ctx, config.Identifier)
    if err != nil {
        return fmt.Errorf("resource not accessible: %w", err)
    }
    
    // 2. Test connectivity with provided credentials
    testConn := &ResourceConnection{
        Host:     instance.IPAddresses[0].IPAddress,
        Port:     5432,
        Database: config.Properties["database"],
        Username: config.Properties["username"],
    }
    
    password := config.Credentials["password"]
    if err := p.testConnection(testConn, password); err != nil {
        return fmt.Errorf("credential validation failed: %w", err)
    }
    
    // 3. Check if resource is already managed
    if p.isResourceManaged(config.Identifier) {
        return fmt.Errorf("resource already managed by another SC stack")
    }
    
    return nil
}
```

#### **Runtime Error Recovery**
```go
func (p *PostgreSQLProcessor) GenerateEnvironment(ctx context.Context, resource *Resource, secrets map[string]string) (map[string]string, error) {
    // Handle missing credentials gracefully
    password, exists := secrets[resource.Credentials.PasswordSecretName]
    if !exists {
        return nil, fmt.Errorf("missing required secret: %s. For adopted resources, ensure credentials are mapped in secrets.yaml", 
            resource.Credentials.PasswordSecretName)
    }
    
    // Validate adopted resource is still accessible
    if resource.Management == "adopted" {
        if err := p.validateResourceAccess(ctx, resource); err != nil {
            return nil, fmt.Errorf("adopted resource no longer accessible: %w. Check cloud resource status and credentials", err)
        }
    }
    
    // Continue with normal processing...
}
```

### **8. Migration Example: ACME Corp Sample-App Service**

#### **Before Migration** (Current Pulumi-based)
```javascript
// Current sample-app service gets credentials from Pulumi stack output
const databaseUrl = process.env.DATABASE_URL || pulumiOutput.postgresConnectionString;
const mongoUrl = process.env.MONGO_URL || pulumiOutput.mongoAtlasUri;
const redisUrl = process.env.REDIS_URL || pulumiOutput.redisInstanceUri;
```

#### **After Migration** (Simple Container with adopted resources)
```yaml
# client.yaml - sample-app
stacks:
  staging:
    config:
      secrets:
        # Seamless access to adopted production resources
        DATABASE_URL: ${resource:postgresql-main.uri}      # Adopted existing PostgreSQL
        MONGO_URL: ${resource:mongodb-cluster.uri}         # Adopted existing MongoDB Atlas
        REDIS_URL: ${resource:redis-primary.uri}           # Adopted existing Redis
        
        # Mixed with new SC-managed resources
        ANALYTICS_DB: ${resource:analytics-db.uri}         # New SC-provisioned PostgreSQL
        METRICS_STORE: ${resource:metrics-redis.uri}       # New SC-provisioned Redis
```

#### **Runtime Environment** (Generated by compute processors)
```bash
# What the sample-app container receives at runtime
DATABASE_URL="postgresql://acme_app:prod_pass@10.1.0.5:5432/acme_production"
MONGO_URL="mongodb+srv://acme_user:atlas_pass@acme-prod.mongodb.net/acme_db"
REDIS_URL="redis://:redis_auth@10.1.0.6:6379"
ANALYTICS_DB="postgresql://sc_analytics:auto_gen_pass@10.1.0.8:5432/analytics"
METRICS_STORE="redis://:auto_gen_auth@10.1.0.9:6379"
```

#### **Application Code** (No Changes Needed)
```javascript
// Application code remains identical - no changes needed!
const databaseUrl = process.env.DATABASE_URL;    // Works with adopted resource
const mongoUrl = process.env.MONGO_URL;          // Works with adopted resource  
const redisUrl = process.env.REDIS_URL;          // Works with adopted resource
const analyticsDb = process.env.ANALYTICS_DB;    // Works with SC-provisioned resource
```

## Success Criteria

### **Functional Requirements**
✅ **Identical Interface**: Adopted and provisioned resources provide identical environment variable structure  
✅ **Seamless Access**: Client services access adopted resources via same `${resource:}` syntax  
✅ **Credential Management**: Existing credentials mapped to SC secrets without modification  
✅ **Property Resolution**: All resource properties (uri, host, port, etc.) work identically  
✅ **Multi-Provider**: Support for GCP, AWS, MongoDB Atlas, and other providers  

### **Non-Functional Requirements**
✅ **Zero Application Changes**: Existing applications work without code modifications  
✅ **Performance**: Resource resolution adds <50ms overhead  
✅ **Reliability**: Graceful handling of missing credentials or inaccessible resources  
✅ **Security**: Credentials never logged or exposed in plain text  
✅ **Debugging**: Clear error messages when adopted resources are misconfigured  

### **Migration Benefits**
✅ **Production Safety**: Use existing resources without risk of data loss  
✅ **Gradual Adoption**: Mix adopted and provisioned resources in same stack  
✅ **Operational Continuity**: Applications continue working during migration  
✅ **Developer Experience**: Same Simple Container patterns for all resources  

## Critical Implementation Changes Required

### **1. Resource Configuration Schema Extensions**

All existing resource configurations need adoption support:

```go
// pkg/clouds/gcloud/postgres.go - NEEDS ENHANCEMENT
type PostgresGcpCloudsqlConfig struct {
    // Existing fields...
    ProjectId  string `json:"projectId" yaml:"projectId"`
    Region     *string `json:"region,omitempty" yaml:"region,omitempty"`
    
    // NEW: Required for resource adoption
    Adopt bool `json:"adopt,omitempty" yaml:"adopt,omitempty"`
    
    // NEW: Connection details for adopted resources  
    InstanceName   string `json:"instanceName,omitempty" yaml:"instanceName,omitempty"`
    ConnectionName string `json:"connectionName,omitempty" yaml:"connectionName,omitempty"`
    Database       string `json:"database,omitempty" yaml:"database,omitempty"`
    Username       string `json:"username,omitempty" yaml:"username,omitempty"`
    // Password comes from secrets.yaml, not config
}

// pkg/clouds/mongodb/atlas.go - NEEDS ENHANCEMENT  
type AtlasConfig struct {
    // Existing fields...
    PublicKey   string `json:"publicKey" yaml:"publicKey"`
    PrivateKey  string `json:"privateKey" yaml:"privateKey"`
    
    // NEW: Required for resource adoption
    Adopt bool `json:"adopt,omitempty" yaml:"adopt,omitempty"`
    
    // NEW: Connection details for adopted resources
    ClusterName string `json:"clusterName,omitempty" yaml:"clusterName,omitempty"`
    ProjectId   string `json:"projectId,omitempty" yaml:"projectId,omitempty"`
    URI         string `json:"uri,omitempty" yaml:"uri,omitempty"`
    // Username/Password come from secrets.yaml
}

// pkg/clouds/aws/s3.go - NEEDS ENHANCEMENT
type S3Bucket struct {
    // Existing fields...
    Name string `json:"name" yaml:"name"`
    
    // NEW: Required for resource adoption
    Adopt bool `json:"adopt,omitempty" yaml:"adopt,omitempty"`
    
    // NEW: Connection details for adopted buckets
    BucketName string `json:"bucketName,omitempty" yaml:"bucketName,omitempty"`
    Region     string `json:"region,omitempty" yaml:"region,omitempty"`
    // Access keys come from secrets.yaml
}
```

### **2. Enhanced Compute Processor Logic**

Each processor needs adoption-aware logic:

```go
// pkg/clouds/pulumi/gcp/compute_proc.go - CRITICAL CHANGES NEEDED
func PostgresComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
    pgCfg, ok := input.Descriptor.Config.Config.(*gcloud.PostgresGcpCloudsqlConfig)
    if !ok {
        return nil, errors.Errorf("failed to convert postgresql config for %q", input.Descriptor.Type)
    }
    
    // NEW: Check if this is an adopted resource
    if pgCfg.Adopt {
        return handleAdoptedPostgres(ctx, stack, input, collector, params, pgCfg)
    } else {
        return handleProvisionedPostgres(ctx, stack, input, collector, params, pgCfg) // Current logic
    }
}

func handleAdoptedPostgres(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams, cfg *gcloud.PostgresGcpCloudsqlConfig) (*api.ResourceOutput, error) {
    // NEW: For adopted resources, query connection details from parent stack exports
    // Parent stack exports adoption metadata during 'sc resource import'
    
    parentStackName := params.ParentStack.StackName
    resourceName := input.Descriptor.Name
    
    // Create StackReference to parent stack (same as provisioned flow)  
    parentRef, err := sdk.NewStackReference(ctx, fmt.Sprintf("%s--%s--adopted-ref", stack.Name, resourceName), &sdk.StackReferenceArgs{
        Name: sdk.String(params.ParentStack.FullReference).ToStringOutput(),
    })
    if err != nil {
        return nil, err
    }
    
    // Query adopted resource connection details from parent stack exports
    hostExport := fmt.Sprintf("%s-host", resourceName)
    host, err := pApi.GetParentOutput(parentRef, hostExport, params.ParentStack.FullReference, false)
    if err != nil {
        return nil, errors.Wrapf(err, "failed to get adopted postgres host from parent stack")
    }
    
    portExport := fmt.Sprintf("%s-port", resourceName) 
    port, err := pApi.GetParentOutput(parentRef, portExport, params.ParentStack.FullReference, false)
    if err != nil {
        return nil, errors.Wrapf(err, "failed to get adopted postgres port from parent stack")
    }
    
    databaseExport := fmt.Sprintf("%s-database", resourceName)
    database, err := pApi.GetParentOutput(parentRef, databaseExport, params.ParentStack.FullReference, false)
    if err != nil {
        return nil, errors.Wrapf(err, "failed to get adopted postgres database from parent stack")
    }
    
    usernameExport := fmt.Sprintf("%s-username", resourceName)
    username, err := pApi.GetParentOutput(parentRef, usernameExport, params.ParentStack.FullReference, false)
    if err != nil {
        return nil, errors.Wrapf(err, "failed to get adopted postgres username from parent stack")
    }
    
    // Password comes from secrets.yaml (not from stack exports for security)
    passwordSecretKey := fmt.Sprintf("POSTGRES_%s_PASSWORD", strings.ToUpper(resourceName))
    
    collector.AddEnvVariableIfNotExist("POSTGRES_HOST", host, input.Descriptor.Type, input.Descriptor.Name, parentStackName)
    collector.AddEnvVariableIfNotExist("POSTGRES_PORT", port, input.Descriptor.Type, input.Descriptor.Name, parentStackName)
    collector.AddEnvVariableIfNotExist("POSTGRES_USER", username, input.Descriptor.Type, input.Descriptor.Name, parentStackName)
    collector.AddEnvVariableIfNotExist("POSTGRES_DATABASE", database, input.Descriptor.Type, input.Descriptor.Name, parentStackName)
    collector.AddSecretEnvVariableIfNotExist("POSTGRES_PASSWORD", fmt.Sprintf("${secret:%s}", passwordSecretKey), input.Descriptor.Type, input.Descriptor.Name, parentStackName)
    
    // Generate same DATABASE_URL format as provisioned resources
    databaseUrl := fmt.Sprintf("postgresql://%s:${secret:%s}@%s:%s/%s?sslmode=require", username, passwordSecretKey, host, port, database)
    collector.AddSecretEnvVariableIfNotExist("DATABASE_URL", databaseUrl, input.Descriptor.Type, input.Descriptor.Name, parentStackName)
    
    collector.AddResourceTplExtension(input.Descriptor.Name, map[string]string{
        "host":     host,
        "port":     port,
        "user":     username, 
        "database": database,
        "uri":      databaseUrl,
        // password intentionally omitted for security
    })
    
    return &api.ResourceOutput{Ref: parentStackName}, nil
}
```

### **3. MongoDB Atlas Processor Enhancement**

```go
// pkg/clouds/pulumi/mongodb/compute_proc.go - CRITICAL CHANGES NEEDED
func ClusterComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
    mongoConfig, ok := input.Descriptor.Config.Config.(*mongodb.AtlasConfig)
    if !ok {
        return nil, errors.Errorf("failed to convert mongodb config for %q", input.Descriptor.Type)
    }
    
    // NEW: Check if this is an adopted resource
    if mongoConfig.Adopt {
        return handleAdoptedMongoDB(ctx, stack, input, collector, params, mongoConfig)
    } else {
        return handleProvisionedMongoDB(ctx, stack, input, collector, params, mongoConfig) // Current logic  
    }
}

func handleAdoptedMongoDB(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams, cfg *mongodb.AtlasConfig) (*api.ResourceOutput, error) {
    // For adopted MongoDB, use existing URI and credentials
    dbName := stack.Name
    userName := stack.Name
    
    // Get credentials from secrets
    uriSecretKey := fmt.Sprintf("MONGODB_%s_URI", strings.ToUpper(input.Descriptor.Name))
    passwordSecretKey := fmt.Sprintf("MONGODB_%s_PASSWORD", strings.ToUpper(input.Descriptor.Name))
    
    collector.AddEnvVariableIfNotExist("MONGO_USER", userName, input.Descriptor.Type, input.Descriptor.Name, "")
    collector.AddEnvVariableIfNotExist("MONGO_DATABASE", dbName, input.Descriptor.Type, input.Descriptor.Name, "")
    collector.AddSecretEnvVariableIfNotExist("MONGO_PASSWORD", fmt.Sprintf("${secret:%s}", passwordSecretKey), input.Descriptor.Type, input.Descriptor.Name, "")
    collector.AddSecretEnvVariableIfNotExist("MONGO_URI", fmt.Sprintf("${secret:%s}", uriSecretKey), input.Descriptor.Type, input.Descriptor.Name, "")
    
    collector.AddResourceTplExtension(input.Descriptor.Name, map[string]string{
        "uri":    fmt.Sprintf("${secret:%s}", uriSecretKey),
        "user":   userName,
        "dbName": dbName,
        // password intentionally omitted for security
    })
    
    return &api.ResourceOutput{Ref: nil}, nil
}
```

### **4. AWS S3 Processor Enhancement**

```go
// pkg/clouds/pulumi/aws/compute_proc.go - ADD NEW FUNCTION
func S3BucketAdoptedComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
    bucketCfg, ok := input.Descriptor.Config.Config.(*aws.S3Bucket)
    if !ok {
        return nil, errors.Errorf("failed to convert bucket config for %q", input.Descriptor.Type)
    }
    
    if bucketCfg.Adopt {
        // For adopted S3 buckets, use existing bucket name and credentials from secrets
        bucketName := bucketCfg.BucketName
        region := bucketCfg.Region
        
        accessKeySecretKey := fmt.Sprintf("S3_%s_ACCESS_KEY", strings.ToUpper(input.Descriptor.Name))
        secretKeySecretKey := fmt.Sprintf("S3_%s_SECRET_KEY", strings.ToUpper(input.Descriptor.Name))
        
        collector.AddEnvVariableIfNotExist("S3_BUCKET", bucketName, input.Descriptor.Type, input.Descriptor.Name, "")
        collector.AddEnvVariableIfNotExist("S3_REGION", region, input.Descriptor.Type, input.Descriptor.Name, "")
        collector.AddSecretEnvVariableIfNotExist("S3_ACCESS_KEY", fmt.Sprintf("${secret:%s}", accessKeySecretKey), input.Descriptor.Type, input.Descriptor.Name, "")
        collector.AddSecretEnvVariableIfNotExist("S3_SECRET_KEY", fmt.Sprintf("${secret:%s}", secretKeySecretKey), input.Descriptor.Type, input.Descriptor.Name, "")
        
        collector.AddResourceTplExtension(input.Descriptor.Name, map[string]string{
            "bucket": bucketName,
            "region": region,
            // keys intentionally omitted for security
        })
        
        return &api.ResourceOutput{Ref: nil}, nil
    } else {
        // Use existing provisioned logic
        return S3BucketComputeProcessor(ctx, stack, input, collector, params)
    }
}
```

### **5. Processor Registration Updates**

```go
// pkg/clouds/pulumi/gcp/init.go - NEEDS UPDATES
func (p *Provider) GetComputeProcessors() map[string]pApi.ComputeProcessorFunc {
    return map[string]pApi.ComputeProcessorFunc{
        "gcp-cloudsql-postgres": PostgresComputeProcessor, // Enhanced version
        "gcp-redis":            RedisComputeProcessor,     // Needs enhancement
        "gcp-bucket":           BucketComputeProcessor,    // Needs enhancement
        // ... other processors
    }
}
```

## Implementation Priority

**Critical Changes Needed (Priority Order)**:
1. **PostgreSQL Processors** (GCP CloudSQL, AWS RDS) - Production databases
2. **MongoDB Atlas Processor** - Cannot recreate production data
3. **Redis Processors** (GCP Memorystore, AWS ElastiCache) - Session data  
4. **Storage Processors** (GCS, S3) - File uploads and media
5. **KMS Processors** - Encryption key access

**Key Success Criteria**:
- ✅ **Identical environment variable output** for adopted vs provisioned resources
- ✅ **No parent stack dependencies** for adopted resources
- ✅ **Credential mapping** from secrets.yaml instead of generation
- ✅ **Resource template extensions** work identically for both types
- ✅ **Error handling** for missing credentials or inaccessible adopted resources

## **✅ CORRECTED ARCHITECTURE UNDERSTANDING**

### **Key Correction: Pulumi-Based State Management**

**❌ WRONG Initial Assumption**: Custom `.sc/stacks/*/state.yaml` files
**✅ CORRECT Architecture**: Pulumi's standard state system (GCP bucket, S3, filesystem, Pulumi Cloud)

### **Actual Implementation Flow**

1. **Resource Import**: `sc resource import` calls Pulumi program that exports connection details
2. **Pulumi State**: All metadata stored in Pulumi backend (same as existing SC architecture)  
3. **Stack Exports**: Parent stack exports adopted resource connection details via `ctx.Export()`
4. **Compute Processors**: Query parent stack exports using `sdk.NewStackReference()` (existing pattern)
5. **Client Access**: Same `${resource:name.property}` syntax works for adopted and provisioned resources

### **Critical Insight**
The compute processor enhancement aligns perfectly with Simple Container's existing Pulumi-based architecture. **No new state management system needed** - just enhanced logic to handle `adopt: true` resources by querying different parent stack exports.

This enhancement transforms compute processors from **provision-only** to **adoption-aware**, enabling seamless migration of existing production infrastructure to Simple Container **while maintaining full compatibility with SC's Pulumi foundation**.
