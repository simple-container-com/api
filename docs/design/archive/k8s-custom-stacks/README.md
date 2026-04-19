# Kubernetes Custom Stacks with ParentEnv Support

## Overview

This design enables deploying custom stack environments (like `staging-preview`) that share the same Kubernetes namespace as their parent environment (like `staging`) while maintaining proper resource isolation and independent routing.

## Problem Statement

Currently, Simple Container creates separate namespaces for each environment in Kubernetes deployments. However, when using `parentEnv` configuration in `client.yaml`, users want to:

1. **Deploy to the same namespace** as the parent environment
2. **Avoid resource conflicts** with existing deployments
3. **Maintain independent routing** with different domains
4. **Isolate resources** (ConfigMaps, Secrets, HPAs, VPAs, etc.)
5. **Keep parent deployment unaffected**

### Current Behavior (Problem)
```yaml
# client.yaml
stacks:
  staging:
    type: single-image
    config:
      # ... staging config
  
  staging-preview:
    type: single-image
    parentEnv: staging
    config:
      # ... preview config
```

**Result**: Deployment fails due to namespace and resource name conflicts.

### Desired Behavior (Solution)
- `staging` → namespace: `staging`, deployment: `my-service`
- `staging-preview` → namespace: `staging`, deployment: `my-service-staging-preview`
- Both deployments coexist with independent domains and resources

## Solution Architecture

### Resource Naming Strategy

#### Current Naming (Single Environment per Namespace)
```
Namespace: staging
├── Deployment: my-service
├── Service: my-service  
├── ConfigMap: my-service-config
├── Secret: my-service-secrets
├── HPA: my-service-hpa
└── VPA: my-service-vpa
```

#### New Naming (Multiple Environments per Namespace)
```
Namespace: staging
├── Deployment: my-service (original staging)
├── Deployment: my-service-staging-preview (custom stack)
├── Service: my-service (original staging)
├── Service: my-service-staging-preview (custom stack)
├── ConfigMap: my-service-config (original)
├── ConfigMap: my-service-staging-preview-config (custom)
├── Secret: my-service-secrets (original)
├── Secret: my-service-staging-preview-secrets (custom)
├── HPA: my-service-hpa (original)
├── HPA: my-service-staging-preview-hpa (custom)
└── VPA: my-service-vpa (original)
└── VPA: my-service-staging-preview-vpa (custom)
```

### Key Design Principles

1. **Namespace Sharing**: Custom stacks deploy to parent's namespace
2. **Resource Suffixing**: All resources get environment-specific suffixes
3. **Independent Routing**: Separate domains and Caddy annotations
4. **Resource Isolation**: No shared ConfigMaps, Secrets, or scaling resources
5. **Parent Protection**: Original deployments remain unchanged

## Configuration Structure

### Server.yaml and Client.yaml Configuration

```yaml
# server.yaml - Infrastructure environments
resources:
  staging-cluster:
    type: gcp-gke-autopilot-cluster
    config:
      projectId: "my-project"
      location: "us-central1"
      # This creates the "staging" environment/namespace

  production-cluster:
    type: gcp-gke-autopilot-cluster
    config:
      projectId: "my-project"  
      location: "us-central1"
      # This creates the "production" environment/namespace
```

```yaml
# client.yaml - Application stacks
stacks:
  # Standard deployment to staging environment
  staging:
    type: single-image
    config:
      image: "gcr.io/project/app:staging"
      port: 8080
      domain: "staging.myapp.com"
  
  # Custom stack deployed to staging environment (same namespace)
  staging-preview:
    type: single-image
    parentEnv: staging  # References "staging" environment from server.yaml
    config:
      image: "gcr.io/project/app:pr-123"
      port: 8080
      domain: "staging-preview.myapp.com"  # Independent domain
  
  # Another custom stack in staging environment
  staging-hotfix:
    type: single-image
    parentEnv: staging  # References "staging" environment from server.yaml
    config:
      image: "gcr.io/project/app:hotfix-456"
      port: 8080
      domain: "staging-hotfix.myapp.com"
```

### Resource Naming Logic

#### Environment Name Resolution
```go
func resolveEnvironmentNames(stackName string, parentEnv *string) (namespace, resourceSuffix string) {
    if parentEnv != nil && *parentEnv != "" {
        // Custom stack: use parent's namespace, add suffix for resources
        namespace = *parentEnv
        resourceSuffix = stackName
    } else {
        // Standard stack: use stack name for both
        namespace = stackName
        resourceSuffix = ""
    }
    return
}
```

#### Resource Name Generation
```go
func generateResourceName(serviceName, resourceSuffix string) string {
    if resourceSuffix == "" {
        return serviceName  // Standard deployment
    }
    return fmt.Sprintf("%s-%s", serviceName, resourceSuffix)  // Custom stack
}
```

## Implementation Details

### Kubernetes Resource Changes

#### 1. Deployment Resource
```yaml
# Original staging deployment (unchanged)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-service
  namespace: staging
  labels:
    app: my-service
    environment: staging
spec:
  # ... deployment spec

---
# Custom stack deployment (new)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-service-staging-preview
  namespace: staging  # Same namespace!
  labels:
    app: my-service
    environment: staging-preview
    parentEnv: staging
spec:
  selector:
    matchLabels:
      app: my-service
      environment: staging-preview  # Different selector
  template:
    metadata:
      labels:
        app: my-service
        environment: staging-preview
    spec:
      # ... pod spec with different image
```

