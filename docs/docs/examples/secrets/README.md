# Simple Container Secrets Examples

This directory contains comprehensive examples of secrets configuration for different deployment scenarios and cloud provider integrations. Each example demonstrates best practices for managing authentication credentials, API keys, and sensitive configuration data.

## Available Examples

### 🔐 [AWS + MongoDB Atlas](./aws-mongodb-atlas/)
**Use Case**: Multi-region AWS deployment with database and CI/CD integrations
- **Authentication**: AWS multi-region credentials, Pulumi tokens
- **Services**: MongoDB Atlas, Cloudflare DNS, CI/CD webhooks
- **Best For**: AWS-centric deployments with external database and notification needs

### 🌐 [GCP Multi-Service Integration](./gcp-auth-cloudflare-mongodb-discord-telegram/)  
**Use Case**: Comprehensive GCP setup with multiple third-party service integrations
- **Authentication**: GCP service accounts (staging + production)
- **Services**: MongoDB Atlas, Cloudflare, Discord, Telegram
- **Best For**: GCP-primary deployments with rich integration ecosystem

### ☸️ [Kubernetes + GCP Hybrid](./kube-and-gcp-auth/)
**Use Case**: Kubernetes cluster management with GCP cloud services
- **Authentication**: Kubernetes cluster access, GCP service accounts
- **Services**: Docker registry, container orchestration
- **Best For**: Hybrid cloud-native and containerized workloads

## Quick Selection Guide

### Choose by Primary Infrastructure

| Infrastructure       | Recommended Example                                                                             | Key Features                                      |
|----------------------|-------------------------------------------------------------------------------------------------|---------------------------------------------------|
| **AWS Focused**      | [aws-mongodb-atlas](./aws-mongodb-atlas/)                                                       | Multi-region AWS, Pulumi IaC, external services   |
| **GCP Focused**      | [gcp-auth-cloudflare-mongodb-discord-telegram](./gcp-auth-cloudflare-mongodb-discord-telegram/) | Multi-environment GCP, comprehensive integrations |
| **Kubernetes First** | [kube-and-gcp-auth](./kube-and-gcp-auth/)                                                       | Container orchestration, hybrid deployments       |

### Choose by Integration Needs

| Integration Type        | AWS Example      | GCP Example         | Kubernetes Example     |
|-------------------------|------------------|---------------------|------------------------|
| **Database**            | MongoDB Atlas ✅  | MongoDB Atlas ✅     | Registry Auth ✅        |
| **DNS Management**      | Cloudflare ✅     | Cloudflare ✅        | -                      |
| **CI/CD Notifications** | Discord, Slack ✅ | Discord, Telegram ✅ | -                      |
| **Container Registry**  | -                | -                   | Docker Hub, GCR, ECR ✅ |
| **Multi-Region**        | US + EU ✅        | Staging + Prod ✅    | Multi-Cluster ✅        |
| **IaC Integration**     | Pulumi ✅         | GCP Native ✅        | Kubernetes Native ✅    |

## Common Configuration Patterns

### Authentication Structure
All examples follow this consistent structure:

```yaml
schemaVersion: 1.0
auth:
  # Cloud provider authentication
  provider-name:
    type: provider-type
    config:
      # Provider-specific configuration
      
values:
  # Application secrets and API keys
  SECRET_NAME: base64-encoded-value
```

### Supported Authentication Types

| Type                  | Description                   | Used In                  |
|-----------------------|-------------------------------|--------------------------|
| `aws-token`           | AWS access key and secret     | AWS Example              |
| `gcp-service-account` | GCP service account JSON      | GCP, Kubernetes Examples |
| `kubernetes`          | Kubeconfig for cluster access | Kubernetes Example       |
| `pulumi-token`        | Pulumi service access token   | AWS Example              |

### Secret Value Patterns

