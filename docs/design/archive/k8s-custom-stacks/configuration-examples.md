# Configuration Examples

## Basic ParentEnv Usage

### Simple Preview Environment

```yaml
# client.yaml
stacks:
  # Main staging environment
  staging:
    type: single-image
    config:
      image: "gcr.io/myproject/myapp:staging"
      port: 8080
      domain: "staging.myapp.com"
      env:
        DATABASE_URL: "postgresql://staging-db:5432/myapp"
        REDIS_URL: "redis://staging-redis:6379"
  
  # Preview environment sharing staging namespace
  staging-preview:
    type: single-image
    parentEnv: staging
    config:
      image: "gcr.io/myproject/myapp:pr-123"
      port: 8080
      domain: "staging-preview.myapp.com"
      env:
        DATABASE_URL: "postgresql://staging-db:5432/myapp_preview"
        REDIS_URL: "redis://staging-redis:6379"
        FEATURE_FLAG_NEW_UI: "true"
```

**Result**:
- **Namespace**: Both deploy to `staging` namespace
- **Deployments**: `myapp` and `myapp-staging-preview`
- **Services**: `myapp` and `myapp-staging-preview`
- **Domains**: `staging.myapp.com` and `staging-preview.myapp.com`

## Multi-Environment Scenarios

### Multiple Preview Environments

```yaml
# client.yaml
stacks:
  # Production environment
  production:
    type: single-image
    config:
      image: "gcr.io/myproject/myapp:v1.2.3"
      port: 8080
      domain: "myapp.com"
      scale:
        min: 3
        max: 10
  
  # Staging environment
  staging:
    type: single-image
    config:
      image: "gcr.io/myproject/myapp:staging"
      port: 8080
      domain: "staging.myapp.com"
      scale:
        min: 1
        max: 3
  
  # PR preview environments (all in staging namespace)
  staging-pr-456:
    type: single-image
    parentEnv: staging
    config:
      image: "gcr.io/myproject/myapp:pr-456"
      port: 8080
      domain: "pr-456.staging.myapp.com"
      scale:
        min: 1
        max: 2
  
  staging-pr-789:
    type: single-image
    parentEnv: staging
    config:
      image: "gcr.io/myproject/myapp:pr-789"
      port: 8080
      domain: "pr-789.staging.myapp.com"
      scale:
        min: 1
        max: 2
  
  # Hotfix testing environment
  staging-hotfix:
    type: single-image
    parentEnv: staging
    config:
      image: "gcr.io/myproject/myapp:hotfix-critical"
      port: 8080
      domain: "hotfix.staging.myapp.com"
      scale:
        min: 1
        max: 3
```

**Result**:
- **Production namespace**: `production` with 1 deployment
- **Staging namespace**: `staging` with 4 deployments
- All staging-related environments share resources and RBAC

## Advanced Configuration Examples

### Different Resource Requirements

```yaml
# client.yaml
stacks:
  # Main staging with standard resources
  staging:
    type: single-image
    config:
      image: "gcr.io/myproject/myapp:staging"
      port: 8080
      domain: "staging.myapp.com"
      resources:
        requests:
          cpu: "100m"
          memory: "256Mi"
        limits:
          cpu: "500m"
          memory: "512Mi"
  
  # Performance testing environment with higher resources
  staging-perf:
    type: single-image
    parentEnv: staging
    config:
      image: "gcr.io/myproject/myapp:perf-test"
      port: 8080
      domain: "perf.staging.myapp.com"
      resources:
        requests:
          cpu: "500m"
          memory: "1Gi"
        limits:
          cpu: "2000m"
          memory: "4Gi"
      scale:
        min: 2
        max: 5
  
  # Load testing environment with minimal resources
  staging-load:
    type: single-image
    parentEnv: staging
    config:
      image: "gcr.io/myproject/myapp:load-test"
      port: 8080
      domain: "load.staging.myapp.com"
      resources:
        requests:
          cpu: "50m"
          memory: "128Mi"
        limits:
          cpu: "200m"
          memory: "256Mi"
      scale:
        min: 1
        max: 1
```

### Different Environment Variables

```yaml
# client.yaml
stacks:
  staging:
    type: single-image
    config:
      image: "gcr.io/myproject/myapp:staging"
      port: 8080
      domain: "staging.myapp.com"
      env:
        NODE_ENV: "staging"
        LOG_LEVEL: "info"
        FEATURE_NEW_CHECKOUT: "false"
        PAYMENT_PROVIDER: "stripe-test"
  
  # A/B testing environment with different feature flags
  staging-ab-test:
    type: single-image
    parentEnv: staging
    config:
      image: "gcr.io/myproject/myapp:ab-test"
      port: 8080
      domain: "ab-test.staging.myapp.com"
      env:
        NODE_ENV: "staging"
        LOG_LEVEL: "debug"
        FEATURE_NEW_CHECKOUT: "true"  # Different feature flag
        FEATURE_ENHANCED_SEARCH: "true"
        PAYMENT_PROVIDER: "stripe-test"
        AB_TEST_VARIANT: "enhanced"
```

