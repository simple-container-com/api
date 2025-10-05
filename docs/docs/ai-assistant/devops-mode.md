# DevOps Mode

DevOps Mode is designed for **infrastructure teams** who manage shared resources and provide the foundation for application teams to build upon.

## 🎯 Overview

**DevOps Mode Focus:**
- Generate `server.yaml` with shared infrastructure resources
- Create `secrets.yaml` for authentication and sensitive configuration
- Set up cloud provider integrations and credentials
- Define reusable templates for application teams
- Manage environment-specific resource configurations

## 🚀 Quick Start

### Basic Infrastructure Setup
```bash
# Start interactive infrastructure setup
sc assistant devops setup

# Set up for specific cloud provider
sc assistant devops setup --cloud aws

# Configure multi-environment infrastructure
sc assistant devops setup --envs staging,production

# Set up specific resource types
sc assistant devops resources --add database,cache,storage
```

### Resource Management
```bash
# List available resource types
sc assistant devops resources --list

# Add specific resources interactively
sc assistant devops resources --add postgres --interactive

# Generate secrets configuration
sc assistant devops secrets --cloud aws
```

## 🏗️ Infrastructure Wizard

DevOps Mode uses an interactive wizard approach instead of project analysis:

### **1. Cloud Provider Selection**
```
🌐 Select your primary cloud provider:

1. AWS (Amazon Web Services)
   ✅ ECS Fargate, RDS, S3, ElastiCache, Lambda
   
2. GCP (Google Cloud Platform)  
   ✅ GKE Autopilot, Cloud SQL, Cloud Storage, Cloud Run
   
3. Azure (Microsoft Azure) [Coming Soon]
   ⏳ Container Apps, PostgreSQL, Blob Storage
   
4. Kubernetes (Cloud-agnostic)
   ✅ Native K8s, Helm operators, YAML manifests

5. Hybrid (Multiple providers)
   🔧 Advanced configuration required

Choice [1-5]: 1
```

### **2. Environment Configuration**
```
📊 Configure your environments:

✅ Development (local docker-compose)
✅ Staging (cloud resources, lower specs)
✅ Production (cloud resources, high availability)

Additional environments? (testing, preview, etc.): testing
```

### **3. Resource Selection**
```
🎯 Select shared resources to provision:

Databases:
☑️ PostgreSQL (recommended for most apps)
☐ MongoDB (document database)
☐ MySQL (legacy compatibility)
☐ Redis (caching & sessions)

Storage:
☑️ S3-compatible bucket (file uploads)
☐ CDN (static asset distribution)

Compute:
☑️ ECS Fargate (containerized apps)
☐ Lambda functions (serverless)
☐ Static site hosting

Monitoring:
☐ Application monitoring
☐ Log aggregation
☐ Alerting (Slack/Email)
```

### **4. Template Definition**
```
📋 Create application templates:

Template Name: web-app
Description: Standard web application template
Supported Stacks: [PostgreSQL, Redis, S3]
Target: ECS Fargate deployment

Template Name: api-service  
Description: Microservice API template
Supported Stacks: [PostgreSQL, Redis]
Target: ECS Fargate with ALB

Create additional templates? (y/n): n
```

## 📁 Generated Files

### 1. **server.yaml** - Infrastructure Configuration

