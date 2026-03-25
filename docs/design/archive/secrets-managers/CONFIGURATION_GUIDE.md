# Configuration Guide: Advanced Secrets Management System

## 📋 Overview

This guide provides practical configuration examples for implementing the Advanced Secrets Management System in Simple Container. All examples use real Simple Container patterns and are designed to work with the existing `.sc/stacks/<stack-name>/` directory structure.

## 🎯 Context-Aware Secret Organization

The advanced secrets management system organizes secrets based on deployment context, ensuring that each stack and environment can only access its appropriate secrets.

### Secret Context Resolution

During deployment, Simple Container provides context information to external secrets managers:

```go
type SecretContext struct {
    ParentStack     string  // e.g., "yourorg/infrastructure" 
    ClientStack     string  // e.g., "production-api"
    Environment     string  // e.g., "production"
    Organization    string  // e.g., "yourorg"
}
```

### Vault Secret Organization

With context-aware paths, secrets are organized as:
```
secret/yourorg/production-api/production/database-password
secret/yourorg/production-api/staging/database-password
secret/yourorg/shared/production/jwt-secret
secret/yourorg/worker-app/production/queue-config
```

### AWS Secrets Manager Organization

Secrets are named using the context template:
```
yourorg/production-api/production/database-password
yourorg/production-api/staging/database-password  
yourorg/shared/production/jwt-secret
yourorg/worker-app/production/queue-config
```

### Benefits of Context-Aware Organization

- **Environment Isolation**: Production secrets are completely separate from staging
- **Stack Isolation**: Each application stack has its own secret namespace
- **Shared Secrets**: Common secrets can be organized under shared paths
- **Security**: No accidental access to secrets from other environments or stacks
- **Audit Trail**: Clear attribution of which deployment accessed which secrets

## 🔧 Basic Configuration Structure

### Server Configuration (server.yaml)

The advanced secrets management system is configured in your `server.yaml` file:

```yaml
# server.yaml
schemaVersion: 1.0

secrets:
  # Configure multiple secrets managers with fallback hierarchy
  managers:
    - type: vault
      priority: 1
      config:
        address: https://vault.company.com
        namespace: simple-container  # Vault Enterprise namespace
        auth:
          type: kubernetes
          role: simple-container-prod
        mount: secret/v2
        path: applications/simple-container
        
    - type: aws-secrets-manager
      priority: 2
      config:
        region: us-east-1
        prefix: simple-container/
        
  # SSH key registry configuration
  ssh_keys:
    sources:
      - type: github-org
        org: your-company
        teams: [platform, devops, sre]
        include_admins: true
        api_token: ${secret:github-api-token}
        cache_duration: 30m
        
      - type: file
        path: /etc/simple-container/authorized_keys
        watch: true
        
    refresh_interval: 1h
    
  # Performance and security settings
  cache:
    memory:
      max_size: 100MB
      default_ttl: 15m
    disk:
      enabled: true
      max_size: 500MB
      encryption: true
      
  security:
    request_timeout: 30s
    max_retries: 3
    audit_logging: true
```

## 🏦 Secrets Manager Configurations

### HashiCorp Vault Integration

#### Kubernetes Authentication (Recommended for K8s deployments)
```yaml
secrets:
  managers:
    - type: vault
      priority: 1
      config:
        address: https://vault.company.com
        namespace: production  # Vault Enterprise namespace
        auth:
          type: kubernetes
          role: simple-container-prod
          service_account: simple-container
          token_path: /var/run/secrets/kubernetes.io/serviceaccount/token
        mount: secret/v2
        # Context-aware path template using stack and environment information
        path_template: "{{.Organization}}/{{.ClientStack}}/{{.Environment}}/{{.Key}}"
        tls:
          verify: true
          ca_cert: /etc/ssl/certs/vault-ca.pem
```

#### Token Authentication (Development environments)
```yaml
secrets:
  managers:
    - type: vault
      priority: 1
      config:
        address: https://vault-dev.company.com
        auth:
          type: token
          token: ${secret:vault-dev-token}
        mount: secret/v1  # KV v1 engine
        path: dev/simple-container
```

#### AppRole Authentication (VM/Container deployments)
```yaml
secrets:
  managers:
    - type: vault
      priority: 1
      config:
        address: https://vault.company.com
        auth:
          type: approle
          role_id: ${secret:vault-role-id}
          secret_id: ${secret:vault-secret-id}
        mount: secret/v2
        path: applications/simple-container
```

