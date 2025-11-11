# Kubernetes + GCP Dual Authentication Example

This example demonstrates how to configure Simple Container with both Kubernetes cluster access and Google Cloud Platform authentication, plus Docker registry credentials for container deployment workflows.

## What This Example Shows

- **Kubernetes Authentication**: Direct cluster access with kubeconfig
- **GCP Service Account**: Google Cloud Platform authentication
- **Docker Registry**: Container registry authentication
- **Hybrid Cloud Setup**: Managing both containerized and cloud-native resources
- **Multi-Cluster Support**: Configure access to different Kubernetes environments

## Configuration Structure

### Authentication Providers

```yaml
auth:
  gcloud:              # Google Cloud Platform authentication
    type: gcp-service-account
    config:
      projectId: project-sensor-434416-u2
      credentials: |-    # Complete service account JSON
        {
          "type": "service_account",
          "project_id": "project-id",
          "private_key_id": "...",
          "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
          "client_email": "deployer-bot@ai-asia-382012.iam.gserviceaccount.com",
          "client_id": "104019626917208012368",
          "auth_uri": "https://accounts.google.com/o/oauth2/auth",
          "token_uri": "https://oauth2.googleapis.com/token",
          "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
          "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/...",
          "universe_domain": "googleapis.com"
        }
        
  kubernetes:          # Kubernetes cluster authentication
    type: kubernetes
    config:
      kubeconfig: |-     # Complete kubeconfig YAML
        ---
        apiVersion: v1
        clusters:
        - cluster:
            insecure-skip-tls-verify: true    # For development only
            server: https://1.2.2.3:6550      # Your cluster API server
          name: k3d-private-production
        contexts:
        - context:
            cluster: k3d-private-production
            user: admin@k3d-private-production
          name: k3d-private-production
        current-context: k3d-private-production
        kind: Config
        preferences: {}
        users:
        - name: admin@k3d-private-production
          user:
            client-certificate-data: base64-encoded-cert
            client-key-data: base64-encoded-key
```

### Application Secrets

```yaml
values:
  # Docker registry access
  docker-registry-username: username
  docker-registry-password: password
```

## How to Customize

### 1. GCP Service Account Setup

#### Create Service Account
```bash
# Create service account
gcloud iam service-accounts create k8s-deployer-bot \
  --display-name="Kubernetes Deployer Bot" \
  --project=your-project-id

# Grant Kubernetes Engine permissions
gcloud projects add-iam-policy-binding your-project-id \
  --member="serviceAccount:k8s-deployer-bot@your-project-id.iam.gserviceaccount.com" \
  --role="roles/container.developer"

# Grant additional permissions for GCP resources
gcloud projects add-iam-policy-binding your-project-id \
  --member="serviceAccount:k8s-deployer-bot@your-project-id.iam.gserviceaccount.com" \
  --role="roles/compute.viewer"

# Create service account key
gcloud iam service-accounts keys create k8s-service-account.json \
  --iam-account=k8s-deployer-bot@your-project-id.iam.gserviceaccount.com
```

### 2. Kubernetes Cluster Access

#### Get Kubeconfig from Different Sources

**For Google Kubernetes Engine (GKE):**
```bash
# Get GKE cluster credentials
gcloud container clusters get-credentials your-cluster-name \
  --zone=us-central1-a \
  --project=your-project-id

# View current kubeconfig
kubectl config view --minify --raw
```

**For Amazon EKS:**
```bash
# Get EKS cluster credentials
aws eks update-kubeconfig --region us-west-2 --name your-cluster-name

# View kubeconfig
kubectl config view --minify --raw
```

**For Local Development (k3d/kind):**
```bash
# Create k3d cluster
k3d cluster create production --port "6550:6443@loadbalancer"

# Get kubeconfig
k3d kubeconfig get production

# For kind clusters
kind export kubeconfig --name production
```

**For Existing Cluster:**
```bash
# Copy from existing kubeconfig
cp ~/.kube/config ./cluster-config.yaml

# Extract specific cluster context
kubectl config view --minify --raw --context=your-context-name
```

#### Secure Kubeconfig Configuration

**Production Setup (Recommended):**
```yaml
kubeconfig: |-
  ---
  apiVersion: v1
  clusters:
  - cluster:
      certificate-authority-data: LS0tLS1CRUd... # Base64 encoded CA cert
      server: https://your-cluster-api-server.com:6443
    name: production-cluster
  contexts:
  - context:
      cluster: production-cluster
      user: service-account-user
    name: production-context
  current-context: production-context
  kind: Config
  users:
  - name: service-account-user
    user:
      token: eyJhbGciOiJSUzI1NiIs... # Service account token
```

