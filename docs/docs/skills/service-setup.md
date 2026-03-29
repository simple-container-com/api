---
title: Service Setup
description: How to set up service configuration (client.yaml)
platform: platform
product: simple-container
category: skills
subcategory: service
date: '2026-03-29'
---

# Service Setup Skill

This skill guides developers through creating service deployment configuration (client.yaml). The client.yaml defines how your service will be deployed, including the deployment type, resources, and environment configuration.

## What is client.yaml?

The `client.yaml` file defines:
- Deployment type (cloud-compose, single-image, static)
- Parent stack reference
- Container configuration
- Resource requirements
- Environment variables and secrets
- Scaling configuration

## Prerequisites

- SC CLI installed (see [Installation](installation.md))
- DevOps stack configured (see [DevOps Setup](devops-setup.md))
- Deployment type determined (see [Deployment Types](deployment-types.md))

## Steps

### Step 1: Determine Deployment Type

Choose your deployment type based on your application:

| Deployment Type | Use Case | Required Files |
|----------------|----------|----------------|
| **cloud-compose** | Multi-container microservices | Dockerfile, docker-compose.yaml |
| **single-image** | Single-container applications | Dockerfile |
| **static** | Static websites | Built static files |

If unsure, see [Deployment Types](deployment-types.md).

### Step 2: Create Service Structure

Create the directory structure:

```bash
# Create service directory
mkdir -p myproject/.sc/stacks/myservice

# Navigate to the directory
cd myproject/.sc/stacks/myservice
```

### Step 3: Create client.yaml

Create the `client.yaml` file based on your deployment type:

#### cloud-compose Example

```yaml
# File: myproject/.sc/stacks/myservice/client.yaml
schemaVersion: 1.0

stacks:
  staging:
    type: cloud-compose
    parent: myproject/devops
    config:
      domain: staging-myservice.myproject.com
      dockerComposeFile: ./docker-compose.yaml
      uses:
        - postgres-main      # Uses DB from server.yaml
        - redis-cache       # Uses cache from server.yaml
      runs:
        - myservice         # Runs myservice container
      size:
        cpu: 512
        memory: 1024
      scale:
        min: 1
        max: 5
        policy:
          cpu:
            max: 70
      env:
        DATABASE_HOST: "${resource:postgres-main.host}"
        DATABASE_NAME: "${resource:postgres-main.database}"
        DATABASE_USER: "${resource:postgres-main.user}"
        REDIS_URL: "${resource:redis-cache.url}"
      secrets:
        DATABASE_PASSWORD: "${resource:postgres-main.password}"
      healthCheck:
        path: /health
        port: 8080
        initialDelaySeconds: 30
        periodSeconds: 10

  prod:
    type: cloud-compose
    parent: myproject/devops
    config:
      domain: myservice.myproject.com
      dockerComposeFile: ./docker-compose.yaml
      uses:
        - postgres-main
        - redis-cache
      runs:
        - myservice
      size:
        cpu: 1024
        memory: 2048
      scale:
        min: 2
        max: 10
        policy:
          cpu:
            max: 70
      env:
        DATABASE_HOST: "${resource:postgres-main.host}"
        DATABASE_NAME: "${resource:postgres-main.database}"
        DATABASE_USER: "${resource:postgres-main.user}"
        REDIS_URL: "${resource:redis-cache.url}"
      secrets:
        DATABASE_PASSWORD: "${resource:postgres-main.password}"
```

#### single-image Example (AWS Lambda)

```yaml
# File: myproject/.sc/stacks/myservice/client.yaml
schemaVersion: 1.0

stacks:
  staging:
    type: single-image
    template: lambda-eu
    parent: myproject/devops
    config:
      domain: staging-myservice.myproject.com
      image:
        dockerfile: ${git:root}/Dockerfile
      timeout: 180
      maxMemory: 2048
      staticEgressIP: true
      cloudExtras:
        lambdaRoutingType: function-url
        lambdaInvokeMode: RESPONSE_STREAM
      uses:
        - postgres-main
      env:
        NODE_ENV: production
      secrets:
        MONGO_URI: "${resource:postgres-main.uri}"

  prod:
    type: single-image
    template: lambda-eu
    parent: myproject/devops
    config:
      domain: myservice.myproject.com
      image:
        dockerfile: ${git:root}/Dockerfile
      timeout: 300
      maxMemory: 4096
      staticEgressIP: true
      cloudExtras:
        lambdaRoutingType: function-url
      uses:
        - postgres-main
      env:
        NODE_ENV: production
      secrets:
        MONGO_URI: "${resource:postgres-main.uri}"
```

