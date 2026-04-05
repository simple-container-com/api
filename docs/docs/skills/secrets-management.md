---
title: Secrets Management
description: How to configure secrets and credentials in SC
platform: platform
product: simple-container
category: skills
subcategory: secrets
date: '2026-03-29'
---

# Secrets Management Skill

This skill guides you through configuring secrets and credentials in Simple Container. Proper secrets management is critical for security and for AI agents to understand how to obtain and use credentials.

## Overview

SC supports three types of placeholders for referencing secrets and resources:

| Placeholder Type | Syntax | Purpose |
|-----------------|--------|---------|
| **Auth** | `${auth:provider}` | Authentication configuration |
| **Secret** | `${secret:name}` | Secrets from secrets.yaml |
| **Resource** | `${resource:resourceName.field}` | Resources from server.yaml |

## secrets.yaml Structure

Create a `secrets.yaml` file in your stack directory:

```yaml
# File: myproject/.sc/stacks/devops/secrets.yaml
schemaVersion: 1.0

secrets:
  # Simple string secrets
  - name: database-password
    value: "${DATABASE_PASSWORD}"

  - name: api-key
    value: "${API_KEY}"

  # Reference auth providers
  - name: aws-credentials
    auth: aws-main

  # Multi-line secrets (use yaml pipe)
  - name: private-key
    value: |
      -----BEGIN RSA PRIVATE KEY-----
      ...key content...
      -----END RSA PRIVATE KEY-----
```

## Placeholder Types

### Auth Placeholders

Auth placeholders reference authentication configurations:

```yaml
# In server.yaml
auth:
  - name: aws-main
    provider: aws
    config:
      accessKeyId: "${AWS_ACCESS_KEY_ID}"
      secretAccessKey: "${AWS_SECRET_ACCESS_KEY}"

# In client.yaml - reference auth
auth:
  - name: runtime-auth
    source: ${auth:aws-main}
```

### Secret Placeholders

Reference secrets from secrets.yaml:

```yaml
# In client.yaml
secrets:
  DATABASE_PASSWORD: "${secret:database-password}"
  API_KEY: "${secret:api-key}"
```

### Resource Placeholders

Reference resources from server.yaml:

```yaml
# In client.yaml
env:
  DATABASE_HOST: "${resource:postgres-main.host}"
  DATABASE_NAME: "${resource:postgres-main.database}"
  DATABASE_USER: "${resource:postgres-main.user}"

secrets:
  DATABASE_PASSWORD: "${resource:postgres-main.password}"
  DATABASE_URL: "${resource:postgres-main.url}"
```

## Common Resource Fields

### AWS RDS PostgreSQL

```yaml
resource: postgres-main
fields:
  host: Database endpoint
  port: Database port (5432)
  database: Database name
  user: Master username
  password: Master password
  url: Full connection string
```

### AWS S3 Bucket

```yaml
resource: s3-assets
fields:
  bucket: Bucket name
  region: Bucket region
  arn: Bucket ARN
  url: Bucket URL
```

### GCP Cloud SQL

```yaml
resource: cloudsql-postgres
fields:
  host: Instance IP
  port: Instance port (5432)
  database: Database name
  user: Username
  password: Password
  instance: Instance connection name
  url: Connection string
```

## Environment Variable References

For secrets that should come from environment variables:

```yaml
secrets:
  - name: database-password
    value: "${DATABASE_PASSWORD}"
```

This allows you to set secrets at runtime:

```bash
export DATABASE_PASSWORD="my-secure-password"
sc deploy -s myservice -e staging
```

## How AI Agents Should Obtain Secrets

### AWS Secrets

```bash
# Get AWS access key
aws iam create-access-key --user-name your-username

# Get account ID
aws sts get-caller-identity --query 'Account'

# Get region
aws configure get region
```

### GCP Secrets

```bash
# Create service account and get key
gcloud iam service-accounts create sc-sa --project your-project
gcloud iam service-accounts keys create key.json \
  --iam-account=sc-sa@your-project.iam.gserviceaccount.com

# Get project ID
gcloud config get-value project
```

### Kubernetes Secrets

```bash
# Get current kubeconfig
kubectl config view --flatten

# Create namespace
kubectl create namespace myproject
```

## Best Practices

1. **Never hardcode secrets** - Always use placeholders
2. **Use environment variables** - For local development
3. **Rotate secrets regularly** - Update secrets.yaml periodically
4. **Use separate secrets per environment** - Different passwords for staging/prod
5. **Audit secret access** - Log who accesses secrets

## Example: Complete Secrets Configuration

### server.yaml

```yaml
schemaVersion: 1.0

project: myproject
name: devops

provider:
  name: aws
  region: us-east-1
  accountId: "${AWS_ACCOUNT_ID}"

auth:
  - name: aws-main
    provider: aws
    config:
      accessKeyId: "${AWS_ACCESS_KEY_ID}"
      secretAccessKey: "${AWS_SECRET_ACCESS_KEY}"

resources:
  - name: postgres-main
    type: aws:rds:postgres
    config:
      instanceClass: db.t3.micro
```

### secrets.yaml

```yaml
schemaVersion: 1.0

secrets:
  - name: rds-password
    value: "${RDS_PASSWORD}"
```

### client.yaml

```yaml
schemaVersion: 1.0

stacks:
  prod:
    type: cloud-compose
    parent: myproject/devops
    config:
      uses:
        - postgres-main
      env:
        DATABASE_HOST: "${resource:postgres-main.host}"
        DATABASE_USER: "${resource:postgres-main.user}"
      secrets:
        DATABASE_PASSWORD: "${resource:postgres-main.password}"
```

## Next Steps

After configuring secrets:

1. [DevOps Setup](devops-setup.md) - If you haven't set up infrastructure
2. [Service Setup](service-setup.md) - Configure your service
3. [Cloud Providers](cloud-providers/) - Provider-specific guides