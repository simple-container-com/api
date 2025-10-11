# AWS + MongoDB Atlas Secrets Example

This example demonstrates how to configure Simple Container secrets for AWS multi-region deployment with MongoDB Atlas and CI/CD integrations.

## What This Example Shows

- **AWS Multi-Region Authentication**: Configure AWS credentials for multiple regions (EU and US)
- **Pulumi Integration**: Pulumi access token for infrastructure management
- **MongoDB Atlas API Keys**: Public and private keys for database management
- **Third-Party Services**: Cloudflare API token for DNS management
- **CI/CD Webhooks**: Discord and Slack webhook URLs for deployment notifications

## Configuration Structure

### Authentication Providers

```yaml
auth:
  aws-eu:          # AWS credentials for Europe region
    type: aws-token
    config:
      account: your-aws-account-id
      accessKey: base64-encoded-access-key
      secretAccessKey: base64-encoded-secret-key
      region: eu-central-1
      
  aws-us:          # AWS credentials for US region
    type: aws-token
    config:
      account: your-aws-account-id
      accessKey: base64-encoded-access-key
      secretAccessKey: base64-encoded-secret-key
      region: us-west-2
      
  pulumi:          # Pulumi service token
    type: pulumi-token
    config:
      credentials: your-pulumi-access-token
```

### Application Secrets

```yaml
values:
  # Cloudflare DNS management
  CLOUDFLARE_API_TOKEN: base64-encoded-token
  
  # MongoDB Atlas database credentials
  MONGODB_ATLAS_PUBLIC_KEY: your-public-key
  MONGODB_ATLAS_PRIVATE_KEY: your-private-key
  
  # CI/CD notification webhooks
  cicd-bot-discord-webhook-url: "https://discord.com/api/webhooks/..."
  cicd-bot-slack-webhook-url: "https://hooks.slack.com/services/..."
```

## How to Customize

### 1. AWS Credentials
Replace the example credentials with your actual AWS access keys:
```bash
# Get your AWS credentials
aws configure list

# Base64 encode them for secrets.yaml
echo -n "your-access-key" | base64
echo -n "your-secret-key" | base64
```

