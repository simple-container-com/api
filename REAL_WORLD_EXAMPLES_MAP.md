# Real-World Examples Map for Simple Container API

This document provides a comprehensive map of all real-world examples available for reference when fixing fictional properties in documentation.

## Official Examples Directory

### Parent Stack Examples (`/home/iasadykov/projects/github/simple-container/examples/examples/parent/`)

#### 1. AWS + MongoDB Atlas + Cloudflare Setup
**File:** `aws-mongodb-atlas-cloudflare.yaml`
**Key Resources Demonstrated:**
- **AWS S3 Bucket** (lines 101-107): Real `corsConfig` with `allowedOrigins`, `allowedMethods`
- **MongoDB Atlas Complete Config** (lines 108-125): `admins`, `developers`, `instanceSize`, `orgId`, `region`, `cloudProvider`, `privateKey`, `publicKey`, `backup` (every, retention)
- **MongoDB Atlas with Network Config** (lines 149-160): `networkConfig` with `allowCidrs`, `privateLinkEndpoint`, `extraProviders` structure
- **MongoDB Atlas extraProviders** (lines 155-160): Proper structure with `AWS` provider, `type: aws-token`, `credentials` reference

#### 2. GCP GKE Autopilot + PostgreSQL + MongoDB Atlas
**File:** `gcp-gke-autopilot-postgres-mongodb-atlas.yaml`
**Key Resources Demonstrated:**
- GCP GKE Autopilot cluster configurations
- PostgreSQL database setups
- MongoDB Atlas integration with GCP

#### 3. Pure Kubernetes with Database Operators
**File:** `pure-kubernetes-with-databases.yaml`
**Key Resources Demonstrated:**
- Kubernetes-native database operators
- Helm chart configurations
- Pure Kubernetes deployments

### Service Stack Examples (`/home/iasadykov/projects/github/simple-container/examples/examples/service/`)

#### 1. AWS ECS Fargate Deployment
**File:** `aws-ecs-fargate-deployment.yaml`
**Key Configurations Demonstrated:**
- **Client Stack Structure** (lines 6-73): Complete `cloud-compose` deployment
- **Resource Usage** (lines 15-17): `uses` directive with resource references
- **Scaling Configuration** (lines 35-41): `scale` with `max`, `min`, `policy.cpu`
- **Security Groups** (lines 32-34): `cloudExtras.securityGroup.ingress.allowOnlyCloudflare`
- **Dependencies** (lines 41-44): Cross-service resource dependencies
- **Alerts Configuration** (lines 20-30): Slack webhooks, memory/CPU thresholds

#### 2. AWS Lambda Single Image
**File:** `aws-lambda-single-image.yaml`
**Key Configurations Demonstrated:**
- Lambda-specific deployment patterns
- Single image deployment configurations

#### 3. Pure Kubernetes Deployment
**File:** `pure-kubernetes.yaml`
**Key Configurations Demonstrated:**
- Kubernetes-native service deployments
- Container configurations without cloud-specific services

#### 4. Static Website
**File:** `static.yaml`
**Key Configurations Demonstrated:**
- Static website deployment patterns
- CDN and storage configurations

#### 5. Docker Compose Reference
**File:** `docker-compose.yaml`
**Key Configurations Demonstrated:**
- Standard Docker Compose structure
- Service definitions and networking

## Production Examples from Organizations

### Primary Reference Examples

#### **aiwayz-sc-config** (`/home/iasadykov/projects/github/fulldiveVR/aiwayz-sc-config/.sc/stacks/aiwayz-sc-config/`)
**Files:** `server.yaml` (382 lines), `secrets.yaml` (2826 bytes)
**Resources Catalog:**
- **Provisioner**: GCP bucket state storage, GCP KMS secrets provider
- **Templates**: 
  - `gcp-static-website` - Static website deployment
  - `gcp-gke-autopilot` - GKE Autopilot with resource references
- **Resources**:
  - **Cloudflare Registrar**: Domain `aiwayz.com` with DNS records
  - **MongoDB Atlas**: M0 instance, Western Europe, GCP provider
  - **GCP Redis**: 2GB memory, europe-west3, custom config
  - **GCP GKE Autopilot Cluster**: v1.27.16, Caddy enabled (2 replicas)
  - **GCP Artifact Registry**: Docker registry, immutable tags disabled
  - **GCP Pub/Sub**: Multiple topics/subscriptions with dead letter policies
**Key Patterns**: GKE template with resource references, comprehensive Pub/Sub configuration, Redis with custom policies

### Integrail Organization (30+ Examples)