### AWS Secrets Manager Integration

#### IAM Role-based Authentication (Recommended for AWS)
```yaml
secrets:
  managers:
    - type: aws-secrets-manager
      priority: 1
      config:
        region: us-east-1
        # Context-aware secret naming template
        secret_name_template: "{{.Organization}}/{{.ClientStack}}/{{.Environment}}/{{.Key}}"
        # Uses IAM role attached to ECS task/EC2 instance
        request_timeout: 10s
        max_retries: 3
```

#### Cross-Account Access with AssumeRole
```yaml
secrets:
  managers:
    - type: aws-secrets-manager
      priority: 1
      config:
        region: us-east-1
        prefix: simple-container/
        role_arn: arn:aws:iam::123456789012:role/simple-container-secrets-role
        external_id: simple-container-external-id
        kms_key_id: arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012
```

#### Multi-Region Configuration
```yaml
secrets:
  managers:
    - type: aws-secrets-manager
      priority: 1
      config:
        region: us-east-1
        prefix: simple-container/
    - type: aws-secrets-manager
      priority: 2
      config:
        region: eu-west-1
        prefix: simple-container/
```

### Azure Key Vault Integration

#### Managed Identity Authentication (Recommended for Azure)
```yaml
secrets:
  managers:
    - type: azure-keyvault
      priority: 1
      config:
        vault_url: https://your-keyvault.vault.azure.net/
        auth:
          type: managed_identity
          client_id: 12345678-1234-1234-1234-123456789012
        prefix: simple-container-
```

#### Service Principal Authentication
```yaml
secrets:
  managers:
    - type: azure-keyvault
      priority: 1
      config:
        vault_url: https://your-keyvault.vault.azure.net/
        auth:
          type: service_principal
          tenant_id: ${secret:azure-tenant-id}
          client_id: ${secret:azure-client-id}
          client_secret: ${secret:azure-client-secret}
        prefix: simple-container-
```

### Google Cloud Secret Manager Integration

#### Workload Identity Authentication (Recommended for GKE)
```yaml
secrets:
  managers:
    - type: gcp-secret-manager
      priority: 1
      config:
        project_id: your-project-123
        auth:
          type: workload_identity
          service_account: simple-container@your-project-123.iam.gserviceaccount.com
        prefix: simple-container-
```

#### Service Account Key Authentication
```yaml
secrets:
  managers:
    - type: gcp-secret-manager
      priority: 1
      config:
        project_id: your-project-123
        auth:
          type: service_account
          credentials: ${secret:gcp-service-account-key}
        prefix: simple-container-
        regions: [us-central1, europe-west1]
```

### Kubernetes Secrets Integration

#### In-Cluster Authentication
```yaml
secrets:
  managers:
    - type: kubernetes
      priority: 1
      config:
        auth:
          type: in_cluster  # Uses service account token
        namespace: simple-container
        prefix: simple-container-
```

#### External Cluster Authentication
```yaml
secrets:
  managers:
    - type: kubernetes
      priority: 1
      config:
        auth:
          type: kubeconfig
          kubeconfig: ${secret:external-cluster-kubeconfig}
        namespace: simple-container
        prefix: simple-container-
```

## 🔑 SSH Key Registry Configurations

### GitHub Organization Integration

#### Basic Organization Setup
```yaml
ssh_keys:
  sources:
    - type: github-org
      org: your-company
      api_token: ${secret:github-api-token}
      cache_duration: 30m
      include_admins: true
      include_outside_collaborators: false
```

#### Team-Specific Access
```yaml
ssh_keys:
  sources:
    - type: github-org
      org: your-company
      teams: [platform-engineering, devops, sre]
      api_token: ${secret:github-api-token}
      cache_duration: 30m
```

#### GitHub Enterprise Server
```yaml
ssh_keys:
  sources:
    - type: github-org
      org: your-company
      base_url: https://github.company.com
      api_token: ${secret:github-enterprise-token}
      teams: [platform, devops]
      cache_duration: 1h
```

### GitHub Repository-Based Keys

#### Public Repository
```yaml
ssh_keys:
  sources:
    - type: github-repo
      repo: your-company/team-ssh-keys
      path: keys/production/
      branch: main
      file_pattern: "*.pub"
      api_token: ${secret:github-api-token}
```

