# Advanced Secrets Management System

## 🔐 Overview

This design document outlines the implementation of an advanced secrets management system for Simple Container that extends beyond the current `secrets.yaml` approach to integrate with enterprise-grade secrets managers and automate SSH key registry management.

## 🎯 Problem Statement

### Current Limitations

**Secrets Management:**
- **Repository-Only Storage**: Secrets are only stored in encrypted `secrets.yaml` files within repositories
- **Manual Management**: All secrets must be manually added and maintained in repository files
- **Limited Integration**: No integration with enterprise secrets management solutions
- **Scalability Issues**: Large teams struggle with centralized secrets distribution and rotation

**SSH Key Management:**
- **Manual Registry**: SSH keys managed through individual `sc secrets allow`/`sc secrets disallow` commands
- **Static Configuration**: No automatic updates when team members join/leave
- **No Central Authority**: Cannot leverage existing GitHub organizations or team memberships
- **Maintenance Overhead**: Requires manual updates for key rotation and team changes

### Enterprise Requirements

**Security & Compliance:**
- Integration with enterprise secrets managers (HashiCorp Vault, AWS Secrets Manager, Azure Key Vault)
- Centralized secrets rotation and access control
- Audit logging for secrets access
- Role-based access control (RBAC) integration

**Operational Efficiency:**
- Automatic SSH key synchronization from GitHub organizations/teams
- Dynamic key registry updates without manual intervention
- Fallback strategies for high availability
- Secret caching for performance optimization

## 🏗️ Solution Architecture

### Context-Aware Hierarchical Secrets Resolution

```mermaid
graph TD
    A[Secret Request: ${secret:name}] --> B[Extract Context: Stack/Env/Org]
    B --> C{Secrets Manager Configured?}
    C -->|Yes| D[Query with Context Path]
    D -->|Found| E[Return Secret Value]
    D -->|Not Found| F[Try Shared Path]
    F -->|Found| E
    F -->|Not Found| G[Query secrets.yaml]
    C -->|No| G[Query secrets.yaml]
    G -->|Found| E
    G -->|Not Found| H[Return Error]
    
    style A fill:#e1f5fe
    style B fill:#fff3e0
    style E fill:#c8e6c9
    style H fill:#ffcdd2
```

### Secrets Manager Integration Points

```yaml
# server.yaml configuration
secrets:
  managers:
    primary:
      type: vault
      config:
        address: https://vault.company.com
        auth:
          type: kubernetes
          role: simple-container-prod
        mount: secret/v2
        path: applications/simple-container
        
    fallback:
      type: aws-secrets-manager
      config:
        region: us-east-1
        prefix: simple-container/
        
  ssh_keys:
    sources:
      - type: github-org
        org: company-name
        teams: [devops, platform-engineering]
        include_admins: true
        
      - type: github-repo
        repo: company-name/ssh-keys
        path: keys/production/
        
      - type: file
        path: /etc/simple-container/authorized_keys
        
      - type: url
        url: https://keys.company.com/api/authorized_keys
        headers:
          Authorization: "Bearer ${secret:keys-api-token}"
        refresh_interval: 1h
```

### Context-Aware Secret Organization

The system automatically organizes secrets based on deployment context, ensuring secure isolation:

**Context Information:**
- **Organization**: Extracted from parent stack (e.g., "yourorg" from "yourorg/infrastructure")
- **Client Stack**: The application stack being deployed (e.g., "production-api")
- **Environment**: The deployment environment (e.g., "production", "staging")
- **Parent Stack**: The infrastructure stack reference

**Secret Path Templates:**

**Vault Example:**
```
secret/yourorg/production-api/production/database-password
secret/yourorg/production-api/staging/database-password
secret/yourorg/shared/production/jwt-secret
```

**AWS Secrets Manager Example:**
```
yourorg/production-api/production/database-password
yourorg/production-api/staging/database-password
yourorg/shared/production/jwt-secret
```

