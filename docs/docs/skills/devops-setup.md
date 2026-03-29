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

provisioner:
  type: pulumi
  config:
    state-storage:
      type: s3-bucket
      config:
        credentials: "${auth:aws-main}"
        provision: true
        account: "${auth:aws-main.projectId}"
        bucketName: myproject-sc-state
        region: us-east-1
    secrets-provider:
      type: aws-kms
      config:
        credentials: "${auth:aws-main}"
        provision: true
        keyName: myproject-kms-key

templates:
  stack-per-app:
    type: ecs-fargate
    config:
      credentials: "${auth:aws-main}"
      account: "${auth:aws-main.projectId}"

resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: YOUR_CLOUDFLARE_ACCOUNT_ID
      zoneName: myproject.com

  resources:
    staging:
      template: stack-per-app
      resources:
        postgres-main:
          type: mongodb-atlas
          config:
            admins: [ "admin" ]
            developers: [ ]
            instanceSize: "M0"
            orgId: YOUR_MONGODB_ORG_ID
            region: "US_EAST_1"
            cloudProvider: AWS
            privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
            publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"

        s3-assets:
          type: s3-bucket
          config:
            credentials: "${auth:aws-main}"

        redis-cache:
          type: aws-elasticache-redis
          config:
            credentials: "${auth:aws-main}"
            nodeType: cache.t3.micro
            numNodes: 1
```

#### GCP Example

```yaml
# File: myproject/.sc/stacks/devops/server.yaml
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
        bucketName: myproject-sc-state
        location: us-central1
    secrets-provider:
      type: gcp-kms
      config:
        provision: true
        projectId: "${auth:gcloud.projectId}"
        keyName: myproject-kms-key
        keyLocation: global
        credentials: "${auth:gcloud}"

templates:
  stack-per-app-gke:
    type: gcp-gke-autopilot
    config:
      projectId: "${auth:gcloud.projectId}"
      credentials: "${auth:gcloud}"

  static-website:
    type: gcp-static-website
    config:
      projectId: "${auth:gcloud.projectId}"
      credentials: "${auth:gcloud}"

resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: YOUR_CLOUDFLARE_ACCOUNT_ID
      zoneName: myproject.com

  resources:
    staging:
      template: stack-per-app-gke
      resources:
        cloudsql-postgres:
          type: gcp-cloudsql
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            tier: db-f1-micro
            region: us-central1

        gcs-assets:
          type: gcp-storage-bucket
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            location: us-central1
```

#### Kubernetes Example

```yaml
# File: myproject/.sc/stacks/devops/server.yaml
schemaVersion: 1.0

provisioner:
  type: pulumi
  config:
    state-storage:
      type: s3-bucket
      config:
        credentials: "${auth:aws-state}"
        provision: true
        account: "${auth:aws-state.projectId}"
        bucketName: myproject-k8s-state
        region: us-east-1
    secrets-provider:
      type: aws-kms
      config:
        credentials: "${auth:aws-state}"
        provision: true
        keyName: myproject-k8s-kms-key

templates:
  stack-per-app-k8s:
    type: kubernetes-k8s
    config:
      kubeconfig: "${auth:k8s.kubeconfig}"

resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: YOUR_CLOUDFLARE_ACCOUNT_ID
      zoneName: myproject.com

  resources:
    staging:
      template: stack-per-app-k8s
      resources:
        postgres-db:
          type: kubernetes-postgres
          config:
            storage: 10Gi
            className: standard

        minio-storage:
          type: kubernetes-minio
          config:
            storage: 20Gi
```

### Step 4: Create secrets.yaml

Create a separate `secrets.yaml` file to store sensitive values. The server.yaml references these using `${secret:NAME}` and `${auth:NAME}` placeholders:

```yaml
# File: myproject/.sc/stacks/devops/secrets.yaml
schemaVersion: 1.0

# Authentication configuration for cloud providers
# These are referenced by ${auth:NAME} in server.yaml
auth:
  aws-main:
    accessKey: "AKIAIOSFODNN7EXAMPLE"
    secretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
    region: us-east-1

  gcloud:
    projectId: "my-gcp-project-id"
    credentials: '{"type": "service_account", ...}'

  k8s:
    kubeconfig: "/path/to/kubeconfig"

# Secret values
# These are referenced by ${secret:NAME} in server.yaml
values:
  # CloudFlare API token (for DNS management)
  CLOUDFLARE_API_TOKEN: "your_cloudflare_api_token_here"

  # MongoDB Atlas credentials (for database resources)
  MONGODB_ATLAS_PUBLIC_KEY: "your_mongodb_public_key"
  MONGODB_ATLAS_PRIVATE_KEY: "your_mongodb_private_key"

  # Other secrets
  POSTGRES_PASSWORD: "secure_password_here"
  STRIPE_SECRET_KEY: "sk_live_..."
  API_KEY: "your_api_key_here"
```

**Important Security Notes:**
- Never commit `secrets.yaml` to version control
- Add `secrets.yaml` to your `.gitignore` file
- Use environment-specific secret files (e.g., `secrets.staging.yaml`, `secrets.prod.yaml`)
- Reference auth using `${auth:AUTH_NAME}` syntax for cloud credentials
- Reference secrets using `${secret:SECRET_NAME}` syntax for application secrets

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

Ensure your credentials are correctly configured:
- **AWS**: Verify `${auth:aws-*}` references match your secrets.yaml
- **GCP**: Ensure service account JSON is valid in secrets.yaml
- **Kubernetes**: Verify kubeconfig path is correct and accessible
- Check that credentials are referenced as `${auth:NAME}` in templates and resources

### Secret Not Found

If SC can't find a secret:
- Verify the secret is defined in `secrets.yaml`
- Check the spelling matches exactly (case-sensitive)
- Ensure secrets.yaml is in the same directory as server.yaml
- Use `${secret:SECRET_NAME}` syntax in server.yaml to reference secrets

### Resource Creation Failed

Check:
- Account has required quotas/limits
- IAM permissions are sufficient for resource creation
- Region/location is available in your cloud account
- Resource type matches your cloud provider (e.g., `mongodb-atlas` vs `gcp-cloudsql`)

## Next Steps

After setting up DevOps configuration:

1. [Service Setup](service-setup.md) - Create service configuration (client.yaml)
2. [Deployment Types](deployment-types.md) - Choose the right deployment type
3. [Secrets Management](secrets-management.md) - Configure secrets