**Development Setup:**
```yaml
kubeconfig: |-
  ---
  apiVersion: v1
  clusters:
  - cluster:
      insecure-skip-tls-verify: true  # Only for development!
      server: https://localhost:6550
    name: k3d-dev-cluster
  # ... rest of config
```

### 3. Docker Registry Configuration

#### Docker Hub
```bash
# Use your Docker Hub credentials
docker-registry-username: your-dockerhub-username
docker-registry-password: your-dockerhub-password-or-token
```

#### Google Container Registry (GCR)
```bash
# Use service account for GCR access
docker-registry-username: _json_key
docker-registry-password: |
  {
    "type": "service_account",
    "project_id": "your-project-id",
    # ... full service account JSON
  }
```

#### Amazon ECR
```bash
# Get ECR login token
aws ecr get-login-password --region us-west-2 | \
  docker login --username AWS --password-stdin your-account-id.dkr.ecr.us-west-2.amazonaws.com

# Use AWS for username and token for password
docker-registry-username: AWS  
docker-registry-password: your-ecr-token
```

#### Private Registry
```bash
# For self-hosted registries
docker-registry-username: your-registry-username
docker-registry-password: your-registry-password
```

## Usage in Configuration Files

### Server Configuration (Infrastructure Secrets)

Infrastructure secrets belong in `server.yaml` for Kubernetes and cloud provider authentication:

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
  kubernetes-gcp:
    type: gke-autopilot
    config:
      credentials: "${auth:gcloud}"
      projectId: "${auth:gcloud.projectId}"
      kubeconfig: "${auth:kubernetes}"

resources:
  container-registry:
    type: gcr
    config:
      credentials: "${auth:gcloud}"
      projectId: "${auth:gcloud.projectId}"

  resources:
    staging:
      template: kubernetes-gcp
      resources:
        mongodb:
          type: mongodb-atlas
          config:
            privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
            publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
            instanceSize: "M10"
        redis:
          type: redis-cloud
          config:
            apiKey: "${secret:REDIS_CLOUD_API_KEY}"
            planId: "basic"
            
    production:
      template: kubernetes-gcp
      resources:
        mongodb:
          type: mongodb-atlas
          config:
            privateKey: "${secret:PROD_MONGODB_ATLAS_PRIVATE_KEY}"
            publicKey: "${secret:PROD_MONGODB_ATLAS_PUBLIC_KEY}"
            instanceSize: "M30"
        redis:
          type: redis-cloud
          config:
            apiKey: "${secret:PROD_REDIS_CLOUD_API_KEY}"
            planId: "professional"
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
        - redis        # Resource provisioned by server.yaml
      runs:
        - web-service
      env:
        NODE_ENV: staging
        # Database/Redis connections provided by ${resource:mongodb.uri} and ${resource:redis.uri}
      secrets:
        # Only secrets the application directly needs
        DOCKER_USERNAME: ${secret:docker-registry-username}
        DOCKER_PASSWORD: ${secret:docker-registry-password}

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
        - redis        # Resource provisioned by server.yaml
      runs:
        - web-service
      env:
        NODE_ENV: production
        # Database/Redis connections provided by ${resource:mongodb.uri} and ${resource:redis.uri}
      secrets:
        # Only secrets the application directly needs
        DOCKER_USERNAME: ${secret:production-docker-registry-username}
        DOCKER_PASSWORD: ${secret:production-docker-registry-password}
```

## Advanced Configuration Patterns

### Multi-Environment Setup
```yaml
# client.yaml - Multiple environments with different resources
schemaVersion: 1.0
stacks:
  staging: &staging
    type: cloud-compose
    parent: mycompany/staging-infrastructure
    config: &staging-config
      domain: staging-app.example.com
      size:
        cpu: 512
        memory: 1024
      uses:
        - mongodb
        - redis
      runs:
        - app-service
      env: &staging-env
        NODE_ENV: staging
        LOG_LEVEL: debug
      secrets: &staging-secrets
        DOCKER_USERNAME: ${secret:staging-docker-registry-username}
        DOCKER_PASSWORD: ${secret:staging-docker-registry-password}
        MONGO_URL: ${resource:mongodb.uri}
        
  production:
    <<: *staging
    type: cloud-compose
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
        LOG_LEVEL: info
      secrets:
        <<: *staging-secrets
        DOCKER_USERNAME: ${secret:production-docker-registry-username}
        DOCKER_PASSWORD: ${secret:production-docker-registry-password}
