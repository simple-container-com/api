---
title: Template Placeholders Guide
description: Comprehensive guide to Simple Container's template engine placeholders for dynamic configuration values
platform: platform
product: simple-container
category: devguide
subcategory: reference
guides: reference
date: '2024-12-07'
---

# **Template Placeholders Guide**

Simple Container provides a powerful template engine that allows you to use dynamic placeholders in your configuration files. These placeholders are resolved at deployment time, enabling flexible and reusable configurations.

**Golang Implementation Reference:** `pkg/provisioner/placeholders/placeholders.go`

## **Template Syntax**

All template placeholders use the syntax: `${extension:path}`

Where:
- `extension` is one of the 7 supported template extensions
- `path` specifies what value to retrieve from that extension

**Important:** All template placeholders can be used in both `client.yaml` and `server.yaml` files, providing flexibility for dynamic configuration at both the parent stack (DevOps) and client stack (developer) levels.

## **Supported Template Extensions**

Simple Container supports 7 template extensions for different types of dynamic values:

### **1. Environment Variables** (`env`)

Access environment variables from the system where Simple Container is running.

**Syntax:** `${env:VARIABLE_NAME}`

**With Default Value:** `${env:VARIABLE_NAME:default_value}`

**Examples:**
```yaml
# Basic environment variable
credentials: "${env:AWS_ACCESS_KEY_ID}"

# Environment variable with default value
region: "${env:AWS_REGION:us-east-1}"

# Database password from environment
password: "${env:DB_PASSWORD}"
```

**Real-World Usage:**

**In server.yaml (Parent Stack):**
```yaml
# From TalkToMe Tech production example
provisioner:
  config:
    secrets-provider:
      type: aws-kms
      config:
        keyName: "${env:KMS_KEY_NAME}"

resources:
  resources:
    production:
      resources:
        mongodb:
          config:
            publicKey: "${env:MONGODB_ATLAS_PUBLIC_KEY}"
            privateKey: "${env:MONGODB_ATLAS_PRIVATE_KEY}"
```

**In client.yaml (Client Stack):**
```yaml
# Environment-specific configuration
stacks:
  staging:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      domain: "${env:STAGING_DOMAIN:staging.myapp.com}"
      env:
        NODE_ENV: "${env:NODE_ENV:development}"
        API_URL: "${env:API_URL}"
```

### **2. Git Repository Information** (`git`)

Access information about the current Git repository.

**Available Properties:**
- `root` - Git repository root directory path

**Syntax:** `${git:root}`

**Examples:**
```yaml
# Reference files relative to git root
dockerComposeFile: ${git:root}/docker-compose.yaml

# Build context from git root
image:
  context: ${git:root}/src
  dockerfile: ${git:root}/src/Dockerfile
```

**Real-World Usage:**

**In client.yaml (Client Stack):**
```yaml
# From blockchain service example
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      dockerComposeFile: ${git:root}/docker-compose.yaml
      image:
        dockerfile: ${git:root}/src/Dockerfile
        context: ${git:root}/src
```

**In server.yaml (Parent Stack):**
```yaml
# Template configuration with git-relative paths
templates:
  my-template:
    type: ecs-fargate
    config:
      buildContext: ${git:root}/build
      configPath: ${git:root}/config
```

### **3. Authentication Configurations** (`auth`)

Access authentication configurations defined in your secrets.yaml file.

**Syntax:** 
- `${auth:auth_name}` - Get credentials value
- `${auth:auth_name.projectId}` - Get project ID (for cloud providers)

**Examples:**
```yaml
# AWS authentication
credentials: "${auth:aws-main}"
account: "${auth:aws-main.projectId}"

# GCP authentication  
projectId: "${auth:gcloud.projectId}"
credentials: "${auth:gcloud}"
```

**Real-World Usage:**

**In server.yaml (Parent Stack):**
```yaml
# From aiwayz-sc-config production example
templates:
  stack-per-app-gke:
    type: gcp-gke-autopilot
    config:
      projectId: "${auth:gcloud.projectId}"
      credentials: "${auth:gcloud}"

resources:
  resources:
    prod:
      resources:
        gke-autopilot-res:
          type: gcp-gke-autopilot-cluster
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
```

**In client.yaml (Client Stack):**
```yaml
# Client stack referencing parent authentication
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [gke-cluster]
      env:
        GCP_PROJECT: "${auth:gcloud.projectId}"
        CLUSTER_CREDENTIALS: "${auth:gcloud}"
```

