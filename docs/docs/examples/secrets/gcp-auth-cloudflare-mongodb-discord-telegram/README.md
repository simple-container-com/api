# GCP Multi-Service Integration Secrets Example

This example demonstrates comprehensive secrets configuration for Google Cloud Platform with multiple third-party service integrations including MongoDB Atlas, Cloudflare, and communication platforms.

## What This Example Shows

- **GCP Service Accounts**: Multi-environment GCP authentication (staging + production)
- **Service Account JSON**: Complete GCP service account credential configuration
- **MongoDB Atlas Integration**: Database management API credentials
- **Cloudflare DNS Management**: API token for DNS automation
- **Communication Platforms**: Discord, Telegram bot configurations
- **Multi-Environment Setup**: Separate GCP projects for staging and production

## Configuration Structure

### GCP Authentication

```yaml
auth:
  gcloud-staging:        # Staging environment GCP auth
    type: gcp-service-account
    config:
      projectId: gcp-project-staging-123
      credentials: |-     # Full service account JSON
        {
          "type": "service_account",
          "project_id": "gcp-project-staging-123",
          "private_key_id": "...",
          "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
          "client_email": "simple-container-deploy-bot@...",
          "client_id": "...",
          "auth_uri": "https://accounts.google.com/o/oauth2/auth",
          "token_uri": "https://oauth2.googleapis.com/token",
          "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
          "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/...",
          "universe_domain": "googleapis.com"
        }
        
  gcloud:               # Production environment GCP auth
    type: gcp-service-account
    config:
      projectId: gcp-project-123
      credentials: |-     # Production service account JSON
        # ... similar structure for production
```

### Application Secrets

```yaml
values:
  # Cloudflare DNS management
  CLOUDFLARE_API_TOKEN: base64-encoded-cloudflare-token
  
  # MongoDB Atlas database management
  MONGODB_ATLAS_PUBLIC_KEY: base64-encoded-public-key
  MONGODB_ATLAS_PRIVATE_KEY: your-private-key-uuid
  
  # Discord bot integration
  cicd-bot-discord-webhook-url: https://discord.com/api/webhooks/...
  
  # Telegram bot integration
  cicd-bot-telegram-chat: "-111111"        # Chat ID (negative for groups)
  cicd-bot-telegram-token: "123456:token"  # Bot token from @BotFather
```

## How to Customize

### 1. GCP Service Account Setup

#### Create Service Account
```bash
# Create service account for Simple Container
gcloud iam service-accounts create simple-container-deploy-bot \
  --display-name="Simple Container Deploy Bot" \
  --project=your-project-id

# Grant necessary permissions
gcloud projects add-iam-policy-binding your-project-id \
  --member="serviceAccount:simple-container-deploy-bot@your-project-id.iam.gserviceaccount.com" \
  --role="roles/editor"

# Create and download service account key
gcloud iam service-accounts keys create service-account.json \
  --iam-account=simple-container-deploy-bot@your-project-id.iam.gserviceaccount.com
```

#### Configure Multiple Environments
```bash
# For staging environment
gcloud projects create your-project-staging-123
gcloud iam service-accounts create simple-container-deploy-bot \
  --project=your-project-staging-123

# For production environment  
gcloud projects create your-project-123
gcloud iam service-accounts create simple-container-deploy-bot \
  --project=your-project-123
```