| Pattern                | Purpose          | Example                                                                |
|------------------------|------------------|------------------------------------------------------------------------|
| `base64-encoded-value` | API tokens, keys | `CLOUDFLARE_API_TOKEN: F0ar1ywR4W2rb1DWJfk0HQ==`                       |
| `direct-value`         | UUIDs, IDs       | `MONGODB_ATLAS_PRIVATE_KEY: 4a6dc8ee-1106-48c7-8bfd-87103f0465ba`      |
| `webhook-url`          | Service webhooks | `cicd-bot-discord-webhook-url: "https://discord.com/api/webhooks/..."` |

## Security Best Practices

### ✅ Essential Security Measures

#### 1. **Encryption at Rest**
```bash
# Initialize secrets management
sc secrets init

# Add and encrypt secrets files
sc secrets add secrets.yaml

# Hide (encrypt) all secrets in repository
sc secrets hide

# Reveal secrets for local development
sc secrets reveal
```

#### 2. **Environment Separation**
```
.sc/
├── stacks/
│   ├── production-app/
│   │   └── secrets.yaml      # Production secrets for this stack
│   └── staging-app/
│       └── secrets.yaml      # Staging secrets for this stack
```

#### 3. **Access Control**
```bash
# Set restrictive permissions
chmod 600 secrets.yaml
chown $USER:$USER secrets.yaml

# Use git-crypt for encrypted version control (optional)
git-crypt init
echo "*.yaml.encrypted filter=git-crypt diff=git-crypt" >> .gitattributes
```

#### 4. **Regular Rotation**
```bash
# Rotate secrets regularly (quarterly recommended)
# 1. Generate new credentials
# 2. Update secrets.yaml
# 3. Deploy with new secrets  
# 4. Validate service functionality
# 5. Revoke old credentials
```

### ❌ Security Anti-Patterns to Avoid

- **Never commit plaintext secrets** to version control
- **Don't share production secrets** across environments
- **Avoid broad permissions** on cloud service accounts  
- **Don't hardcode secrets** in application code or Dockerfiles
- **Never use default/admin accounts** for automated deployments

## Getting Started

### 1. Choose Your Example
Based on your infrastructure and integration needs, select the most appropriate example from the table above.

### 2. Copy and Customize
```bash
# Copy the example directory
cp -r docs/examples/secrets/your-chosen-example .sc/

# Rename for clarity
mv .sc/your-chosen-example .sc/secrets
```

### 3. Update Configuration
1. **Replace placeholder values** with your actual credentials
2. **Base64 encode sensitive values** where indicated
3. **Update project IDs, account numbers, etc.** to match your setup
4. **Test authentication** before full deployment

### 4. Setup and Deploy
```bash
# Add the secrets file to the standardized location
sc secrets add .sc/stacks/your-stack/secrets.yaml

# Hide (encrypt) all secrets
sc secrets hide

# Deploy to production (secrets are automatically used)
sc deploy -s your-stack -e production
```

## Integration with Client Configuration

All secrets examples integrate seamlessly with Simple Container client configurations:

### Server Configuration (Infrastructure Secrets)

Infrastructure authentication belongs in `server.yaml`:

```yaml
# server.yaml - Infrastructure authentication and resource management
schemaVersion: 1.0
provisioner:
  type: pulumi
  config:
    state-storage:
      type: s3-bucket
      config:
        credentials: "${auth:aws}"
        provision: false
        account: "${auth:aws.projectId}"
        bucketName: example-company-sc-state
    secrets-provider:
      type: aws-kms
      config:
        credentials: "${auth:aws}"
        provision: true
        keyName: simple-container-kms-key

templates:
  app-template:
    type: ecs-fargate
    config:
      credentials: "${auth:aws}"
      account: "${auth:aws.projectId}"

resources:
  resources:
    staging:
      template: app-template
      resources:
        mongodb:
          type: mongodb-atlas
          config:
            privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
            publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
            instanceSize: "M10"
            
    production:
      template: app-template
      resources:
        mongodb:
          type: mongodb-atlas
          config:
            privateKey: "${secret:PROD_MONGODB_ATLAS_PRIVATE_KEY}"
            publicKey: "${secret:PROD_MONGODB_ATLAS_PUBLIC_KEY}"
            instanceSize: "M30"
```