#### Private Repository with Subdirectories
```yaml
ssh_keys:
  sources:
    - type: github-repo
      repo: your-company/infrastructure-keys
      path: ssh-keys/
      branch: main
      recursive: true
      file_pattern: "*.pub"
      api_token: ${secret:github-private-repo-token}
```

### File-Based SSH Key Sources

#### Single Authorized Keys File
```yaml
ssh_keys:
  sources:
    - type: file
      path: /etc/simple-container/authorized_keys
      format: openssh
      watch: true  # Monitor file changes
```

#### Directory of Key Files
```yaml
ssh_keys:
  sources:
    - type: directory
      path: /etc/simple-container/keys/
      pattern: "*.pub"
      recursive: true
      watch: true
```

### URL-Based SSH Key Sources

#### HTTP API Endpoint
```yaml
ssh_keys:
  sources:
    - type: url
      url: https://keys.company.com/api/authorized_keys
      method: GET
      headers:
        Authorization: "Bearer ${secret:keys-api-token}"
        User-Agent: "simple-container/1.0"
      format: openssh
      refresh_interval: 1h
      timeout: 30s
      verify_ssl: true
```

#### Internal Service with Custom Authentication
```yaml
ssh_keys:
  sources:
    - type: url
      url: https://internal-keys.company.com/keys
      method: GET
      headers:
        X-API-Key: ${secret:internal-keys-api-key}
        X-Service: simple-container
      format: openssh
      refresh_interval: 30m
```

## 📁 Stack Configuration Examples

### Production Stack with Vault Integration

**File: `.sc/stacks/production-api/secrets.yaml`**
```yaml
schemaVersion: 1.0

# Authentication for cloud providers
auth:
  aws:
    type: aws-token
    config:
      account: "123456789012"
      accessKey: "AKIA..."
      secretAccessKey: "wJal..."
      region: us-east-1

# Secret values (actual values, not placeholders)
values:
  # Database credentials
  database-host: "prod-db.company.com"
  database-username: "api_user"
  database-password: "secure-prod-password-123"
  
  # API keys  
  stripe-api-key: "sk_live_..."
  sendgrid-api-key: "SG...."
  
  # JWT secrets
  jwt-secret: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
  
  # External service tokens
  monitoring-token: "mon_live_..."
```

**File: `.sc/stacks/production-api/client.yaml`**
```yaml
schemaVersion: 1.0

stacks:
  production:
    type: single-image
    parent: your-org/infrastructure
    config:
      template: ecs-fargate
      maxMemory: 2048
      
      # Environment variables (non-sensitive)
      env:
        NODE_ENV: production
        LOG_LEVEL: info
        
      # Secret references (using ${secret:name} placeholders)
      secrets:
        DATABASE_URL: "postgres://${secret:database-username}:${secret:database-password}@${secret:database-host}:5432/api"
        STRIPE_API_KEY: "${secret:stripe-api-key}"
        SENDGRID_API_KEY: "${secret:sendgrid-api-key}"
        JWT_SECRET: "${secret:jwt-secret}"
        MONITORING_TOKEN: "${secret:monitoring-token}"
        
      uses:
        - postgresql
        - redis
```

### Multi-Environment Configuration

**File: `.sc/stacks/staging-api/client.yaml`**
```yaml
schemaVersion: 1.0

stacks:
  staging:
    type: single-image
    parent: your-org/infrastructure
    config:
      template: ecs-fargate
      maxMemory: 1024
      
      env:
        NODE_ENV: staging
        LOG_LEVEL: debug
        
      secrets:
        DATABASE_URL: "postgres://${secret:staging-db-username}:${secret:staging-db-password}@${secret:staging-db-host}:5432/api"
        STRIPE_API_KEY: "${secret:stripe-test-api-key}"
        JWT_SECRET: "${secret:staging-jwt-secret}"
        
      uses:
        - postgresql
```

### Kubernetes Deployment with External Secrets

**File: `.sc/stacks/k8s-app/secrets.yaml`**
```yaml
schemaVersion: 1.0

auth:
  kubernetes:
    type: kubernetes
    config:
      kubeconfig: |
        apiVersion: v1
        clusters:
          - cluster:
              certificate-authority-data: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0t..."
              server: https://k8s-api.company.com
            name: production-cluster
        contexts:
          - context:
              cluster: production-cluster
              user: admin
            name: production
        current-context: production
        users:
          - name: admin
            user:
              token: "eyJhbGciOiJSUzI1NiIsImtpZCI6I..."

values:
  # Kubernetes service account token
  k8s-admin-token: "eyJhbGciOiJSUzI1NiIsImtpZCI6I..."
  
  # CA certificate data
  k8s-ca-cert: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0t..."
  
  # Application secrets
  app-database-password: "k8s-secure-password"
  app-redis-password: "redis-secure-password"
```