## Multi-Service Applications

### Microservices with ParentEnv

```yaml
# client.yaml
stacks:
  # Main staging services
  staging-api:
    type: single-image
    config:
      image: "gcr.io/myproject/api:staging"
      port: 3000
      domain: "api.staging.myapp.com"
  
  staging-web:
    type: single-image
    config:
      image: "gcr.io/myproject/web:staging"
      port: 8080
      domain: "staging.myapp.com"
      env:
        API_URL: "https://api.staging.myapp.com"
  
  # Preview environment for both services
  staging-preview-api:
    type: single-image
    parentEnv: staging-api
    config:
      image: "gcr.io/myproject/api:pr-123"
      port: 3000
      domain: "api.pr-123.staging.myapp.com"
  
  staging-preview-web:
    type: single-image
    parentEnv: staging-web
    config:
      image: "gcr.io/myproject/web:pr-123"
      port: 8080
      domain: "pr-123.staging.myapp.com"
      env:
        API_URL: "https://api.pr-123.staging.myapp.com"
```

**Result**:
- **staging-api namespace**: `api` and `api-staging-preview` deployments
- **staging-web namespace**: `web` and `web-staging-preview` deployments
- Services can communicate via updated environment variables

## Database and External Service Integration

### Shared vs Isolated Resources

```yaml
# client.yaml
stacks:
  # Main staging with dedicated database
  staging:
    type: single-image
    config:
      image: "gcr.io/myproject/myapp:staging"
      port: 8080
      domain: "staging.myapp.com"
      env:
        DATABASE_URL: "postgresql://staging-db.staging.svc.cluster.local:5432/myapp"
        REDIS_URL: "redis://staging-redis.staging.svc.cluster.local:6379"
        S3_BUCKET: "myapp-staging-uploads"
  
  # Preview environment sharing some resources, isolating others
  staging-preview:
    type: single-image
    parentEnv: staging
    config:
      image: "gcr.io/myproject/myapp:pr-456"
      port: 8080
      domain: "pr-456.staging.myapp.com"
      env:
        # Shared database with different schema/database
        DATABASE_URL: "postgresql://staging-db.staging.svc.cluster.local:5432/myapp_pr456"
        # Shared Redis with different key prefix
        REDIS_URL: "redis://staging-redis.staging.svc.cluster.local:6379"
        REDIS_KEY_PREFIX: "pr456:"
        # Isolated S3 bucket
        S3_BUCKET: "myapp-staging-pr456-uploads"
        # Preview-specific feature flags
        FEATURE_NEW_DASHBOARD: "true"
```

## CI/CD Integration Examples

### GitHub Actions Integration

```yaml
# .github/workflows/preview.yml
name: Deploy Preview Environment

on:
  pull_request:
    types: [opened, synchronize]

jobs:
  deploy-preview:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Generate preview environment name
        id: env-name
        run: |
          PR_NUMBER=${{ github.event.number }}
          echo "env_name=staging-pr-${PR_NUMBER}" >> $GITHUB_OUTPUT
      
      - name: Update client.yaml
        run: |
          cat >> client.yaml << EOF
          
          ${{ steps.env-name.outputs.env_name }}:
            type: single-image
            parentEnv: staging
            config:
              image: "gcr.io/myproject/myapp:pr-${{ github.event.number }}"
              port: 8080
              domain: "pr-${{ github.event.number }}.staging.myapp.com"
              env:
                DATABASE_URL: "postgresql://staging-db:5432/myapp_pr${{ github.event.number }}"
                FEATURE_BRANCH: "${{ github.head_ref }}"
          EOF
      
      - name: Deploy with Simple Container
        run: |
          sc deploy ${{ steps.env-name.outputs.env_name }}
      
      - name: Comment PR with preview URL
        uses: actions/github-script@v6
        with:
          script: |
            github.rest.issues.createComment({
              issue_number: ${{ github.event.number }},
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: '🚀 Preview environment deployed: https://pr-${{ github.event.number }}.staging.myapp.com'
            })
```

### Cleanup Workflow

```yaml
# .github/workflows/cleanup-preview.yml
name: Cleanup Preview Environment

on:
  pull_request:
    types: [closed]

jobs:
  cleanup-preview:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Remove preview environment
        run: |
          PR_NUMBER=${{ github.event.number }}
          sc destroy staging-pr-${PR_NUMBER}
```

## Resource Isolation Examples

### Separate ConfigMaps and Secrets