#### **DevOps Infrastructure** (`/home/iasadykov/projects/github/integrail/devops/.sc/stacks/integrail/`)
**Files:** `server.yaml` (237 lines), `secrets.yaml` (95102 bytes)
**Resources Catalog:**
- **Provisioner**: AWS S3 bucket state storage, AWS KMS secrets provider
- **Templates**:
  - `ecs-fargate` - EU and US regions with different AWS accounts
  - `aws-static-website` - Static website deployment
  - `aws-lambda` - Serverless functions
- **Resources**:
  - **Cloudflare Registrar**: Domain `integrail.ai` with extensive DNS records (SendGrid, DomainKey)
**Key Patterns**: Multi-region AWS setup, extensive DNS configuration, large secrets file (95KB)

#### **Backend Services** (`/home/iasadykov/projects/github/integrail/baas/.sc/stacks/baas/`)
**Files:** `client.yaml` (113 lines)
**Resources Catalog:**
- **Client Stack Type**: `single-image` (Lambda deployment)
- **Template**: `lambda-eu` (from parent stack)
- **Parent**: `integrail/integrail`
- **Key Configurations**:
  - **Lambda Routing**: `function-url` type
  - **Lambda Invoke Mode**: `RESPONSE_STREAM`
  - **Static Egress IP**: Enabled for NAT
  - **Timeout**: 180 seconds, 2048MB memory
  - **Uses**: `mongodb-nest` resource from parent
  - **Environment**: Chrome browser, Ollama integration, OpenAI GPT-4
**Key Patterns**: Lambda response streaming, static IP for external calls, AI/ML service integration

#### **Vector Database** (`/home/iasadykov/projects/github/integrail/milvus/.sc/stacks/milvus/`)
**Files:** `client.yaml` (29 lines)
**Resources Catalog:**
- **Client Stack Type**: `cloud-compose` (ECS deployment)
- **Parent**: `integrail/integrail`
- **Key Configurations**:
  - **Load Balancer**: Network Load Balancer (NLB)
  - **Scaling**: Min 1, Max 3, CPU threshold 70%
  - **Size**: 1024 CPU, 2048MB memory
  - **Domain**: Not proxied through Cloudflare
  - **Runs**: `milvus` service from docker-compose
**Key Patterns**: NLB for high-performance vector database, auto-scaling configuration

#### **AWS Bedrock Gateway** (`/home/iasadykov/projects/github/integrail/bedrock-access-gateway/.sc/stacks/bedrock-access-gateway/`)
**Files:** `client.yaml` (30 lines)
**Resources Catalog:**
- **Client Stack Type**: `single-image` (Lambda deployment)
- **Template**: `lambda-eu` (from parent stack)
- **Parent**: `integrail/integrail`
- **Key Configurations**:
  - **Lambda Routing**: `function-url` type
  - **Lambda Invoke Mode**: `RESPONSE_STREAM`
  - **AWS Bedrock Roles**: Specific IAM roles for AI model access
    - `bedrock:InvokeModel`
    - `bedrock:InvokeModelWithResponseStream`
    - `bedrock:CreateModelInvocationJob`
  - **AI Model Configuration**: Claude 3 Sonnet, Cohere embeddings
  - **Cross-Region Inference**: Enabled for better availability
  - **Timeout**: 60 seconds
**Key Patterns**: AWS Bedrock integration, AI-specific IAM roles, cross-region inference

#### **Storage Service** (`/home/iasadykov/projects/github/integrail/storage-service/.sc/stacks/storage-service/`)
**Files:** `client.yaml` (95 lines)
**Resources Catalog:**
- **Client Stack Type**: `single-image` (Lambda deployment)
- **Template**: `lambda-eu` (from parent stack)
- **Parent**: `integrail/integrail`
- **Key Configurations**:
  - **Lambda Routing**: `function-url` type
  - **Lambda Invoke Mode**: `RESPONSE_STREAM`
  - **Scheduled Jobs**: Cron-based cleanup automation
    - `cron(0 * * * ? *)` - Every hour cleanup
    - Automated API calls with Bearer token authentication
  - **Multiple Resource Usage**: S3 storage + MongoDB
  - **Timeout**: 120 seconds, 512MB memory
  - **Uses**: `integrail-storage`, `mongodb-nest`
**Key Patterns**: Lambda scheduled jobs with cron expressions, automated cleanup, multiple resource dependencies

#### **Code Executor** (`/home/iasadykov/projects/github/integrail/code-executor/.sc/stacks/code-executor/`)
**Files:** `client.yaml` (57 lines)
**Resources Catalog:**
- **Client Stack Type**: Mixed - `single-image` (staging), `cloud-compose` (beta)
- **Template**: `lambda-eu` (staging), `stack-per-app-eu` (beta)
- **Parent**: `integrail/integrail`
- **Key Configurations**:
  - **Staging**: Lambda with 40s timeout, response streaming
  - **Beta**: ECS with high resources (2048 CPU, 4096MB memory, 40GB ephemeral)
  - **Scaling**: Min 2, Max 6, CPU threshold 30% (very low for code execution)
  - **Security**: Cloudflare-only ingress for beta environment
  - **Deno Runtime**: `DENO_DIR: /deno` configuration
  - **Environment Switch**: Different deployment types per environment