### 2. MongoDB Atlas Configuration
1. Visit [MongoDB Atlas Console](https://cloud.mongodb.com/)
2. Navigate to **Access Manager** → **API Keys**
3. Create API Key with **Project Owner** or **Organization Member** role
4. Copy public key and private key
5. Base64 encode the public key: `echo -n "your-public-key" | base64`

### 3. Cloudflare API Token
1. Go to [Cloudflare API Tokens](https://dash.cloudflare.com/profile/api-tokens)
2. Create Custom Token with:
   - **Zone:Read** for all zones
   - **DNS:Edit** for specific zones
3. Copy token and base64 encode: `echo -n "your-token" | base64`

### 4. Discord Webhook Setup
1. Open Discord Server Settings
2. Navigate to **Integrations** → **Webhooks**
3. Click **New Webhook**
4. Configure webhook name and channel
5. Copy webhook URL

### 5. Telegram Bot Configuration

#### Create Bot
```bash
# Message @BotFather on Telegram
/newbot
# Follow prompts to create bot
# Copy the bot token (format: 123456789:ABC-DEF...)
```

#### Get Chat ID
```bash
# Add bot to channel/group first, then:
curl "https://api.telegram.org/bot<BOT_TOKEN>/getUpdates"
# Look for "chat":{"id":-111111} in response
```

## Usage in Configuration Files

### Server Configuration (Infrastructure Secrets)

Infrastructure secrets belong in `server.yaml` for cloud provider authentication and resource provisioning:

```yaml
# server.yaml - Infrastructure authentication and resource management
schemaVersion: 1.0
provisioner:
  type: pulumi
  config:
    state-storage:
      type: gcp-bucket
      config:
        credentials: "${auth:gcloud}"
        projectId: "${auth:gcloud.projectId}"
        bucketName: example-company-sc-state
        location: europe-west3
    secrets-provider:
      type: gcp-kms
      config:
        credentials: "${auth:gcloud}"
        provision: true
        keyRing: simple-container-secrets
        location: global

templates:
  cloud-compose-gcp:
    type: gcp-cloud-run
    config:
      credentials: "${auth:gcloud}"
      projectId: "${auth:gcloud.projectId}"

resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: your-cloudflare-account-id
      zoneName: example.com

  resources:
    staging:
      template: cloud-compose-gcp
      resources:
        mongodb-atlas:
          type: mongodb-atlas
          config:
            privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
            publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
            instanceSize: "M10"
            region: "EUROPE_WEST_1"
            
    production:
      template: cloud-compose-gcp
      resources:
        mongodb-atlas:
          type: mongodb-atlas
          config:
            privateKey: "${secret:PROD_MONGODB_ATLAS_PRIVATE_KEY}"
            publicKey: "${secret:PROD_MONGODB_ATLAS_PUBLIC_KEY}"
            instanceSize: "M30"
            region: "EUROPE_WEST_1"
```

### Client Configuration (Application Secrets)

Application secrets belong in `client.yaml` only for direct application integration:

```yaml
# client.yaml - Application secrets (only what the app directly needs)
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
        - mongodb-atlas  # Resource provisioned by server.yaml
      runs:
        - web-service
      env:
        NODE_ENV: staging
      secrets:
        # Application-level secrets only (not infrastructure secrets)
        DISCORD_WEBHOOK: ${secret:cicd-bot-discord-webhook-url}
        TELEGRAM_CHAT: ${secret:cicd-bot-telegram-chat}
        TELEGRAM_TOKEN: ${secret:cicd-bot-telegram-token}
        # Database connection provided by ${resource:mongodb-atlas.uri}
          
  production:
    type: cloud-compose
    parent: mycompany/production-infrastructure
    config:
      domain: app.example.com
      size:
        cpu: 2048
        memory: 4096
      uses:
        - mongodb-atlas  # Resource provisioned by server.yaml
      runs:
        - web-service
      env:
        NODE_ENV: production
      secrets:
        # Production application secrets (communication only)
        DISCORD_WEBHOOK: ${secret:prod-cicd-bot-discord-webhook-url}
        TELEGRAM_CHAT: ${secret:prod-cicd-bot-telegram-chat}
        TELEGRAM_TOKEN: ${secret:prod-cicd-bot-telegram-token}
        # Database connection provided by ${resource:mongodb-atlas.uri}
```

## Advanced Configuration Patterns

### Environment-Specific Secrets

```yaml
# Use different secret names for environments
values:
  # Staging secrets
  STAGING_MONGODB_ATLAS_PUBLIC_KEY: staging-encoded-key
  STAGING_CLOUDFLARE_API_TOKEN: staging-encoded-token
  
  # Production secrets  
  PROD_MONGODB_ATLAS_PUBLIC_KEY: prod-encoded-key
  PROD_CLOUDFLARE_API_TOKEN: prod-encoded-token
```

### Multi-Environment Secret Management

```yaml
# client.yaml - Real environment-specific secrets
schemaVersion: 1.0
stacks:
  staging: &staging
    type: cloud-compose
    parent: mycompany/staging-infrastructure
    config: &staging-config
      domain: staging-app.example.com
      size:
        cpu: 1024
        memory: 2048
      uses:
        - mongodb-atlas
      runs:
        - web-service
      env: &staging-env
        NODE_ENV: staging
      secrets: &staging-secrets
        # Application secrets for staging
        MONGODB_ATLAS_PUBLIC_KEY: ${secret:MONGODB_ATLAS_PUBLIC_KEY}
        MONGODB_ATLAS_PRIVATE_KEY: ${secret:MONGODB_ATLAS_PRIVATE_KEY}
        CLOUDFLARE_API_TOKEN: ${secret:CLOUDFLARE_API_TOKEN}
        DISCORD_WEBHOOK: ${secret:cicd-bot-discord-webhook-url}
        TELEGRAM_CHAT: ${secret:cicd-bot-telegram-chat}
        TELEGRAM_TOKEN: ${secret:cicd-bot-telegram-token}
        
  production:
    <<: *staging
    parent: mycompany/production-infrastructure
    config:
      <<: *staging-config
      domain: app.example.com
      size:
        cpu: 2048
        memory: 4096
      env:
        <<: *staging-env
        NODE_ENV: production
      secrets:
        # Production secrets use different names
        MONGODB_ATLAS_PUBLIC_KEY: ${secret:PROD_MONGODB_ATLAS_PUBLIC_KEY}
        MONGODB_ATLAS_PRIVATE_KEY: ${secret:PROD_MONGODB_ATLAS_PRIVATE_KEY}
        CLOUDFLARE_API_TOKEN: ${secret:PROD_CLOUDFLARE_API_TOKEN}
        DISCORD_WEBHOOK: ${secret:prod-cicd-bot-discord-webhook-url}
        TELEGRAM_CHAT: ${secret:prod-cicd-bot-telegram-chat}
        TELEGRAM_TOKEN: ${secret:prod-cicd-bot-telegram-token}
```

## Security Best Practices

### ✅ Service Account Security
- **Principle of Least Privilege**: Grant minimal required IAM roles
- **Key Rotation**: Rotate service account keys every 90 days
- **Environment Separation**: Use separate service accounts for staging/prod
- **Audit Logging**: Enable Cloud Audit Logs for service account usage

### ✅ Secret Management
- **Encrypt Everything**: Use `sc secrets add` to add and encrypt secret files
- **Base64 Encoding**: Encode all tokens before storing
- **Regular Rotation**: Update API keys and tokens quarterly
- **Environment Isolation**: Never share secrets between environments

### ✅ Communication Security
- **Bot Token Security**: Treat Telegram bot tokens as highly sensitive
- **Webhook Validation**: Implement webhook signature validation where possible
- **Channel Restrictions**: Limit Discord/Telegram bot permissions to required channels

### ❌ Common Mistakes to Avoid
- **Don't**: Store plaintext service account JSON in version control
- **Don't**: Use production credentials in development/staging
- **Don't**: Share bot tokens or API keys in chat/email
- **Don't**: Grant excessive IAM permissions to service accounts

## Testing and Validation

### Verify GCP Authentication
```bash
# Test service account authentication
gcloud auth activate-service-account --key-file=service-account.json
gcloud auth list
gcloud projects list
```

### Test MongoDB Atlas API
```bash
# Test API access with your credentials
curl -u "$MONGODB_ATLAS_PUBLIC_KEY:$MONGODB_ATLAS_PRIVATE_KEY" \
  "https://cloud.mongodb.com/api/atlas/v1.0/groups" \
  | jq '.results[].name'
```

### Test Cloudflare API
```bash
# List zones with your token
curl -X GET "https://api.cloudflare.com/client/v4/zones" \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -H "Content-Type: application/json" \
  | jq '.result[].name'
```

### Test Communication Endpoints
```bash
# Test Discord webhook
curl -X POST "$DISCORD_WEBHOOK_URL" \
  -H "Content-Type: application/json" \
  -d '{"content": "🚀 Simple Container deployment test"}'

# Test Telegram bot
curl -X POST "https://api.telegram.org/bot$TELEGRAM_TOKEN/sendMessage" \
  -d "chat_id=$TELEGRAM_CHAT" \
  -d "text=🚀 Simple Container deployment test"
```

## Integration Examples

### Environment-Specific Secret Management
```bash
# Add secrets to the standardized location
sc secrets add .sc/stacks/your-app/secrets.yaml

# Deploy to different environments (same secrets file used)
sc deploy -s your-app -e production
sc deploy -s your-app -e staging
```

### CI/CD Integration with Secrets
```bash
# In your CI/CD pipeline, ensure secrets are available
sc secrets list

# Deploy using the secrets configured for each environment
sc deploy -s your-app -e staging   # Uses staging secrets
sc deploy -s your-app -e production # Uses production secrets
```

## Related Examples

- **AWS Integration**: See `../aws-mongodb-atlas/` for AWS-based setup
- **Kubernetes**: See `../kube-and-gcp-auth/` for Kubernetes + GCP integration
- **Server Configuration**: Check server examples for GCP infrastructure setup

This configuration enables comprehensive GCP-based deployments with integrated database management, DNS automation, and real-time deployment notifications across multiple communication platforms.
