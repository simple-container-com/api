---
title: Supported Resources
description: Complete reference of all supported cloud resources and their properties for defining resources in the parent stack
platform: platform
product: simple-container
category: devguide
subcategory: reference
guides: reference
date: '2024-12-07'
---

# **Supported Resources Reference**

This document provides a comprehensive reference of all supported cloud resources and their properties that can be defined in the **parent stack**. The parent stack is managed by DevOps teams and provides the core infrastructure that microservices consume.

## **Understanding Simple Container Architecture**

Simple Container uses a **separation of concerns** architecture where:

- **Parent Stack** (`server.yaml`) - DevOps-managed infrastructure and deployment templates
- **Client Stack** (`client.yaml`) - Developer-managed service configurations that consume parent resources

### **Configuration Types and Their Purpose**

- **TemplateType** → `templates` section in `server.yaml` - **Deployment patterns** (HOW to deploy services)
- **ResourceType** → `resources` section in `server.yaml` - **Shared infrastructure** (provisioned with `sc provision`)
- **AuthType** → `auth` section in `secrets.yaml` - **Authentication providers**
- **SecretsType** → `secrets` section in `secrets.yaml` - **Secret management**
- **RegistrarType** → `registrar` section in `server.yaml` - **Domain registration**
- **StateStorageType** → `provisioner.stateStorage` section in `server.yaml` - **Terraform state storage**
- **SecretsProviderType** → `provisioner.secretsProvider` section in `server.yaml` - **Secret encryption**

### **How Client Stacks Consume Parent Resources**

Client stacks (`client.yaml`) consume parent stack resources using:

- **`parent`** directive - Specifies which parent stack to use
- **Environment matching** - By default, `stacks.staging` consumes `resources.resources.staging` from parent
- **`parentEnv`** directive - Allows custom stack names to consume specific parent environments
- **`uses`** directive - Specifies which resources from parent environment to consume

```yaml
# client.yaml examples
stacks:
  # Environment matching: 'staging' stack consumes 'resources.resources.staging' from parent
  staging:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [mongodb-shared-us, s3-storage]  # Consume parent resources
      domain: staging.myapp.com
  
  # Custom stack name with parentEnv directive
  customer-a:
    type: cloud-compose
    parent: myorg/infrastructure
    parentEnv: production  # Consumes resources.resources.production from parent
    config:
      uses: [mongodb-shared-us, s3-storage]
      domain: customer-a.myapp.com
```

## **Supported Cloud Providers**

- **AWS** - Amazon Web Services
- **Google Cloud Platform (GCP)** - Google Cloud
- **Kubernetes** - Kubernetes-native resources
- **MongoDB Atlas** - MongoDB Atlas database clusters
- **Cloudflare** - Domain registration and DNS
- **File System** - Local development resources

---

## **AWS Provider**

### **Templates** (`TemplateType` → `templates` section in `server.yaml`)

Templates define **deployment patterns** - HOW services are deployed. Client stacks reference these templates to deploy their services.

#### **ECS Fargate** (`ecs-fargate`)

Deployment template for containerized applications on AWS ECS using Fargate.

**Golang Struct Reference:** `pkg/clouds/aws/ecs_fargate.go:EcsFargateConfig`

```yaml
# server.yaml - Parent Stack (DevOps managed)
templates:
  stack-per-app-us:
    type: ecs-fargate
    config: &aws-us-cfg
      credentials: "${auth:aws-us}"
      account: "${auth:aws-us.projectId}"
  
  stack-per-app-eu:
    type: ecs-fargate
    config: &aws-eu-cfg
      credentials: "${auth:aws-eu}"
      account: "${auth:aws-eu.projectId}"

resources:
  resources:
    production:
      template: stack-per-app-us
    staging:
      template: stack-per-app-eu
```

**How Client Stacks Use This Template:**
```yaml
# client.yaml - Client Stack (Developer managed)
stacks:
  production:  # Matches resources.resources.production from parent
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [mongodb-shared, s3-storage]  # Consumes parent resources
      domain: customer-a.myapp.com
      runs: [web-app]
  
  # Or with custom stack name using parentEnv
  customer-a:
    type: cloud-compose
    parent: myorg/infrastructure
    parentEnv: production  # Consumes resources.resources.production from parent
    config:
      uses: [mongodb-shared, s3-storage]
      domain: customer-a.myapp.com
      runs: [web-app]
```

#### **AWS Lambda** (`aws-lambda`)

Deployment template for serverless functions on AWS Lambda.

**Golang Struct Reference:** `pkg/clouds/aws/aws_lambda.go:LambdaInput`

```yaml
# server.yaml - Parent Stack (DevOps managed)
templates:
  lambda-us:
    type: aws-lambda
    config: &aws-us-cfg
      credentials: "${auth:aws-us}"
      account: "${auth:aws-us.projectId}"
  
  lambda-eu:
    type: aws-lambda
    config: &aws-eu-cfg
      credentials: "${auth:aws-eu}"
      account: "${auth:aws-eu.projectId}"

resources:
  resources:
    production:
      template: lambda-us
    staging:
      template: lambda-eu
```

**See Also:**

- [Lambda Functions Examples](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/lambda-functions/) - Complete Lambda configurations with Dockerfile and advanced patterns
- [AI Gateway Example](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/lambda-functions/ai-gateway/) - AWS Bedrock integration with specific IAM roles
- [Storage Service Example](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/lambda-functions/storage-service/) - Scheduled cleanup with cron expressions
- [Scheduler Example](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/lambda-functions/scheduler/) - High-frequency scheduling (every minute)
- [Cost Analytics Example](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/lambda-functions/cost-analytics/) - AWS cost analysis with comprehensive IAM permissions
- [Billing System Example](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/lambda-functions/billing-system/) - Multi-environment with YAML anchors

#### **Static Website** (`aws-static-website`)

Deployment template for static websites on AWS S3 with CloudFront.

**Golang Struct Reference:** `pkg/clouds/aws/static_website.go:StaticSiteInput`

```yaml
# server.yaml - Parent Stack (DevOps managed)
templates:
  static-us:
    type: aws-static-website
    config: &aws-us-cfg
      credentials: "${auth:aws-us}"
      account: "${auth:aws-us.projectId}"
  
  static-eu:
    type: aws-static-website
    config: &aws-eu-cfg
      credentials: "${auth:aws-eu}"
      account: "${auth:aws-eu.projectId}"

resources:
  resources:
    production:
      template: static-us
    staging:
      template: static-eu
```

