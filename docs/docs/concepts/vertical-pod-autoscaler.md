---
title: Vertical Pod Autoscaler (VPA) Concepts
description: Understanding Vertical Pod Autoscaler configuration and best practices in Simple Container
platform: platform
product: simple-container
category: devguide
subcategory: learning
guides: tutorials
date: '2024-12-07'
---

# **Vertical Pod Autoscaler (VPA) in Simple Container**

## **Overview**

**Vertical Pod Autoscaler (VPA)** automatically adjusts CPU and memory requests for your containers based on actual usage patterns. Simple Container provides built-in VPA support for both application deployments and infrastructure components like Caddy ingress controllers.

## **Key Benefits**

### **💰 Cost Optimization**
- **Prevents over-provisioning**: Reduces wasted resources and cloud costs
- **Right-sizing**: Automatically adjusts to actual usage patterns
- **Resource efficiency**: Optimizes cluster utilization

### **🚀 Performance Optimization**
- **Prevents resource starvation**: Ensures adequate resources during load spikes
- **Automatic scaling**: Responds to changing workload demands
- **Reduced manual tuning**: Eliminates guesswork in resource allocation

### **🔧 Operational Efficiency**
- **Hands-off management**: Reduces manual resource configuration
- **Data-driven decisions**: Based on actual usage metrics
- **Continuous optimization**: Adapts to changing application behavior

## **VPA Configuration Levels**

Simple Container supports VPA configuration at multiple levels:

### **1. Application Level** (`client.yaml`)

Configure VPA for your applications using `cloudExtras`:

```yaml
# client.yaml
stacks:
  production:
    config:
      cloudExtras:
        vpa:
          enabled: true
          updateMode: "Auto"
          minAllowed:
            cpu: "100m"
            memory: "128Mi"
          maxAllowed:
            cpu: "2"
            memory: "4Gi"
```

### **2. Infrastructure Level** (`server.yaml`)

Configure VPA for infrastructure components like Caddy:

```yaml
# server.yaml
resources:
  production:
    resources:
      gke-cluster:
        type: gcp-gke-autopilot-cluster
        config:
          caddy:
            vpa:
              enabled: true
              updateMode: "Auto"
              minAllowed:
                cpu: "50m"
                memory: "64Mi"
```

## **VPA Update Modes**

Understanding VPA update modes is crucial for production deployments:

### **Off Mode**
```yaml
vpa:
  updateMode: "Off"
```
- **Behavior**: Only provides resource recommendations
- **Use case**: Testing, analysis, and planning
- **Impact**: No automatic changes to running pods

### **Initial Mode**
```yaml
vpa:
  updateMode: "Initial"
```
- **Behavior**: Sets resources only when pods are created
- **Use case**: Conservative approach for critical applications
- **Impact**: New pods get optimized resources, existing pods unchanged

### **Auto Mode**
```yaml
vpa:
  updateMode: "Auto"
```
- **Behavior**: Updates resources by recreating pods (equivalent to Recreate mode)
- **Use case**: Recommended for stateless applications and ingress controllers
- **Impact**: Brief service interruption during pod recreation

### **InPlaceOrRecreate Mode** (Preview)
```yaml
vpa:
  updateMode: "InPlaceOrRecreate"
```
- **Behavior**: Updates resources in-place when possible, recreates if needed
- **Use case**: Advanced scenarios with minimal disruption tolerance
- **Impact**: May cause brief interruptions for some resource changes

## **Resource Boundaries**

VPA resource boundaries prevent runaway resource allocation:

### **Minimum Allowed Resources**
```yaml
vpa:
  minAllowed:
    cpu: "100m"      # Prevent resource starvation
    memory: "128Mi"   # Ensure basic functionality
```

### **Maximum Allowed Resources**
```yaml
vpa:
  maxAllowed:
    cpu: "4"          # Control maximum costs
    memory: "8Gi"     # Prevent memory exhaustion
```

### **Controlled Resources**
```yaml
vpa:
  controlledResources: ["cpu", "memory"]  # Specify which resources to manage
```

## **VPA Best Practices**

### **1. Environment-Specific Configuration**

Use different VPA settings for different environments:

```yaml
# Production: Aggressive optimization
production:
  cloudExtras:
    vpa:
      updateMode: "Auto"
      maxAllowed:
        cpu: "4"
        memory: "8Gi"

# Staging: Conservative approach
staging:
  cloudExtras:
    vpa:
      updateMode: "Initial"
      maxAllowed:
        cpu: "2"
        memory: "4Gi"

# Development: Recommendation only
development:
  cloudExtras:
    vpa:
      updateMode: "Off"
      maxAllowed:
        cpu: "1"
        memory: "2Gi"
```

### **2. Combining VPA with Manual Limits**

VPA works alongside manual resource specifications:

```yaml
# Manual resource limits
resources:
  limits:
    cpu: "2"
    memory: "4Gi"
  requests:
    cpu: "500m"    # VPA will adjust this
    memory: "1Gi"  # VPA will adjust this

# VPA configuration
vpa:
  enabled: true
  maxAllowed:
    cpu: "2"      # Matches manual limit
    memory: "4Gi" # Matches manual limit
```

### **3. Ingress Controller Considerations**

For critical infrastructure like Caddy ingress controllers:

```yaml
caddy:
  vpa:
    enabled: true
    updateMode: "Auto"  # Safer for ingress controllers
    minAllowed:
      cpu: "50m"              # Ensure minimum availability
      memory: "64Mi"
    maxAllowed:
      cpu: "1"                # Reasonable upper bound
      memory: "1Gi"
```

## **Monitoring VPA**

### **Check VPA Status**
```bash
# List all VPAs in the cluster
kubectl get vpa

# Get detailed information about a specific VPA
kubectl describe vpa <vpa-name>

# View current recommendations
kubectl get vpa <vpa-name> -o yaml
```

### **Understanding VPA Output**
```yaml
status:
  recommendation:
    containerRecommendations:
    - containerName: web-app
      target:
        cpu: "250m"     # Recommended CPU request
        memory: "512Mi" # Recommended memory request
      upperBound:
        cpu: "500m"     # Upper confidence bound
        memory: "1Gi"
      lowerBound:
        cpu: "100m"     # Lower confidence bound
        memory: "256Mi"
```

## **Common Patterns**

### **Microservices Pattern**
```yaml
# Each microservice with appropriate VPA settings
web-api:
  cloudExtras:
    vpa:
      enabled: true
      updateMode: "Auto"
      maxAllowed:
        cpu: "1"
        memory: "2Gi"

background-worker:
  cloudExtras:
    vpa:
      enabled: true
      updateMode: "Auto"
      maxAllowed:
        cpu: "2"
        memory: "4Gi"
```

### **Multi-Tenant Pattern**
```yaml
# Different VPA settings per tenant
tenant-a:
  cloudExtras:
    vpa:
      maxAllowed:
        cpu: "2"
        memory: "4Gi"

tenant-enterprise:
  cloudExtras:
    vpa:
      maxAllowed:
        cpu: "8"
        memory: "16Gi"
```

## **Troubleshooting**

### **VPA Not Providing Recommendations**
- Ensure VPA controller is installed in the cluster
- Check if pods have sufficient runtime (usually 24+ hours)
- Verify resource usage patterns exist

### **Recommendations Too High/Low**
- Adjust `minAllowed` and `maxAllowed` boundaries
- Check if workload patterns are representative
- Consider using `controlledResources` to limit scope

### **Pods Not Being Updated**
- Verify VPA update mode is not "Off"
- Check VPA has sufficient RBAC permissions
- Ensure resource boundaries allow for changes

## **Integration with Simple Container**

VPA integrates seamlessly with Simple Container's architecture:

- **Parent Stack**: DevOps configures VPA for infrastructure (Caddy, operators)
- **Client Stack**: Developers configure VPA for applications
- **Environment Separation**: Different VPA settings per environment
- **Resource Sharing**: VPA optimizes shared infrastructure resources

This separation ensures that VPA configuration follows Simple Container's principle of separation of concerns while providing automatic resource optimization across the entire stack.

## **Next Steps**

- [VPA Configuration Example](../examples/kubernetes-vpa/) - Complete VPA setup example
- [GKE Autopilot Guide](../guides/parent-gcp-gke-autopilot.md) - VPA with GKE Autopilot
- [Supported Resources Reference](../reference/supported-resources.md) - VPA configuration reference