**Key Benefits:**
- **Environment Isolation**: Production secrets cannot be accessed by staging deployments
- **Stack Isolation**: Each application has its own secret namespace
- **Shared Secrets**: Common configurations can be organized under shared paths
- **Audit Clarity**: Clear attribution of which deployment accessed which secrets
- **Security**: Prevents accidental cross-environment or cross-application secret access

## 🔧 Core Components

### 1. Secrets Manager Interface

```go
type SecretsManager interface {
    // Core secrets operations
    GetSecret(ctx context.Context, key string) (*SecretValue, error)
    SetSecret(ctx context.Context, key string, value *SecretValue) error
    DeleteSecret(ctx context.Context, key string) error
    ListSecrets(ctx context.Context, prefix string) ([]string, error)
    
    // Health and connectivity
    HealthCheck(ctx context.Context) error
    Close() error
}

type SecretValue struct {
    Value     string            `json:"value"`
    Metadata  map[string]string `json:"metadata"`
    Version   string            `json:"version"`
    ExpiresAt *time.Time        `json:"expires_at,omitempty"`
}
```

### 2. SSH Key Registry Manager

```go
type SSHKeyRegistry interface {
    // Key management
    GetAuthorizedKeys(ctx context.Context) ([]SSHKey, error)
    RefreshKeys(ctx context.Context) error
    ValidateKey(ctx context.Context, key SSHKey) error
    
    // Source management
    AddSource(source SSHKeySource) error
    RemoveSource(sourceID string) error
    ListSources() []SSHKeySource
}

type SSHKey struct {
    Key         string            `json:"key"`
    Type        string            `json:"type"`        // ssh-rsa, ed25519, etc.
    Comment     string            `json:"comment"`
    Source      string            `json:"source"`      // github-org, file, url
    Metadata    map[string]string `json:"metadata"`
    AddedAt     time.Time         `json:"added_at"`
    ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
    Fingerprint string            `json:"fingerprint"`
}
```

### 3. Unified Secrets Resolver

```go
type SecretsResolver struct {
    managers    []SecretsManager
    fallback    *SecretsYamlStore
    cache       *SecretsCache
    logger      *Logger
    metrics     *MetricsCollector
}

func (sr *SecretsResolver) ResolveSecret(ctx context.Context, key string) (*SecretValue, error) {
    // Try each configured secrets manager in order
    for i, manager := range sr.managers {
        if secret, err := manager.GetSecret(ctx, key); err == nil {
            sr.metrics.RecordSecretResolution(key, fmt.Sprintf("manager-%d", i))
            return secret, nil
        }
    }
    
    // Fallback to secrets.yaml
    if secret, err := sr.fallback.GetSecret(ctx, key); err == nil {
        sr.metrics.RecordSecretResolution(key, "secrets-yaml")
        return secret, nil
    }
    
    return nil, fmt.Errorf("secret %s not found in any configured source", key)
}
```

## 🌐 Supported Secrets Managers

### HashiCorp Vault
- **KV Secrets Engine**: Version 1 and 2 support
- **Authentication**: Token, Kubernetes, AppRole, AWS IAM, GCP IAM
- **Dynamic Secrets**: Database credentials, AWS keys, etc.
- **Secret Rotation**: Automatic credential rotation support

### AWS Secrets Manager
- **Native Integration**: AWS SDK with IAM role-based access
- **Automatic Rotation**: Lambda-based secret rotation
- **Cross-Region Replication**: Multi-region secret availability
- **Resource-Based Policies**: Fine-grained access control

### Azure Key Vault
- **Key and Secret Management**: Certificates, keys, and secrets
- **Managed Identity**: Azure AD integration
- **Hardware Security Modules**: HSM-backed key storage
- **Access Policies**: RBAC and access policy support

### Google Secret Manager
- **IAM Integration**: Google Cloud IAM permissions
- **Versioned Secrets**: Multiple secret versions with rollback
- **Audit Logging**: Cloud Audit Log integration
- **Regional Storage**: Data residency compliance

### Kubernetes Secrets
- **Native K8s Integration**: Direct Kubernetes API access
- **Service Account Auth**: RBAC-based secret access
- **External Secrets Operator**: Integration with external providers
- **Secret Rotation**: Automatic secret updates

