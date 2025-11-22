# Resource Adoption Guide

Adopt existing cloud infrastructure into Simple Container without downtime or modifications. This guide shows how to import existing resources and immediately gain access to Simple Container's deployment and management capabilities.

## 🎯 **What is Resource Adoption?**

Resource adoption allows you to **import existing cloud resources** into Simple Container's management without creating new resources or modifying existing ones. Your applications continue running unchanged while gaining access to Simple Container's features.

### **Supported Resources**

| Resource Type          | Provider     | Description                   | Status      |
|------------------------|--------------|-------------------------------|-------------|
| **MongoDB Atlas**      | MongoDB      | Existing Atlas clusters       | ✅ Available |
| **Cloud SQL Postgres** | Google Cloud | Existing PostgreSQL instances | ✅ Available |
| **Redis Memorystore**  | Google Cloud | Existing Redis instances      | ✅ Available |
| **GKE Autopilot**      | Google Cloud | Existing Kubernetes clusters  | ✅ Available |
| **GCS Buckets**        | Google Cloud | Existing storage buckets      | ✅ Available |

## 🚀 **Quick Start**

### **Step 1: Identify Resources to Adopt**

List your existing resources that you want to adopt:

```bash
# MongoDB Atlas - Get cluster names
mongocli atlas clusters list --projectId YOUR_PROJECT_ID

# GCP Cloud SQL - Get instance names  
gcloud sql instances list

# GCP Redis - Get instance IDs
gcloud redis instances list --region=your-region

# GKE Clusters - Get cluster names
gcloud container clusters list

# GCS Buckets - Get bucket names
gcloud storage buckets list
```

### **Step 2: Configure Resource Adoption**

Add `adopt: true` and resource identifiers to your `server.yaml`:

```yaml
# server.yaml
resources:
  resources:
    prod:
      # Adopt existing MongoDB Atlas cluster
      mongodb:
        type: mongodb-atlas
        config:
          adopt: true                           # Enable adoption
          clusterName: "your-existing-cluster"  # Existing cluster name
          orgId: "${secret:MONGODB_ATLAS_ORG_ID}"
          projectId: "${secret:MONGODB_ATLAS_PROJECT_ID}"
          # ... other Atlas configuration

      # Adopt existing Cloud SQL instance
      postgres:
        type: gcp-cloudsql-postgres
        config:
          adopt: true                           # Enable adoption
          instanceName: "your-existing-instance"
          connectionName: "project:region:instance"
          rootPassword: "${secret:POSTGRES_ROOT_PASSWORD}"
          # ... other PostgreSQL configuration
```

### **Step 3: Deploy and Import**

```bash
# Deploy the parent stack - imports existing resources
sc provision -s infrastructure

# Expected output:
# ✅ Adopting MongoDB Atlas cluster your-existing-cluster (not creating)
# ✅ Adopting Cloud SQL instance your-existing-instance (not creating)
# ✅ Creating Artifact Registry (provisioning new resource)
```

### **Step 4: Deploy Services**

Your services can now use the adopted resources:

```yaml
# client.yaml
stacks:
  prod:
    type: cloud-compose
    parent: your-org/infrastructure
    config:
      uses:
        - mongodb    # Uses adopted MongoDB cluster
        - postgres   # Uses adopted PostgreSQL instance
      # ... service configuration
```

## 📋 **Resource-Specific Adoption**

### **MongoDB Atlas Adoption**

Adopt existing MongoDB Atlas clusters:

```yaml
mongodb:
  type: mongodb-atlas
  config:
    adopt: true
    clusterName: "production-cluster-abc123"  # Existing cluster name
    orgId: "${secret:MONGODB_ATLAS_ORG_ID}"
    projectId: "${secret:MONGODB_ATLAS_PROJECT_ID}"
    projectName: "Production MongoDB Project"
    region: "WESTERN_EUROPE"
    cloudProvider: GCP
    privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
    publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
```

**Requirements:**
- MongoDB Atlas API keys with project access
- Existing cluster name and project ID
- Cluster must be accessible from your applications

### **GCP Cloud SQL Postgres Adoption**

Adopt existing PostgreSQL instances:

```yaml
postgres:
  type: gcp-cloudsql-postgres
  config:
    adopt: true
    instanceName: "production-postgres-instance"
    connectionName: "my-project:us-central1:production-postgres-instance"
    rootPassword: "${secret:POSTGRES_ROOT_PASSWORD}"
    projectId: "${auth:gcloud.projectId}"
    credentials: "${auth:gcloud}"
    version: "POSTGRES_14"
    region: "us-central1"
    usersProvisionRuntime:
      type: "gke"
      resourceName: "gke-cluster"
```

**Requirements:**
- GCP service account with Cloud SQL Admin permissions
- Existing instance name and connection name
- Root password for database user creation

### **GCP Redis Adoption**

Adopt existing Redis Memorystore instances:

```yaml
redis:
  type: gcp-redis
  config:
    adopt: true
    instanceId: "production-redis-cache"
    projectId: "${auth:gcloud.projectId}"
    credentials: "${auth:gcloud}"
    region: "us-central1"
```

**Requirements:**
- GCP service account with Redis Admin permissions
- Existing Redis instance ID
- Instance must be accessible from your applications

### **GKE Autopilot Adoption**

Adopt existing GKE Autopilot clusters:

```yaml
gke-cluster:
  type: gcp-gke-autopilot-cluster
  config:
    adopt: true
    clusterName: "production-gke-cluster"
    projectId: "${auth:gcloud.projectId}"
    credentials: "${auth:gcloud}"
    location: "us-central1"
    caddy:
      enable: true
      namespace: "caddy"
      replicas: 2
```

