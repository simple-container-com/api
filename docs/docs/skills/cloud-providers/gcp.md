---
title: GCP Setup
description: GCP-specific setup guide for Simple Container
platform: platform
product: simple-container
category: skills
subcategory: cloud-provider
date: '2026-03-29'
---

# GCP Setup Skill

This skill guides you through setting up Google Cloud Platform (GCP) credentials and resources for Simple Container. Follow these steps to configure GCP authentication and create required resources.

## Prerequisites

- GCP account with appropriate permissions
- Google Cloud CLI installed (`gcloud --version`)
- SC CLI installed (see [Installation](../installation.md))

## Steps

### Step 1: Install Google Cloud CLI

If you haven't already, install the Google Cloud CLI:

```bash
# Download Google Cloud SDK
curl https://sdk.cloud.google.com | bash

# Initialize
gcloud init

# Verify installation
gcloud version
```

### Step 2: Create GCP Project

Create a new GCP project or use an existing one:

```bash
# Create new project
gcloud projects create myproject-sc --name="Simple Container Project"

# Set current project
gcloud config set project myproject-sc
```

### Step 3: Enable Required APIs

Enable the required GCP APIs:

```bash
# Enable GKE (if using GKE Autopilot)
gcloud services enable container.googleapis.com

# Enable Cloud Run
gcloud services enable run.googleapis.com

# Enable Cloud SQL
gcloud services enable sqladmin.googleapis.com

# Enable Artifact Registry
gcloud services enable artifactregistry.googleapis.com

# Enable Cloud Storage
gcloud services enable storage-api.googleapis.com
```

### Step 4: Create Service Account

Create a service account with required permissions:

```bash
# Create service account
gcloud iam service-accounts create sc-service-account \
  --project=myproject-sc \
  --display-name="Simple Container Service Account"

# Get your email address
SERVICE_ACCOUNT_EMAIL="sc-service-account@myproject-sc.iam.gserviceaccount.com"
```

### Step 5: Assign Permissions

Grant required roles to the service account:

```bash
# Get service account email
SERVICE_ACCOUNT_EMAIL="sc-service-account@myproject-sc.iam.gserviceaccount.com"

# Assign roles
gcloud projects add-iam-policy-binding myproject-sc \
  --member="serviceAccount:${SERVICE_ACCOUNT_EMAIL}" \
  --role="roles/owner"

# Or for more granular permissions:
gcloud projects add-iam-policy-binding myproject-sc \
  --member="serviceAccount:${SERVICE_ACCOUNT_EMAIL}" \
  --role="roles/container.admin"

gcloud projects add-iam-policy-binding myproject-sc \
  --member="serviceAccount:${SERVICE_ACCOUNT_EMAIL}" \
  --role="roles/run.admin"

gcloud projects add-iam-policy-binding myproject-sc \
  --member="serviceAccount:${SERVICE_ACCOUNT_EMAIL}" \
  --role="roles/storage.admin"

gcloud projects add-iam-policy-binding myproject-sc \
  --member="serviceAccount:${SERVICE_ACCOUNT_EMAIL}" \
  --role="roles/cloudsql.admin"
```

### Step 6: Create Service Account Key

Create and download the service account key:

```bash
# Get project ID
PROJECT_ID=$(gcloud config get-value project)

# Create key file
gcloud iam service-accounts keys create key.json \
  --iam-account=sc-service-account@${PROJECT_ID}.iam.gserviceaccount.com

# View key (for setting environment variable)
cat key.json

# Or set directly from file
export GCP_SERVICE_ACCOUNT_KEY=$(cat key.json | base64 -w 0)
```

### Step 7: Configure Authentication

Authenticate using the service account:

```bash
# Get project ID
PROJECT_ID=$(gcloud config get-value project)

# Activate service account
gcloud auth activate-service-account \
  --key-file=key.json

# Set default project
gcloud config set project ${PROJECT_ID}

# Get project number
gcloud projects describe ${PROJECT_ID} --format="value(projectNumber)"
```

### Step 8: Create Artifact Registry Repository

Create a Docker repository for your images:

```bash
# Get project ID
PROJECT_ID=$(gcloud config get-value project)

# Create Artifact Registry repository
gcloud artifacts repositories create myproject-repo \
  --repository-format=docker \
  --location=us-central1 \
  --description="Simple Container images"

# Verify
gcloud artifacts repositories list
```

### Step 9: Set Environment Variables

For SC to use your GCP credentials, set these environment variables:

```bash
# Project ID
export GCP_PROJECT_ID="myproject-sc"

# Service account key (base64 encoded)
export GCP_SERVICE_ACCOUNT_KEY="$(cat key.json | base64 -w 0)"

# Default region
export GCP_REGION="us-central1"
```

### Step 10: Verify Setup

Verify your GCP setup works with SC:

```bash
# Check GCP configuration
gcloud config list

# Test authentication
gcloud auth list
```

## Environment Variables for GCP

| Variable | Description | Required |
|----------|-------------|----------|
| `GCP_PROJECT_ID` | GCP project ID | Yes |
| `GCP_SERVICE_ACCOUNT_KEY` | Service account key (JSON or base64) | Yes |
| `GCP_REGION` | Default GCP region | Yes |
| `GCP_ZONE` | Default GCP zone | No |

## Example: Full GCP server.yaml

```yaml
schemaVersion: 1.0

project: myproject
name: devops

provider:
  name: gcp
  region: ${GCP_REGION}
  projectId: ${GCP_PROJECT_ID}

auth:
  - name: gcp-main
    provider: gcp
    config:
      credentials: ${GCP_SERVICE_ACCOUNT_KEY}

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
```

## Common Issues

### "Service account not found"

Verify the service account exists:
```bash
gcloud iam service-accounts list
```

### "API not enabled"

Enable required APIs:
```bash
gcloud services enable container.googleapis.com
gcloud services enable run.googleapis.com
```

### "Permission denied"

Check IAM permissions:
```bash
gcloud projects get-iam-policy myproject-sc
```

## Next Steps

After GCP setup:

1. [DevOps Setup](../devops-setup.md) - Create server.yaml with GCP resources
2. [Service Setup](../service-setup.md) - Configure your service deployment