## 🔑 SSH Key Registry Sources

### GitHub Organization/Team Integration

```yaml
ssh_keys:
  sources:
    - type: github-org
      org: company-name
      teams: [platform, devops, sre]  # Optional: specific teams
      include_admins: true            # Include org admins
      include_outside_collaborators: false
      api_token: ${secret:github-api-token}
      cache_duration: 30m
      
    - type: github-repo
      repo: company-name/team-ssh-keys
      path: keys/production/          # Optional: subdirectory
      branch: main                    # Optional: specific branch
      file_pattern: "*.pub"           # Optional: key file pattern
      api_token: ${secret:github-repo-token}
```

### File-Based Sources

```yaml
ssh_keys:
  sources:
    - type: file
      path: /etc/simple-container/authorized_keys
      format: openssh                 # openssh, authorized_keys
      watch: true                     # Auto-reload on file changes
      
    - type: directory
      path: /etc/simple-container/keys/
      pattern: "*.pub"
      recursive: true
      watch: true
```

### URL-Based Sources

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

## 🔒 Security Considerations

### Secrets Manager Security
- **Encryption in Transit**: TLS 1.2+ for all external communications
- **Authentication**: Strong authentication mechanisms (mTLS, IAM roles, service accounts)
- **Authorization**: Role-based access control and least privilege principles
- **Audit Logging**: Comprehensive logging of all secrets operations
- **Secret Rotation**: Support for automatic secret rotation workflows

### SSH Key Security
- **Key Validation**: Cryptographic validation of all SSH keys
- **Source Verification**: Verification of key sources and authenticity
- **Access Logging**: Audit trail for SSH key additions/removals
- **Expiration Handling**: Support for time-limited SSH keys
- **Key Revocation**: Immediate key removal upon source changes

### Network Security
- **Private Networks**: Support for VPC/VNET private endpoints
- **Certificate Pinning**: SSL certificate validation for external sources
- **Rate Limiting**: Protection against API abuse and DoS attacks
- **Failure Isolation**: Circuit breakers for unhealthy external services

## 📊 Performance & Reliability

### Caching Strategy

```go
type SecretsCache struct {
    // L1: In-memory cache with TTL
    memoryCache *cache.LRUCache
    
    // L2: Encrypted disk cache for persistence
    diskCache *cache.DiskCache
    
    // Configuration
    defaultTTL     time.Duration
    maxSize        int
    encryptionKey  []byte
}
```

### High Availability
- **Multiple Secrets Managers**: Primary/fallback configuration
- **Circuit Breakers**: Fail fast on unhealthy services
- **Retry Logic**: Exponential backoff with jitter
- **Graceful Degradation**: Fallback to cached values during outages

### Monitoring & Observability
- **Metrics Collection**: Response times, success rates, cache hit ratios
- **Health Checks**: Regular connectivity and authentication validation
- **Alerting**: Notifications for secrets manager failures and SSH key source issues
- **Distributed Tracing**: End-to-end request tracing for debugging

## 🚀 Benefits

### For Organizations
- **Centralized Security**: Leverage existing enterprise secrets infrastructure
- **Compliance Ready**: Audit trails and access controls for SOC2/ISO27001
- **Operational Efficiency**: Automated SSH key management reduces manual overhead
- **Scalability**: Supports large teams with complex access requirements

### For Development Teams
- **Simplified Workflows**: Transparent secret resolution without workflow changes
- **Better Security**: Enterprise-grade secrets management without complexity
- **Automatic Updates**: SSH keys automatically sync with team changes
- **Reduced Friction**: Less manual secret and key management

### For Security Teams
- **Audit Visibility**: Comprehensive logging of all secrets access
- **Policy Enforcement**: Centralized access control and rotation policies
- **Incident Response**: Quick revocation and rotation capabilities
- **Compliance Support**: Built-in support for security frameworks and standards

---

**Status**: This advanced secrets management system will transform Simple Container from a repository-centric secrets approach to an enterprise-ready solution that integrates with existing security infrastructure while maintaining simplicity for development teams.