```yaml
schemaVersion: 1.0

# Provisioner configuration
provisioner:
  pulumi:
    backend: s3
    state-storage:
      type: s3-bucket
      bucketName: mycompany-sc-state
      region: us-east-1
    secrets-provider:
      type: aws-kms
      kmsKeyId: "arn:aws:kms:us-east-1:123456789:key/abc123"
  auth:
    aws: "${auth:aws}"

# Reusable templates for application teams
templates:
  web-app:
    type: aws-ecs-fargate
    ecsClusterResource: ecs-cluster
    ecrRepositoryResource: app-registry
    
  api-service:
    type: aws-ecs-fargate  
    ecsClusterResource: ecs-cluster
    ecrRepositoryResource: api-registry

# Shared infrastructure resources
resources:
  # Staging environment
  staging:
    # Compute cluster
    ecs-cluster:
      type: aws-ecs-cluster
      name: mycompany-staging-cluster
      
    # Container registries
    app-registry:
      type: aws-ecr-repository
      name: mycompany-apps-staging
      
    api-registry:
      type: aws-ecr-repository  
      name: mycompany-apis-staging
      
    # Database
    postgres-db:
      type: aws-rds-postgres
      name: mycompany-staging-db
      instanceClass: db.t3.micro
      allocatedStorage: 20
      engineVersion: "15.4"
      username: dbadmin
      password: "${secret:staging-db-password}"
      databaseName: applications
      
    # Cache
    redis-cache:
      type: aws-elasticache-redis
      name: mycompany-staging-cache
      nodeType: cache.t3.micro
      numCacheNodes: 1
      
    # Storage
    uploads-bucket:
      type: s3-bucket
      name: mycompany-staging-uploads
      allowOnlyHttps: true

  # Production environment
  production:
    # Compute cluster
    ecs-cluster:
      type: aws-ecs-cluster
      name: mycompany-prod-cluster
      
    # Container registries (shared with staging)
    app-registry:
      type: aws-ecr-repository
      name: mycompany-apps-prod
      
    # Database with high availability
    postgres-db:
      type: aws-rds-postgres
      name: mycompany-prod-db
      instanceClass: db.r5.large
      allocatedStorage: 100
      multiAZ: true
      backupRetentionPeriod: 7
      engineVersion: "15.4"
      username: dbadmin
      password: "${secret:prod-db-password}"
      databaseName: applications
      
    # Cache cluster
    redis-cache:
      type: aws-elasticache-redis
      name: mycompany-prod-cache
      nodeType: cache.r5.large
      numCacheNodes: 3
      replicationGroups: true
      
    # Storage with CDN
    uploads-bucket:
      type: s3-bucket
      name: mycompany-prod-uploads
      allowOnlyHttps: true
      
    uploads-cdn:
      type: aws-cloudfront-distribution
      originS3Bucket: uploads-bucket
      priceClass: PriceClass_100

# DNS and domain management
registrar:
  cloudflare:
    credentials: "${auth:cloudflare}"
    accountId: "${secret:cloudflare-account-id}"
    zoneName: "mycompany.com"
    dnsRecords:
      - name: "api"
        type: "CNAME"
        value: "${resource:ecs-cluster.dnsName}"
      - name: "staging-api"
        type: "CNAME" 
        value: "${resource:ecs-cluster.dnsName}"
```

### 2. **secrets.yaml** - Authentication & Secrets
```yaml
# Authentication for cloud providers
auth:
  aws:
    account: "123456789012"
    accessKey: "${secret:aws-access-key}"
    secretAccessKey: "${secret:aws-secret-key}"
    region: us-east-1
    
  cloudflare:
    credentials: "${secret:cloudflare-api-token}"

# Secret values (managed with sc secrets add)
values:
  # Database passwords
  staging-db-password: "staging-secure-password-123"
  prod-db-password: "production-ultra-secure-password-456"
  
  # Cloud credentials
  aws-access-key: "AKIA..."
  aws-secret-key: "secret..."
  cloudflare-api-token: "token..."
  cloudflare-account-id: "account..."
  
  # Application secrets
  jwt-secret: "super-secret-jwt-key"
  third-party-api-key: "external-service-key"
```

### 3. **cfg.default.yaml** - Simple Container Configuration
```yaml
# Simple Container local configuration
privateKeyPath: ~/.ssh/id_rsa
publicKeyPath: ~/.ssh/id_rsa.pub
projectName: mycompany-infrastructure
```

## 🎛️ Command Options

### **Setup Command Options**
```bash
# Interactive wizard (recommended)
sc assistant devops setup --interactive

# Specify cloud provider
sc assistant devops setup --cloud aws
sc assistant devops setup --cloud gcp  
sc assistant devops setup --cloud kubernetes

# Multi-cloud setup
sc assistant devops setup --cloud aws,gcp --primary aws

# Environment configuration
sc assistant devops setup --envs development,staging,production
sc assistant devops setup --skip-env development

# Resource selection
sc assistant devops setup --resources database,cache,storage
sc assistant devops setup --database postgres --cache redis

# Template creation
sc assistant devops setup --templates web-app,api-service,worker
```

### **Resource Management Options**
```bash
# List available resource types by provider
sc assistant devops resources --list --cloud aws
sc assistant devops resources --list --cloud gcp

# Add specific resources
sc assistant devops resources --add postgres --env staging
sc assistant devops resources --add s3-bucket --env production

# Resource templates
sc assistant devops resources --template database-cluster
sc assistant devops resources --template cache-cluster

# Bulk resource management
sc assistant devops resources --file resources.yaml
```

### **Secrets Management Options**
```bash
# Initialize secrets for cloud provider
sc assistant devops secrets --init --cloud aws

# Add authentication credentials
sc assistant devops secrets --auth aws --interactive

# Generate random secrets
sc assistant devops secrets --generate jwt-secret,api-key

# Import from existing system
sc assistant devops secrets --import-from aws-secrets-manager
```

