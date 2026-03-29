---
title: DevOps Setup
description: How to set up DevOps infrastructure configuration (server.yaml)
platform: platform
product: simple-container
category: skills
subcategory: devops
date: '2026-03-29'
---

# DevOps Setup Skill

This skill guides DevOps teams through creating the infrastructure stack configuration (server.yaml). This configuration defines shared resources that service deployments will reference.

## What is server.yaml?

The `server.yaml` file defines:
- Cloud provider and region
- Shared resources (databases, storage, queues)
- Network configuration
- Authentication and secrets
- Parent stack references

## Prerequisites

- SC CLI installed (see [Installation](installation.md))
- Cloud provider account with required permissions
- Project name decided

## Steps

### Step 1: Choose Your Cloud Provider

Select your target cloud provider:

| Provider | Use Case |
|----------|----------|
| AWS | ECS Fargate, Lambda, S3 |
| GCP | GKE Autopilot, Cloud Run, Cloud Storage |
| Kubernetes | Self-hosted K8s deployments |

See [Cloud Providers](cloud-providers/) for provider-specific setup.

### Step 2: Create Project Structure

Create the directory structure:

```bash
# Create project directory
mkdir -p myproject/.sc/stacks/devops

# Navigate to the directory
cd myproject/.sc/stacks/devops
```

### Step 3: Create server.yaml

Create the `server.yaml` file based on your provider:

#### AWS Example

```yaml
# File: myproject/.sc/stacks/devops/server.yaml
schemaVersion: 1.0

# Project and stack identification
project: myproject
name: devops

# Cloud provider configuration
provider:
  name: aws
  region: us-east-1
  accountId: "${AWS_ACCOUNT_ID}"

# Authenticate using environment variables or IAM role
auth:
  - name: aws-main
    provider: aws
    config:
      accessKeyId: "${AWS_ACCESS_KEY_ID}"
      secretAccessKey: "${AWS_SECRET_ACCESS_KEY}"

# Shared resources that services will use
resources:
  # Primary database
  - name: postgres-main
    type: aws:rds:postgres
    config:
      instanceClass: db.t3.micro
      allocatedStorage: 20
      multiAz: false

  # Object storage
  - name: s3-assets
    type: aws:s3:bucket
    config:
      publicAccess: false
      versioning: true

  # Cache layer
  - name: redis-cache
    type: aws:elasticache:redis
    config:
      nodeType: cache.t3.micro
      numNodes: 1

# Network configuration
networking:
  vpc:
    cidr: 10.0.0.0/16
  subnets:
    - 10.0.1.0/24  # us-east-1a
    - 10.0.2.0/24  # us-east-1b
```

#### GCP Example

```yaml
# File: myproject/.sc/stacks/devops/server.yaml
schemaVersion: 1.0

project: myproject
name: devops

provider:
  name: gcp
  region: us-central1
  projectId: "${GCP_PROJECT_ID}"

auth:
  - name: gcp-main
    provider: gcp
    config:
      credentials: "${GCP_SERVICE_ACCOUNT_KEY}"

resources:
  - name: cloudsql-postgres
    type: gcp:cloudsql:postgres
    config:
      tier: db-f1-micro
      region: us-central1

  - name: gcs-assets
    type: gcp:storage:bucket
    config:
      location: us-central1
      publicAccess: false
```

#### Kubernetes Example

```yaml
# File: myproject/.sc/stacks/devops/server.yaml
schemaVersion: 1.0

project: myproject
name: devops

provider:
  name: kubernetes
  context: my-cluster

auth:
  - name: k8s-main
    provider: kubernetes
    config:
      kubeconfig: "${KUBECONFIG_PATH}"

resources:
  - name: postgres-db
    type: kubernetes:postgres
    config:
      storage: 10Gi
      className: standard

  - name: s3-compatible
    type: kubernetes:minio
    config:
      storage: 20Gi
```

### Step 4: Create secrets.yaml (Optional)

If your resources require secrets, create `secrets.yaml`:

```yaml
# File: myproject/.sc/stacks/devops/secrets.yaml
schemaVersion: 1.0

secrets:
  # Database passwords
  - name: postgres-main-password
    value: "${POSTGRES_PASSWORD}"

  # API keys
  - name: api-key
    value: "${API_KEY}"

  # External service credentials
  - name: stripe-key
    value: "${STRIPE_SECRET_KEY}"
```

### Step 5: Verify Configuration

Validate your configuration:

```bash
# Navigate to project directory
cd myproject

# Validate server.yaml
sc validate -f .sc/stacks/devops/server.yaml

# Check configuration
sc stacks list
```

## Complete Example Files

For complete working examples see:
- [AWS DevOps Example](../../examples/ecs-deployments/backend-service/)
- [GCP DevOps Example](../../examples/gke-autopilot/comprehensive-setup/)
- [Kubernetes DevOps Example](../../examples/kubernetes-native/streaming-platform/)

## Common Issues

### Authentication Failed

Ensure your credentials are correct:
- AWS: Check `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`
- GCP: Verify service account has required permissions
- Kubernetes: Verify kubeconfig context is correct

### Resource Creation Failed

Check:
- Account has required quotas
- Permissions are sufficient
- Region is available

## Next Steps

After setting up DevOps configuration:

1. [Service Setup](service-setup.md) - Create service configuration (client.yaml)
2. [Deployment Types](deployment-types.md) - Choose the right deployment type
3. [Secrets Management](secrets-management.md) - Configure secrets