**See Also:**

- [Static Websites Examples](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/static-websites/) - Complete static website configurations
- [Documentation Site Example](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/static-websites/documentation-site/) - MkDocs documentation deployment
- [Landing Page Example](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/static-websites/landing-page/) - Main website with SPA configuration
- [Admin Dashboard Example](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/static-websites/admin-dashboard/) - Admin UI with multi-environment setup
- [Customer Portal Example](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/static-websites/customer-portal/) - Customer-facing UI deployment
- [Media Store Example](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/static-websites/media-store/) - Media-specific static hosting

### **Resources** (`ResourceType` → `resources` section in `server.yaml`)

#### **S3 Bucket** (`s3-bucket`)

Creates and manages AWS S3 buckets.

**Golang Struct Reference:** `pkg/clouds/aws/bucket.go:S3Bucket`

**JSON Schema:** [S3Bucket Schema](https://github.com/simple-container-com/api/tree/main/docs/schemas/aws/s3bucket.json)

```yaml
# server.yaml - Parent Stack
resources:
  resources:
    production:
      resources:
        my-s3-bucket:
          type: s3-bucket
          config:
            # AWS account configuration (inherited from AccountConfig)
            credentials: "${auth:aws-us}"
            account: "${auth:aws-us.projectId}"
            
            # S3 bucket specific properties (from S3Bucket struct)
            name: "my-application-storage"        # Bucket name
            allowOnlyHttps: true                  # Force HTTPS-only access
```

#### **ECR Repository** (`ecr-repository`)

Creates and manages AWS Elastic Container Registry repositories.

**Golang Struct Reference:** `pkg/clouds/aws/ecr_repository.go:EcrRepository`

**JSON Schema:** [EcrRepository Schema](https://github.com/simple-container-com/api/tree/main/docs/schemas/aws/ecrrepository.json)

```yaml
# server.yaml - Parent Stack
resources:
  resources:
    production:
      resources:
        my-ecr-repo:
          type: ecr-repository
          config:
            # AWS account configuration (inherited from AccountConfig)
            credentials: "${auth:aws-us}"
            account: "${auth:aws-us.projectId}"
            
            # ECR repository specific properties (from EcrRepository struct)
            name: "my-app"                        # Repository name
            lifecyclePolicy:                      # Image lifecycle management
              rules:
                - rulePriority: 1
                  description: "Keep only 3 last images"
                  selection:
                    tagStatus: "any"              # any, tagged, untagged
                    countType: "imageCountMoreThan"
                    countNumber: 3
                  action:
                    type: "expire"                # Action to take when rule matches
```

#### **RDS PostgreSQL** (`aws-rds-postgres`)

Creates and manages AWS RDS PostgreSQL databases.

**Golang Struct Reference:** `pkg/clouds/aws/rds_postgres.go:PostgresConfig`

```yaml
# server.yaml - Parent Stack
resources:
  resources:
    production:
      resources:
        my-postgres-db:
          type: aws-rds-postgres
          config:
            # AWS account configuration (inherited from AccountConfig)
            credentials: "${auth:aws-us}"
            account: "${auth:aws-us.projectId}"
            
            # PostgreSQL specific properties (from PostgresConfig struct)
            name: "my-postgres-db"               # Database instance identifier
            instanceClass: "db.t3.micro"        # Instance size
            allocateStorage: 20                  # Storage size in GB
            engineVersion: "14.9"               # PostgreSQL version
            username: "postgres"                # Master username
            password: "${env:DB_PASSWORD}"      # Master password
            databaseName: "myapp"               # Initial database name
```

#### **RDS MySQL** (`aws-rds-mysql`)

Creates and manages AWS RDS MySQL databases.

**Golang Struct Reference:** `pkg/clouds/aws/rds_mysql.go:MysqlConfig`

```yaml
# server.yaml - Parent Stack
resources:
  resources:
    production:
      resources:
        my-mysql-db:
          type: aws-rds-mysql
          config:
            # AWS account configuration (inherited from AccountConfig)
            credentials: "${auth:aws-us}"
            account: "${auth:aws-us.projectId}"
            
            # MySQL specific properties (from MysqlConfig struct)
            name: "my-mysql-db"                  # Database instance identifier
            instanceClass: "db.t3.micro"        # Instance size
            allocateStorage: 20                  # Storage size in GB
            engineVersion: "8.0"                # MySQL version
            username: "admin"                   # Master username
            password: "${env:DB_PASSWORD}"      # Master password
            databaseName: "myapp"               # Initial database name
            engineName: "mysql"                 # Engine name (optional)
```

### **Authentication** (`AuthType` → `auth` section in `secrets.yaml`)

#### **AWS Token Authentication** (`aws-token`)

Configures AWS authentication using access tokens.

**Golang Struct Reference:** `pkg/clouds/aws/auth.go:AccountConfig`

```yaml
# secrets.yaml (managed with: sc secrets add .sc/stacks/<parent>/secrets.yaml)
schemaVersion: 1.0
auth:
  aws-account:
    type: aws-token
    config:
      # AWS account configuration properties (from AccountConfig struct)
      account: "123456789012"                    # AWS account ID
      accessKey: "AKIAIOSFODNN7EXAMPLE"          # Exact literal value - NO placeholders
      secretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"  # Exact literal value
      region: us-east-1                          # AWS region

values:
  # Exact literal values - NO placeholders processed in secrets.yaml
  DATABASE_URL: "postgresql://user:pass@host:5432/db"
  API_KEY: "your-secret-api-key-here"
```

### **Secrets Management** (`SecretsType` → `secrets` section in `secrets.yaml`)

#### **AWS Secrets Manager** (`aws-secrets-manager`)

Manages secrets using AWS Secrets Manager.

```yaml
# secrets.yaml (managed with: sc secrets add .sc/stacks/<parent>/secrets.yaml)
schemaVersion: 1.0
auth:
  aws-account:
    type: aws-token
    config:
      account: "123456789012"
      accessKey: "AKIAIOSFODNN7EXAMPLE"
      secretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
      region: us-east-1

values:
  # Exact literal values - NO placeholders processed in secrets.yaml
  DATABASE_PASSWORD: "mySecurePassword123"
  API_KEY: "sk-1234567890abcdef"
  CLOUDFLARE_API_TOKEN: "gEYRal5hQm4XJWE5WROP6DAEsdb3NxOgQUcpKjzB"
```

### **Provisioner Configuration** (goes to `provisioner` section in `server.yaml`)

#### **S3 State Storage** (`s3-bucket`)

Stores Terraform state in AWS S3.

```yaml
# server.yaml
stacks:
  production:
    provisioner:
      stateStorage:
        type: s3-bucket
        config:
          region: us-east-1
          accountId: "123456789012"
          name: "myapp-terraform-state"
```

#### **AWS KMS Secrets Provider** (`aws-kms`)

Encrypts secrets using AWS KMS.

```yaml
# server.yaml
stacks:
  production:
    provisioner:
      secretsProvider:
        type: aws-kms
        config:
          region: us-east-1
          keyId: "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012"
```

---

## **Google Cloud Platform (GCP) Provider**

### **Templates** (`TemplateType` → `templates` section in `server.yaml`)

#### **Cloud Run** (`cloudrun`)

Deploys containerized applications on Google Cloud Run.

**Golang Struct Reference:** `pkg/clouds/gcloud/cloudrun.go:CloudRunInput`

**JSON Schema:** [CloudRunInput Schema](https://github.com/simple-container-com/api/tree/main/docs/schemas/gcp/cloudruninput.json)

```yaml
# server.yaml - Parent Stack (DevOps managed)
templates:
  my-cloudrun-template:
    type: cloudrun
    config:
      # GCP credentials and project (from TemplateConfig struct)
      projectId: "${auth:gcp-main.projectId}"
      credentials: "${auth:gcp-main}"

resources:
  resources:
    production:
      template: my-cloudrun-template
```

**How Client Stacks Use This Template:**
```yaml
# client.yaml - Client Stack (Developer managed)
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [shared-database]  # Consumes parent resources
      domain: myapp.example.com
      runs: [web-app]
```

**See Also:**

- [ECS Deployments Examples](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/ecs-deployments/) - Complete ECS deployment configurations with docker-compose.yaml and Dockerfile
- [Backend Service Example](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/ecs-deployments/backend-service/) - Node.js backend with MongoDB integration
- [Blockchain Service Example](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/ecs-deployments/blockchain-service/) - Blockchain integration with cross-service dependencies

#### **GKE Autopilot** (`gcp-gke-autopilot`)

Template for deploying applications to GKE Autopilot clusters. References GKE cluster and Artifact Registry resources.

**Golang Struct Reference:** `pkg/clouds/gcloud/gke_autopilot.go:GkeAutopilotTemplate`

```yaml
# server.yaml - Parent Stack (DevOps managed)
templates:
  stack-per-app-gke:
    type: gcp-gke-autopilot
    config:
      projectId: "${auth:gcloud.projectId}"
      credentials: "${auth:gcloud}"
      gkeClusterResource: gke-autopilot-res        # References the GKE cluster resource
      artifactRegistryResource: artifact-registry-res  # References the artifact registry resource

resources:
  resources:
    production:
      template: stack-per-app-gke
      resources:
        gke-autopilot-res:
          type: gcp-gke-autopilot-cluster
          config:
            gkeMinVersion: 1.27.16-gke.1296000
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            location: europe-west3
            caddy:
              enable: true
              namespace: caddy
              replicas: 2
        artifact-registry-res:
          type: gcp-artifact-registry
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            location: europe-west3
            docker:
              immutableTags: true
```

**See Also:**

- [GKE Autopilot Comprehensive Setup](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/gke-autopilot/comprehensive-setup/) - Complete GCP setup with GKE, Artifact Registry, Pub/Sub, Redis, MongoDB Atlas
- [Parent Stacks Examples](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/parent-stacks/) - Multi-region and hybrid cloud parent stack configurations

#### **Static Website** (`gcp-static-website`)

Hosts static websites on Google Cloud Storage with Cloud CDN.

**Golang Struct Reference:** `pkg/clouds/gcloud/static_website.go:StaticSiteInput`

```yaml
# server.yaml
stacks:
  production:
    templates:
      my-gcp-static-site-template:
        type: gcp-static-website
        config:
          projectId: "my-gcp-project"
          name: "my-static-website"
```

### **Resources** (`ResourceType` → `resources` section in `server.yaml`)

#### **GKE Autopilot Cluster** (`gcp-gke-autopilot-cluster`)

Creates and manages Google Kubernetes Engine Autopilot clusters as a resource.

**Golang Struct Reference:** `pkg/clouds/gcloud/gke_autopilot.go:GkeAutopilotResource`

**JSON Schema:** [GkeAutopilotResource Schema](https://github.com/simple-container-com/api/tree/main/docs/schemas/gcp/gkeautopilotresource.json)

```yaml
# server.yaml - Resource Definition
resources:
  resources:
    production:
      resources:
        my-gke-cluster:
          type: gcp-gke-autopilot-cluster
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            location: "europe-west3"
            zone: "europe-west3-a"                       # GKE zone (required)
            gkeMinVersion: "1.27.16-gke.1296000"
            caddy:
              enable: true
              namespace: caddy
              replicas: 2
        my-artifact-registry:
          type: gcp-artifact-registry
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            location: "europe-west3"
            docker:
              immutableTags: false
```

#### **GKE Autopilot Template** (`gcp-gke-autopilot`)

Template for deploying applications to GKE Autopilot clusters. References GKE cluster and Artifact Registry resources.

**Golang Struct Reference:** `pkg/clouds/gcloud/gke_autopilot.go:GkeAutopilotTemplate`

```yaml
# server.yaml - Template Definition
templates:
  gke-app-template:
    type: gcp-gke-autopilot
    config:
      projectId: "${auth:gcloud.projectId}"
      credentials: "${auth:gcloud}"
      gkeClusterResource: my-gke-cluster        # References the GKE cluster resource
      artifactRegistryResource: my-artifact-registry  # References the artifact registry resource

resources:
  resources:
    production:
      template: gke-app-template  # Uses the template defined above
      resources:
        # ... resource definitions as shown above
```

**Complete Example:**

```yaml
# server.yaml - Complete GKE Autopilot Setup
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
        name: my-app-state
        location: europe-west3

templates:
  gke-stack:
    type: gcp-gke-autopilot
    config:
      projectId: "${auth:gcloud.projectId}"
      credentials: "${auth:gcloud}"
      gkeClusterResource: gke-autopilot-cluster
      artifactRegistryResource: artifact-registry

resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: "your-account-id"
      zoneName: "example.com"
  resources:
    production:
      template: gke-stack
      resources:
        gke-autopilot-cluster:
          type: gcp-gke-autopilot-cluster
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            location: "europe-west3"
            gkeMinVersion: "1.27.16-gke.1296000"
            caddy:
              enable: true
              namespace: caddy
              replicas: 2
        artifact-registry:
          type: gcp-artifact-registry
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            location: "europe-west3"
            docker:
              immutableTags: false
```

#### **GCP Bucket** (`gcp-bucket`)

Creates and manages Google Cloud Storage buckets.

**Golang Struct Reference:** `pkg/clouds/gcloud/bucket.go:GcpBucket`

```yaml
# server.yaml - Parent Stack
resources:
  resources:
    production:
      resources:
        my-gcp-bucket:
          type: gcp-bucket
          config:
            # GCP credentials and project (inherited from Credentials)
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            
            # GCP Bucket specific properties (from GcpBucket struct)
            name: "my-application-storage"           # Bucket name
            location: "US"                           # Bucket location
```

#### **Artifact Registry** (`gcp-artifact-registry`)

Creates and manages Google Artifact Registry repositories.

**Golang Struct Reference:** `pkg/clouds/gcloud/artifactregistry.go:ArtifactRegistryConfig`

```yaml
# server.yaml - Parent Stack
resources:
  resources:
    production:
      resources:
        my-artifact-registry:
          type: gcp-artifact-registry
          config:
            # GCP credentials and project (inherited from Credentials)
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            
            # Artifact Registry specific properties (from ArtifactRegistryConfig struct)
            location: "europe-west3"                 # Registry location
            public: false                            # Whether registry is public (optional)
            docker:                                  # Docker-specific settings (optional)
              immutableTags: false                   # Whether tags are immutable
            domain: "my-domain.com"                  # Custom domain (optional)
            basicAuth:                               # Basic auth configuration (optional)
              username: "registry-user"
              password: "${env:REGISTRY_PASSWORD}"
```

### **Database Resources**

#### **Cloud SQL PostgreSQL** (`gcp-cloudsql-postgres`)

Creates and manages Google Cloud SQL PostgreSQL databases.

**Configuration Properties:**
```yaml
resources:
  my-cloudsql-postgres:
    type: gcp-cloudsql-postgres
    config:
      projectId: "my-gcp-project"
      region: "us-central1"
      instanceId: "my-postgres-instance"
      databaseVersion: "POSTGRES_14"
      tier: "db-f1-micro"
      diskSize: 10
      diskType: "PD_SSD"
      backupEnabled: true
      backupStartTime: "02:00"
      maintenanceWindow:
        day: 7  # Sunday
        hour: 3
      authorizedNetworks:
        - "0.0.0.0/0"  # Allow all IPs (not recommended for production)
```

#### **Redis** (`gcp-redis`)

Creates and manages Google Cloud Memorystore Redis instances.

**Configuration Properties:**
```yaml
resources:
  my-redis:
    type: gcp-redis
    config:
      projectId: "my-gcp-project"
      region: "us-central1"
      instanceId: "my-redis-instance"
      memorySizeGb: 1
      tier: "BASIC"
      redisVersion: "REDIS_6_X"
      authEnabled: true
      transitEncryptionMode: "SERVER_AUTHENTICATION"
```

### **Messaging Resources**

#### **Pub/Sub** (`gcp-pubsub`)

Creates and manages Google Cloud Pub/Sub topics and subscriptions.

**Golang Struct Reference:** `pkg/clouds/gcloud/pubsub.go:PubSubConfig`

```yaml
# server.yaml - Parent Stack
resources:
  resources:
    production:
      resources:
        my-pubsub:
          type: gcp-pubsub
          config:
            # GCP credentials and project (inherited from Credentials)
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            
            # Pub/Sub specific properties (from PubSubConfig struct)
            labels:                                  # Resource labels
              env: production
            topics:                                  # Topics configuration
              - name: "my-topic"
                messageRetentionDuration: "86400s"  # Message retention duration
                labels:
                  type: "application"
            subscriptions:                           # Subscriptions configuration
              - name: "my-subscription"
                topic: "my-topic"
                ackDeadlineSec: 600                  # Acknowledgment deadline in seconds
                exactlyOnceDelivery: true            # Enable exactly-once delivery
                messageRetentionDuration: "86400s"   # Message retention duration
                deadLetterPolicy:                    # Dead letter policy (optional)
                  deadLetterTopic: "projects/my-project/topics/dead-letter"
                  maxDeliveryAttempts: 5
                labels:
                  subscriber: "my-service"
```

### **Authentication & Secrets**

#### **GCP Service Account** (`gcp-service-account`)

Configures GCP authentication using service accounts.

**Configuration Properties:**
```yaml
auth:
  gcp-account:
    type: gcp-service-account
    config:
      projectId: "my-gcp-project"
      serviceAccountKey: "${env:GCP_SERVICE_ACCOUNT_KEY}"
```

#### **GCP Secrets Manager** (`gcp-secrets-manager`)

Manages secrets using Google Secret Manager.

**Configuration Properties:**
```yaml
secrets:
  provider: gcp-secrets-manager
  config:
    projectId: "my-gcp-project"
    secretsPrefix: "myapp-"
```

---

## **Kubernetes Resources**

### **Templates (Compute)**

#### **Kubernetes Cloud Run** (`kubernetes-cloudrun`)

Deploys applications to Kubernetes clusters.

**Golang Struct Reference:** `pkg/clouds/k8s/templates.go:CloudrunTemplate`

```yaml
# server.yaml - Parent Stack
templates:
  k8s-cloudrun-template:
    type: kubernetes-cloudrun
    config:
      # Kubernetes configuration (from KubernetesConfig struct)
      kubeconfig: "${auth:k8s-cluster.kubeconfig}"
      
      # Docker registry credentials (from RegistryCredentials struct)
      registryUrl: "my-registry.com"
      username: "${secret:REGISTRY_USERNAME}"
      password: "${secret:REGISTRY_PASSWORD}"
      
      # CloudrunTemplate specific properties
      caddyResource: "my-caddy-resource"       # Name of the caddy resource in base stack (optional)
      useSSL: true                             # Whether to assume connection must be over HTTPS only (optional, default: true)
```

**See Also:**

- [Kubernetes Native Examples](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/kubernetes-native/streaming-platform/) - Streaming platform with hardcoded IPs, N8N integration, zero-downtime configs
- [Advanced Configs Examples](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/advanced-configs/high-resource/) - High-resource AI development environment with Kubernetes integration

### **Infrastructure Resources**

#### **Caddy Reverse Proxy** (`kubernetes-caddy`)

Deploys Caddy as a reverse proxy and load balancer.

**Golang Struct Reference:** `pkg/clouds/k8s/caddy.go:CaddyConfig`

```yaml
# server.yaml - Parent Stack
resources:
  resources:
    production:
      resources:
        my-caddy:
          type: kubernetes-caddy
          config:
            # Kubernetes configuration (from KubernetesConfig struct)
            kubeconfig: "${auth:k8s-cluster.kubeconfig}"
            
            # CaddyConfig specific properties (from CaddyConfig struct)
            enable: true                             # Enable Caddy deployment (optional)
            namespace: "caddy-system"                # Kubernetes namespace (optional)
            image: "caddy:2.7-alpine"               # Caddy Docker image (optional)
            replicas: 2                             # Number of replicas (optional)
            serviceType: "LoadBalancer"             # Service type (optional, default: LoadBalancer)
            useSSL: true                            # Use SSL by default (optional, default: true)
            usePrefixes: false                      # Use prefixes instead of domains (optional, default: false)
            provisionIngress: false                 # Provision ingress for Caddy (optional, default: false)
```

### **Database Operators (Helm Charts)**

#### **PostgreSQL Operator** (`kubernetes-helm-postgres-operator`)

Installs PostgreSQL operator via Helm.

**Golang Struct Reference:** `pkg/clouds/k8s/postgres.go:HelmPostgresOperator`

```yaml
# server.yaml - Parent Stack
resources:
  resources:
    production:
      resources:
        postgres-operator:
          type: kubernetes-helm-postgres-operator
          config:
            # Kubernetes configuration (from KubernetesConfig struct)
            kubeconfig: "${auth:k8s-cluster.kubeconfig}"
            
            # HelmChartConfig properties
            namespace: "postgres-operator"               # Namespace for PostgreSQL instances (optional)
            operatorNamespace: "postgres-operator"       # Namespace for operator itself (optional)
            values:                                      # Helm chart values (optional)
              postgresql:
                image: "postgres:14"
            
            # HelmPostgresOperator specific properties
            volumeSize: "10Gi"                          # Volume size for PostgreSQL instances (optional)
            numberOfInstances: 1                        # Number of PostgreSQL instances (optional)
            version: "14"                               # PostgreSQL version (optional)
            pg_hba:                                     # PostgreSQL HBA entries (optional)
              - "host all all 0.0.0.0/0 md5"
            initSQL: "CREATE DATABASE myapp;"          # Initial SQL to run (optional)
```

#### **MongoDB Operator** (`kubernetes-helm-mongodb-operator`)

Installs MongoDB operator via Helm.

**Golang Struct Reference:** `pkg/clouds/k8s/postgres.go:HelmMongodbOperator`

```yaml
# server.yaml - Parent Stack
resources:
  resources:
    production:
      resources:
        mongodb-operator:
          type: kubernetes-helm-mongodb-operator
          config:
            # Kubernetes configuration (from KubernetesConfig struct)
            kubeconfig: "${auth:k8s-cluster.kubeconfig}"
            
            # HelmChartConfig properties
            namespace: "mongodb-operator"               # Namespace for MongoDB instances (optional)
            operatorNamespace: "mongodb-operator"       # Namespace for operator itself (optional)
            values:                                      # Helm chart values (optional)
              mongodb:
                image: "mongo:6.0"
            
            # HelmMongodbOperator specific properties
            version: "6.0"                              # MongoDB version (optional)
            replicas: 3                                 # Number of MongoDB replicas (optional)
```

#### **RabbitMQ Operator** (`kubernetes-helm-rabbitmq-operator`)

Installs RabbitMQ operator via Helm.

**Golang Struct Reference:** `pkg/clouds/k8s/postgres.go:HelmRabbitmqOperator`

```yaml
# server.yaml - Parent Stack
resources:
  resources:
    production:
      resources:
        rabbitmq-operator:
          type: kubernetes-helm-rabbitmq-operator
          config:
            # Kubernetes configuration (from KubernetesConfig struct)
            kubeconfig: "${auth:k8s-cluster.kubeconfig}"
            
            # HelmChartConfig properties
            namespace: "rabbitmq-operator"              # Namespace for RabbitMQ instances (optional)
            operatorNamespace: "rabbitmq-operator"      # Namespace for operator itself (optional)
            values:                                     # Helm chart values (optional)
              rabbitmq:
                image: "rabbitmq:3.12-management"
            
            # HelmRabbitmqOperator specific properties
            replicas: 3                                # Number of RabbitMQ replicas (optional)
```

#### **Redis Operator** (`kubernetes-helm-redis-operator`)

Installs Redis operator via Helm.

**Golang Struct Reference:** `pkg/clouds/k8s/postgres.go:HelmRedisOperator`

```yaml
# server.yaml - Parent Stack
resources:
  resources:
    production:
      resources:
        redis-operator:
          type: kubernetes-helm-redis-operator
          config:
            # Kubernetes configuration (from KubernetesConfig struct)
            kubeconfig: "${auth:k8s-cluster.kubeconfig}"
            
            # HelmChartConfig properties
            namespace: "redis-operator"                 # Namespace for Redis instances (optional)
            operatorNamespace: "redis-operator"         # Namespace for operator itself (optional)
            values:                                     # Helm chart values (optional)
              redis:
                image: "redis:7.0-alpine"
```

### **Authentication**

#### **Kubeconfig** (`kubeconfig`)

Configures Kubernetes cluster authentication.

**Golang Struct Reference:** `pkg/clouds/k8s/auth.go:KubernetesConfig`

**Configuration Properties:**
```yaml
auth:
  k8s-cluster:
    type: kubeconfig
    config:
      kubeconfig: "${env:KUBECONFIG_CONTENT}"
      context: "my-cluster-context"
      namespace: "default"
```

---

## **MongoDB Atlas Resources**

### **Database Resources**

#### **MongoDB Atlas Cluster** (`mongodb-atlas`)

Creates and manages MongoDB Atlas database clusters.

**Golang Struct Reference:** `pkg/clouds/mongodb/mongodb.go:AtlasConfig`

**JSON Schema:** [AtlasConfig Schema](https://github.com/simple-container-com/api/tree/main/docs/schemas/mongodb/atlasconfig.json)

```yaml
# server.yaml - Parent Stack (production example)
resources:
  resources:
    staging:
      resources:
        mongodb:
          type: mongodb-atlas
          config:
            # Atlas API credentials
            publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
            privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
            
            # Organization and cluster configuration
            orgId: 67bc72f86e5ef36f7584d7d0              # Atlas organization ID
            projectId: "67bc72f86e5ef36f7584d7d1"          # Atlas project ID (required)
            projectName: "my-staging-project"             # Atlas project name (required)
            instanceSize: "M10"                           # Instance size
            region: "EU_CENTRAL_1"                        # Atlas region
            cloudProvider: AWS                            # Cloud provider
            
            # Access control
            admins: [ "vitaly", "dmitriy" ]              # Admin user emails
            developers: [ ]                               # Developer user emails
            
            # Backup configuration
            backup:
              every: 4h                                   # Backup frequency
              retention: 24h                              # Retention period
    
    production:
      resources:
        mongodb:
          type: mongodb-atlas
          config:
            publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
            privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
            orgId: 67bc72f86e5ef36f7584d7d0
            projectId: "67bc72f86e5ef36f7584d7d1"          # Atlas project ID (required)
            projectName: "my-production-project"          # Atlas project name (required)
            instanceSize: "M30"                           # Larger instance for production
            region: "EU_CENTRAL_1"
            cloudProvider: AWS
            admins: [ "vitaly", "dmitriy" ]
            developers: [ ]
            backup:
              every: 1h                                   # More frequent backups
              retention: 168h                             # Longer retention (1 week)
```

**See Also:**

- [ECS Deployments Examples](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/ecs-deployments/) - Services using MongoDB Atlas resources
- [Backend Service Example](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/ecs-deployments/backend-service/) - Node.js backend with MongoDB integration
- [Meteor App Example](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/ecs-deployments/meteor-app/) - Meteor.js application with MongoDB
- [Parent Stacks Examples](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/parent-stacks/aws-multi-region/) - Multi-region setup with MongoDB Atlas

---

## **Cloudflare Resources**

### **DNS & Domain Resources**

#### **Domain Registrar** (`cloudflare`)

Manages domain registration and DNS through Cloudflare. **Special resource type** that goes to `resources.registrar` section.

**Golang Struct Reference:** `pkg/clouds/cloudflare/cloudflare.go:RegistrarConfig`

```yaml
# server.yaml - Parent Stack (production example)
resources:
  registrar:
    type: cloudflare
    config:
      # Cloudflare authentication
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"    # Cloudflare API token
      accountId: 23c5ca78cfb4721d9a603ed695a2623e      # Cloudflare account ID
      zoneName: amagenta.ai                            # DNS zone name
      
      # DNS records configuration - SPF email configuration
      dnsRecords:
        - name: "@"                                    # Root domain SPF record
          type: TXT                                    # TXT record for SPF
          value: v=spf1 include:_spf.google.com ~all   # Google email SPF
          proxied: false                               # SPF records should not be proxied
        - name: "@"                                    # HubSpot email integration
          type: TXT
          value: include:143683367.spf06.hubspotemail.net  # HubSpot SPF
          proxied: false
```

**See Also:**

- [Parent Stacks Examples](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/parent-stacks/aws-multi-region/) - Complete parent stack with Cloudflare DNS configuration
- [GKE Autopilot Setup](https://github.com/simple-container-com/api/tree/main/docs/docs/examples/gke-autopilot/comprehensive-setup/) - GCP setup with Cloudflare domain management

---

## **Provisioner Configuration**

### **State Storage and Secrets Management**

The provisioner manages two key components:
- **State Storage**: Stores Pulumi's state (supports `s3-bucket`, `fs`, `gcp-bucket`, `pulumi-cloud`)
- **Secrets Provider**: Provides encryption for created resources' confidential outputs

### **State Storage Options**

#### **File System State Storage** (`fs`)

Stores Pulumi state locally on the file system for local development.

**Golang Struct Reference:** `pkg/clouds/fs/fs.go:StateStorageConfig`

```yaml
# server.yaml - Parent Stack
schemaVersion: 1.0
provisioner:
  type: pulumi
  config:
    state-storage:
      type: fs
      config:
        path: file:///${user:homeDir}/.sc/pulumi/state    # Local file system path for Pulumi state
```

### **Secrets Management**

#### **Passphrase Secrets Provider** (`passphrase`)

Encrypts secrets using a passphrase for local development.

**Golang Struct Reference:** `pkg/clouds/fs/fs.go:SecretsProviderConfig`

```yaml
# server.yaml - Parent Stack
schemaVersion: 1.0
provisioner:
  type: pulumi
  config:
    state-storage:
      type: gcp-bucket
      config:
        credentials: "${auth:gcloud}"
        projectId: "${auth:gcloud.projectId}"
        provision: false
        name: my-sc-state
        location: europe-west3
    secrets-provider:
      type: passphrase
      config:
        passPhrase: pass-phrase              # Passphrase for encrypting secrets
```

---

## **Container Registry Resources**

### **GCP Artifact Registry** (`gcp-artifact-registry`)

Creates and manages Google Cloud Artifact Registry repositories for Docker images.

```yaml
# server.yaml - Parent Stack
resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: 87152c65fca76d443751a37a91a77c17
      zoneName: mycompany.com
  resources:
    prod:
      template: stack-per-app-gke
      resources: &resources
        company-registry: &registry
          type: gcp-artifact-registry
          config: &registry-cfg
            projectId: "${auth:gcloud.projectId}"      # GCP project ID
            credentials: "${auth:gcloud}"              # GCP authentication
            location: europe-west3                     # Registry location
            docker:                                    # Docker-specific settings
              immutableTags: true                      # Whether tags are immutable
    prod-ru:
      template: static-website
      resources: {}
```

---

## **Alert & Notification Resources**

### **Discord Alerts**

Sends alerts to Discord channels.

**Configuration Properties:**
```yaml
alerts:
  discord:
    webhookUrl: "${env:DISCORD_WEBHOOK_URL}"
    channel: "alerts"
    username: "Simple Container Bot"
```

### **Slack Alerts**

Sends alerts to Slack channels.

**Configuration Properties:**
```yaml
alerts:
  slack:
    webhookUrl: "${env:SLACK_WEBHOOK_URL}"
    channel: "#alerts"
    username: "Simple Container Bot"
```

### **Telegram Alerts**

Sends alerts to Telegram chats.

**Configuration Properties:**
```yaml
alerts:
  telegram:
    botToken: "${env:TELEGRAM_BOT_TOKEN}"
    chatId: "${env:TELEGRAM_CHAT_ID}"
```

---

## **Complete Example: Multi-Cloud Parent Stack**

Here's a complete example showing how to define resources across multiple cloud providers in a parent stack:

```yaml
# server.yaml - Parent Stack Configuration
schemaVersion: 1.0

# Provisioner configuration
provisioner:
  type: pulumi
  config:
    state-storage:
      type: s3-bucket
      config:
        credentials: "${auth:aws-main}"
        account: "${auth:aws-main.projectId}"
        name: "myapp-terraform-state"
        provision: false
    
    secrets-provider:
      type: aws-kms
      config:
        credentials: "${auth:aws-main}"
        account: "${auth:aws-main.projectId}"
        keyName: "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012"
        provision: false

# Templates - deployment patterns
templates:
  stack-per-app-us:
    type: ecs-fargate
    config: &aws-us-cfg
      credentials: "${auth:aws-main}"
      account: "${auth:aws-main.projectId}"
  
  gcp-cloudrun:
    type: cloudrun
    config: &gcp-cfg
      credentials: "${auth:gcp-main}"
      projectId: "${auth:gcp-main.projectId}"

# Domain registrar
resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: 87152c65fca76d443751a37a91a77c17
      zoneName: myapp.com
  
  # Environment-specific resources
  resources:
    production:
      template: stack-per-app-us
      resources:
        # MongoDB Atlas for database
        main-database:
          type: mongodb-atlas
          config:
            publicKey: "${env:MONGODB_ATLAS_PUBLIC_KEY}"
            privateKey: "${env:MONGODB_ATLAS_PRIVATE_KEY}"
            orgId: "60b5d0f0f2a1b2c3d4e5f6g7"
            projectId: "60b5d0f0f2a1b2c3d4e5f6g8"
            instanceSize: "M30"
            region: "US_EAST_1"
            cloudProvider: "AWS"
            admins: ["devops@example.com"]
            developers: ["dev-team@example.com"]
        
        # S3 for file storage
        file-storage:
          type: s3-bucket
          config:
            credentials: "${auth:aws-main}"
            account: "${auth:aws-main.projectId}"
            name: "myapp-files"
            allowOnlyHttps: true
    
    staging:
      template: gcp-cloudrun
      resources:
        # Shared staging database
        staging-database:
          type: mongodb-atlas
          config:
            publicKey: "${env:MONGODB_ATLAS_PUBLIC_KEY}"
            privateKey: "${env:MONGODB_ATLAS_PRIVATE_KEY}"
            orgId: "60b5d0f0f2a1b2c3d4e5f6g7"
            projectId: "60b5d0f0f2a1b2c3d4e5f6g8"
            instanceSize: "M10"
            region: "US_EAST_1"
            cloudProvider: "AWS"
```

---

## **Resource Inheritance and Sharing**

Resources defined in the parent stack can be shared across multiple client stacks:

### **Parent Stack Resource Pool**
```yaml
# server.yaml
resources:
  # Shared database for standard customers
  shared-db:
    type: mongodb-atlas
    config:
      instanceSize: "M30"
      
  # Dedicated database for enterprise customers  
  enterprise-db:
    type: mongodb-atlas
    config:
      instanceSize: "M80"
      dedicatedTenant: true
```

### **Client Stack Resource Selection**
```yaml
# client.yaml
stacks:
  customer-standard:
    parentStack: production
    uses: [shared-db]  # Uses shared resources
    
  customer-enterprise:
    parentStack: production
    uses: [enterprise-db]  # Uses dedicated resources
```

This separation allows DevOps to define resource pools once while giving developers flexibility to choose appropriate resources for their specific needs.

---

## **Best Practices**

### **Resource Naming**
- Use descriptive names that indicate purpose: `user-database`, `file-storage`, `api-cluster`
- Include environment indicators: `prod-database`, `staging-cluster`
- Use consistent naming conventions across your organization

### **Resource Sizing**
- Start with smaller instance sizes and scale up based on actual usage
- Use auto-scaling features where available
- Monitor resource utilization to optimize costs

### **Security**
- Always use encrypted storage and transmission
- Implement proper access controls and authentication
- Use secrets management for sensitive configuration
- Enable audit logging where available

### **High Availability**
- Use multi-AZ deployments for critical resources
- Configure appropriate backup and retention policies
- Implement health checks and monitoring
- Plan for disaster recovery scenarios

### **Cost Optimization**
- Use shared resources for development and testing environments
- Implement resource tagging for cost tracking
- Set up billing alerts and budgets
- Regularly review and optimize resource usage

## **Multidimensional Resource Allocation Examples**

Simple Container's architecture enables sophisticated resource allocation patterns where DevOps defines resource pools once, and developers flexibly allocate customers to appropriate resources.

### **Example: SaaS Platform with Multiple Customer Tiers**

**Parent Stack - Resource Pools (DevOps managed):**
```yaml
# server.yaml - Infrastructure managed by DevOps once
schemaVersion: 1.0

# Deployment templates
templates:
  web-app-template:
    type: ecs-fargate
    config: &aws-cfg
      credentials: "${auth:aws-main}"
      account: "${auth:aws-main.projectId}"
  
  api-service-template:
    type: cloudrun
    config: &gcp-cfg
      credentials: "${auth:gcp-main}"
      projectId: "${auth:gcp-main.projectId}"

# Resource pools
resources:
  resources:
    production:
      template: web-app-template
      resources: &shared-resources
        # Shared databases for standard customers
        mongodb-shared-us:
          type: mongodb-atlas
          config:
            publicKey: "${env:MONGODB_ATLAS_PUBLIC_KEY}"
            privateKey: "${env:MONGODB_ATLAS_PRIVATE_KEY}"
            orgId: "60b5d0f0f2a1b2c3d4e5f6g7"
            projectId: "60b5d0f0f2a1b2c3d4e5f6g8"
            instanceSize: "M30"
            region: "US_EAST_1"
            cloudProvider: "AWS"
            
        mongodb-shared-eu:
          type: mongodb-atlas
          config:
            publicKey: "${env:MONGODB_ATLAS_PUBLIC_KEY}"
            privateKey: "${env:MONGODB_ATLAS_PRIVATE_KEY}"
            orgId: "60b5d0f0f2a1b2c3d4e5f6g7"
            projectId: "60b5d0f0f2a1b2c3d4e5f6g8"
            instanceSize: "M30"
            region: "EU_WEST_1"
            cloudProvider: "AWS"
            
        # Dedicated databases for enterprise
        mongodb-enterprise-1:
          type: mongodb-atlas
          config:
            publicKey: "${env:MONGODB_ATLAS_PUBLIC_KEY}"
            privateKey: "${env:MONGODB_ATLAS_PRIVATE_KEY}"
            orgId: "60b5d0f0f2a1b2c3d4e5f6g7"
            projectId: "60b5d0f0f2a1b2c3d4e5f6g8"
            instanceSize: "M80"
            region: "US_EAST_1"
            cloudProvider: "AWS"
        
        # Storage resources
        s3-shared-storage:
          type: s3-bucket
          config:
            credentials: "${auth:aws-main}"
            account: "${auth:aws-main.projectId}"
            name: "myapp-shared-storage"
            allowOnlyHttps: true
            
        s3-enterprise-storage:
          type: s3-bucket
          config:
            credentials: "${auth:aws-main}"
            account: "${auth:aws-main.projectId}"
            name: "myapp-enterprise-storage"
            allowOnlyHttps: true
    
    staging:
      template: api-service-template
      resources:
        # Smaller staging database
        mongodb-staging:
          type: mongodb-atlas
          config:
            publicKey: "${env:MONGODB_ATLAS_PUBLIC_KEY}"
            privateKey: "${env:MONGODB_ATLAS_PRIVATE_KEY}"
            orgId: "60b5d0f0f2a1b2c3d4e5f6g7"
            projectId: "60b5d0f0f2a1b2c3d4e5f6g8"
            instanceSize: "M10"
            region: "US_EAST_1"
            cloudProvider: "AWS"
```

**Client Stacks - Customer Allocation (Developer managed):**
```yaml
# client.yaml - Flexible customer resource allocation
stacks:
  # Standard US customers - shared resources
  customer-standard-1:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      template: web-app-template
      uses: [mongodb-shared-us, s3-shared-storage]  # Shared resources
      domain: customer1.myapp.com
      runs: [web-app]
      
  customer-standard-2:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      template: web-app-template
      uses: [mongodb-shared-us, s3-shared-storage]  # Same shared resources
      domain: customer2.myapp.com
      runs: [web-app]
  
  # EU customer - EU resources for compliance
  customer-eu:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      template: api-service-template  # Different deployment pattern
      uses: [mongodb-shared-eu, s3-shared-storage]  # EU database
      domain: customer-eu.myapp.com
      runs: [api-service]
  
  # Enterprise customer - dedicated resources
  enterprise-customer:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      template: web-app-template
      uses: [mongodb-enterprise-1, s3-enterprise-storage]  # Dedicated resources
      domain: enterprise.myapp.com
      runs: [web-app]
      scale:
        min: 5
        max: 20  # Higher scaling for enterprise
```

### **Scaling Benefits Demonstrated**

**1. Resource Pool Management:**
- DevOps defines resource pools once (`mongodb-shared-us`, `mongodb-enterprise-1`)
- Developers allocate customers flexibly using `uses` directive
- Easy migration between tiers by changing `uses` configuration

**2. Cost Optimization:**
- Standard customers share `mongodb-shared-us` (cost-effective)
- Enterprise customers get dedicated `mongodb-enterprise-1` (performance)
- Automatic resource utilization optimization

**3. Geographic Compliance:**
- EU customers automatically use `mongodb-shared-eu` for data residency
- US customers use `mongodb-shared-us`
- Simple configuration change for compliance

**4. Performance Tier Migration:**
```yaml
# Before: Customer on shared resources
customer-upgrade:
  uses: [mongodb-shared-us]
  
# After: Customer on dedicated resources (one line change!)
customer-upgrade:
  uses: [mongodb-enterprise-1]  # Zero downtime migration
```

### **Real-World Scaling Scenarios**

**Adding 100 New Customers:**
```yaml
# Traditional approach: 5000+ lines of infrastructure code
# Simple Container: 5 lines per customer = 500 lines total

customer-001:
  parent: myorg/infrastructure
  config:
    uses: [mongodb-shared-us]
    domain: customer001.myapp.com

customer-002:
  parent: myorg/infrastructure
  config:
    uses: [mongodb-shared-us]
    domain: customer002.myapp.com
    
# ... 98 more customers with minimal configuration
```

**Multi-Region Expansion:**
```yaml
# Add new parent stack for EU region
# .sc/stacks/myapp-eu/server.yaml
resources:
  mongodb-eu-cluster:
    type: mongodb-atlas
    config:
      region: EU_WEST_1

# Client stacks choose regions easily
eu-customer:
  parent: myorg/myapp-eu  # EU parent stack
  config:
    uses: [mongodb-eu-cluster]
```

This comprehensive reference covers all supported resources in Simple Container. The multidimensional resource allocation approach enables organizations to scale from startup to enterprise without operational complexity growth.

For specific implementation examples and tutorials, refer to the [How-To Guides](./howto/) section.