### Client Configuration (Application Secrets)

Application secrets belong in `client.yaml` only for direct application needs:

```yaml
# client.yaml - Application secrets (minimal, most handled by server.yaml)
schemaVersion: 1.0
stacks:
  staging:
    type: single-image
    parent: mycompany/staging-infrastructure
    config:
      domain: staging-app.example.com
      uses:
        - mongodb      # Resource provisioned by server.yaml
      env:
        NODE_ENV: staging
        # Database connection provided by ${resource:mongodb.uri}
      secrets:
        # Application-level secrets only
        API_KEY: ${secret:YOUR_API_KEY}
        
  production:
    type: single-image
    parent: mycompany/production-infrastructure
    config:
      domain: app.example.com
      uses:
        - mongodb      # Resource provisioned by server.yaml
      env:
        NODE_ENV: production
        # Database connection provided by ${resource:mongodb.uri}
      secrets:
        # Production application secrets
        API_KEY: ${secret:PROD_API_KEY}
```

## Advanced Topics

### Multi-Environment Management
- Use separate secrets files for different environments
- Implement environment-specific secret naming conventions
- Configure deployment pipelines with appropriate secret references

### Secret Rotation Strategies
- Implement automated secret rotation using cloud provider tools
- Use versioned secrets for zero-downtime rotations
- Monitor secret age and set up rotation alerts

### Compliance and Auditing
- Enable audit logging for secret access
- Implement secrets scanning in CI/CD pipelines
- Document secret access patterns for compliance reviews

### Integration Testing
- Create test scripts to validate secret functionality
- Implement health checks that verify service connectivity
- Set up monitoring for secret-dependent services

## Troubleshooting

### Common Issues

#### Authentication Failures
```bash
# List available secrets
sc secrets list

# Reveal secrets to verify they exist
sc secrets reveal

# Test cloud provider authentication directly
# AWS example:
aws sts get-caller-identity

# GCP example:
gcloud auth list
gcloud projects list

# Kubernetes example:
kubectl cluster-info
kubectl get nodes
```

#### Secret Access Problems
```bash
# List all secrets in your project
sc secrets list

# Reveal encrypted secrets (requires your private key)
sc secrets reveal --verbose

# Check if secrets file exists and has correct permissions
ls -la .sc/secrets.yaml
ls -la .sc/secrets/

# Verify your SSH key is properly configured
ssh-add -l
cat ~/.ssh/id_rsa.pub
```

#### Deployment Issues
```bash
# Check file structure before deployment
ls -la .sc/stacks/

# Deploy with verbose output to see errors
sc deploy -s your-stack -e staging

# Test deployment without actual deployment (provision only)
sc provision -s infrastructure

# Check if required secrets are available
sc secrets list
```

#### Permission and Access Issues
```bash
# Check file permissions on secrets
ls -la .sc/secrets.yaml
chmod 600 .sc/secrets.yaml  # Fix if needed

# Verify cloud provider access with native CLI tools
# AWS:
aws configure list
aws sts get-caller-identity

# GCP: 
gcloud auth list
gcloud config list

# Kubernetes:
kubectl config current-context
kubectl auth can-i create deployments
```

## Related Documentation

- **[Simple Container CLI Reference](../../reference/cli.md)** - Complete command documentation
- **[Client Configuration Guide](../../guides/client-configuration.md)** - Application setup
- **[Server Configuration Guide](../../guides/server-configuration.md)** - Infrastructure setup
- **[Security Best Practices](../../guides/security.md)** - Comprehensive security guidance

## Contributing

To add a new secrets example:

1. **Create a new directory** following the naming convention
2. **Include a secrets.yaml** with comprehensive examples
3. **Write a detailed README.md** following the existing pattern
4. **Add integration examples** showing client.yaml usage
5. **Include testing and validation scripts**
6. **Update this overview README** with the new example

Each example should demonstrate real-world patterns while maintaining security best practices and clear documentation for users at all experience levels.