**Key Patterns**: Mixed deployment types per environment, very low CPU scaling threshold (30%), high ephemeral storage, Deno runtime

#### **Billing Systems** (`/home/iasadykov/projects/github/integrail/billing/.sc/stacks/billing/`)
**Files:** `client.yaml` (47 lines)
**Resources Catalog:**
- **Client Stack Type**: `single-image` (Lambda deployment)
- **Template**: `lambda-eu` (from parent stack)
- **Parent**: `integrail/integrail`
- **Key Configurations**:
  - **Lambda Routing**: `function-url` type
  - **Multi-Environment**: staging, test, beta, prod with YAML anchors
  - **Parent Environment**: `beta` uses `parentEnv: prod`
  - **Timeout**: 300 seconds, 512MB memory
  - **Uses**: `mongodb-nest` resource from parent
  - **Domain Pattern**: `{env}-billing.integrail.ai` structure
**Key Patterns**: Multi-environment with YAML anchors, parent environment inheritance, long timeout for billing operations

#### **Agent Marketplace** (`/home/iasadykov/projects/github/integrail/agent-marketplace/.sc/stacks/agents-marketplace/`)
**Files:** `client.yaml` (1233 bytes) - Access restricted by .gitignore
**Resources Catalog:**
- **Access Status**: Configuration file exists but is restricted by .gitignore
- **File Size**: 1233 bytes indicates substantial configuration
**Key Patterns**: Private/sensitive configuration management

#### **Scheduler** (`/home/iasadykov/projects/github/integrail/scheduler/.sc/stacks/scheduler/`)
**Files:** `client.yaml` (44 lines)
**Resources Catalog:**
- **Client Stack Type**: `single-image` (Lambda deployment)
- **Template**: `lambda-eu` (from parent stack)
- **Parent**: `integrail/integrail`
- **Key Configurations**:
  - **Lambda Routing**: `function-url` type
  - **High-Frequency Scheduling**: `cron(* * * * ? *)` - Every minute execution
  - **Automated Reporting**: JSON body with `{"report":true}`
  - **Timeout**: 300 seconds, 512MB memory
  - **Uses**: `mongodb-nest` resource from parent
  - **Bearer Token Authentication**: API key in Authorization header
**Key Patterns**: High-frequency Lambda scheduling (every minute), automated reporting system, Bearer token authentication

#### **Cost Management** (`/home/iasadykov/projects/github/integrail/cost-management/.sc/stacks/cost-management/`)
**Files:** `client.yaml` (138 lines)
**Resources Catalog:**
- **Client Stack Type**: `single-image` (Lambda deployment)
- **Template**: `lambda-eu` (from parent stack)
- **Parent**: `integrail/integrail`
- **Key Configurations**:
  - **Lambda Invoke Mode**: `RESPONSE_STREAM`
  - **Extensive AWS Cost Explorer Roles**: 20+ IAM permissions for cost analysis
    - Cost Explorer: `ce:GetCostAndUsage`, `ce:GetAnomalies`, `ce:GetAnomalyMonitors`
    - Budgets: `budgets:ViewBudget`
    - CloudWatch: `cloudwatch:GetMetricData`, `cloudwatch:ListMetrics`
    - CloudWatch Logs: Full log analysis permissions
  - **Daily Scheduling**: `cron(0 0 * * ? *)` - Every day at midnight UTC
  - **HubSpot Integration**: Daily sync with CRM system
  - **High Resources**: 600s timeout (10 minutes), 1024MB memory
  - **Uses**: `mongodb-nest` resource from parent
**Key Patterns**: Comprehensive AWS cost analysis IAM roles, daily CRM synchronization, high-resource Lambda for data processing

### AlphaMind Organization (10+ Examples)

#### **DevOps Infrastructure** (`/home/iasadykov/projects/github/alphamind-co/devops/.sc/stacks/alphamind-co/`)
**Files:** `server.yaml` (61 lines), `secrets.yaml` (4427 bytes)
**Resources Catalog:**
- **Provisioner**: AWS S3 bucket state storage, Passphrase secrets provider
- **Templates**:
  - `aws-static-website` - Static website deployment
  - `aws-lambda` - Serverless functions
  - `ecs-fargate` - Container deployment
- **Resources**:
  - **Cloudflare Registrar**: Domain `alphamind.co`
  - **MongoDB Atlas**: M0 staging, M30 production with backup (1h/24h)