## 🔄 Migration from Current System

### Step 1: Enable Advanced Secrets Management

**Add to server.yaml:**
```yaml
secrets:
  # Start with fallback to existing secrets.yaml
  managers: []  # Empty initially
  
  # Enable SSH key registry
  ssh_keys:
    sources:
      - type: github-org
        org: your-company
        api_token: ${secret:github-api-token}
```

### Step 2: Configure External Secrets Manager

**Add Vault integration:**
```yaml
secrets:
  managers:
    - type: vault
      priority: 1
      config:
        address: https://vault.company.com
        auth:
          type: kubernetes
          role: simple-container-prod
        mount: secret/v2
        path: applications/simple-container
  
  ssh_keys:
    sources:
      - type: github-org
        org: your-company
        api_token: ${secret:github-api-token}
```

### Step 3: Migrate Secrets to External Manager

**Store secrets in Vault:**
```bash
# Store secrets in Vault (example commands)
vault kv put secret/applications/simple-container/production \
  database-password="secure-prod-password" \
  jwt-secret="your-jwt-secret" \
  stripe-api-key="sk_live_..."

vault kv put secret/applications/simple-container/staging \
  database-password="staging-password" \
  jwt-secret="staging-jwt-secret" \
  stripe-api-key="sk_test_..."
```

### Step 4: Update Client Configuration

**No changes needed in client.yaml** - existing `${secret:name}` references work automatically:
```yaml
# This continues to work unchanged
secrets:
  DATABASE_PASSWORD: "${secret:database-password}"
  JWT_SECRET: "${secret:jwt-secret}"
  STRIPE_API_KEY: "${secret:stripe-api-key}"
```

## 🔍 Troubleshooting Configurations

### Validation Commands

**Test secrets manager connectivity:**
```bash
# Check Vault connectivity
vault status

# Test AWS credentials
aws sts get-caller-identity

# Verify GCP authentication
gcloud auth list

# Test Kubernetes access
kubectl cluster-info
```

**Validate Simple Container integration:**
```bash
# List available secrets (should show secrets from all sources)
sc secrets list

# Test secret resolution
sc secrets reveal --verbose

# Verify SSH key registry
sc secrets allowed-keys
```

### Common Configuration Issues

#### Vault Authentication Issues
```yaml
# Issue: Token expired or invalid role
secrets:
  managers:
    - type: vault
      config:
        auth:
          type: kubernetes
          role: simple-container-prod  # Ensure this role exists in Vault
          service_account: simple-container  # Must match K8s service account
```

#### AWS Permissions Issues
```yaml
# Issue: Insufficient IAM permissions
secrets:
  managers:
    - type: aws-secrets-manager
      config:
        region: us-east-1
        # Ensure IAM role has secretsmanager:GetSecretValue permission
        # for secrets with prefix simple-container/*
```

#### SSH Key Source Issues
```yaml
# Issue: GitHub API rate limiting
ssh_keys:
  sources:
    - type: github-org
      org: your-company
      api_token: ${secret:github-api-token}  # Use personal access token
      cache_duration: 1h  # Increase cache duration
```

## 📊 Monitoring and Observability

### Metrics Configuration

**Enable comprehensive monitoring:**
```yaml
secrets:
  observability:
    metrics:
      enabled: true
      endpoint: /metrics
      labels:
        service: simple-container
        environment: production
        
    logging:
      level: info
      format: json
      audit: true
      
    tracing:
      enabled: true
      endpoint: https://jaeger.company.com
      sample_rate: 0.1
```

### Health Check Configuration

**Configure health endpoints:**
```yaml
secrets:
  health_checks:
    enabled: true
    endpoint: /health
    checks:
      - name: vault-connectivity
        type: secrets_manager
        manager: vault
        interval: 30s
        
      - name: aws-secrets-connectivity
        type: secrets_manager
        manager: aws-secrets-manager
        interval: 60s
        
      - name: ssh-key-sources
        type: ssh_keys
        interval: 5m
```

---

**Status**: This configuration guide provides comprehensive, production-ready examples for implementing the Advanced Secrets Management System with all supported secrets managers and SSH key sources, following Simple Container's established patterns and architecture.