### 2. MongoDB Atlas Setup
1. Log into [MongoDB Atlas](https://cloud.mongodb.com/)
2. Go to **Access Manager** → **API Keys**
3. Create new API key with appropriate permissions
4. Use the public key directly and private key as-is

### 3. Pulumi Token
1. Go to [Pulumi Console](https://app.pulumi.com/)
2. Navigate to **Settings** → **Access Tokens**
3. Create new token and copy the value

### 4. Cloudflare API Token
1. Go to [Cloudflare Dashboard](https://dash.cloudflare.com/profile/api-tokens)
2. Create token with **Zone:Read** and **DNS:Edit** permissions
3. Base64 encode the token

### 5. CI/CD Webhooks
Configure webhook URLs for your notification services:
- **Discord**: Server Settings → Integrations → Webhooks
- **Slack**: App Settings → Incoming Webhooks

## Usage in Configuration Files

### Server Configuration (Infrastructure Secrets)

Infrastructure secrets belong in `server.yaml` for cloud provider and infrastructure management:

```yaml
# server.yaml - Infrastructure authentication and resource management
schemaVersion: 1.0
provisioner:
  type: pulumi
  config:
    state-storage:
      type: s3-bucket
      config:
        credentials: "${auth:aws-us}"
        provision: false
        account: "${auth:aws-us.projectId}"
        bucketName: example-company-sc-state
    secrets-provider:
      type: aws-kms
      config:
        credentials: "${auth:aws-us}"
        provision: true
        keyName: simple-container-kms-key

templates:
  ecs-fargate-us:
    type: ecs-fargate
    config:
      credentials: "${auth:aws-us}"
      account: "${auth:aws-us.projectId}"
  ecs-fargate-eu:
    type: ecs-fargate
    config:
      credentials: "${auth:aws-eu}"
      account: "${auth:aws-eu.projectId}"

resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: your-cloudflare-account-id
      zoneName: example.com

  resources:
    staging:
      template: ecs-fargate-us
      resources:
        mongodb:
          type: mongodb-atlas
          config:
            privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
            publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
            instanceSize: "M10"
            region: "US_EAST_1"
            
    production:
      template: ecs-fargate-eu
      resources:
        mongodb:
          type: mongodb-atlas
          config:
            privateKey: "${secret:PROD_MONGODB_ATLAS_PRIVATE_KEY}"
            publicKey: "${secret:PROD_MONGODB_ATLAS_PUBLIC_KEY}"
            instanceSize: "M30"
            region: "EU_CENTRAL_1"
```

### Client Configuration (Application Secrets)

Application secrets belong in `client.yaml` only for direct application needs:

```yaml
# client.yaml - Application secrets (minimal, most handled by server.yaml)
schemaVersion: 1.0
stacks:
  staging:
    type: cloud-compose
    parent: mycompany/staging-infrastructure
    config:
      domain: staging-app.example.com
      size:
        cpu: 1024
        memory: 2048
      uses:
        - mongodb      # Resource provisioned by server.yaml
      runs:
        - web-service
      env:
        NODE_ENV: staging
        # Database connection provided by ${resource:mongodb.uri}
      secrets:
        # Application-level secrets only (not infrastructure secrets)
        DISCORD_WEBHOOK: ${secret:cicd-bot-discord-webhook-url}
        SLACK_WEBHOOK: ${secret:cicd-bot-slack-webhook-url}

  production:
    type: cloud-compose
    parent: mycompany/production-infrastructure
    config:
      domain: app.example.com
      size:
        cpu: 2048
        memory: 4096
      uses:
        - mongodb      # Resource provisioned by server.yaml
      runs:
        - web-service
      env:
        NODE_ENV: production
        # Database connection provided by ${resource:mongodb.uri}
      secrets:
        # Production application secrets (communication only)
        DISCORD_WEBHOOK: ${secret:prod-cicd-bot-discord-webhook-url}
        SLACK_WEBHOOK: ${secret:prod-cicd-bot-slack-webhook-url}
```

## Security Best Practices

### ✅ Do
- **Encrypt secrets**: Use `sc secrets add` to add and encrypt secret files
- **Separate environments**: Use different stack directories for prod/staging environments
- **Rotate regularly**: Update API keys and tokens periodically
- **Limit permissions**: Use minimal required permissions for each service

### ❌ Don't
- **Commit plaintext**: Never commit unencrypted secrets to version control
- **Share widely**: Limit access to secrets files to necessary team members
- **Reuse across environments**: Use separate credentials for production vs development

## Environment-Specific Deployment

For multiple environments, use separate stack directories:

```
.sc/
├── stacks/
│   ├── production-app/
│   │   ├── client.yaml        # Production client config
│   │   └── secrets.yaml       # Production secrets
│   └── staging-app/
│       ├── client.yaml        # Staging client config  
│       └── secrets.yaml       # Staging secrets
```

Deploy to different environments:
```bash
# Production deployment
sc deploy -s production-app -e production

# Staging deployment
sc deploy -s staging-app -e staging
```

## Related Examples

- **Multi-cloud**: See `../gcp-auth-cloudflare-mongodb-discord-telegram/` for GCP integration
- **Kubernetes**: See `../kube-and-gcp-auth/` for Kubernetes authentication
- **Server Configuration**: Check the server examples for infrastructure setup

## Troubleshooting

### AWS Authentication Issues
```bash
# Verify AWS credentials
aws sts get-caller-identity --region eu-central-1

# Test with different region
aws sts get-caller-identity --region us-west-2
```

### MongoDB Atlas Connection
```bash
# Test MongoDB Atlas API access
curl -u "$MONGODB_ATLAS_PUBLIC_KEY:$MONGODB_ATLAS_PRIVATE_KEY" \
  "https://cloud.mongodb.com/api/atlas/v1.0/groups"
```

### Webhook Testing
```bash
# Test Discord webhook
curl -X POST "your-discord-webhook-url" \
  -H "Content-Type: application/json" \
  -d '{"content": "Test message from Simple Container"}'

# Test Slack webhook  
curl -X POST "your-slack-webhook-url" \
  -H "Content-Type: application/json" \
  -d '{"text": "Test message from Simple Container"}'
```

This configuration enables secure, multi-region AWS deployments with integrated database management and CI/CD notifications.