**Key Patterns**: Passphrase secrets provider, staging/production MongoDB size differences, backup configuration

#### **Gagarin IDO Service** (`/home/iasadykov/projects/github/alphamind-co/gagarin-ido-service/.sc/stacks/gagarin-ido-service/`)
**Files:** `client.yaml` (60 lines)
**Resources Catalog:**
- **Client Stack Type**: `cloud-compose` (ECS deployment)
- **Parent**: `alphamind-co/alphamind-co`
- **Key Configurations**:
  - **Scaling**: Min 2, Max 3, CPU threshold 45% (lower than typical)
  - **Size**: 1024 CPU, 2048MB memory
  - **Uses**: `mongodb` resource from parent
  - **GraphQL Integration**: External GraphQL API endpoints
  - **Resource References**: `${resource:mongodb.uri}` pattern
  - **Environment**: Production Node.js with staging/production domains
**Key Patterns**: Lower CPU scaling threshold (45%), GraphQL API integration, resource URI references

#### **NestJS Backend** (`/home/iasadykov/projects/github/alphamind-co/nest-ido-backend/.sc/stacks/nest-ido-backend/`)
**Files:** `client.yaml` (72 lines)
**Resources Catalog:**
- **Client Stack Type**: `cloud-compose` (ECS deployment)
- **Parent**: `alphamind-co/alphamind-co`
- **Key Configurations**:
  - **Cross-Service Dependencies**: References `gagarin-ido-service` MongoDB resource
  - **Dependency Pattern**: `${dependency:gagarin-ido-service.mongodb.uri}`
  - **Version**: "10" (high version number for mature service)
  - **Size**: 1024 CPU, 2048MB memory
  - **Uses**: `mongodb` resource from parent
  - **Blockchain Integration**: Multiple smart contract addresses
  - **External APIs**: Brevo email, Claimr, Telegram bot
**Key Patterns**: Cross-service dependencies, blockchain contract integration, external API management

#### **Admin UI** (`/home/iasadykov/projects/github/alphamind-co/gagarin-admin-ui/.sc/stacks/gagarin-admin-ui/`)
**Files:** `client.yaml` (16 lines)
**Resources Catalog:**
- **Client Stack Type**: `static` (Static website deployment)
- **Parent**: `alphamind-co/alphamind-co`
- **Template**: `static-site` (from parent stack)
- **Key Configurations**:
  - **Bundle Directory**: `${git:root}/build` for React/Vue build output
  - **Multi-Environment**: staging and production domains
  - **SPA Configuration**: `index.html` for both index and error documents
  - **Domain Pattern**: `{env}-admin.alphamind.co` structure
**Key Patterns**: Admin UI deployment, SPA configuration, multi-environment static hosting

#### **Customer UI** (`/home/iasadykov/projects/github/alphamind-co/gagarin-customer-ui/.sc/stacks/gagarin-customer-ui/`)
**Files:** `client.yaml` (16 lines)
**Resources Catalog:**
- **Client Stack Type**: `static` (Static website deployment)
- **Parent**: `alphamind-co/alphamind-co`
- **Template**: `static-site` (from parent stack)
- **Key Configurations**:
  - **Bundle Directory**: `${git:root}/build` for React/Vue build output
  - **Multi-Environment**: staging and production domains
  - **SPA Configuration**: `index.html` for both index and error documents
  - **Domain Pattern**: `{env}-app.alphamind.co` structure (customer-facing)
**Key Patterns**: Customer UI deployment, SPA configuration, customer-facing domain naming

#### **Account Service** (`/home/iasadykov/projects/github/alphamind-co/gagarin-account-service/.sc/stacks/gagarin-account-service/`)
**Files:** `client.yaml` (34 lines)
**Resources Catalog:**
- **Client Stack Type**: `cloud-compose` (ECS deployment)
- **Parent**: `alphamind-co/alphamind-co`
- **Key Configurations**:
  - **Database**: PostgreSQL instead of MongoDB (different from other services)
  - **Resource References**: `${resource:postgres.user}`, `${resource:postgres.password}` patterns
  - **Blockchain Integration**: BNB Chain with Sepolia Linea testnet RPC
  - **Size**: 1024 CPU, 2048MB memory
  - **Uses**: `postgres` resource from parent
  - **Multi-Environment**: staging and production domains
  - **CORS**: Wildcard origin for development
**Key Patterns**: PostgreSQL resource references, blockchain testnet integration, account management service