```yaml
# Generated Kubernetes resources

# Original staging ConfigMap (unchanged)
apiVersion: v1
kind: ConfigMap
metadata:
  name: myapp-config
  namespace: staging
data:
  app.properties: |
    environment=staging
    log.level=info
    feature.newUI=false

---
# Preview environment ConfigMap (isolated)
apiVersion: v1
kind: ConfigMap
metadata:
  name: myapp-staging-preview-config
  namespace: staging
data:
  app.properties: |
    environment=staging-preview
    log.level=debug
    feature.newUI=true
    feature.experimental=true

---
# Original staging Secret (unchanged)
apiVersion: v1
kind: Secret
metadata:
  name: myapp-secrets
  namespace: staging
type: Opaque
data:
  database-password: <base64-encoded>
  api-key: <base64-encoded>

---
# Preview environment Secret (isolated)
apiVersion: v1
kind: Secret
metadata:
  name: myapp-staging-preview-secrets
  namespace: staging
type: Opaque
data:
  database-password: <base64-encoded>
  api-key: <base64-encoded-different>
```

## Monitoring and Observability

### Environment-Specific Metrics

```yaml
# client.yaml with monitoring labels
stacks:
  staging:
    type: single-image
    config:
      image: "gcr.io/myproject/myapp:staging"
      port: 8080
      domain: "staging.myapp.com"
      # Monitoring configuration
      cloudExtras:
        podAnnotations:
          prometheus.io/scrape: "true"
          prometheus.io/port: "8080"
          prometheus.io/path: "/metrics"
  
  staging-preview:
    type: single-image
    parentEnv: staging
    config:
      image: "gcr.io/myproject/myapp:pr-123"
      port: 8080
      domain: "staging-preview.myapp.com"
      # Same monitoring, different labels
      cloudExtras:
        podAnnotations:
          prometheus.io/scrape: "true"
          prometheus.io/port: "8080"
          prometheus.io/path: "/metrics"
          # Additional labels for preview tracking
          preview.environment: "staging-preview"
          preview.pr: "123"
```

**Result**: Prometheus will collect metrics from both deployments with different labels, enabling environment-specific dashboards and alerts.

## Validation Examples

### Valid Configurations

```yaml
# ✅ VALID: Basic parentEnv usage
stacks:
  staging:
    type: single-image
    config:
      image: "gcr.io/project/app:staging"
      domain: "staging.example.com"
  
  staging-preview:
    type: single-image
    parentEnv: staging  # Different from stack name - triggers custom stack behavior
    config:
      image: "gcr.io/project/app:preview"
      domain: "preview.staging.example.com"

# ✅ VALID: Self-reference (treated as standard stack)
stacks:
  staging:
    type: single-image
    parentEnv: staging  # Same as stack name - treated as standard deployment
    config:
      image: "gcr.io/project/app:staging"
      domain: "staging.example.com"

# ✅ VALID: Multiple custom stacks
stacks:
  production:
    type: single-image
    config:
      image: "gcr.io/project/app:prod"
      domain: "example.com"
  
  prod-hotfix:
    type: single-image
    parentEnv: production
    config:
      image: "gcr.io/project/app:hotfix"
      domain: "hotfix.example.com"
  
  prod-canary:
    type: single-image
    parentEnv: production
    config:
      image: "gcr.io/project/app:canary"
      domain: "canary.example.com"
```

### Server.yaml and Client.yaml Relationship

```yaml
# server.yaml - Infrastructure environments
resources:
  staging-cluster:
    type: gcp-gke-autopilot-cluster
    config:
      projectId: "my-project"
      location: "us-central1"
      # This creates the "staging" environment

  production-cluster:
    type: gcp-gke-autopilot-cluster
    config:
      projectId: "my-project"
      location: "us-central1"
      # This creates the "production" environment
```

```yaml
# client.yaml - Application stacks
stacks:
  # Standard deployment to staging environment
  staging:
    type: single-image
    config:
      image: "gcr.io/project/app:staging"
      domain: "staging.example.com"
  
  # Custom stack deployed to staging environment (same namespace)
  staging-preview:
    type: single-image
    parentEnv: staging  # References "staging" environment from server.yaml
    config:
      image: "gcr.io/project/app:preview"
      domain: "preview.staging.example.com"
```

### Invalid Configurations

```yaml
# ❌ INVALID: parentEnv references non-existent server.yaml environment  
stacks:
  staging-preview:
    type: single-image
    parentEnv: nonexistent  # ERROR: Must reference environment from server.yaml
    config:
      image: "gcr.io/project/app:preview"

# ❌ INVALID: Same domain for different stacks in same namespace
stacks:
  staging:
    type: single-image
    config:
      image: "gcr.io/project/app:staging"
      domain: "staging.example.com"
  
  staging-preview:
    type: single-image
    parentEnv: staging
    config:
      image: "gcr.io/project/app:preview"
      domain: "staging.example.com"  # ERROR: Domain conflict
```

These examples demonstrate the flexibility and power of the parentEnv feature while maintaining clear boundaries and avoiding common configuration pitfalls.
