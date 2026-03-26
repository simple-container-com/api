# Scale Configuration Examples for Kubernetes HPA

This document provides comprehensive examples of Simple Container's cloud-agnostic scale configuration that automatically enables HPA for Kubernetes deployments while remaining compatible with ECS and other platforms.

## Basic Examples

### 1. Simple CPU-Based Scaling

**Use Case**: Basic web application that scales based on CPU usage

```yaml
# client.yaml - Uses existing Simple Container scale pattern
stacks:
  web-app:
    type: single-image
    config:
      scale:
        min: 2          # HPA minReplicas
        max: 10         # HPA maxReplicas  
        policy:
          cpu:
            max: 70     # Scale when CPU > 70%
```

**Generated HPA Resource**:
```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: web-app-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: web-app
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

### 2. Memory-Based Scaling

**Use Case**: Memory-intensive application (data processing, caching)

```yaml
# client.yaml - Simple Container pattern
stacks:
  data-processor:
    type: single-image
    config:
      size:
        limits:
          cpu: "1000"
          memory: "2048"
        requests:
          cpu: "500"
          memory: "1024"
      scale:
        min: 1
        max: 8
        policy:
          memory:
            max: 80     # Scale when Memory > 80%
```

### 3. Multi-Metric Scaling

**Use Case**: Application that needs to scale on both CPU and memory

```yaml
# client.yaml - Simple Container pattern
stacks:
  api-server:
    type: single-image
    config:
      size:
        limits:
          cpu: "2000"
          memory: "4096"
        requests:
          cpu: "500"
          memory: "1024"
      scale:
        min: 3
        max: 20
        policy:
          cpu:
            max: 60     # Scale when CPU > 60%
          memory:
            max: 75     # OR when Memory > 75%
```

**Behavior**: HPA will scale up when **either** CPU exceeds 60% **or** Memory exceeds 75%.

### 4. Static Scaling (No Autoscaling)

**Use Case**: Background worker with fixed capacity requirements

```yaml
# client.yaml - When min == max, no HPA is created
stacks:
  background-worker:
    type: single-image
    config:
      scale:
        min: 5
        max: 5          # Fixed 5 replicas, no autoscaling
```

**Behavior**: Deployment will have exactly 5 replicas. No HPA resource is created.

## Platform Compatibility Examples

### 5. ECS Compatible Configuration

**Use Case**: Same configuration works for both Kubernetes and ECS

```yaml
# client.yaml - Works for both K8s and ECS
stacks:
  cross-platform-app:
    type: single-image
    config:
      scale:
        min: 2
        max: 10
        policy:
          cpu:
            max: 70
```

**Platform Behavior**:
- **Kubernetes/GKE**: Creates HPA with minReplicas=2, maxReplicas=10, CPU target=70%
- **AWS ECS**: Creates autoscaling target with min=2, max=10, CPU target=70%
- **Static deployment**: Uses max=10 as fixed replica count

## Migration Examples

### 6. Migrating from Static Scaling

**Before (Static Scaling)**:
```yaml
# client.yaml - Current approach
stacks:
  legacy-app:
    type: single-image
    config:
      scale:
        min: 5
        max: 5  # Same as min = static scaling
```

**After (Enable Autoscaling)**:
```yaml
# client.yaml - Enable autoscaling
stacks:
  legacy-app:
    type: single-image
    config:
      scale:
        min: 5          # Keep same minimum
        max: 15         # Allow scaling up
        policy:
          cpu:
            max: 70     # Scale when CPU > 70%
```

## Validation Examples

### 7. Common Configuration Errors

**Missing Resource Requests**:
```yaml
# ❌ INVALID - No resource requests defined
stacks:
  invalid-app:
    type: single-image
    config:
      scale:
        min: 2
        max: 10
        policy:
          cpu:
            max: 70
```

**Error**: `CPU resource requests must be defined when using CPU-based scaling`

**Fixed Version**:
```yaml
# ✅ VALID - Resource requests defined
stacks:
  valid-app:
    type: single-image
    config:
      size:
        requests:
          cpu: "100m"
          memory: "128Mi"
      scale:
        min: 2
        max: 10
        policy:
          cpu:
            max: 70
```

## Best Practices

### 8. Production-Ready Configuration

**Use Case**: Production application with comprehensive scaling strategy

```yaml
# client.yaml - Production example
stacks:
  production-api:
    type: single-image
    config:
      size:
        limits:
          cpu: "2000m"
          memory: "4Gi"
        requests:
          cpu: "500m"
          memory: "1Gi"
      scale:
        min: 5          # Always maintain minimum capacity
        max: 50         # Allow significant scaling
        policy:
          cpu:
            max: 60     # Conservative CPU target
          memory:
            max: 70     # Conservative memory target
      cloudExtras:
        disruptionBudget:
          maxUnavailable: 2    # Maintain availability during scaling
```

### 9. Development Environment

**Use Case**: Development environment with minimal resources

```yaml
# client.yaml - Development example
stacks:
  dev-api:
    type: single-image
    config:
      size:
        requests:
          cpu: "100m"
          memory: "256Mi"
      scale:
        min: 1          # Minimal resources in dev
        max: 3          # Limited scaling
        policy:
          cpu:
            max: 80     # Higher threshold for dev
```

## Key Benefits

### ✅ **Simple Configuration**
- Uses existing Simple Container scale patterns
- No HPA-specific configuration needed
- Cloud-agnostic approach

### ✅ **Maximum Compatibility**
- Same configuration works across Kubernetes, ECS, and static deployments
- Backward compatible with existing configurations
- Follows Simple Container's "less configuration, maximum impact" philosophy

### ✅ **Reasonable Defaults**
- Automatic HPA creation when `min != max` and policy is defined
- Smart detection of scaling requirements
- No boilerplate configuration required

These examples demonstrate how Simple Container's existing scale configuration naturally enables horizontal autoscaling for Kubernetes deployments while maintaining compatibility with other platforms and following SC's core principles of simplicity and cloud-agnostic design.