#### **Public Media Store** (`/home/iasadykov/projects/github/alphamind-co/public-media-store/.sc/stacks/public-media-store/`)
**Files:** `client.yaml` (11 lines)
**Resources Catalog:**
- **Client Stack Type**: `static` (Static website deployment)
- **Parent**: `alphamind-co/alphamind-co`
- **Template**: `static-site` (from parent stack)
- **Key Configurations**:
  - **Bundle Directory**: `${git:root}/bundle` for media assets
  - **Domain**: `public-media.alphamind.co` (media-specific subdomain)
  - **Different Error Document**: `error.html` instead of `index.html`
  - **Production Only**: Single environment deployment
**Key Patterns**: Media-specific static hosting, custom error document, production-only deployment

### FullDive VR Examples

#### **Browser API** (`/home/iasadykov/projects/github/fulldiveVR/browser-api/.sc/stacks/base/`)
**Files:** `server.yaml` (7 lines)
**Resources Catalog:**
- **Stack Inheritance Pattern**: Uses `inherit: <base stack>` for provisioner
- **Registrar Inheritance**: Uses `inherit: <base-stack>` for registrar configuration
**Key Patterns**: Base stack inheritance pattern for shared configurations across services

#### **AI Statistics** (`/home/iasadykov/projects/github/fulldiveVR/ai-stats/.sc/stacks/ai-stats-v2/`)
**Files:** `client.yaml` (151 lines), `docker-compose.yaml` (5494 bytes)
**Resources Catalog:**
- **Client Stack Type**: `cloud-compose` (ECS deployment)
- **Parent**: `aiwayz`
- **Key Configurations**:
  - **Hardcoded Cluster IP**: `34.165.19.145` for external cluster reference
  - **Multi-Service Deployment**: `ai-stats-web`, `ai-stats-worker`
  - **Node.js Memory**: `--max-old-space-size=4096` for large datasets
  - **Langfuse Configuration**: AI/ML analytics platform with experimental features
  - **S3 Integration**: Multiple S3 prefixes (events/, media/, exports/)
  - **ClickHouse Integration**: Analytics database configuration
  - **Complex Environment**: 50+ environment variables for AI platform
**Key Patterns**: Hardcoded cluster IP, multi-service AI platform, extensive S3 integration, analytics database

#### **Streaming Services** (`/home/iasadykov/projects/github/fulldiveVR/streams/.sc/stacks/streams/`)
**Files:** `client.yaml` (50 lines), `docker-compose.yaml` (1731 bytes), `Dockerfile` (842 bytes), `entrypoint.sh` (448 bytes)
**Resources Catalog:**
- **Client Stack Type**: `cloud-compose` (ECS deployment)
- **Parent**: `aiwayz`
- **Key Configurations**:
  - **Hardcoded Cluster IPs**: Different IPs per environment (staging: 34.165.19.145, prod: 34.80.35.88)
  - **Hardcoded Database IP**: `10.120.0.3` for shared PostgreSQL instance
  - **N8N Integration**: Workflow automation with disabled modules
  - **Disruption Budget**: `minAvailable: 0` for zero-downtime deployments
  - **Rolling Update**: `maxSurge: 0` for controlled updates
  - **Multi-Domain**: Different domains per environment (aiwayz.com vs aiwize.com)
**Key Patterns**: Hardcoded infrastructure IPs, N8N workflow automation, zero-downtime deployment configuration

#### **AI Wize Code Gateway** (`/home/iasadykov/projects/github/fulldiveVR/ai-wize-code-gateway/.sc/stacks/code-gateway/`)
**Files:** `client.yaml` (37 lines), `docker-compose.yaml` (788 bytes), `Dockerfile` (858 bytes)
**Resources Catalog:**
- **Client Stack Type**: `cloud-compose` (ECS deployment)
- **Parent**: `aiwayz`
- **Key Configurations**:
  - **Hardcoded Cluster IP**: `34.165.19.145` for external cluster reference
  - **High-Resource Runtime**: 32GB memory, 16 CPU for code execution environments
  - **Dynamic Pod Management**: 10m timeout, 10Gi volume, ephemeral storage
  - **AI Integration**: Claude Sonnet 4 model, LLM proxy service
  - **GitHub OAuth**: Complete OAuth flow with redirect URI
  - **Docker Registry**: Private registry with authentication
  - **Kubernetes Integration**: Direct kubeconfig and namespace management
**Key Patterns**: High-resource code execution environment, AI-powered development tools, dynamic Kubernetes pod management

#### **Aiwize Combiner Service** (`/home/iasadykov/projects/github/fulldiveVR/aiwize-combiner-service/.sc/stacks/base/`)
**Files:** `server.yaml` (7 lines), `secrets.yaml` (311 bytes)
**Resources Catalog:**
- **Stack Inheritance Pattern**: Uses `inherit: <base stack>` for provisioner
- **Registrar Inheritance**: Uses `inherit: <base-stack>` for registrar configuration
- **Secrets File**: 311 bytes of configuration data
**Key Patterns**: Base stack inheritance pattern for shared configurations, similar to Browser API