## 🌐 Cloud Provider Templates

### **AWS Configuration**
```yaml
# AWS-optimized resources
provisioner:
  pulumi:
    backend: s3
    state-storage:
      type: s3-bucket
      bucketName: "${var:company}-sc-state-${var:environment}"
      region: "${var:aws-region}"
    secrets-provider:
      type: aws-kms
      kmsKeyId: "alias/simple-container-${var:environment}"

templates:
  aws-web-app:
    type: aws-ecs-fargate
    ecsClusterResource: ecs-cluster
    ecrRepositoryResource: web-registry
    vpcResource: main-vpc
    loadBalancerResource: main-alb
    
  aws-api-service:
    type: aws-ecs-fargate
    ecsClusterResource: ecs-cluster
    ecrRepositoryResource: api-registry
    targetGroupResource: api-targets
```

### **GCP Configuration**
```yaml
# GCP-optimized resources  
provisioner:
  pulumi:
    backend: gcs
    state-storage:
      type: gcp-bucket
      bucketName: "${var:company}-sc-state-${var:environment}"
      location: "${var:gcp-region}"
    secrets-provider:
      type: gcp-secret-manager
      projectId: "${var:gcp-project-id}"

templates:
  gcp-web-app:
    type: gcp-cloud-run
    artifactRegistryResource: web-registry
    cloudSqlResource: postgres-db
    
  gcp-gke-app:
    type: gcp-gke-autopilot
    gkeClusterResource: gke-cluster
    artifactRegistryResource: app-registry
```

### **Kubernetes Configuration**
```yaml
# Kubernetes-native resources
provisioner:
  pulumi:
    backend: kubernetes
    state-storage:
      type: kubernetes-secret
      namespace: simple-container-system
      secretName: pulumi-state
    secrets-provider:
      type: kubernetes-secret
      namespace: simple-container-system

templates:
  k8s-web-app:
    type: kubernetes-deployment
    helmOperatorResource: postgres-operator
    ingressResource: main-ingress
    
  k8s-worker:
    type: kubernetes-job
    cronSchedule: "0 2 * * *"
    helmOperatorResource: redis-operator
```

## 🎯 Resource Categories

### **Compute Resources**
| Resource Type | AWS | GCP | Kubernetes |
|---------------|-----|-----|------------|
| **Container Platform** | ECS Fargate | Cloud Run | Deployment |
| **Kubernetes Cluster** | EKS | GKE Autopilot | Native |
| **Serverless Functions** | Lambda | Cloud Functions | Knative |
| **Static Sites** | S3 + CloudFront | Cloud Storage + CDN | Ingress |

### **Database Resources**  
| Resource Type | AWS | GCP | Kubernetes |
|---------------|-----|-----|------------|
| **PostgreSQL** | RDS PostgreSQL | Cloud SQL PostgreSQL | Helm PostgreSQL |
| **MongoDB** | DocumentDB | MongoDB Atlas | Helm MongoDB |
| **MySQL** | RDS MySQL | Cloud SQL MySQL | Helm MySQL |
| **Redis** | ElastiCache | Memorystore | Helm Redis |

### **Storage Resources**
| Resource Type | AWS | GCP | Kubernetes |
|---------------|-----|-----|------------|
| **Object Storage** | S3 | Cloud Storage | MinIO |
| **Block Storage** | EBS | Persistent Disk | PV/PVC |
| **File Storage** | EFS | Filestore | NFS |
| **CDN** | CloudFront | Cloud CDN | Ingress |

## 💡 Best Practices

### **Environment Strategy**
- ✅ **Staging Mirrors Production**: Same resources, smaller scale
- ✅ **Development Uses Local**: Docker Compose for dev environment
- ✅ **Testing Environment**: Separate environment for CI/CD
- ✅ **Preview Environments**: Dynamic environments for feature branches

### **Resource Naming**
- ✅ **Consistent Naming**: Use company/project prefixes
- ✅ **Environment Suffixes**: `-staging`, `-prod`, `-dev`
- ✅ **Resource Type Prefixes**: `db-`, `cache-`, `bucket-`
- ✅ **Avoid Hardcoding**: Use template placeholders

### **Security Configuration**
- ✅ **Secrets Management**: Use cloud-native secret stores
- ✅ **Least Privilege**: Minimal IAM permissions
- ✅ **Network Security**: VPCs, security groups, firewalls
- ✅ **Encryption**: Encrypt data at rest and in transit

