---
title: Deployment Types
description: How to determine the correct deployment type for your service
platform: platform
product: simple-container
category: skills
subcategory: deployment
date: '2026-03-29'
---

# Deployment Types Skill

This skill helps you determine the correct deployment type for your service. Choosing the right deployment type is crucial for successful deployment.

## Overview

Simple Container supports three deployment types:

| Deployment Type | Use Case | Required Files | Example Platforms |
|----------------|----------|----------------|-------------------|
| **cloud-compose** | Multi-container microservices | Dockerfile, docker-compose.yaml | Kubernetes, ECS Fargate |
| **single-image** | Single-container applications | Dockerfile | AWS Lambda, Cloud Run |
| **static** | Static websites | Static files | AWS S3, GCP Cloud Storage |

## Decision Tree

Use this decision tree to select the correct deployment type:

```
START
  │
  ▼
Is this a static website (HTML/CSS/JS only)?
  │
  ├─YES──▶ Use "static" type
  │
  └─NO──▶ Does your application require multiple containers?
          │
          ├─YES──▶ Use "cloud-compose" type
          │
          └─NO──▶ Does your application need server-side processing?
                  │
                  ├─YES──▶ Use "single-image" type
                  │
                  └─NO──▶ Consider "static" with serverless functions
```

## Detailed Type Descriptions

### cloud-compose

**When to use:**
- Microservices architecture with multiple containers
- Need for databases, caches, message queues
- Complex networking between services
- Stateful applications requiring persistent volumes

**Example architectures:**
- API + Database + Cache + Queue
- Frontend + Backend + Worker + Scheduler
- Multiple microservices communicating via API

**Required files:**
- `Dockerfile` - Image definition
- `docker-compose.yaml` - Container orchestration

**Example client.yaml:**
```yaml
stacks:
  staging:
    type: cloud-compose
    parent: myproject/devops
    config:
      dockerComposeFile: ./docker-compose.yaml
      runs:
        - api
        - worker
```

### single-image

**When to use:**
- Single-container application
- Serverless deployment (Lambda, Cloud Run)
- Simple applications without complex dependencies
- Stateless services

**Example architectures:**
- REST API
- GraphQL API
- WebSocket server
- Background worker

**Required files:**
- `Dockerfile` - Must expose a single port

**Example client.yaml:**
```yaml
stacks:
  staging:
    type: single-image
    template: lambda-eu
    parent: myproject/devops
    config:
      image:
        dockerfile: ${git:root}/Dockerfile
      timeout: 180
```

### static

**When to use:**
- Static websites (React, Vue, Angular)
- Documentation sites
- Landing pages
- Single-page applications (SPA)

**Required files:**
- `bundleDir` - Directory containing static files

**Example client.yaml:**
```yaml
stacks:
  prod:
    type: static
    parent: myproject/devops
    config:
      bundleDir: ${git:root}/public
      domain: myproject.com
      indexDocument: index.html
```

## Technology-Specific Guidance

### Node.js Applications

| Architecture | Deployment Type |
|--------------|-----------------|
| Express API (single service) | single-image (Lambda) |
| Express + Redis + PostgreSQL | cloud-compose |
| NestJS microservices | cloud-compose |
| Next.js SSR | cloud-compose |
| React SPA | static |

### Python Applications

| Architecture | Deployment Type |
|--------------|-----------------|
| FastAPI (single service) | single-image (Lambda) |
| FastAPI + PostgreSQL + Redis | cloud-compose |
| Django + Celery | cloud-compose |
| Flask with background tasks | cloud-compose |

### Go Applications

| Architecture | Deployment Type |
|--------------|-----------------|
| REST API (single binary) | single-image (Lambda) |
| API + Database + Cache | cloud-compose |
| gRPC microservices | cloud-compose |

### Static Sites

| Framework | Deployment Type |
|-----------|-----------------|
| React (create-react-app) | static |
| Vue (Vue CLI/Vite) | static |
| Angular | static |
| Next.js (static export) | static |
| Gatsby | static |
| Hugo | static |
| Docusaurus | static |

## Examples with Frameworks

### Node.js + Express (single-image)

```yaml
stacks:
  prod:
    type: single-image
    template: lambda-eu
    config:
      image:
        dockerfile: ${git:root}/Dockerfile
```

```dockerfile
FROM node:18-alpine
WORKDIR /app
COPY . .
RUN npm install
EXPOSE 8080
CMD ["node", "index.js"]
```

### Node.js + Express + PostgreSQL (cloud-compose)

```yaml
stacks:
  prod:
    type: cloud-compose
    config:
      dockerComposeFile: ./docker-compose.yaml
      uses:
        - postgres-main
```

```yaml
version: '3.8'
services:
  api:
    build: .
    ports:
      - "8080:8080"
  db:
    image: postgres:15
```

### React SPA (static)

```yaml
stacks:
  prod:
    type: static
    config:
      bundleDir: ${git:root}/build
```

## Quick Reference

Use this quick reference to decide:

| Question | Answer | Type |
|----------|--------|------|
| Is it only HTML/CSS/JS? | Yes | static |
| Does it need multiple containers? | Yes | cloud-compose |
| Is it a single binary/service? | Yes | single-image |
| Deploying to Lambda/Cloud Run? | Yes | single-image |
| Need persistent volumes? | Yes | cloud-compose |

## See Also

- [Service Available Deployment Schemas](../reference/service-available-deployment-schemas.md)
- [Service Setup](service-setup.md)
- [Template Configuration Requirements](../ai-assistant/templates-config-requirements.md)