```

### Single-Image with Lambda Functions
```yaml
# client.yaml - For serverless/function deployments
schemaVersion: 1.0
stacks:
  staging:
    type: single-image
    template: lambda-eu
    parent: mycompany/infrastructure
    config:
      domain: staging-functions.example.com
      timeout: 300
      maxMemory: 1024
      uses:
        - mongodb
      cloudExtras:
        lambdaRoutingType: function-url
      env:
        NODE_ENV: staging
        STAGE: staging
      secrets:
        API_KEY: ${secret:staging-function-api-key}
        MONGO_URL: ${resource:mongodb.uri}
        GCP_SERVICE_ACCOUNT_KEY: ${secret:staging-gcp-service-account}
```

### Docker Compose Integration
```yaml
# docker-compose.yaml - Container orchestration (referenced by client.yaml)
version: '3.8'
services:
  web-service:
    build: .
    ports:
      - "3000:3000"
    environment:
      - NODE_ENV=${NODE_ENV}
      - LOG_LEVEL=${LOG_LEVEL}
      - MONGO_URL=${MONGO_URL}
      - REDIS_URL=${REDIS_URL}
    volumes:
      - ./:/app
    depends_on:
      - redis
      - mongo

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

  mongo:
    image: mongo:7
    ports:
      - "27017:27017"
```

## Security Best Practices

### ✅ Kubernetes Security
- **RBAC**: Use Role-Based Access Control with minimal required permissions
- **Service Accounts**: Use dedicated service accounts, never default
- **TLS**: Always use certificate-based authentication in production
- **Network Policies**: Implement network segmentation
- **Secret Management**: Store sensitive data in Kubernetes Secrets, not ConfigMaps

### ✅ GCP Security  
- **IAM Roles**: Follow principle of least privilege
- **Key Rotation**: Rotate service account keys every 90 days
- **Audit Logging**: Enable Cloud Audit Logs
- **VPC Security**: Use private GKE clusters where possible

### ✅ Container Registry Security
- **Image Scanning**: Enable vulnerability scanning
- **Signed Images**: Use container image signing
- **Private Registries**: Use private registries for production images
- **Access Control**: Limit registry access with appropriate IAM roles

### ❌ Security Anti-Patterns
- **Don't**: Use `insecure-skip-tls-verify: true` in production
- **Don't**: Store kubeconfig with admin privileges
- **Don't**: Use default service accounts for deployments
- **Don't**: Commit registry passwords to version control
- **Don't**: Grant excessive cluster permissions

## Testing and Validation

### Test Kubernetes Access
```bash
# Test kubectl connectivity
kubectl --kubeconfig=./test-kubeconfig.yaml get nodes
kubectl --kubeconfig=./test-kubeconfig.yaml get pods --all-namespaces

# Test specific namespace access
kubectl --kubeconfig=./test-kubeconfig.yaml get pods -n production
```

### Test GCP Authentication
```bash
# Activate service account
gcloud auth activate-service-account --key-file=service-account.json

# Test GCP access
gcloud projects list
gcloud compute instances list
gcloud container clusters list
```

### Test Docker Registry Access
```bash
# Test Docker Hub
echo "$DOCKER_PASSWORD" | docker login --username "$DOCKER_USERNAME" --password-stdin

# Test GCR
echo "$GCP_SERVICE_ACCOUNT_KEY" | docker login -u _json_key --password-stdin gcr.io

# Test ECR
aws ecr get-login-password --region us-west-2 | \
  docker login --username AWS --password-stdin 123456789012.dkr.ecr.us-west-2.amazonaws.com
```

### Integration Testing
```bash
# Full deployment test
sc deploy -s your-stack -e staging

# Validate Kubernetes resources
kubectl get deployments,services,ingresses -n production

# Check GCP resources  
gcloud run services list
gcloud compute instances list
```

## Common Use Cases

### 1. **GKE with GCP Integration**
- Deploy applications to Google Kubernetes Engine
- Use GCP services (Cloud SQL, Cloud Storage, etc.)
- Integrated logging and monitoring

### 2. **Multi-Cloud Kubernetes**
- Kubernetes cluster on one provider
- Additional cloud services on another
- Hybrid deployment strategy

### 3. **On-Premises + Cloud**
- On-premises Kubernetes cluster
- GCP services for managed databases/storage
- Hybrid connectivity via VPN/Interconnect

### 4. **Development to Production Pipeline**
- Local development with k3d/kind
- Staging on managed Kubernetes (GKE/EKS)
- Production with additional GCP services

## Related Examples

- **AWS Integration**: See `../aws-mongodb-atlas/` for AWS-based authentication
- **GCP Multi-Service**: See `../gcp-auth-cloudflare-mongodb-discord-telegram/` for comprehensive GCP setup
- **Server Configuration**: Check server examples for Kubernetes infrastructure setup

This configuration enables powerful hybrid deployments combining the flexibility of Kubernetes with the managed services of Google Cloud Platform, suitable for both cloud-native and containerized workloads.