### TalkToMe Tech Examples

#### **DevOps** (`/home/iasadykov/projects/github/talktome-tech/devops/.sc/stacks/talktome-tech/`)
**Files:** `server.yaml` (64 lines), `secrets.yaml` (3477 bytes)
**Resources Catalog:**
- **Provisioner**: AWS S3 bucket state storage, AWS KMS secrets provider
- **Templates**:
  - `ecs-fargate` - EU region deployment
- **Resources**:
  - **Cloudflare Registrar**: Domain `amagenta.ai` with empty DNS records array
  - **AWS S3 Bucket**: `talktome-media-storage` for media files
  - **MongoDB Atlas**: M10 instance, EU Central, AWS provider with backup (4h/24h)
**Key Patterns**: Shared staging/production resources using YAML anchors, backup configuration

#### **Main Application** (`/home/iasadykov/projects/github/talktome-tech/talktome/.sc/stacks/talktome/`)
**Files:** `client.yaml` (42 lines)
**Resources Catalog:**
- **Client Stack Type**: `cloud-compose` (ECS deployment)
- **Parent**: `devops/talktome-tech`
- **Key Configurations**:
  - **Multi-Resource Usage**: MongoDB + S3 media storage
  - **Meteor.js Application**: Meteor settings and mail URL configuration
  - **Security**: Cloudflare-only ingress protection
  - **Size**: 1024 CPU, 2048MB memory
  - **Uses**: `mongodb`, `talktome-media-storage` resources from parent
  - **Multi-Environment**: staging and production with different secrets
  - **Resource References**: `${resource:mongodb.uri}` pattern
**Key Patterns**: Meteor.js deployment, multi-resource usage, Cloudflare security, environment-specific secrets

### MyBridge Examples

#### **DevOps Infrastructure** (`/home/iasadykov/projects/github/mybridge/devops/.sc/stacks/mybridge/`)
**Files:** `server.yaml` (85 lines), `secrets.yaml` (5692 bytes)
**Resources Catalog:**
- **Provisioner**: AWS S3 bucket state storage, AWS KMS secrets provider
- **Templates**:
  - `ecs-fargate` - EU region deployment
  - `aws-static-website` - Static website deployment
  - `aws-lambda` - Serverless functions
- **Resources**:
  - **Cloudflare Registrar**: Domain `amagenta.ai` with SPF records
  - **DNS Records**: SPF configuration for Google and HubSpot email
**Key Patterns**: Email SPF configuration, HubSpot integration, Google email services

#### **Blog Service** (`/home/iasadykov/projects/github/mybridge/blog/.sc/stacks/blog/`)
**Files:** `client.yaml` (46 lines), `docker-compose.yaml` (2106 bytes), `Caddyfile` (508 bytes), `caddy.Dockerfile` (152 bytes)
**Resources Catalog:**
- **Client Stack Type**: `cloud-compose` (ECS deployment)
- **Parent**: `devops/mybridge`
- **Key Configurations**:
  - **Multi-Service Deployment**: Caddy + Blog application
  - **MySQL Database**: Complete MySQL resource references
  - **Database Configuration**: Multiple environment variables for database connection
  - **Gmail SMTP Integration**: Complete Gmail configuration for blog notifications
  - **Size**: 1024 CPU, 2048MB memory
  - **Uses**: `mysql` resource from parent
  - **Custom Caddy**: Custom Caddyfile and Dockerfile for reverse proxy
  - **Multi-Environment**: staging and production domains
**Key Patterns**: Multi-service deployment with reverse proxy, MySQL resource references, Gmail SMTP integration, blog-specific configuration

#### **Map Service** (`/home/iasadykov/projects/github/mybridge/map/.sc/stacks/map/`)
**Files:** `client.yaml` (41 lines)
**Resources Catalog:**
- **Client Stack Type**: `cloud-compose` (ECS deployment)
- **Parent**: `devops/mybridge`
- **Key Configurations**:
  - **Meteor.js Application**: Similar to TalkToMe Tech main app
  - **MongoDB Integration**: `${resource:mongodb.uri}` pattern
  - **Security**: Cloudflare-only ingress protection
  - **Size**: 1024 CPU, 2048MB memory
  - **Uses**: `mongodb` resource from parent
  - **Multi-Environment**: staging and production with different secrets
  - **Main Domain**: Production uses root domain `mybridge.tech`
**Key Patterns**: Meteor.js deployment, MongoDB resource references, Cloudflare security, root domain production deployment

### Simple Container Project Examples