#### single-image Example (GCP Cloud Run)

```yaml
# File: myproject/.sc/stacks/myservice/client.yaml
schemaVersion: 1.0

stacks:
  staging:
    type: single-image
    template: cloud-run-eu
    parent: myproject/devops
    config:
      domain: staging-myservice.myproject.com
      image:
        dockerfile: ${git:root}/Dockerfile
      port: 8080
      cpu: 1
      memory: 512Mi
      minInstances: 0
      maxInstances: 5
      uses:
        - cloudsql-postgres
      env:
        NODE_ENV: production
      secrets:
        DATABASE_URL: "${resource:cloudsql-postgres.url}"
```

#### static Example

```yaml
# File: myproject/.sc/stacks/landing-page/client.yaml
schemaVersion: 1.0

stacks:
  staging:
    type: static
    parent: myproject/devops
    config:
      bundleDir: ${git:root}/public
      domain: staging.myproject.com
      indexDocument: index.html
      errorDocument: index.html
      location: us-east-1

  prod:
    type: static
    parent: myproject/devops
    config:
      bundleDir: ${git:root}/public
      domain: myproject.com
      indexDocument: index.html
      errorDocument: index.html
      location: us-east-1
```

### Step 4: Create Supporting Files

Depending on your deployment type, create supporting files:

#### Dockerfile (for single-image and cloud-compose)

```dockerfile
# Example: Node.js API
FROM node:18-alpine
WORKDIR /app
COPY package*.json ./
RUN npm ci --only=production
COPY . .
EXPOSE 8080
CMD ["node", "server.js"]
```

#### docker-compose.yaml (for cloud-compose)

```yaml
version: '3.8'
services:
  myservice:
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - "8080:8080"
    environment:
      NODE_ENV: production
      DATABASE_URL: ${DATABASE_URL}
      REDIS_URL: ${REDIS_URL}
    depends_on:
      - db
      - cache
  db:
    image: postgres:15-alpine
    environment:
      POSTGRES_PASSWORD: ${DATABASE_PASSWORD}
    volumes:
      - db-data:/var/lib/postgresql/data
  cache:
    image: redis:7-alpine

volumes:
  db-data:
```

### Step 5: Verify Configuration

Validate your configuration:

```bash
# Navigate to project directory
cd myproject

# Validate client.yaml
sc validate -f .sc/stacks/myservice/client.yaml

# Check what will be deployed
sc deploy --dry-run -s myservice -e staging
```

## Configuration Templates

For complete configuration templates, see:
- [Template Configuration Requirements](../ai-assistant/templates-config-requirements.md)
- [Deployment Schemas](../reference/service-available-deployment-schemas.md)

## Common Issues

### Reference to Missing Resource

Ensure the parent stack has the referenced resource:
```yaml
uses:
  - postgres-main  # Must exist in server.yaml
```

### Incorrect Image Path

For single-image, ensure Dockerfile path is correct:
```yaml
image:
  dockerfile: ${git:root}/Dockerfile
```

### Missing Environment Variables

List all required environment variables:
```yaml
env:
  DATABASE_HOST: "${resource:postgres-main.host}"
  DATABASE_NAME: "${resource:postgres-main.database}"
  DATABASE_USER: "${resource:postgres-main.user}"
secrets:
  DATABASE_PASSWORD: "${resource:postgres-main.password}"
```

## Next Steps

After service configuration:

1. [Secrets Management](secrets-management.md) - Configure secrets
2. Deploy your service: `sc deploy -s myservice -e staging`