### **4. Secrets Management** (`secret`)

Access secrets defined in your secrets.yaml file with support for inheritance.

**Syntax:** `${secret:SECRET_NAME}`

**Examples:**
```yaml
# API tokens
credentials: "${secret:CLOUDFLARE_API_TOKEN}"

# Database credentials
password: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"

# Service keys
apiKey: "${secret:OPENAI_API_KEY}"
```

**Real-World Usage:**
```yaml
# From MyBridge production example
resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: 23c5ca78cfb4721d9a603ed695a2623e
      zoneName: amagenta.ai

# From billing system example
stacks:
  production:
    config:
      secrets:
        API_KEY: ${secret:billing-api-key}
        STRIPE_SECRET_KEY: ${secret:stripe-secret-key}
```

**Inheritance Support:**
Secrets support inheritance from parent stacks. If a secret is not found in the current stack, Simple Container will look in the parent stack.

### **5. Stack Variables** (`var`)

Access custom variables defined in your stack configuration.

**Syntax:** `${var:VARIABLE_NAME}`

**Examples:**
```yaml
# Define variables in server.yaml
variables:
  app_version: "1.2.3"
  environment: "production"

# Use variables in configuration
image: "myapp:${var:app_version}"
domain: "${var:environment}.myapp.com"
```

**Real-World Usage:**
```yaml
# Stack variables for environment-specific configuration
variables:
  cluster_size: "3"
  instance_type: "m5.large"

resources:
  my-cluster:
    config:
      nodeCount: "${var:cluster_size}"
      instanceType: "${var:instance_type}"
```

### **6. Stack Metadata** (`stack`)

Access metadata about the current stack being processed.

**Available Properties:**
- `name` - Current stack name

**Syntax:** `${stack:name}`

**Examples:**
```yaml
# Use stack name in resource naming
bucketName: "myapp-${stack:name}-storage"

# Environment-specific configuration
domain: "${stack:name}.myapp.com"

# Stack-specific tags
tags:
  Environment: "${stack:name}"
  Stack: "${stack:name}"
```

**Real-World Usage:**
```yaml
# Environment-specific resource naming
resources:
  app-storage:
    type: s3-bucket
    config:
      name: "mycompany-${stack:name}-storage"
      
stacks:
  staging:
    config:
      domain: "${stack:name}-api.mycompany.com"  # Results in: staging-api.mycompany.com
  production:
    config:
      domain: "api.mycompany.com"
```

### **7. Current User Information** (`user`)

Access information about the current system user running Simple Container.

**Available Properties:**
- `homeDir` or `home` - User's home directory
- `username` - Username
- `id` - User ID
- `name` - Full name

**Syntax:** `${user:property}`

**Examples:**
```yaml
# Local development paths
statePath: "${user:homeDir}/.sc/state"

# User-specific configuration
configPath: "${user:home}/.config/myapp"

# User identification
deployedBy: "${user:username}"
```

**Real-World Usage:**
```yaml
# From local development configuration
provisioner:
  config:
    state-storage:
      type: fs
      config:
        path: "file:///${user:homeDir}/.sc/pulumi/state"

# User-specific local paths
volumes:
  - "${user:home}/.aws:/root/.aws:ro"
  - "${user:home}/.kube:/root/.kube:ro"
```

## **Advanced Template Patterns**

### **Combining Multiple Placeholders**

You can combine multiple placeholders in a single value:

```yaml
# Combine stack name with environment variable
domain: "${stack:name}-${env:COMPANY_DOMAIN:example.com}"

# User and git-based paths
buildPath: "${user:home}/builds/${git:root}"

# Environment-specific authentication
credentials: "${auth:${env:CLOUD_PROVIDER}-${stack:name}}"
```

### **Conditional Configuration with Environment Variables**

```yaml
# Different configurations based on environment
instanceSize: "${env:INSTANCE_SIZE:M10}"
region: "${env:AWS_REGION:us-east-1}"

# Environment-specific secrets
apiKey: "${secret:${env:ENVIRONMENT:staging}-api-key}"
```

### **Cross-Stack References**

```yaml
# Reference configurations from other stacks
parentStack: "${env:PARENT_STACK:default-infrastructure}"
inheritFrom: "${var:parent_stack_name}"
```

## **Template Resolution Order**

Simple Container resolves templates in the following order:

1. **Stack Inheritance Resolution** - Resolve parent stack relationships
2. **Template Processing** - Process all placeholders in each stack
3. **Extension Resolution** - Resolve each extension type:
   - `env` - Environment variables
   - `git` - Git repository information  
   - `auth` - Authentication configurations
   - `secret` - Secrets (with inheritance support)
   - `var` - Stack variables
   - `stack` - Stack metadata
   - `user` - Current user information

## **Best Practices**

### **1. Use Environment Variables for Sensitive Data**
```yaml
# Good - sensitive data from environment
credentials: "${env:AWS_SECRET_ACCESS_KEY}"

# Avoid - hardcoded sensitive data
credentials: "AKIAIOSFODNN7EXAMPLE"
```

### **2. Provide Default Values for Optional Configuration**
```yaml
# Good - provides fallback
region: "${env:AWS_REGION:us-east-1}"
timeout: "${env:TIMEOUT:30}"

# Consider - may fail if not set
region: "${env:AWS_REGION}"
```

### **3. Use Git Root for Relative Paths**
```yaml
# Good - relative to git root
dockerComposeFile: ${git:root}/docker-compose.yaml

# Avoid - absolute paths that may not exist
dockerComposeFile: /home/user/project/docker-compose.yaml
```

### **4. Leverage Stack Names for Environment-Specific Resources**
```yaml
# Good - automatically environment-specific
bucketName: "myapp-${stack:name}-storage"
domain: "${stack:name}.myapp.com"

# Avoid - requires manual changes per environment
bucketName: "myapp-production-storage"
```

### **5. Use Secrets for API Keys and Tokens**
```yaml
# Good - managed through secrets system
apiToken: "${secret:CLOUDFLARE_API_TOKEN}"

# Avoid - environment variables for secrets (less secure)
apiToken: "${env:CLOUDFLARE_API_TOKEN}"
```

## **Error Handling**

When a template placeholder cannot be resolved, Simple Container will:

1. **Environment Variables** - Return empty string if not set (unless default provided)
2. **Git Information** - Return error if not in a git repository
3. **Auth Configurations** - Return error if auth not found
4. **Secrets** - Return error if secret not found (checks inheritance)
5. **Variables** - Return error if variable not defined in stack
6. **Stack Metadata** - Return error if property not available
7. **User Information** - Return error if user information cannot be determined

## **Real-World Examples from Production**

### **Multi-Cloud Parent Stack**
```yaml
# From aiwayz-sc-config production
provisioner:
  config:
    state-storage:
      type: gcp-bucket
      config:
        credentials: "${auth:gcloud}"
        projectId: "${auth:gcloud.projectId}"
        bucketName: aiwayz-sc-state
        location: europe-west3

templates:
  stack-per-app-gke:
    type: gcp-gke-autopilot
    config:
      projectId: "${auth:gcloud.projectId}"
      credentials: "${auth:gcloud}"
      gkeClusterResource: gke-autopilot-res
      artifactRegistryResource: artifact-registry-res
```

### **AWS Multi-Region Setup**
```yaml
# From MyBridge production
templates:
  stack-per-app-eu:
    type: ecs-fargate
    config:
      credentials: "${auth:aws-eu}"
      account: "${auth:aws-eu.projectId}"
  
  lambda-eu:
    type: aws-lambda
    config:
      credentials: "${auth:aws-eu}"
      account: "${auth:aws-eu.projectId}"
```

### **Environment-Specific MongoDB Configuration**
```yaml
# From TalkToMe Tech production
resources:
  staging:
    resources:
      mongodb:
        type: mongodb-atlas
        config:
          publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
          privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
          orgId: 67bc72f86e5ef36f7584d7d0
          instanceSize: "M10"
          region: "EU_CENTRAL_1"
```

### **Local Development Configuration**
```yaml
# Local development with user-specific paths
provisioner:
  config:
    state-storage:
      type: fs
      config:
        path: "file:///${user:homeDir}/.sc/pulumi/state"

# Git-relative paths for consistency
stacks:
  local:
    config:
      dockerComposeFile: ${git:root}/docker-compose.yaml
      buildContext: ${git:root}/src
```

## **See Also**

- [Supported Resources Reference](supported-resources.md) - Complete resource configuration guide
- [Examples Directory](examples/) - Production-tested configuration examples
- [Parent Stack Examples](examples/parent-stacks/) - Multi-region and hybrid cloud configurations
- [Authentication Guide](authentication.md) - Setting up auth configurations for template placeholders

This template system enables Simple Container to provide flexible, reusable, and secure configuration management across different environments and deployment scenarios.