#### **API Documentation** (`/home/iasadykov/projects/github/simple-container/api/.sc/stacks/docs/`)
**Files:** `client.yaml` (11 lines)
**Resources Catalog:**
- **Client Stack Type**: `static` (Static website deployment)
- **Parent**: `dist`
- **Key Configurations**:
  - **Bundle Directory**: `${git:root}/docs/site` for MkDocs output
  - **Domain**: `docs.simple-container.com`
  - **Static Website**: `index.html`, `404.html` documents
  - **Location**: `EUROPE-CENTRAL2` (GCP region)
**Key Patterns**: MkDocs documentation deployment, GCP static hosting, European region

#### **Landing Page** (`/home/iasadykov/projects/github/simple-container/landing/.sc/stacks/landing/`)
**Files:** `client.yaml` (11 lines), `server.yaml` (690 bytes), `secrets.yaml` (2961 bytes)
**Resources Catalog:**
- **Client Stack Type**: `static` (Static website deployment)
- **Parent**: `landing`
- **Key Configurations**:
  - **Bundle Directory**: `${git:root}/public` for static files
  - **Domain**: `simple-container.com` (main website)
  - **Static Website**: `index.html` for both index and error documents
  - **Location**: `EUROPE-CENTRAL2` (GCP region)
  - **Server Configuration**: Separate server.yaml with parent stack resources
**Key Patterns**: Main website deployment, SPA configuration (same index/error document), European hosting

#### **Welder Service** (`/home/iasadykov/projects/github/simple-container/welder/.sc/stacks/welder/`)
**Files:** `client.yaml` (11 lines), `server.yaml` (690 bytes), `secrets.yaml` (2933 bytes), `.gitignore` (23 bytes)
**Resources Catalog:**
- **Client Stack Type**: `static` (Static website deployment)
- **Parent**: `welder`
- **Key Configurations**:
  - **Bundle Directory**: `${git:root}/docs/site` for documentation output
  - **Domain**: `welder.simple-container.com` (subdomain)
  - **Static Website**: `index.html`, `404.html` documents
  - **Location**: `EUROPE-CENTRAL2` (GCP region)
  - **Server Configuration**: Separate server.yaml with parent stack resources
  - **Git Ignore**: Configuration file management
**Key Patterns**: Documentation subdomain deployment, standard static website configuration, European hosting

## Key Configuration Patterns Identified

### MongoDB Atlas Patterns
- **extraProviders Structure**: Uses `AWS` (not `aws`) with `credentials: "${auth:aws-us}"` reference
- **Network Configuration**: `allowCidrs` array, `privateLinkEndpoint.providerName`
- **Backup Configuration**: `every: "4h"`, `retention: "24h"` format
- **Complete Structure**: All required fields like `admins`, `developers`, `instanceSize`, `orgId`, `region`, `cloudProvider`

### AWS S3 Patterns
- **CORS Configuration**: `corsConfig.allowedOrigins`, `corsConfig.allowedMethods`
- **Credentials Reference**: `credentials: "${auth:aws-eu}"` pattern
- **Basic Properties**: `name`, `allowOnlyHttps` (not fictional properties)

### Security Group Patterns
- **Cloudflare Integration**: `cloudExtras.securityGroup.ingress.allowOnlyCloudflare: true`
- **Custom CIDR Blocks**: Specific IP ranges and security configurations

### Scaling and Alerts Patterns
- **Scaling Configuration**: `scale.max`, `scale.min`, `scale.policy.cpu.max`
- **Alert Configuration**: Slack webhooks, memory/CPU thresholds with proper structure
- **Dependencies**: Cross-service resource dependencies with proper referencing

### Resource Usage Patterns
- **Uses Directive**: `uses: [resource-name1, resource-name2]` for consuming parent resources
- **Parent References**: `parent: myproject/devops` for referencing parent stacks
- **Environment Variables**: Proper `env` and `secrets` sections with resource references

## How to Use This Map

When fixing fictional properties in documentation:

1. **Reference Specific Files**: Use exact file paths and line numbers for verification
2. **Compare Structures**: Match documentation examples against real-world usage patterns
3. **Verify Complex Configurations**: Check nested structures like MongoDB Atlas `extraProviders`
4. **Validate Service Patterns**: Use service stack examples for deployment configurations
5. **Cross-Reference Properties**: Ensure all properties exist in actual Go structs

This map provides authoritative real-world examples to reference when identifying and fixing fictional properties, ensuring documentation accuracy and implementability.

## Resource Type Cross-Reference Table

### AWS Resources
| Resource Type        | Real Examples                                     | Key Properties Verified                                                |
|----------------------|---------------------------------------------------|------------------------------------------------------------------------|
| `s3-bucket`          | TalkToMe Tech (media storage)                     | `name`, `allowOnlyHttps`                                               |
| `ecs-fargate`        | Integrail DevOps, AlphaMind DevOps, TalkToMe Tech | Multi-region, different accounts                                       |
| `aws-lambda`         | Integrail BAAS                                    | `lambdaRoutingType: function-url`, `lambdaInvokeMode: RESPONSE_STREAM` |
| `aws-static-website` | Integrail DevOps, AlphaMind DevOps                | Static site deployment                                                 |
| `aws-kms`            | Integrail DevOps, TalkToMe Tech                   | Secrets provider configuration                                         |