### **Cost Optimization**
- ✅ **Right-sizing**: Start small, scale based on usage
- ✅ **Reserved Instances**: Use reserved capacity for production
- ✅ **Auto-scaling**: Configure automatic scaling policies
- ✅ **Resource Cleanup**: Implement automatic cleanup for dev/test

## 🔧 Advanced Configuration

### **Multi-Region Setup**
```yaml
# Global resources with regional failover
resources:
  production:
    primary-cluster:
      type: aws-ecs-cluster
      name: mycompany-prod-primary
      region: us-east-1
      
    replica-cluster:
      type: aws-ecs-cluster
      name: mycompany-prod-replica  
      region: us-west-2
      
    global-db:
      type: aws-rds-postgres
      name: mycompany-prod-db
      region: us-east-1
      readReplicas:
        - region: us-west-2
          instanceClass: db.r5.large
```

### **Custom Resource Types**
```yaml
# Define custom composite resources
customResources:
  database-cluster:
    description: "PostgreSQL with Redis cache"
    resources:
      - type: aws-rds-postgres
        name: "${var:name}-db"
      - type: aws-elasticache-redis
        name: "${var:name}-cache"
        
  web-tier:
    description: "Load balancer with auto-scaling"
    resources:
      - type: aws-application-load-balancer
        name: "${var:name}-alb"
      - type: aws-ecs-service
        name: "${var:name}-service"
        targetGroup: "${resource:alb.defaultTarget}"
```

## 📋 Team Workflows

### **1. Initial Infrastructure Setup**
```bash
# DevOps team sets up foundation
sc assistant devops setup --interactive

# Review generated configuration
cat .sc/stacks/infrastructure/server.yaml

# Set up secrets
sc secrets add aws-access-key
sc secrets add aws-secret-key
sc secrets add staging-db-password

# Deploy infrastructure
sc provision -s infrastructure -e staging
sc provision -s infrastructure -e production
```

### **2. Application Team Enablement**
```bash
# Share infrastructure details with dev teams
# Review generated server.yaml to see available resources
cat .sc/stacks/infrastructure/server.yaml

# Developers can now reference these resources
# in their client.yaml files using:
# parent: infrastructure
# uses: [postgres-db, redis-cache, uploads-bucket]
```

### **3. Resource Updates**
```bash
# Add new resource to infrastructure
sc assistant devops resources --add mongodb --env staging

# Update existing resource
sc assistant devops resources --update postgres-db --scale-up

# Deploy changes
sc provision -s infrastructure -e staging
```

## 🔍 Troubleshooting

### **Setup Issues**
```bash
# Cloud credentials not configured
sc assistant devops secrets --init --cloud aws

# Permission denied errors
# Check IAM permissions for Simple Container operations

# State backend issues
# Verify S3 bucket exists and is accessible
aws s3 ls s3://mycompany-sc-state/
```

### **Resource Conflicts**
```bash
# Resource name conflicts
# Use unique prefixes: company-env-resource format
# Example: mycompany-staging-postgres

# Environment isolation
# Ensure separate resources per environment
# Never share production resources with staging
```

### **Template Issues**
```bash
# Template not found errors
# Ensure template names match between server.yaml and client.yaml

# Resource references not working
# Check resource names are correct and environment matches
cat .sc/stacks/infrastructure/server.yaml | grep -A 20 "resources:"
```

## 📋 Examples by Cloud Provider

### **AWS Examples**
- [AWS Multi-tier Architecture](examples/aws-multi-tier.md)
- [AWS Microservices with ECS](examples/aws-microservices.md)
- [AWS Serverless Setup](examples/aws-serverless.md)

### **GCP Examples**
- [GCP with GKE Autopilot](examples/gcp-gke.md)
- [GCP Cloud Run Services](examples/gcp-cloud-run.md)
- [GCP Hybrid Architecture](examples/gcp-hybrid.md)

### **Kubernetes Examples**
- [On-premises Kubernetes](examples/k8s-on-prem.md)
- [Multi-cloud Kubernetes](examples/k8s-multi-cloud.md)
- [Kubernetes with Operators](examples/k8s-operators.md)

## 🔗 Next Steps

1. **[Complete infrastructure setup →](getting-started.md#devops-setup)**
2. **[Enable application teams →](../concepts/parent-stacks.md)**
3. **[Monitor and scale resources →](../advanced/scaling-advantages.md)**
4. **[Set up CI/CD pipelines →](../guides/deployment.md)**