#### 2. Service Resource
```yaml
# Original staging service (unchanged)
apiVersion: v1
kind: Service
metadata:
  name: my-service
  namespace: staging
  annotations:
    caddy.ingress.kubernetes.io/domain: "staging.myapp.com"
spec:
  selector:
    app: my-service
    environment: staging

---
# Custom stack service (new)
apiVersion: v1
kind: Service
metadata:
  name: my-service-staging-preview
  namespace: staging
  annotations:
    caddy.ingress.kubernetes.io/domain: "staging-preview.myapp.com"
spec:
  selector:
    app: my-service
    environment: staging-preview  # Routes to custom deployment
```

#### 3. ConfigMap Resource
```yaml
# Original staging config (unchanged)
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-service-config
  namespace: staging
data:
  # staging-specific config

---
# Custom stack config (new, isolated)
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-service-staging-preview-config
  namespace: staging
data:
  # staging-preview-specific config
```

### Label Strategy

#### Standard Labels (All Resources)
```yaml
labels:
  app: my-service                    # Service name (consistent)
  environment: staging-preview       # Actual environment name
  parentEnv: staging                 # Parent environment (for custom stacks)
  simplecontainer.com/stack: staging-preview
  simplecontainer.com/service: my-service
```

#### Selector Strategy
```yaml
# Deployment selector
selector:
  matchLabels:
    app: my-service
    environment: staging-preview  # Unique per custom stack

# Service selector  
selector:
  app: my-service
  environment: staging-preview  # Routes to specific deployment
```

## Routing and Domain Management

### Independent Domain Configuration

Each custom stack gets its own domain configuration:

```yaml
# staging-preview service
metadata:
  annotations:
    caddy.ingress.kubernetes.io/domain: "staging-preview.myapp.com"
    caddy.ingress.kubernetes.io/path: "/"

# staging service (unchanged)
metadata:
  annotations:
    caddy.ingress.kubernetes.io/domain: "staging.myapp.com"
    caddy.ingress.kubernetes.io/path: "/"
```

### Caddy Configuration Generation

Caddy will see multiple services in the same namespace with different domains:

```caddyfile
# Generated Caddyfile
staging.myapp.com {
    reverse_proxy my-service.staging.svc.cluster.local:8080
}

staging-preview.myapp.com {
    reverse_proxy my-service-staging-preview.staging.svc.cluster.local:8080
}

staging-hotfix.myapp.com {
    reverse_proxy my-service-staging-hotfix.staging.svc.cluster.local:8080
}
```

## Scaling and Resource Management

### Independent HPA/VPA Resources

Each custom stack gets its own autoscaling resources:

```yaml
# Original staging HPA (unchanged)
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: my-service-hpa
  namespace: staging
spec:
  scaleTargetRef:
    name: my-service

---
# Custom stack HPA (new)
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: my-service-staging-preview-hpa
  namespace: staging
spec:
  scaleTargetRef:
    name: my-service-staging-preview  # Targets custom deployment
```

### Resource Isolation Benefits

1. **Independent Scaling**: Each deployment scales based on its own metrics
2. **Isolated Configuration**: Separate ConfigMaps prevent config conflicts
3. **Secret Isolation**: Each deployment has its own secrets
4. **Monitoring Separation**: Metrics and logs are tagged by environment

## Migration and Deployment Strategy

### Deployment Flow

1. **Determine Namespace**: Check if `parentEnv` is specified
2. **Generate Resource Names**: Add suffix for custom stacks
3. **Create Resources**: Deploy with environment-specific names and selectors
4. **Configure Routing**: Set up independent domain routing
5. **Validate Isolation**: Ensure no conflicts with parent resources

### Rollback Strategy

Custom stacks can be removed without affecting parent deployments:

```bash
# Remove custom stack (safe operation)
kubectl delete deployment my-service-staging-preview -n staging
kubectl delete service my-service-staging-preview -n staging
kubectl delete configmap my-service-staging-preview-config -n staging
# Parent staging deployment remains unaffected
```

## Benefits

### For Development Teams
- **Preview Environments**: Easy PR preview deployments
- **Hotfix Testing**: Test fixes alongside production
- **Feature Branches**: Independent feature testing
- **Cost Efficiency**: Share namespace resources

### For Operations
- **Resource Consolidation**: Fewer namespaces to manage
- **Simplified RBAC**: Same permissions for related environments
- **Monitoring Efficiency**: Related environments in same namespace
- **Network Policies**: Easier to configure shared policies

## Limitations and Considerations

### Current Limitations
- **Namespace Resource Limits**: All deployments share namespace quotas
- **Network Policies**: May need adjustment for proper isolation
- **Service Mesh**: May require additional configuration for traffic splitting

### Best Practices
- **Resource Naming**: Always use descriptive suffixes
- **Label Consistency**: Maintain consistent labeling strategy
- **Domain Management**: Use clear domain naming conventions
- **Monitoring**: Tag metrics with environment labels

## Future Enhancements

### Phase 2 Features
- **Traffic Splitting**: Route percentage of traffic to custom stacks
- **Auto-cleanup**: Automatic removal of stale preview environments
- **Resource Quotas**: Per-environment resource limits within namespace
- **Advanced Routing**: Path-based routing in addition to domain-based

### Integration Features
- **CI/CD Integration**: Automatic preview environment creation from PRs
- **Monitoring Dashboard**: Environment-specific metrics and logs
- **Cost Tracking**: Per-environment cost allocation
- **Security Scanning**: Independent security scans per environment

## Implementation Priority

### Phase 1: Core Functionality
1. Resource naming strategy implementation
2. Kubernetes resource generation with suffixes
3. Independent domain routing
4. Basic validation and conflict detection

### Phase 2: Advanced Features
1. Automatic cleanup mechanisms
2. Enhanced monitoring and observability
3. Advanced routing configurations
4. Integration with CI/CD pipelines

This design enables flexible deployment strategies while maintaining the simplicity and reliability that Simple Container users expect.
