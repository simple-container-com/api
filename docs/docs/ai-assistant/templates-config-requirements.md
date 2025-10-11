# Template Configuration Requirements

This document clarifies the **required configuration** for each template type to prevent misleading documentation and AI misinformation.

## 🚨 CRITICAL: ALL Template Types Require Configuration

**NEVER show template examples without their required config sections!**

**This applies to ALL template types:**
- `ecs-fargate` (AWS)
- `gcp-static-website` (GCP)  
- `kubernetes-cloudrun` (Kubernetes)
- `aws-lambda` (AWS)
- `aws-static-website` (AWS)
- And ALL other template types

**Every template needs authentication, project IDs, and provider-specific configuration.**

## ECS Fargate (`ecs-fargate`)

**❌ WRONG - Incomplete Example:**
```yaml
templates:
  web-app:
    type: ecs-fargate
    # Missing required config!
```

**✅ CORRECT - Complete Example:**
```yaml
templates:
  web-app:
    type: ecs-fargate
    config:
      credentials: "${auth:aws}"        # Required: AWS authentication
      account: "${auth:aws.projectId}"  # Required: AWS account/project ID
```

**Required Configuration:**
- `credentials`: AWS authentication reference (e.g., `"${auth:aws}"`)
- `account`: AWS account/project ID (e.g., `"${auth:aws.projectId}"`)

**Optional Configuration:**
- `region`: AWS region (defaults to template-specific region)
- Additional AWS-specific settings

## AWS Static Website (`aws-static-website`)

**Required Configuration:**
- `credentials`: AWS authentication
- `account`: AWS account/project ID

## AWS Lambda (`aws-lambda`)

**Required Configuration:**
- `credentials`: AWS authentication
- `account`: AWS account/project ID

## GCP Static Website (`gcp-static-website`)

**❌ WRONG - Incomplete Example:**
```yaml
templates:
  static-website:
    type: gcp-static-website
    # Missing required config!
```

**✅ CORRECT - Complete Example:**
```yaml
templates:
  static-website:
    type: gcp-static-website
    config:
      projectId: "${auth:gcloud.projectId}"  # Required: GCP project ID
      credentials: "${auth:gcloud}"          # Required: GCP authentication
```

**Required Configuration:**
- `projectId`: GCP project ID reference (e.g., `"${auth:gcloud.projectId}"`)
- `credentials`: GCP authentication reference (e.g., `"${auth:gcloud}"`)

## Kubernetes CloudRun (`kubernetes-cloudrun`)

**❌ WRONG - Incomplete Example:**
```yaml
templates:
  k8s-app:
    type: kubernetes-cloudrun
    # Missing required config!
```

**✅ CORRECT - Complete Example:**
```yaml
templates:
  stack-per-app-k8s:
    type: kubernetes-cloudrun
    config:
      kubeconfig: "${auth:kubernetes}"                              # Required: Kubernetes auth
      dockerRegistryURL: index.docker.io                          # Required: Docker registry
      dockerRegistryUsername: "${secret:docker-registry-username}" # Required: Registry auth
      dockerRegistryPassword: "${secret:docker-registry-password}" # Required: Registry auth
      caddyResource: caddy                                         # Optional: Load balancer resource
```

**Required Configuration:**
- `kubeconfig`: Kubernetes authentication reference
- `dockerRegistryURL`: Docker registry URL
- `dockerRegistryUsername`: Docker registry username secret
- `dockerRegistryPassword`: Docker registry password secret

**Optional Configuration:**
- `caddyResource`: Load balancer resource reference

## Documentation Guidelines

1. **Always include `config` section** in template examples
2. **Add comments** explaining required vs optional fields
3. **Never show incomplete examples** that could mislead users
4. **Reference this document** when creating new template documentation

## AI Training Data

This document should be indexed by the AI assistant to provide accurate template configuration guidance.