### GCP Resources
| Resource Type               | Real Examples    | Key Properties Verified                                            |
|-----------------------------|------------------|--------------------------------------------------------------------|
| `gcp-gke-autopilot-cluster` | aiwayz-sc-config | `gkeMinVersion`, `location`, `caddy` config                        |
| `gcp-artifact-registry`     | aiwayz-sc-config | `location`, `docker.immutableTags`                                 |
| `gcp-pubsub`                | aiwayz-sc-config | `labels`, `subscriptions`, `ackDeadlineSec`, `exactlyOnceDelivery` |
| `gcp-redis`                 | aiwayz-sc-config | `memorySizeGb`, `region`, `redisConfig.maxmemory-policy`           |
| `gcp-bucket`                | aiwayz-sc-config | State storage configuration                                        |
| `gcp-kms`                   | aiwayz-sc-config | Secrets provider configuration                                     |

### MongoDB Atlas
| Resource Type   | Real Examples                                     | Key Properties Verified                                                   |
|-----------------|---------------------------------------------------|---------------------------------------------------------------------------|
| `mongodb-atlas` | aiwayz-sc-config, AlphaMind DevOps, TalkToMe Tech | `instanceSize` (M0, M10, M30), `backup` config, `region`, `cloudProvider` |

### Cloudflare
| Resource Type | Real Examples                                                       | Key Properties Verified               |
|---------------|---------------------------------------------------------------------|---------------------------------------|
| `cloudflare`  | aiwayz-sc-config, Integrail DevOps, AlphaMind DevOps, TalkToMe Tech | `zoneName`, `dnsRecords`, `accountId` |

### Deployment Patterns
| Pattern Type          | Real Examples                                                                                | Key Configurations Verified                                                             |
|-----------------------|----------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------|
| `single-image`        | Integrail BAAS, Bedrock Gateway, Storage Service, Billing, Code Executor (staging)           | Lambda with response streaming, static egress IP, scheduled jobs, AWS Bedrock IAM roles |
| `cloud-compose`       | Integrail Milvus, Code Executor (beta), AlphaMind Gagarin/Nest, FullDive VR AI Stats/Streams | NLB load balancer, auto-scaling, hardcoded cluster IPs, cross-service dependencies      |
| `static`              | Simple Container Docs/Landing/Welder                                                         | GCP static hosting, MkDocs deployment, SPA configuration                                |
| Stack inheritance     | FullDive VR Browser API                                                                      | `inherit: <base stack>` pattern                                                         |
| Mixed per environment | Integrail Code Executor                                                                      | Lambda (staging) + ECS (beta) in same service                                           |

### Secrets Management
| Provider Type | Real Examples                                    | Key Configurations Verified |
|---------------|--------------------------------------------------|-----------------------------|
| `aws-kms`     | Integrail DevOps, TalkToMe Tech, MyBridge DevOps | KMS key provisioning        |
| `gcp-kms`     | aiwayz-sc-config                                 | GCP KMS configuration       |
| `passphrase`  | AlphaMind DevOps                                 | Passphrase-based encryption |

### Advanced Configuration Patterns
| Pattern Type                  | Real Examples                | Key Configurations Verified                           |
|-------------------------------|------------------------------|-------------------------------------------------------|
| Lambda scheduled jobs         | Integrail Storage Service    | `cron(0 * * * ? *)` expressions, automated cleanup    |
| AWS Bedrock integration       | Integrail Bedrock Gateway    | Specific IAM roles for AI model access                |
| Hardcoded infrastructure      | FullDive VR AI Stats/Streams | Cluster IPs, database IPs for external resources      |
| Cross-service dependencies    | AlphaMind Nest Backend       | `${dependency:service.resource.uri}` patterns         |
| Zero-downtime deployment      | FullDive VR Streams          | `minAvailable: 0`, `maxSurge: 0` configurations       |
| High-performance scaling      | Integrail Code Executor      | 40GB ephemeral storage, very low CPU thresholds (30%) |
| Multi-environment inheritance | Integrail Billing            | `parentEnv: prod` for beta environment                |
| Blockchain integration        | AlphaMind Nest Backend       | Multiple smart contract addresses                     |
| Email service integration     | MyBridge DevOps              | SPF records for Google/HubSpot                        |
| Private configuration         | Integrail Agent Marketplace  | .gitignore restricted sensitive configs               |