**Requirements:**
- GCP service account with GKE Admin permissions
- Existing cluster name and location
- Cluster must be accessible for deployments

### **GCS Bucket Adoption**

Adopt existing Google Cloud Storage buckets:

```yaml
storage:
  type: gcp-bucket
  config:
    adopt: true
    bucketName: "production-app-storage"
    location: "us-central1"
    projectId: "${auth:gcloud.projectId}"
    credentials: "${auth:gcloud}"
```

**Requirements:**
- GCP service account with Storage Admin permissions
- Existing bucket name
- Bucket must be accessible for your applications

## 🔧 **Multi-Environment Adoption**

Adopt resources across multiple environments with consistent naming:

```yaml
# server.yaml
resources:
  resources:
    # Production Environment
    prod:
      mongodb:
        type: mongodb-atlas
        config:
          adopt: true
          clusterName: "prod-cluster-xyz"
          # ... configuration

      postgres:
        type: gcp-cloudsql-postgres
        config:
          adopt: true
          instanceName: "prod-postgres-instance"
          # ... configuration

    # Staging Environment  
    staging:
      mongodb:
        type: mongodb-atlas
        config:
          adopt: true
          clusterName: "staging-cluster-abc"
          # ... configuration

      postgres:
        type: gcp-cloudsql-postgres
        config:
          adopt: true
          instanceName: "staging-postgres-instance"
          # ... configuration
```

**Benefits:**
- ✅ Same resource names across environments
- ✅ Identical client.yaml configuration
- ✅ Easy environment switching
- ✅ Consistent developer experience

## 🛡️ **Security & Best Practices**

### **Secrets Management**

Store sensitive adoption data securely:

```yaml
# secrets.yaml
values:
  # MongoDB Atlas
  MONGODB_ATLAS_ORG_ID: "your-org-id"
  MONGODB_ATLAS_PUBLIC_KEY: "your-public-key"
  MONGODB_ATLAS_PRIVATE_KEY: "your-private-key"
  
  # PostgreSQL passwords per environment
  POSTGRES_ROOT_PASSWORD_PROD: "secure-prod-password"
  POSTGRES_ROOT_PASSWORD_STAGING: "secure-staging-password"

auth:
  gcloud:
    type: gcp-service-account
    config:
      projectId: "your-gcp-project"
      credentials: |-
        {
          "type": "service_account",
          # ... complete service account JSON
        }
```

### **Permission Requirements**

Ensure service accounts have minimal required permissions:

**MongoDB Atlas:**
- Project Read access
- Cluster Read access
- Database User Admin (for user creation)

**Google Cloud:**
- Cloud SQL Admin (for PostgreSQL)
- Redis Admin (for Redis)
- Kubernetes Engine Admin (for GKE)
- Storage Admin (for GCS)

### **Validation Checklist**

Before adoption:
- [ ] Resources are accessible from your applications
- [ ] Service accounts have required permissions
- [ ] Resource names and IDs are correct
- [ ] Secrets are properly encrypted
- [ ] Network connectivity is configured

## 🔍 **Troubleshooting**

### **Common Issues**

**Resource Not Found:**
```bash
Error: failed to import resource "cluster-name"
```
- Verify resource name/ID is correct
- Check service account permissions
- Ensure resource exists in specified project/region

**Permission Denied:**
```bash
Error: insufficient permissions to access resource
```
- Verify service account has required roles
- Check resource-specific permissions
- Ensure API is enabled for the project

**Network Connectivity:**
```bash
Error: connection timeout to adopted resource
```
- Verify network configuration
- Check firewall rules
- Ensure VPC connectivity if required

### **Validation Commands**

Test resource accessibility:

```bash
# Test MongoDB Atlas connection
mongosh "mongodb+srv://cluster-name.xxx.mongodb.net/test"

# Test PostgreSQL connection  
psql -h CONNECTION_NAME -U postgres -d postgres

# Test Redis connection
redis-cli -h REDIS_HOST -p REDIS_PORT ping

# Test GKE cluster access
kubectl --kubeconfig=kubeconfig get nodes

# Test GCS bucket access
gsutil ls gs://bucket-name
```

## 📈 **Benefits of Resource Adoption**

### **Immediate Value**
- ✅ **Zero Downtime** - Applications continue running unchanged
- ✅ **No Resource Modification** - Existing resources remain untouched
- ✅ **Instant SC Features** - Immediate access to deployment capabilities
- ✅ **Unified Management** - Single interface for all infrastructure

### **Long-Term Benefits**
- ✅ **Consistent Environments** - Same configuration across dev/staging/prod
- ✅ **Simplified Operations** - Single deployment workflow
- ✅ **Enhanced Security** - Centralized secrets management
- ✅ **Better Scaling** - Automatic resource optimization

### **Cost Optimization**
- ✅ **Resource Reuse** - No duplicate infrastructure costs
- ✅ **Efficient Scaling** - Optimize existing resource utilization
- ✅ **Reduced Complexity** - Lower operational overhead

## 🎓 **Next Steps**

After successful resource adoption:

1. **Deploy Services** - Use adopted resources in your applications
2. **Monitor Usage** - Track resource utilization and performance
3. **Optimize Configuration** - Fine-tune adopted resource settings
4. **Expand Adoption** - Adopt additional resources as needed
5. **Team Training** - Educate team on Simple Container workflows

## 📞 **Support**

Need help with resource adoption?

- **Documentation**: [Simple Container Docs](../index.md)
- **Examples**: [Resource Adoption Examples](../examples/resource-adoption/)
- **Community**: [GitHub Discussions](https://github.com/simple-container-com/api/discussions)
- **Support**: [support@simple-container.com](mailto:support@simple-container.com)

---

**Ready to adopt your existing infrastructure?** Start with our [Quick Start](#quick-start) guide above! 🚀
