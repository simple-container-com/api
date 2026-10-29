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

This document lists every cloud resource type SC can provision in a parent
stack (`server.yaml`), with the config keys each accepts.

## Where resources live in `server.yaml`

The `resources` block in `server.yaml` is **three nesting levels deep** — the
literal key `resources:` appears three times. This is intentional: the outer
`resources` is the per-stack container, the middle `resources` is the per-env
map, and the innermost `resources` is the per-resource map. Skipping any
level is invalid.

```yaml
schemaVersion: "1.0"
# ...
resources:                              # (1) per-stack container
  registrar:                            #     single registrar shared across envs
    type: cloudflare
    config: { ... }
  resources:                            # (2) per-env map
    staging:                            #     env name
      template: ecs-fargate-template    #     optional; ref into templates[]
      resources:                        # (3) per-resource map
        main-db:                        #     user-chosen resource name
          type: aws-rds-postgres        #     see the type tables below
          name: main-db
          config:
            # resource-specific keys (see per-type sections)
    production:
      # ... same shape
```

For the full canonical shape (provisioner, secrets, cicd, templates,
variables) the source of truth is the Go struct `ServerDescriptor` in
`pkg/api/server.go` of the [simple-container-com/api](https://github.com/simple-container-com/api)
repository.

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
      dockerComposeFile: ./docker-compose.yaml
      uses: [mongodb-shared-us, s3-storage]  # Consume parent resources
      domain: staging.myapp.com
  
  # Custom stack name with parentEnv directive
  customer-a:
    type: cloud-compose
    parent: myorg/infrastructure
    parentEnv: production  # Consumes resources.resources.production from parent
    config:
      dockerComposeFile: ./docker-compose.yaml
      uses: [mongodb-shared-us, s3-storage]
      domain: customer-a.myapp.com
```

## **Supported Cloud Providers**

- **AWS** - Amazon Web Services
- **Google Cloud Platform (GCP)** - Google Cloud
- **Yandex Cloud** - Serverless Containers, Object Storage (incl. static websites), Cloud DNS
- **Cloud.ru** - Evolution Container Apps, Artifact Registry, Cloud DNS, and compute VMs
  on both the Evolution and the Advanced (ex-SberCloud) estate
- **VM & self-hosted Kubernetes** - a plain machine (`vm`) and a k3s cluster on it
  (`k3s-cluster`), on any cloud or none
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
      dockerComposeFile: ./docker-compose.yaml
      uses: [mongodb-shared, s3-storage]  # Consumes parent resources
      domain: customer-a.myapp.com
      runs: [web-app]
  
  # Or with custom stack name using parentEnv
  customer-a:
    type: cloud-compose
    parent: myorg/infrastructure
    parentEnv: production  # Consumes resources.resources.production from parent
    config:
      dockerComposeFile: ./docker-compose.yaml
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

**Client Access:**

When this resource is used in a client stack via the `uses` section, Simple Container automatically injects environment variables and template placeholders for S3 bucket access.

**For complete details on environment variables and template placeholders, see:** [Template Placeholders Advanced - AWS S3 Bucket](../concepts/template-placeholders-advanced.md#s3-bucket)

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

**Client Access:**

When this resource is used in a client stack via the `uses` section, Simple Container automatically injects environment variables and template placeholders for PostgreSQL database connection.

**For complete details on environment variables and template placeholders, see:** [Template Placeholders Advanced - AWS RDS PostgreSQL](../concepts/template-placeholders-advanced.md#rds-postgresql)

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

**Client Access:**

When this resource is used in a client stack via the `uses` section, Simple Container automatically injects environment variables and template placeholders for MySQL database connection.

**For complete details on environment variables and template placeholders, see:** [Template Placeholders Advanced - AWS RDS MySQL](../concepts/template-placeholders-advanced.md#rds-mysql)

#### **CloudTrail Security Alerts** (`aws-cloudtrail-security-alerts`)

Creates CloudWatch metric filters and alarms for security-relevant CloudTrail events, aligned with the AWS Security Hub/CIS CloudWatch controls (CloudWatch.1 through CloudWatch.14).

**Golang Struct Reference:** `pkg/clouds/aws/cloudtrail_security_alerts.go:CloudTrailSecurityAlertsConfig`

```yaml
# server.yaml - Parent Stack
resources:
  resources:
    production:
      resources:
        cloudtrail-security:
          type: aws-cloudtrail-security-alerts
          config:
            # AWS account configuration (inherited from AccountConfig)
            credentials: "${auth:aws-us}"
            account: "${auth:aws-us.projectId}"

            # CloudTrail log group (required)
            logGroupName: "aws-cloudtrail-logs-s3-buckets"
            logGroupRegion: "us-west-2"  # Optional: if different from default region

            # CloudTrail trail pre-flight check (optional, recommended for
            # compliance deployments). When trailName is set, SC refuses to
            # deploy unless the trail has log-file validation enabled — so
            # you can't accidentally alert on top of a tamperable trail.
            trailName: "cloudtrail_events"
            # requireLogFileValidation: false  # downgrade failure to warning

            # Notification channels — any combination, or none
            email:
              addresses:
                - security@company.com
            slack:
              webhookUrl: "${secret:security-slack-webhook}"
            # discord:
            # webhookUrl: "${secret:security-discord-webhook}"
            # telegram:
            # chatID: "-1001234567890"
            # token: "${secret:security-telegram-token}"

            # Alert selectors (all default to false)
            alerts:
              rootAccountUsage: true        # CIS CloudWatch.1
              unauthorizedApiCalls: true     # CIS CloudWatch.2  (threshold: 5)
              consoleLoginWithoutMfa: true   # CIS CloudWatch.3
              iamPolicyChanges: true         # CIS CloudWatch.4
              cloudTrailTampering: true      # CIS CloudWatch.5
              failedConsoleLogins: true      # CIS CloudWatch.6
              kmsKeyDeletion: true           # CIS CloudWatch.7
              s3BucketPolicyChanges: true    # CIS CloudWatch.8
              configChanges: true            # CIS CloudWatch.9
              securityGroupChanges: true     # CIS CloudWatch.10
              naclChanges: true              # CIS CloudWatch.11
              networkGatewayChanges: true    # CIS CloudWatch.12
              routeTableChanges: true        # CIS CloudWatch.13
              vpcChanges: true               # CIS CloudWatch.14
```

**Notification enrichment:** When webhook delivery is enabled (Slack/Discord/Telegram), each alert message includes the CloudTrail events that actually fed the alarm — event name, actor (assumed-role name for CI deploys, IAM user name otherwise), source IP, and UTC timestamp — looked up via `logs:FilterLogEvents` on the CloudTrail log group within the alarm's evaluation window. The Lambda execution role is granted `logs:FilterLogEvents` scoped to just the configured log-group ARN (not `*`). Enrichment is best-effort: if the lookup fails (permission/timeout/region mismatch) the notification still goes out with just the alarm metadata.

**Compliance:** SOC 2 (CC6/CC7), ISO 27001:2022 (A.5/A.8), NIST 800-53 (AU-6, AC-2, SI-4)

**Scope:** CloudTrail log groups are account-wide. Declare this resource in **exactly one environment block per AWS account** — declaring it in multiple environments (e.g. `staging` and `prod`) that target the same account produces duplicate metric filters that all match the same events, leading to duplicate notifications. Multiple accounts (e.g. EU vs US) get independent alert sets, which is supported.

**Note:** Does not require `uses` in client stacks — the resource monitors the CloudTrail log group directly.

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
  DATABASE_URL: "postgresql://<USER>:<PASS>@<host>:5432/db"
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
  CLOUDFLARE_API_TOKEN: "<your-cloudflare-api-token>"
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
      dockerComposeFile: ./docker-compose.yaml
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
            gkeMinVersion: 1.33.4-gke.1245000  # Check: gcloud container get-server-config --location=europe-west3
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
            gkeMinVersion: "1.33.4-gke.1245000"  # Check: gcloud container get-server-config --location=europe-west3
            
            # Networking Configuration
            privateVpc: true                             # Creates dedicated VPC with automatic peering (optional)
            externalEgressIp:
              enabled: true                              # Enables CloudNAT with static IP (optional)
              # existing: "projects/my-project/regions/europe-west3/addresses/my-static-ip"  # Use existing IP (optional)

            # Control plane reachability (optional). Left out entirely, GKE
            # keeps its default: an IP endpoint that accepts connections from
            # any address on the internet. Naming a key here hands that field
            # to Pulumi, so omit the ones you want left alone.
            #
            # Removing this block later does NOT restore the defaults, it plans
            # the allow list away and re-opens the control plane. To widen
            # access, widen the list.
            #
            # If an allow list locks you out, recover through the GCP API, which
            # does not go through the cluster endpoint:
            #   gcloud container clusters update CLUSTER --region REGION \
            #     --master-authorized-networks=YOUR.IP.ADDR/32
            controlPlaneAccess:
              # IAM-authorised endpoint, reachable from any source address.
              # Callers without a stable address, hosted CI runners above all,
              # need this before the allow list below can be narrowed.
              dnsEndpoint: true
              # GKE defaults this to true, which authorises every Google Cloud
              # tenant alongside the list.
              allowGcpPublicCidrs: false
              authorizedNetworks:
                - name: office
                  cidr: 203.0.113.0/24
              # ipEndpoint: false   # only once nothing dials the IP endpoint;
                                    # the generated kubeconfig then uses the DNS
                                    # endpoint. IPv6 entries above need a
                                    # dual-stack cluster.
            caddy:
              enable: true
              namespace: caddy
              replicas: 2
              # VPA Configuration for Caddy ingress controller
              vpa:
                enabled: true
                updateMode: "Auto"  # Off, Initial, Auto, InPlaceOrRecreate
                minAllowed:
                  cpu: "50m"
                  memory: "64Mi"
                maxAllowed:
                  cpu: "1"
                  memory: "1Gi"
                controlledResources: ["cpu", "memory"]
              # Optional: Manual resource limits alongside VPA
              resources:
                limits:
                  cpu: "500m"
                  memory: "512Mi"
                requests:
                  cpu: "100m"
                  memory: "128Mi"
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
            gkeMinVersion: "1.33.4-gke.1245000"  # Check: gcloud container get-server-config --location=europe-west3
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

Creates and manages Google Cloud Storage buckets with S3-compatible access via HMAC keys.

**Golang Struct Reference:** `pkg/clouds/gcloud/bucket.go:GcpBucket`

**JSON Schema:** [GcpBucket Schema](https://github.com/simple-container-com/api/tree/main/docs/schemas/gcp/gcpbucket.json)

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
            name: "my-application-storage"           # Bucket name (optional, defaults to resource name)
            location: "US"                           # Bucket location
```

**Client Access:**

When this resource is used in a client stack via the `uses` section, Simple Container automatically injects environment variables and template placeholders for GCS bucket access with S3-compatible HMAC authentication.

**For complete details on environment variables and template placeholders, see:** [Template Placeholders Advanced - GCP Bucket](../concepts/template-placeholders-advanced.md#gcp-bucket)

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

##### Image retention (`cleanupPolicies`)

Artifact Registry keeps every image version forever unless a cleanup policy says otherwise, and storage is billed per GB. Retention can be declared here so it is reviewed as code.

```yaml
            cleanupPolicies:
              - name: delete-untagged-older-30d
                action: DELETE
                condition:
                  tagState: UNTAGGED
                  olderThan: 30d                     # or "2592000s"; both accepted, integers only
              - name: keep-most-recent-20
                action: KEEP                         # KEEP wins over a matching DELETE
                mostRecentVersions:
                  keepCount: 20
            cleanupPolicyDryRun: true                # report only; set false to delete
```

Three behaviours are worth knowing before you use this.

**Omitting `cleanupPolicies` means Simple Container does not manage retention.** Any policy set outside SC — through `gcloud` or the console — is left alone. This is the default and it is deliberate: the field is authoritative in the provider, so a resource that declares nothing would otherwise *delete* whatever is configured.

**Declaring it makes Simple Container authoritative.** Policies set outside SC are then replaced by the declared list on the next provision. An explicitly empty list (`cleanupPolicies: []`) means "managed, and I want none", which is how retention is removed.

Note two asymmetries. Writing the key with no value (`cleanupPolicies:` alone) decodes as *absent*, i.e. unmanaged — visually almost identical to `[]`, which removes every policy. And once SC has managed the field, **deleting the block does not return the repository to out-of-band control**: Pulumi's `ignoreChanges` carries the previous value forward from state, so retention freezes at the last declared list and later console edits are reverted on each provision. Genuinely handing the field back requires removing the property from stack state.

**`cleanupPolicyDryRun` defaults to `true`.** Nothing is deleted until it is explicitly set to `false`. Dry run evaluates the policies and reports what they would remove, so run it first and read the result: deleting an image that is still deployed makes the next node reschedule fail to pull, and no provision can restore a deleted layer.

Policies are validated while the Pulumi program is evaluated, and one is rejected if it would match far more than it appears to: an empty `condition`, a `DELETE` that does not set `olderThan` or target `tagState: UNTAGGED`, a `KEEP` with no `keepCount`, an empty prefix, or a non-positive duration. Prefixes are deliberately **not** accepted as narrowing a `DELETE` — they select which packages a policy covers, not which ages, so `packageNamePrefixes` alone would delete the running version of that package.

An unrecognised key in `server.yaml` is silently ignored rather than rejected, so a mistyped condition field would otherwise produce a policy matching every version, which is what these checks are for.

### **Database Resources**

#### **Cloud SQL PostgreSQL** (`gcp-cloudsql-postgres`)

Creates and manages Google Cloud SQL PostgreSQL databases.

**Golang Struct Reference:** `pkg/clouds/gcloud/postgres.go:PostgresGcpCloudsqlConfig`

**JSON Schema:** [PostgresGcpCloudsqlConfig Schema](https://github.com/simple-container-com/api/tree/main/docs/schemas/gcp/postgresgcpcloudsqlconfig.json)

**Configuration Properties:**
```yaml
# server.yaml - Parent Stack
resources:
  resources:
    production:
      resources:
        my-cloudsql-postgres:
          type: gcp-cloudsql-postgres
          config:
            # GCP credentials and project (inherited from Credentials)
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            
            # PostgreSQL configuration (from PostgresGcpCloudsqlConfig struct)
            project: "my-gcp-project"                    # GCP project ID
            version: "POSTGRES_14"                       # PostgreSQL version
            tier: "db-f1-micro"                         # Instance tier
            region: "us-central1"                       # GCP region
            maxConnections: 100                          # Maximum connections (optional, deprecated: prefer databaseFlags)
            databaseFlags:                               # Arbitrary Cloud SQL database flags (optional)
              cloudsql.iam_authentication: "on"          # e.g. enable IAM database authentication
              log_min_duration_statement: "500"          # max_connections here overrides maxConnections
            deletionProtection: true                     # Enable deletion protection (optional)
            queryInsightsEnabled: false                  # Enable query insights (optional)
            queryStringLength: 1024                      # Query string length limit (optional)
            privateNetwork: "projects/my-gcp-project/global/networks/my-vpc"  # Private IP on this VPC (optional, restarts an existing instance)
            privateServicesAccessRange: "10.30.0.0/20"   # SC creates Private Services Access on privateNetwork (optional)
            proxyPrivateIp: true                         # proxy dials the private IP, public IP stays on (optional)
            publicIpEnabled: true                        # false removes the public IP; implies proxyPrivateIp (optional)
            usersProvisionRuntime:                       # User provisioning runtime (optional)
              type: "kubernetes"
              resourceName: "postgres-job-runner"
```

**Private IP:** `privateNetwork` needs Private Services Access on the VPC. With `privateServicesAccessRange` (an RFC 1918 CIDR, /24 or larger) SC reserves the range and creates the servicenetworking peering before the instance; leave it unset if the VPC already has one. The peering is one per VPC, so set the range on at most one resource per network, and treat it as immutable: removing it or destroying the stack keeps the range and the peering (delete them with `gcloud services vpc-peerings delete` and `gcloud compute addresses delete` once no instance uses them). Not supported with `adopt: true`. Adding a private IP to an existing instance restarts it; the proxy keeps using the public IP until `proxyPrivateIp: true` (or `publicIpEnabled: false`). To move an existing fleet without a cutover, set `proxyPrivateIp: true` first, and only after the instance shows a `PRIVATE` address (`gcloud sql instances describe <instance> --format='value(ipAddresses)'`): a client deployed earlier gets a proxy with nothing to dial. Each client stack switches on its next deploy while the public IP still serves the rest. Before turning the public IP off, confirm no proxy still lacks the flag: `kubectl get pods -A -o json | jq -r '.items[] | select(any(.spec.initContainers[]?, .spec.containers[]?; .name == "cloudsql-proxy" and (.args | index("--private-ip") | not))) | .metadata.namespace + "/" + .metadata.name'` must print nothing.

**Database flag removal semantics:** on freshly provisioned instances, deleting an entry from `databaseFlags` reverts that flag to its engine default on the next update (static flags restart the instance). On **adopted** instances (`adopt: true`), flags are only managed while `databaseFlags` is set — deleting the whole block re-enters drift protection and leaves the last-applied flags in place; to disable a flag, set it to its off value (e.g. `"off"`) instead of removing the line.

**Client Access:**

When this resource is used in a client stack via the `uses` section, Simple Container automatically injects environment variables and template placeholders for PostgreSQL connection details.

**For complete details on environment variables and template placeholders, see:** [Template Placeholders Advanced - GCP PostgreSQL Cloud SQL](../concepts/template-placeholders-advanced.md#postgresql-cloud-sql)

**Connectivity (Cloud SQL Auth Proxy):**

Your container connects to Cloud SQL over `localhost` — the injected `*.host`/`*.port` placeholders resolve to `localhost:5432`. Simple Container runs the [Cloud SQL Auth Proxy](https://cloud.google.com/sql/docs/postgres/sql-proxy) for you as a **native sidecar** (a Kubernetes init container with `restartPolicy: Always`) alongside the app container in long-lived Deployments, and in the db-user-init Job that grants the stack's database user its roles. Because it is a native sidecar with a startup probe on the proxy's `--health-check` endpoint, **Kubernetes does not start your app container until the proxy is accepting connections** — so the app never races ahead and logs `connection refused` to `localhost:5432` on pod startup, restart, node scale-down, or VPA eviction. Requires GKE ≥ 1.29 (the `SidecarContainers` feature, GA in 1.33). No extra configuration is needed.

#### **Redis** (`gcp-redis`)

Creates and manages Google Cloud Memorystore Redis instances.

**Golang Struct Reference:** `pkg/clouds/gcloud/redis.go:RedisConfig`

**JSON Schema:** [RedisConfig Schema](https://github.com/simple-container-com/api/tree/main/docs/schemas/gcp/redisconfig.json)

**Configuration Properties:**
```yaml
# server.yaml - Parent Stack
resources:
  resources:
    production:
      resources:
        my-redis:
          type: gcp-redis
          config:
            # GCP credentials and project (inherited from Credentials)
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            
            # Redis configuration (from RedisConfig struct)
            project: "my-gcp-project"               # GCP project ID
            version: "6.2"                         # Redis version
            region: "us-central1"                  # GCP region
            memorySizeGb: 1                        # Memory size in GB
            redisConfig:                           # Redis configuration map
              maxmemory-policy: "allkeys-lru"
              timeout: "300"
```

**Client Access:**

When this resource is used in a client stack via the `uses` section, Simple Container automatically injects environment variables and template placeholders for Redis connection details.

**For complete details on environment variables and template placeholders, see:** [Template Placeholders Advanced - GCP Redis Memorystore](../concepts/template-placeholders-advanced.md#redis-memorystore)

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
      credentials: "${env:GCP_SERVICE_ACCOUNT_KEY}"   # service-account key JSON
```

**Ambient credentials (no key).** Leave `credentials` empty and every GCP client uses
Application Default Credentials instead: in GitHub Actions, the file that
`google-github-actions/auth` writes after exchanging the job's OIDC token through Workload
Identity Federation; elsewhere, a metadata server or `gcloud auth application-default login`.
No long-lived key exists to leak. A configured value must still be a service-account key.

```yaml
auth:
  gcloud:
    type: gcp-service-account
    config:
      projectId: "my-gcp-project"
      credentials: ""                                   # use ADC
      serviceAccount: deployer@my-gcp-project.iam.gserviceaccount.com  # static websites only
```

`serviceAccount` is read only in ambient mode, and only by resources that grant the deploying
identity access to themselves (a static website's bucket write binding), which otherwise take
the email from the key.

In ambient mode, inside GitHub Actions:

- The credentials file stays available when the job has no checkout step. The action clones
  the repository itself and replaces the workspace, so it first copies credential files it
  finds there (`GOOGLE_APPLICATION_CREDENTIALS`, `CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE`,
  `GOOGLE_GHA_CREDS_PATH`) to a private directory and points the variables at the copies.
- gcloud is signed in with that file (`gcloud auth login --cred-file`), because GKE
  kubeconfigs authenticate through `gke-gcloud-auth-plugin`, which asks gcloud, and gcloud
  does not read Application Default Credentials. The sign-in uses a gcloud configuration
  private to the run (`CLOUDSDK_CONFIG`), so later steps of the job keep their account.
  Outside GitHub Actions gcloud is left as the user configured it.
- With `provision: true` on the state storage, the identity reads the state bucket's
  metadata (`storage.buckets.get`). Without it, the bucket is taken to exist and its
  lifecycle is not checked; it is created only when Cloud Storage reports it missing.

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

## **Yandex Cloud Provider**

### **Templates** (`TemplateType` → `templates` section in `server.yaml`)

#### **Serverless Container** (`yc-serverless-container`)

Deploys a single container image as a Yandex Cloud Serverless Container. Analogous
to AWS Lambda under `single-image`.

**Golang Struct Reference:** `pkg/clouds/yandex/serverless_container.go:ServerlessContainerInput`

```yaml
# server.yaml — Parent Stack (DevOps managed)
templates:
  yc-container:
    type: yc-serverless-container
    config:
      credentials: "${auth:yc}"
      # `registryId` and `serviceAccountId` are optional; if unset SC
      # creates one Container Registry per stack and one SA per container
      # with `container-registry.images.puller` + `lockbox.payloadViewer`.
```

```yaml
# client.yaml — Client Stack
stacks:
  staging:
    type: single-image
    template: yc-container
    parent: myproject/devops
    config:
      domain: myservice.example.ru
      image: { dockerfile: ${git:root}/Dockerfile }
      maxMemory: 128     # min 128 MB, multiple of 128
      timeout: 30        # 30 s free tier, up to 3600 s paid
      uses: [blobs]
      env: { LOG_LEVEL: info }
      secrets:
        SOME_TOKEN: "${secret:MY_SERVICE_TOKEN}"
```

#### **Static Website** (`yc-static-website`)

Serves a `type: static` client stack from an Object Storage bucket with website
hosting — the YC analogue of `aws-static-website`, without a CDN in front (Russian
ISPs cut Cloudflare responses after 16 KB, which is why the `.ru` sites are not
Cloudflare-fronted). One deploy:

- creates the bucket **named after the domain** (YC serves bucket-hosted HTTPS only
  when bucket name == host), website hosting on (index + error document), public
  read of objects only (listing stays private);
- attaches a Certificate Manager certificate — an existing one by `certificateId` /
  `certificateName` (e.g. a zone wildcard), or, with neither set, a new Let's
  Encrypt certificate validated by a DNS-CNAME challenge written into the same zone
  (the deploy waits for validation, typically a few minutes on first deploy);
- writes `CNAME <domain>. → <domain>.website.yandexcloud.net.` in the yc-dns zone;
- uploads `bundleDir` with `Cache-Control` rules and deletes objects no longer in
  the build (`aws s3 sync --delete` semantics). Defaults: the index document
  `no-cache`, `assets/**` `public, max-age=31536000, immutable`, everything else
  `public, max-age=3600`. The upload is skipped during a preview.

Uploads use a dedicated service account with `storage.editor` on that bucket only.
The deploying service account needs `storage.admin` (create the bucket, set its
access flags and IAM), `dns.editor`, `iam.serviceAccounts.admin`,
`certificate-manager.editor` (managed cert) or `certificate-manager.certificates.downloader`
(adopted cert).

**Golang Struct Reference:** `sc-plugin-yandex/api/static_website.go:StaticWebsiteInput`

```yaml
# server.yaml — Parent Stack
templates:
  yc-site:
    type: yc-static-website
    config:
      credentials: "${auth:yc}"
      zoneName: simple-forge.ru                # yc-dns zone the CNAME goes into
      certificateName: simple-forge-ru-cert    # or certificateId; omit both for a managed LE cert
      spaFallback: true                        # unknown paths → index document
      # cacheControl:                          # optional; first match wins
      #   - { pattern: "assets/**", value: "public, max-age=31536000, immutable" }
      #   - { pattern: "*.html",    value: "no-cache" }
```

```yaml
# client.yaml — Client Stack
stacks:
  prod:
    type: static
    template: yc-site
    parent: myproject/devops
    config:
      bundleDir: ${git:root}/dist
      domain: twopeaks.simple-forge.ru         # must be a subdomain inside zoneName
      indexDocument: index.html
      # errorDocument: 404.html                # overrides spaFallback
```

### **Resources** (`ResourceType` → `resources` section in `server.yaml`)

#### **Object Storage Bucket** (`yc-bucket`)

Provisions a YC Object Storage bucket, its owning service account, folder-role
binding, and a static S3 key pair. The S3 credentials are exported so any client
stack listing `uses: [<name>]` picks them up as `${resource:<name>.*}`.

**Golang Struct Reference:** `pkg/clouds/yandex/bucket.go:ObjectStorageBucket`

```yaml
# server.yaml — Parent Stack
resources:
  resources:
    staging:
      resources:
        blobs:
          type: yc-bucket
          config:
            credentials: "${auth:yc}"
            name: myproject-blobs
            forceDestroy: true       # allow `sc destroy` to empty the bucket
```

#### **Object Storage (state backend)** (`yc-object-storage`)

Pulumi state in a YC Object Storage bucket, reached via the S3-compatible API and
`gocloud`'s `s3blob` backend. `provision: false` is required — a state backend
that provisions itself is chicken-and-egg, and there is no ProvisionFunc registered
for this state-storage type. Create the bucket by hand first (`yc storage bucket
create --name <name>`).

```yaml
# server.yaml
provisioner:
  type: pulumi
  config:
    state-storage:
      type: yc-object-storage
      config:
        credentials: "${auth:yc}"
        bucketName: myproject-sc-yc-state
        provision: false
```

The three query parameters SC emits on the state URL — `endpoint`, `region`,
`s3ForcePathStyle` — are the exact set gocloud accepts on this backend. Adding
any other parameter fails login-time.

#### **Container Registry** (bridged, auto-created)

SC creates one `yandex_container_registry` per stack (named `<image-name>-registry`)
during the deploy of a `yc-serverless-container` — it is not a stand-alone
resource type in `server.yaml`. Every image push must collapse `--` runs in the
repository segment: YC Container Registry enforces the *legacy* Docker repository
grammar, which allows at most one separator between components. SC's default
image name is `<stack>--<env>`, so `toRepositoryName` collapses it to
`<stack>-<env>` before pushing. Container, SA and trigger names *do* accept `--`.

### **Authentication** (`AuthType` → `auth` section in `secrets.yaml`)

#### **YC Service Account** (`yc-service-account`)

The credentials blob for every `yc-*` resource. Combines the service-account key
(gRPC auth) with a static access key pair (S3-signing auth), cloud and folder ids,
and region/zone defaults.

```yaml
# secrets.yaml
auth:
  yc:
    type: yc-service-account
    config:
      cloudId: b1g...
      folderId: b1g...
      serviceAccountKey: |
        { "id": "...", "service_account_id": "...", ... }
      accessKey: "YCAJE..."
      secretAccessKey: "..."
      region: ru-central1
      zone: ru-central1-a
```

**Important:** `${auth:yc}` resolves into the opaque `Credentials` blob and does
NOT fan out into sibling `folderId`, `cloudId` etc. Custom resources must call
`api.ConvertAuth(cfg, &cfg.AccountConfig)` themselves.

### **Registrar** (`RegistrarType` → `registrars` section in `server.yaml`)

#### **Yandex Cloud DNS** (`yc-dns`)

Publishes services under a YC-hosted zone through an API Gateway edge. Two
lookup quirks worth knowing:

- The `zoneName` is the YC **resource** name, not the DNS zone name — the zone
  serving `example.ru.` is a resource named `example-ru`. Set `zoneId:`
  directly to skip guessing.
- The certificate is **adopted**, never issued: `certificateId` is required and
  must reference a wildcard cert covering the zone. A second managed certificate
  for the same domain contends with the first's renewal via the single
  `_acme-challenge.<zone>` CNAME target and fails silently up to 90 days later.

```yaml
# server.yaml
resources:
  registrars:
    yandex:
      type: yc-dns
      config:
        credentials: "${auth:yc}"
        zoneName: example-ru               # YC resource name
        # or: zoneId: dns...
        certificateId: fpqu...             # adopted wildcard cert
```

Records under `NS`, `SOA` and anything below `_acme-challenge.` are refused at
conversion time. TTL defaults to 300 (`Ttl 1` is not the "automatic" sentinel
that some other providers use).

### **Provisioner Configuration**

#### **Passphrase Secrets Provider** (`passphrase`)

Only functional secrets-provider for YC today — `yc-kms` has no ProvisionFunc
registered in the current build. Any random string works.

```yaml
# server.yaml
provisioner:
  type: pulumi
  config:
    secrets-provider:
      type: passphrase
      config:
        passPhrase: "${secret:state-passphrase}"
```

### **Runtime secrets** (`yc-lockbox`, auto-created)

Runtime secrets are delivered as **Lockbox references**, not `${secret:…}`
placeholders. SC creates one `LockboxSecret` per stack, one `LockboxSecretVersion`
holding every entry, and the container revision receives references keyed by
entry name. A stack with no `secrets:` entries produces no Lockbox resource at
all — YC rejects a `LockboxSecretVersion` with zero entries. There is no
`${lockbox:…}` interpolation.

On `sc destroy` the Lockbox secret enters YC's usual `PENDING_DELETE` state; this
is YC platform behaviour, not an SC bug.

For the full YC guide (parent + client + destroy hazards), see
[Yandex Cloud](../guides/parent-yandex-cloud.md).

---

## **Cloud.ru Provider**

**Cloud.ru sells two unrelated clouds under one brand, one console and one bill**,
and a credential for one does not authenticate against the other. Which one your
account has decides every type below, so settle this first:

| | **Evolution** | **Advanced** (ex-SberCloud) |
|---|---|---|
| What it is | Cloud.ru's own platform | the ex-SberCloud estate, OpenStack-descended by way of Huawei Cloud |
| Credential | service-account key id + secret → `iam.api.cloud.ru` | Keystone access/secret pair → `iam.<region>.hc.sbercloud.ru` |
| SC auth type | `cloudru-evolution-key` | `cloudru-access-key` |
| Regions | **none** — one global endpoint, placement is the AZ (`ru.AZ-1`…`ru.AZ-3`) | `ru-moscow-1` and friends; `region` is required |
| Serverless | **Container Apps** (`cloudru-container-app`) | — |
| Machines | `vm` with `provider: cloudru-evolution-vm` | `vm` with `provider: cloudru-vm` |
| DNS | Cloud DNS (`cloudru-dns`) | ex-SberCloud DNS, not wired into SC |
| Registry | Artifact Registry (the same key pair is the `docker login`) | SWR, not wired into SC |

**Golang Struct Reference:** `pkg/clouds/cloudru/init.go` — the package doc states
the split and why there are two auth types rather than one with optional fields.

There is no `cloudru` *resource* type. A machine in either estate is a plain `vm`
resource (see [VM & Self-Hosted Kubernetes](#vm-self-hosted-kubernetes) below)
whose `provider:` names the Cloud.ru driver; everything above the cloud API —
cloud-init, SSH readiness, compose, k3s — is the cloud-independent code those
types already share.

### **Templates** (`TemplateType` → `templates` section in `server.yaml`)

#### **Container App** (`cloudru-container-app`)

Deploys one `type: single-image` client stack as a Cloud.ru **Evolution**
Container App: public HTTPS, scale to zero. This cloud's analogue of
`yc-serverless-container` and of an AWS Lambda.

Reached over Cloud.ru's public REST API, not through a Terraform provider —
Container Apps appear in none of them, including the parameterized bridge of
`cloud-ru/evo-terraform`.

**Golang Struct Reference:** `pkg/clouds/cloudru/container_app.go:ContainerAppTemplateConfig`

```yaml
# server.yaml — Parent Stack (DevOps managed)
templates:
  cloudru-app:
    type: cloudru-container-app
    config:
      credentials: "${auth:cloudru}"
      # Optional. When empty SC creates (and reuses) one Artifact Registry named
      # after the client stack with the environment stripped, so every env of a
      # stack shares a registry and the repository inside it carries `--<env>`.
      # Set this to share one registry across several stacks.
      # registryName: myproject
```

```yaml
# client.yaml — Client Stack
stacks:
  staging:
    type: single-image
    template: cloudru-app
    parent: myproject/devops
    config:
      image: { dockerfile: ${git:root}/Dockerfile }
      maxMemory: 256       # default 256 MB
      timeout: 10          # default 10 s
      env: { LOG_LEVEL: info }
      secrets:
        SOME_TOKEN: "${secret:MY_SERVICE_TOKEN}"
      cloudExtras:
        containerPort: 8080   # the one field a stack moving to this cloud must add
```

`cloudExtras` on this template (`ContainerAppExtras`):

| Key | Default | Notes |
|---|---|---|
| `containerPort` | `8080` | the port the image listens on. Cloud.ru needs it explicitly, unlike YC (which injects `PORT`) and Lambda (which has no port) |
| `privileged` | `false` | runs the container as root. **Create-only** — the platform's patch request has no such field, so changing it on a live app is a destroy-and-recreate, and the driver says so rather than patching around it |
| `cpu` | `"0.1"` | CPU *request* as a core fraction |
| `concurrency` | platform default | simultaneous requests per instance before scale-out |
| `idleTimeout` | `60s` | how long an idle instance survives before scale-to-zero. Cold start after it is ~3.1 s against ~0.68 s warm, so anything interactive wants `scale.min: 1` rather than a longer idle window |
| `certificateId` | — | **required to use a custom `domain` at all**: the platform wildcard covers only its own `*.containerapps.ru` zone, and there is no ACME on this path |
| `containerName` | derived | the name of the container inside the app template; regex-checked independently of the app's own, same 5-character minimum |

**`PORT` is a reserved environment-variable name** and setting it is not a
warning or a per-variable skip — the create/update call fails and **no revision is
published**:

```
code 3, Invalid argument; fieldViolations[0]: field "name",
"The container env variable name 'PORT' is reserved. …; input=PORT"
```

Same reservation as YC Serverless Containers. Use `cloudExtras.containerPort`.

**Names are regex-checked** against `^[a-z][a-z0-9-]{3,48}[a-z0-9]$` (5–50
characters) and a violation comes back as a bare 400 naming no field, so SC checks
it first. The five-character minimum catches the obvious container name `app`.

**An unprivileged container cannot bind a low port or write to a root-owned path
in its own image.** Plain `nginx` CrashLoops on
`mkdir("/var/cache/nginx/client_temp") failed (13: Permission denied)`. The fix is
an image that runs as a normal user, or `privileged: true` granted deliberately —
SC will not default it on, because that would hand root to every image it ever
deploys on this cloud.

**Stack outputs**, for client stack `<stack>` and env `<env>`:

| Output | Contents |
|---|---|
| `<stack>-<env>-container-app-url` | the public HTTPS URL |
| `<stack>-<env>-container-app-id` | the Container App id |
| `<stack>-<env>-container-app-revision` | the live revision |

### **Authentication** (`AuthType` → `auth` section in `secrets.yaml`)

#### **Evolution service-account key** (`cloudru-evolution-key`)

```yaml
# secrets.yaml
auth:
  cloudru:
    type: cloudru-evolution-key
    config:
      keyId: 01234567-89ab-cdef-0123-456789abcdef
      keySecret: "<the key secret, verbatim>"
      projectId: 01234567-89ab-cdef-0123-456789abcdef
```

> **Write the values verbatim — a `${secret:…}` here is passed to the cloud
> literally.** An `auth:` entry lives in `secrets.yaml`, which *is* the encrypted
> store, and SC does not expand placeholders inside it. The failure is expensive
> because it is late: Cloud.ru IAM answers `HTTP 400 … (key_id) is invalid
> argument: key_id does not match required format`, which reads like a revoked or
> mistyped key, **after** the stack has already created resources. SC now refuses
> an unexpanded placeholder at validation and says this. `${secret:…}` belongs in
> the plaintext files — `server.yaml` and `client.yaml`.

`projectId` is required and is not inferable from the key: every Evolution
endpoint is project-scoped, and a call that omits it answers **403
`non_authorized`** — which reads exactly like a missing role and sends you off
granting permissions that were never the problem. It is the UUID in the console
URL as `projectId=`.

The same key pair is also the `docker login` credential for the project's Artifact
Registry, so there is deliberately no separate registry credential to configure.

#### **Advanced access key** (`cloudru-access-key`)

```yaml
# secrets.yaml
auth:
  cloudru-advanced:
    type: cloudru-access-key
    config:
      accessKey: "..."
      secretKey: "..."
      region: ru-moscow-1        # defaults to ru-moscow-1
      # projectName: ru-moscow-1 # Keystone project; defaults to the region
      # domainName / domainId    # normally derived from the key pair
      # securityToken: "..."     # only with a TEMPORARY key pair
      # authUrl: https://iam.ru-moscow-1.hc.sbercloud.ru/v3
```

The scoping vocabulary is Keystone's, not AWS's: a *project* is the tenancy
resources are created in (Cloud.ru names it after the region), and a *domain* is
the account above it. Both are normally inferable from the key pair.

There is deliberately **no `projectId`** here: the upstream provider's scoping
inputs are `project_name`, `domain_name` and `domain_id`, so a `projectId` field
would accept an id and silently drop it.

Region is not one input among many on this estate — it also picks the Keystone
endpoint and the project resources are created in. Setting a resource's `region`
re-derives all three together; an explicit `projectName` or `authUrl` still wins.

### **Registrar** (`RegistrarType` → `registrars` section in `server.yaml`)

#### **Cloud.ru Cloud DNS** (`cloudru-dns`)

Writes records into an existing Cloud.ru **Evolution** Cloud DNS zone.

**Golang Struct Reference:** `pkg/clouds/cloudru/registrar_dns.go:DnsRegistrarConfig`

**JSON Schema:** [DnsRegistrarConfig Schema](https://github.com/simple-container-com/api/tree/main/docs/schemas/cloudru/dnsregistrarconfig.json)

```yaml
# server.yaml
resources:
  registrars:
    cloudru:
      type: cloudru-dns
      config:
        keyId: 01234567-89ab-cdef-0123-456789abcdef
        keySecret: "<verbatim>"
        projectId: 01234567-89ab-cdef-0123-456789abcdef   # also the zone-lookup scope
        zoneName: example.ru          # as it appears in a domain
        zoneId: 01234567-89ab-...     # optional: from .../dns/zones/public/<zoneId>/edit
        # recordTtl: 300              # honoured; defaults to 300 when unset — see below
        dnsRecords:
          - name: api.example.ru      # relative on the wire; the apex is an empty name
            type: A
            value: 203.0.113.10
```

Set `zoneId` when a project holds several zones. Without it the zone is found by
name inside `projectId`, and a lookup scoped to the wrong project **does not fail** —
it finds nothing, which is indistinguishable from a project with no zones. There is no
DNS-specific scope: an earlier version of this page described a `parentId` for a "Cloud
DNS service instance", which the real API does not have.

The zone is **looked up and never created**, for the same reason as the Yandex
registrar: a public zone is authoritative for a real domain, so creating one from
a deploy either collides with the delegation that already exists or produces a
second zone nobody's nameservers point at. `zoneName` is matched against the zone's
`domain` — **not** its `name`, which the console slugs (`example.ru.` is stored with
`name: example-ru`).

Delegate the domain to `evo-cns01.cloud.ru` and `evo-cns02.cloud.ru` before expecting a
record to resolve. There is no verification step and nothing to check delegation with:
an Evolution zone has no `valid`, `validationText` or `delegated` field, only a `state`
and an always-empty `confirmState`, and it is authoritative as soon as it exists. Verify
with `dig NS` yourself.

Four measured behaviours shape how this registrar writes (details and the API-level
reasoning in [DNS Management](../guides/dns-management.md#4-cloudru-cloud-dns-registrar-cloudru-dns)):

| Behaviour | Consequence |
|---|---|
| A record is **not addressable**; a `PATCH` replaces the zone's whole record array | SC reads the zone, merges, writes it all back, and provisions records **serially** — parallel writes would lose one |
| Writes are **asynchronous**, and the zone reads back `records: []` mid-flight | SC polls the operation; reading too early looks exactly like the write having wiped the zone |
| **`done: true` is not convergence** — a write issued while the previous one settles silently discards records, up to emptying the zone, and still reports success | SC re-reads the zone after `done`, re-issues the write once, and **errors loudly** rather than reporting a green deploy over a damaged zone |
| `ttl` is **honoured**, but an omitted one is stored as a literal `0` rather than defaulted | SC always sends a TTL — `recordTtl`, or 300 when unset |

Record names are **relative to the zone** on the wire and a fully-qualified one is
refused; SC converts, so `api.example.ru` and `api` both arrive as `api`, and the apex
(`example.ru`, `@`, or empty) arrives as the empty string. A wildcard is a flag on the
record, not a `*` in the name. Types are lowercase on the wire; SC lowercases them.

### **Container Registry** (Artifact Registry, auto-created)

Container Apps resolve images from the calling project's **own** Artifact Registry
and from nowhere else, so there is always one — SC's or yours. A registry is
therefore not optional on this cloud in the way it is elsewhere; see
`registryName` on the template above. The `docker login` credential is the same
service-account key pair, not a separate one.

### **Status and what is not covered**

| | State |
|---|---|
| Container Apps (`cloudru-container-app`) | smoke-tested live |
| `vm` + `vm-compose`, both drivers | smoke-tested live, including a VM SC created itself |
| `vm` + `k3s-cluster` + `k3s-cloudrun` | smoke-tested live |
| `cloudru-dns` | smoke-tested live against a delegated apex (2026-10-24) — create, idempotent re-apply, TTL change, a second record, selective destroy, each confirmed by public resolution. The service account needs the `cdns.admin` role in the project |
| Custom domain on a Container App | needs `cloudExtras.certificateId`; untested |
| Advanced: SWR registry, EVS CSI for k3s | bridged in the provider, not wired into SC |

---

## **VM & Self-Hosted Kubernetes**

These types describe a **machine**, not a managed cloud service, so they are not
under any one provider heading. Two resources and two templates:

| Type | Kind | What it is |
|---|---|---|
| `vm` | resource | one or more Linux machines SC can reach over SSH, plus the optional Caddy front door that terminates TLS on them |
| `k3s-cluster` | resource | a k3s control plane installed on the hosts of a `vm` resource; exports a kubeconfig |
| `vm-compose` | template | runs a `cloud-compose` or `single-image` client stack as a Docker Compose project on the machine |
| `k3s-cloudrun` | template | deploys a client stack onto a `k3s-cluster` through the ordinary Kubernetes path |

Which cloud creates the machine is the **resource's** `provider:` field, not a
separate resource type per cloud. Three drivers are implemented today:

| `provider:` | Status | Notes |
|---|---|---|
| `byo` | **implemented** | the machine already exists; you give SC its address and a deploy key. The one driver that works in every cloud on earth |
| `cloudru-vm` | **implemented, never run** | Cloud.ru **Advanced** (ex-SberCloud), through the bridged `terraform-provider-sbercloud`. Runs cloud-init, so SC generates and pins the machine's SSH host key itself. SC has only ever had an **Evolution** account, so this driver is written off a provider schema and has provisioned nothing: it is **refused at validation** unless the resource also sets `allowUnverifiedProvider: true` |
| `cloudru-evolution-vm` | **implemented** | Cloud.ru **Evolution**. Regionless (place with `zones:`, never `region:`) and **no cloud-init at all** — see the note below |
| `yc-vm`, `openstack-vm`, `vkcs-vm`, `mws-vm`, `timeweb-vm` | not implemented | refused at validation with the roadmap item named; until one lands, create the machine in the cloud console and declare it as `byo` |

The driver table — which cloud runs cloud-init, which has regions, which can
manage its own DNS — is `pkg/clouds/vm/capabilities.go:Drivers`, and it is one
table rather than `if provider == …` branches in shared code. A capability left at
its zero value there means **"not measured yet"**, not "absent".

**What a cloud driver requires, and what it refuses.** The mirror image of `byo`:

- `publicIP: true` is **required**. Every delivery step — readiness, image ship,
  compose up, the Caddy reload — is an outbound SSH connection from wherever
  `sc deploy` runs, and SC has no bastion, no VPN and no in-VPC runner. Refused at
  validation rather than discovered as a dial timeout against a machine that was
  created and billed successfully.
- `hosts` is rejected — SC creates the machine.
- `region` is **required** on a region-scoped cloud and **refused** on a
  regionless one (`cloudru-evolution-vm`): one global endpoint, and placement is
  the availability zone. A `region:` SC silently dropped would read as placement
  you controlled.
- `zones` takes at most one entry, because one machine. Multi-zone pools arrive
  with multi-host `vm` resources.
- `network.id` and `network.subnets` must be given together. A driver that picked
  "some subnet in the VPC" would place the machine in a zone, or behind a route
  table, you did not choose.
- `size` is in SC's own units (`cores`, `memory`, `disk`, `diskType`), not a
  per-cloud flavour name; each driver maps it on. It defaults to **2 cores / 4Gi**
  — the smallest shape that runs a compose stack plus the Caddy ingress without
  the build cache pushing it into swap. The default has to exist, because a cloud
  driver picks a flavour by *filtering* the cloud's flavour list: "no size given"
  is not "the cloud's default machine", it is a filter matching hundreds of
  flavours of which the API's first wins, and that can differ between two
  `sc deploy` runs.
- `image` is an SC-level alias (default `ubuntu-2404-lts`) that each driver maps to
  its own image id.
- `extraSshKeys` is refused on a cloud that runs no cloud-init: extra keys are
  authorised by writing them into user-data, and a cloud that accepts none has
  nowhere to put them — its create call takes exactly one public key, the deploy
  key SC generates. Add such a key by hand, or keep console access for incidents.
- `ingressPorts` requires a driver that can write firewall rules. On one that
  cannot, SC logs the list it would have opened and tells you to open it yourself,
  because a port it cannot reach is not a port it can promise.
- `allowUnverifiedProvider` is **required** by a driver SC has never actually
  provisioned with, and **refused** by every other. The flag is keyed off the
  driver's own `Unverified` note rather than its name, so it goes away on its own
  the moment a driver is live-verified — and setting it on a verified driver is an
  error rather than a no-op, because a stale exception reads as one that still
  applies. Today exactly one driver needs it: `cloudru-vm`.

**`cloudru-evolution-vm` has no cloud-init, and three things follow from it.**
Measured, not assumed: Cloud.ru Evolution's `POST /vms` answers `extra_forbidden`
to `user_data` and to every spelling of it, because the image renders a fixed
template from the slots it declares and nothing else reaches the first boot. So
(1) docker is installed afterwards over SSH rather than on first boot, (2) the
readiness gate is the `byo` one — preconditions, not the `cloud-init.done` marker
— and (3) **SC cannot pre-seed the machine's SSH host key**, so the first
connection has nothing to verify and trust-on-first-use is all that is left. That
is a real exposure: the deploy key and every secret of every stack on the machine
ride that connection. Read the key off the Cloud.ru console once and paste it into
the resource's `hostKey:` (same format as `hosts[].hostKey`), and the next
provision pins it. The field is accepted by exactly the drivers that cannot be
given a key and refused by the ones that can, so it is never dead config.

The same call also refuses `subnet_id`, `security_group_ids`, `interfaces`,
`networks`, `nics` and `floating_ip`, so the machine lands on the availability
zone's **default** subnet and **default** security group. Ingress rules are opened
on whichever group it actually joined, read back off its own interface.

**Which ports a cloud driver opens.** Four sources, and only the last one is yours
to write:

1. **22/tcp**, always.
2. **80/tcp, 443/tcp, 443/udp** when `ingress.enable` is set.
3. **Whatever the machine's consumers in the same parent stack declare.** A
   `k3s-cluster` resource, or a `vm-compose` template, that names this `vm`
   resource *is* the declaration — so `6443/tcp` and `30000-32767/tcp` come with
   a cluster, and the compose host-port range (`20000-30000/tcp`) comes with a
   template, without being restated. Three of the cluster's ports are
   **node-to-node** and are permitted from the machine's own subnet rather than
   the internet: `8472/udp` (flannel VXLAN), `10250/tcp` (kubelet),
   `2379-2380/tcp` (etcd client and peer).
4. **`ingressPorts` on the resource**, for what inference cannot see — a port one
   of your own processes binds on the machine.

The resolved list is exported as `<vm>-vm-open-ports`, and a `vm-compose` deploy
reads it back to warn when the host port it publishes is not reachable from the
internet. The ranges are deduplicated by the hole they open rather than by their
label, and where two sources ask for the same port at different scopes the wider
one wins.

All drivers resolve the list with the same code (`pkg/clouds/vm/ports.go`), so two
clouds cannot drift apart; what differs is enforcement. Advanced scopes a
node-to-node rule with `RemoteGroupId` — the machine's own security group.
Evolution has no group of its own (see above), so SC resolves the VM's private
subnet CIDR after the create, and when that lookup fails it **says so and leaves
the port shut** rather than widening it to `0.0.0.0/0`. A destroy closes only the
rules SC opened: they carry a description naming the VM, and the sweep matches on
that prefix, so rules added by hand in a shared default group survive.

Worked end-to-end examples: [Compose on a VM](../examples/vm-compose/index.md)
and [k3s Clusters](../examples/k3s/index.md).

### **Resources** (`ResourceType` → `resources` section in `server.yaml`)

#### **Virtual Machine** (`vm`)

**Golang Struct Reference:** `pkg/clouds/vm/vm.go:VmConfig`

**JSON Schema:** [VmConfig Schema](https://github.com/simple-container-com/api/tree/main/docs/schemas/vm/vmconfig.json)

```yaml
# server.yaml - Parent Stack
resources:
  resources:
    staging:
      resources:
        portal-vm:
          type: vm
          config:
            provider: byo              # the machine already exists
            hosts:
              - address: 203.0.113.10
                user: ubuntu           # defaults to `sc`
                port: 22               # defaults to 22
                privateKey: "${secret:PORTAL_VM_DEPLOY_KEY}"
                # `ssh-keyscan -t ed25519 203.0.113.10` output pastes in verbatim
                hostKey: "203.0.113.10 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA..."
            # Optional: the Caddy container that owns ports 80/443 on the machine.
            ingress:
              enable: true
              email: ops@example.com   # required when `enable` is set
              # image: registry.example.ru/mirror/caddy:2.11
              # imageDelivery: ssh-tar # default `registry` (the machine pulls it)
              # acmeCa: https://acme.example.ru/directory
              # acmeEabKeyId: "..."
              # acmeEabMacKey: "${secret:ACME_EAB_MAC_KEY}"
            # For machines with no route to Docker Hub or the distro mirrors:
            # registryMirrors: ["https://mirror.example.ru"]
            # packageMirror: "https://mirror.example.ru/ubuntu"
```

Per-host keys (`hosts[]`): `address` (**required**), `user`, `port`,
`privateKey` (**required** — write it as `${secret:...}`; password and
ssh-agent authentication are not supported), `privateKeyPassword`, `hostKey`.

`hostKey` is optional and is the one optional field whose absence has a real
cost. A deploy onto a VM carries the stack's entire env file, every
`${secret:...}` in it, and the deploy key itself; with no pinned host key the
SSH client has nothing to check the machine against. A CI runner's
`known_hosts` is empty, so there is no safe implicit fallback — the
provisioning tier warns loudly when this is unset. It is the machine's
**public** key, so it is deliberately not treated as a secret: an operator
should be able to compare it against the cloud console by eye.

**What `provider: byo` refuses.** `region`, `zones`, `image`, `size`,
`publicIP`, `network`, `extraSshKeys` and `ingressPorts` are rejected at
validation rather than accepted and ignored — SC does not create the machine, so
it cannot apply them. Configure those in the cloud console; on a `byo` host the
ports are yours to open, and SC prints the list it would have asked for so you
do not have to derive it. Note that this is a refusal, not a warning: an operator
certain the NodePort range is open is exactly the incident `ingressPorts` exists
to prevent. `byo` also currently accepts **exactly
one** host; multi-host pools arrive with the k3s work (`sc-k3s-cluster-resource`).
The reverse holds for the cloud drivers — `hosts` is only valid with `byo` — and
each cloud driver's own requirements are in the
[driver table above](#vm-self-hosted-kubernetes).

**A machine SC creates itself** looks like this instead. Same resource type, same
everything above it; `hosts` is gone and the machine's shape takes its place:

```yaml
# server.yaml - Parent Stack
resources:
  resources:
    staging:
      resources:
        portal-vm:
          type: vm
          config:
            provider: cloudru-evolution-vm
            credentials: "${auth:cloudru}"
            # No `region:` — this cloud has none; placement is the zone.
            zones: [ru.AZ-1]
            publicIP: true             # required: SC deploys over SSH from outside
            image: ubuntu-2404-lts     # SC alias, mapped per cloud
            size:
              cores: 2
              memory: 4Gi
              disk: 40Gi
            sshUser: sc                # the default; not root, not the image's own
            # Paste `ssh-keyscan -t ed25519 <address>` output after the first
            # provision — this cloud runs no cloud-init, so SC cannot seed it.
            # hostKey: "45.132.179.77 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA..."
            # Optional: holes inference cannot derive. `preset` names a bundle
            # (`vm-compose`, `k3s`, `http`) and excludes `port`.
            ingressPorts:
              - port: 9100             # node_exporter
              - port: 7946
                protocol: udp          # defaults to tcp
              - port: 32000
                portEnd: 32100         # a range, inclusive
            ingress:
              enable: true
              email: ops@example.ru
```

With `provider: cloudru-vm` (the Advanced estate) the differences are the
credential type, that `region: ru-moscow-1` is required rather than refused, and
that the resource must also carry `allowUnverifiedProvider: true` — that driver
has provisioned nothing yet.

**Stack outputs** (read with `sc stack outputs`), for resource name `<vm>` and
host index `<i>`:

| Output | Contents |
|---|---|
| `<vm>-vm-host-count` | number of hosts |
| `<vm>-vm-host-<i>-address` / `-user` / `-port` | how to reach host `<i>` |
| `<vm>-vm-host-<i>-private-key` / `-private-key-password` | **secret-marked** |
| `<vm>-vm-host-<i>-host-key` | the pinned public host key |
| `<vm>-vm-ingress-enabled` | whether the Caddy front door is on |
| `<vm>-vm-open-ports` | the resolved firewall list, e.g. `ssh=tcp/22,k3s-api=tcp/6443,k3s-nodeport=tcp/30000-32767,k3s-kubelet=tcp/10250@nodes`; `@nodes` marks a range permitted only from the machine's own subnet. Empty on a driver that manages no firewall |

> `sc stack outputs` prints secret-marked values in the clear. Redirect it to a
> mode-600 file (`umask 077`) rather than reading it on a shared terminal.

#### **k3s Cluster** (`k3s-cluster`)

A single-node (today) k3s control plane installed over SSH on the hosts of a
`vm` resource. It exports a kubeconfig and an ingress IP **under the same output
names GKE Autopilot uses**, which is why the whole Kubernetes layer above it —
Deployment, Service, `cloudExtras`, probes, helm operators, the Caddy edge —
works unchanged.

**Golang Struct Reference:** `pkg/clouds/k3s/cluster.go:K3sClusterConfig`

**JSON Schema:** [K3sClusterConfig Schema](https://github.com/simple-container-com/api/tree/main/docs/schemas/k3s/k3sclusterconfig.json)

```yaml
# server.yaml - Parent Stack
resources:
  resources:
    staging:
      resources:
        cluster-vm:
          type: vm
          config:
            provider: byo
            hosts:
              - address: 203.0.113.10
                user: ubuntu
                privateKey: "${secret:CLUSTER_VM_DEPLOY_KEY}"
                hostKey: "203.0.113.10 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA..."
            # No `ingress` here on purpose — see the ports note below.

        ru-cluster:
          type: k3s-cluster
          config:
            vmResource: cluster-vm     # named, not described
            # version: v1.36.5+k3s1    # pinned by default; a CHANNEL name is refused
            # endpoint: k8s.example.ru # what the exported kubeconfig points at
            # tlsSans: [k8s-internal.example.ru, 10.0.0.10]
            # disable: [traefik]       # REPLACES the default, does not add to it
            # extraServerArgs: ["--kube-apiserver-arg=v=2"]
            # installScriptUrl: https://mirror.example.ru/get-k3s.sh
```

| Key | Default | Notes |
|---|---|---|
| `vmResource` | — (**required**) | name of a `vm` resource in the same stack and env; provisioned after it and reads its host facts |
| `version` | `v1.36.5+k3s1` | pinned deliberately: k3s's installer resolves a release *channel* at install time, so the default would make the control-plane version depend on the day the deploy ran. A channel name is refused |
| `endpoint` | the node's address | also becomes the first `--tls-san`. The kubeconfig has to work from wherever `sc deploy` runs — for CI that is the runner, not the machine |
| `tlsSans` | endpoint + node address | spend these freely: adding a SAN later re-issues the certificate and restarts the API server; an unused SAN costs nothing |
| `disable` | derived from the host | valid entries: `traefik`, `servicelb`, `local-storage`, `metrics-server`. An explicit empty list means "disable nothing" (that is how you get Traefik back) |
| `extraServerArgs` | — | passed to `k3s server` verbatim; flags SC manages itself (`--tls-san`, `--disable`, `--write-kubeconfig*`) are refused rather than silently fought |
| `installScriptUrl` | `https://get.k3s.io` | for a network that cannot reach Rancher's |

**One machine has one port 443.** The `vm` resource's `ingress` and k3s's own
load-balancer implementation (klipper-lb, k3s's `servicelb`) both want 80/443,
so one of them owns them:

- **no `ingress` on the vm resource** → k3s owns 80/443 and `servicelb` stays enabled;
- **`ingress.enable: true`** → the compose Caddy owns them, and SC **disables
  `servicelb` automatically** — otherwise a `LoadBalancer` Service that can never
  get a port sits `<pending>` forever with nothing saying why.

You do not have to coordinate this by hand; the `k3s-cluster` resource reads the
machine's ingress setting and adjusts. k3s's bundled Traefik is a third
contender for the same ports, which is why SC disables it by default.

**Stack outputs**, for cluster resource name `<cluster>` and env `<env>` — the
names are byte-for-byte those of a GKE Autopilot cluster:

| Output | Contents |
|---|---|
| `<cluster>--<env>-kubeconfig` | **secret-marked** kubeconfig with `server:` rewritten to the endpoint |
| `<cluster>--<env>-ingress-ip` | the node's address |

### **Templates (Compute)**

#### **Compose on a VM** (`vm-compose`)

Runs a client stack as a Docker Compose project on the machine, with blue/green
slots and a health gate. Accepts **both** `type: cloud-compose` and
`type: single-image` client stacks.

**Golang Struct Reference:** `pkg/clouds/vm/compose.go:ComposeTemplateConfig`

**JSON Schema:** [ComposeTemplateConfig Schema](https://github.com/simple-container-com/api/tree/main/docs/schemas/vm/composetemplateconfig.json)

```yaml
# server.yaml - Parent Stack
templates:
  compose-on-vm:
    type: vm-compose
    config:
      vmResource: portal-vm            # name of the `vm` resource to deploy onto
      imageDelivery: ssh-tar           # default; `registry` to have the machine pull
      externalImages: ship             # default under `ssh-tar`; `pull` under `registry`
      strategy: blue-green             # default; `recreate` trades 502s for RAM
      keepImages: 3                    # previous versions kept for rollback
      # Only needed with `imageDelivery: registry`:
      # dockerRegistryURL: registry.example.ru/mycompany
      # dockerRegistryUsername: "${secret:RU_REGISTRY_USERNAME}"
      # dockerRegistryPassword: "${secret:RU_REGISTRY_PASSWORD}"

resources:
  resources:
    staging:
      template: compose-on-vm
      resources:
        portal-vm:
          type: vm
          config: { provider: byo, hosts: [ ... ] }
```

| Key | Default | Notes |
|---|---|---|
| `vmResource` | — (**required**) | name of a `vm` resource in the same parent stack |
| `imageDelivery` | `ssh-tar` | `ssh-tar` streams the built image over the deploy's own SSH connection, so **no registry has to be reachable from the machine**. `registry` has the machine pull it |
| `externalImages` | `ship` under `ssh-tar`, `pull` under `registry` | what happens to images SC did not build (a `mongo`, a `redis`). `ship` streams them alongside your own image in one stream; `pull` leaves them to the machine. **Pin third-party tags** when shipping — the re-ship is triggered by the reference, so a tag that moves upstream is not noticed. `ship` requires `ssh-tar` |
| `strategy` | `blue-green` | `blue-green` brings the new slot up beside the old and gates on its health; `recreate` stops first — a few seconds of 502 in exchange for not running both slots at once, the honest choice on a 2 GB machine |
| `keepImages` | `3` | `0` is a valid explicit choice and means "keep none" |

**How Client Stacks Use This Template:**

```yaml
# client.yaml — a `cloud-compose` stack
stacks:
  staging:
    type: cloud-compose
    template: compose-on-vm
    parent: mycompany/mycompany
    config:
      # REQUIRED for every `cloud-compose` stack, on every cloud — there is no
      # default and no fallback. Omitting it fails the deploy with
      # `failed to read docker-compose config from "<stackDir>"/""`.
      dockerComposeFile: ./docker-compose.yaml
      domain: portal.example.com
      runs:
        - portal
      env:
        ENV: staging
      secrets:
        MONGO_URI: "${secret:staging-portal-mongo-uri}"
```

```yaml
# client.yaml — a `single-image` stack (no compose file; `image` instead)
stacks:
  staging:
    type: single-image
    template: compose-on-vm
    parent: mycompany/mycompany
    config:
      image:
        dockerfile: ${git:root}/Dockerfile
      maxMemory: 512
      domain: portal.example.com
```

SC passes `PORT=8080` to a `single-image` container, the same as Lambda and a YC
Serverless Container do, so an image that runs on either needs no change.

**What this template refuses, and what it only warns about.** A field SC cannot
honour on a VM is an error, not a silent no-op:

| Field | Behaviour | Why |
|---|---|---|
| `domainProxied` | **error** | the machine answers the ACME challenge itself; a Cloudflare proxy in front of it breaks that |
| `clusterIPAddress` | **error** | there is no cluster |
| `scale.max > 1` | **error** | one machine runs one replica — use `{min: 1, max: 1}`, or move to `k3s-cluster` |
| `scale.min: 0` | warning | has no effect on a VM (nothing scales to zero) |
| `scale.policy` | warning | ignored on a VM |

**DNS and TLS.** With `ingress.enable: true` on the vm resource and a `domain`
on the client stack, the stack publishes **no host port** — 443 is the only way
in — and the URL output becomes `https://<domain>`. You still create the DNS
record yourself: an A record at the machine's address, **not** proxied. Set one
without the other and SC warns and deploys on a deterministic high host port;
the stack answers either way, and the warning is what keeps "my certificate
never appeared" from being silent.

**Stack outputs**, for client stack `<stack>`:

| Output | Contents |
|---|---|
| `<stack>-vm-compose-url` | `https://<domain>`, or `http://<address>:<port>` when the stack is on a host port |
| `<stack>-vm-compose-slot` | which blue/green slot is live |

#### **k3s Cloud Run** (`k3s-cloudrun`)

The same deployment path as `kubernetes-cloudrun`, except that it names a
`k3s-cluster` resource instead of carrying a kubeconfig — a cluster SC
provisions has no kubeconfig until it has been provisioned, so the value can
only arrive as a parent-stack output. Same shape as `gcp-gke-autopilot`'s
`gkeClusterResource`.

**Golang Struct Reference:** `pkg/clouds/k3s/cloudrun.go:K3sCloudrunTemplate`

**JSON Schema:** [K3sCloudrunTemplate Schema](https://github.com/simple-container-com/api/tree/main/docs/schemas/k3s/k3scloudruntemplate.json)

```yaml
# server.yaml - Parent Stack
templates:
  k3s-run:
    type: k3s-cloudrun
    config:
      k3sClusterResource: ru-cluster
      # Required, and there is no alternative on this path: a kubelet can only
      # PULL an image. Point it at a registry the MACHINE can reach.
      dockerRegistryURL: registry.example.ru/mycompany
      dockerRegistryUsername: "${secret:RU_REGISTRY_USERNAME}"
      dockerRegistryPassword: "${secret:RU_REGISTRY_PASSWORD}"
      useSSL: true                     # default; `false` while still on a host port
      # caddyResource: my-caddy        # once an in-cluster Caddy edge exists
```

| Key | Default | Notes |
|---|---|---|
| `k3sClusterResource` | — (**required**) | name of the `k3s-cluster` resource to deploy through |
| `dockerRegistryURL` + username/password | — (**required**) | unlike `vm-compose`, there is no `ssh-tar` escape: Kubernetes can only pull. A k3s stand with no reachable registry cannot deploy, which is surfaced at config time rather than at image-pull time |
| `useSSL` | `true` | `false` is the honest setting while you are still reaching the cluster on a host port with no certificate |
| `caddyResource` | — | name of a Caddy resource in the parent stack |

Client stacks use this exactly as they use `kubernetes-cloudrun` — including
`dockerComposeFile` for a `cloud-compose` stack:

```yaml
# client.yaml
stacks:
  staging:
    type: cloud-compose
    template: k3s-run
    parent: mycompany/mycompany
    config:
      dockerComposeFile: ./docker-compose.yaml
      domain: portal.example.ru
      runs: [portal]
```

> **The in-cluster Caddy edge is not wireable to a `k3s-cluster` yet.** The
> `kubernetes-caddy` resource takes its kubeconfig as a literal string in its own
> config, and a cluster provisioned in the same stack has no kubeconfig until it
> has been provisioned — the same limitation `k3s-cloudrun` exists to work around
> for the deploy path, still open for the edge. Until it is wired, the front door
> on a k3s stand is the node itself: leave `ingress` off the vm resource, let k3s
> keep 80/443, and expose a workload with `cloudExtras.serviceType: LoadBalancer`
> (klipper-lb binds the node's ports). A compose-side Caddy on the same machine is
> the other option, and is what the [vm-compose example](../examples/vm-compose/byo-host/README.md)
> does.

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
            useResourceName: false                  # Name every object <resource>-<env> instead of caddy-<env>,
                                                    # so a second Caddy fits in an env that has one (optional, default: false).
                                                    # See the warning below before setting it on a live Caddy.

            # Scheduling (optional)
            podAnnotations:                         # Pod template only; simple-container.com/ and pulumi.com/ keys rejected
              cluster-autoscaler.kubernetes.io/safe-to-evict: "false"   # keep autoscaler scale-down from evicting Caddy
            topologySpreadConstraints:              # Spread replicas; labelSelector defaults to the Caddy pod labels
              - topologyKey: kubernetes.io/hostname
                maxSkew: 1
                whenUnsatisfiable: ScheduleAnyway
            nodeSelector:                           # e.g. a non-production ingress on GKE Spot (Autopilot adds the toleration; tolerations are not configurable here)
              cloud.google.com/gke-spot: "true"
            
            # VPA Configuration for automatic resource optimization (optional)
            vpa:
              enabled: true                         # Enable VPA for Caddy (optional)
              updateMode: "Auto"              # VPA update mode: Off, Initial, Auto, InPlaceOrRecreate (optional)
              minAllowed:                           # Minimum allowed resources (optional)
                cpu: "50m"
                memory: "64Mi"
              maxAllowed:                           # Maximum allowed resources (optional)
                cpu: "1"
                memory: "1Gi"
              controlledResources: ["cpu", "memory"] # Resources VPA should control (optional)
            
            # Manual resource limits (optional, can be used alongside VPA)
            resources:
              limits:
                cpu: "500m"
                memory: "512Mi"
              requests:
                cpu: "100m"
                memory: "128Mi"
```

> **Setting `useResourceName` on a Caddy that is already live blackholes its domains
> until every child stack is redeployed.** The rename makes Pulumi *replace* the Caddy,
> which swaps its `LoadBalancer` Service, so the parent stack exports a new ingress IP.
> The A records pointing at the old IP were written by the **child** stacks and live in
> *their* Pulumi state — the parent deploy cannot rewrite them. So the parent goes green
> while every domain that Caddy serves still resolves to a dead address, until each child
> stack deploys again. Set the flag when adding a **new** Caddy; to split an existing one,
> plan the child redeploys as part of the same change.
>
> A second Caddy on the **same cluster** also needs its own `namespace:`. `useResourceName`
> renames the Caddy's objects, not the namespace they live in — that defaults to `caddy`
> either way, and two Caddies sharing a cluster will collide on it at apply time.

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
            
            # Simple backup configuration (backwards compatible)
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
            
            # Advanced multi-tier backup configuration
            backup:
              advanced:
                # Hourly snapshots: Every 1 hour (retained 2 days)
                hourly:
                  every: 1                  # Every 1 hour
                  retainFor: 2              # Retain for 2 days (unit: days - default)
                  
                # Daily snapshots: Every day (retained 35 days) 
                daily:
                  every: 1                  # Every 1 day
                  retainFor: 35             # Retain for 35 days (unit: days - default)
                  
                # Weekly snapshots: Every week (retained 8 weeks)
                weekly:
                  every: 1                  # Every 1 week
                  retainFor: 8              # Retain for 8 weeks (unit: weeks - default)
                  
                # Monthly snapshots: 1st of month (retained 6 months)
                monthly:
                  every: 1                  # Every 1 month  
                  retainFor: 6              # Retain for 6 months (unit: months - default)
                  
                # Point-in-Time Recovery: Continuous oplog streaming
                pointInTimeRecovery:
                  enabled: true             # Enable PITR
                  oplogSizeGB: 2.0         # 2GB oplog size
                  oplogMinRetentionHours: 24 # Minimum 24 hours retention

```

**Backup Security:**

MongoDB Atlas encrypts **all backups automatically by default** using your cloud provider's encryption:

- **Automatic encryption**: No configuration required - all snapshots are encrypted at rest
- **Cloud provider managed**: Encryption keys managed by AWS/GCP/Azure
- **Universal**: Applies to all cluster tiers and backup types (hourly, daily, weekly, monthly)
- **Secure by design**: Ensures backup data is always protected on disk

For enterprise customers requiring additional control, MongoDB Atlas also supports Customer Key Management (CKM) using your own KMS keys, but this requires separate configuration outside of Simple Container.

**Client Access:**

When this resource is used in a client stack via the `uses` section, Simple Container automatically injects environment variables and template placeholders for MongoDB Atlas database connection.

**For complete details on environment variables and template placeholders, see:** [Template Placeholders Advanced - MongoDB Atlas Cluster](../concepts/template-placeholders-advanced.md#mongodb-atlas-cluster)

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

**Golang Struct Reference:** `pkg/clouds/fs/fs_state.go:FileSystemStateStorage`

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

**Golang Struct Reference:** `pkg/clouds/fs/fs_state.go:PassphraseSecretsProvider`

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

### **CloudWatch Monitoring Alerts**

Monitoring alerts for ECS services and Application Load Balancers with CloudWatch integration.

**ECS Service Monitoring:**
```yaml
alerts:
  maxCPU:
    alertName: "High CPU Usage"
    description: "ECS service CPU usage exceeded threshold"
    threshold: 80.0
    periodSec: 300
  maxMemory:
    alertName: "High Memory Usage"
    description: "ECS service memory usage exceeded threshold"
    threshold: 85.0
    periodSec: 300
  maxErrors:
    alertName: "High Error Rate"
    description: "Application error rate exceeded threshold"
    threshold: 10.0
    periodSec: 60
    errorLogMessageRegexp: "ERROR|FATAL|Exception"
```

**Application Load Balancer Monitoring:**
```yaml
alerts:
  serverErrors:
    alertName: "ALB Server Errors"
    description: "5XX error rate exceeded threshold"
    threshold: 10.0
    periodSec: 300
  unhealthyHosts:
    alertName: "ALB Unhealthy Hosts"
    description: "Unhealthy host count exceeded threshold"
    threshold: 1
    periodSec: 300
  responseTime:
    alertName: "ALB High Response Time"
    description: "Response time exceeded threshold"
    threshold: 5.0
    periodSec: 300
```

**Email Notifications:**
```yaml
alerts:
  email:
    addresses:
      - "team-alerts@company.com"
      - "oncall@company.com" 
      - "devops-team@company.com"
  
  # Your alert configurations (serverErrors, unhealthyHosts, etc.)
  serverErrors:
    alertName: "production-server-errors"
    description: "High 5XX error rate detected"
    threshold: 5.0
    periodSec: 300
```

> **Email Integration**: Each notification channel works independently. When email addresses are configured, email notifications are sent alongside any configured webhook channels. Email recipients must confirm their subscription to receive notifications.

**Complete Monitoring Configuration:**
```yaml
alerts:
  # Notification channels
  slack:
    webhookUrl: "${secret:SLACK_WEBHOOK_URL}"
  discord:
    webhookUrl: "${secret:DISCORD_WEBHOOK_URL}"
  email:
    addresses:
      - "alerts@company.com"
      - "devops@company.com"
  
  # ECS monitoring
  maxCPU:
    alertName: "prod-high-cpu"
    description: "Production CPU usage too high"
    threshold: 70.0
    periodSec: 300
  maxMemory:
    alertName: "prod-high-memory"
    description: "Production memory usage too high"
    threshold: 80.0
    periodSec: 300
  
  # Load balancer monitoring
  serverErrors:
    alertName: "prod-server-errors"
    description: "Production server error rate too high"
    threshold: 5.0
    periodSec: 300
  responseTime:
    alertName: "prod-slow-response"
    description: "Production response time too slow"
    threshold: 2.0
    periodSec: 300
```

**Alert Configuration Properties:**

- **`alertName`** - Unique identifier for the CloudWatch alarm
- **`description`** - Human-readable description of the alert
- **`threshold`** - Numeric threshold value that triggers the alarm
- **`periodSec`** - Evaluation period in seconds (60-86400)
- **`errorLogMessageRegexp`** - (maxErrors only) Regex pattern to match error log messages

**CloudWatch Metrics and Dimensions:**

- **`serverErrors`** - Uses `HTTPCode_Target_5XX_Count` metric with LoadBalancer dimension
- **`unhealthyHosts`** - Uses `UnHealthyHostCount` metric with LoadBalancer + TargetGroup dimensions  
- **`responseTime`** - Uses `TargetResponseTime` metric with LoadBalancer dimension
- All metrics use full AWS load balancer identifiers (including AWS-generated suffixes) for reliable targeting

**Notification Channel Properties:**

- **`email.addresses`** - Array of email addresses for email notifications
- **`slack.webhookUrl`** - Slack webhook URL (use secrets management)
- **`discord.webhookUrl`** - Discord webhook URL (use secrets management)
- **`telegram.chatID`** &**`telegram.token`** - Telegram bot configuration

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
    # `template` is a stack-level field, alongside `type` and `parent` — not a
    # member of `config`.
    template: web-app-template
    parent: myorg/infrastructure
    config:
      dockerComposeFile: ./docker-compose.yaml
      uses: [mongodb-shared-us, s3-shared-storage]  # Shared resources
      domain: customer1.myapp.com
      runs: [web-app]
      
  customer-standard-2:
    type: cloud-compose
    template: web-app-template
    parent: myorg/infrastructure
    config:
      dockerComposeFile: ./docker-compose.yaml
      uses: [mongodb-shared-us, s3-shared-storage]  # Same shared resources
      domain: customer2.myapp.com
      runs: [web-app]
  
  # EU customer - EU resources for compliance
  customer-eu:
    type: cloud-compose
    template: api-service-template  # Different deployment pattern
    parent: myorg/infrastructure
    config:
      dockerComposeFile: ./docker-compose.yaml
      uses: [mongodb-shared-eu, s3-shared-storage]  # EU database
      domain: customer-eu.myapp.com
      runs: [api-service]
  
  # Enterprise customer - dedicated resources
  enterprise-customer:
    type: cloud-compose
    template: web-app-template
    parent: myorg/infrastructure
    config:
      dockerComposeFile: ./docker-compose.yaml
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

For end-to-end deployment examples that use these resources, see the [Examples](../examples/README.md) section and the [Guides](../guides/index.md).