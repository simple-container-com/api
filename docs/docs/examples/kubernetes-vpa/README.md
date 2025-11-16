---
title: Vertical Pod Autoscaler (VPA) Configuration
description: Complete guide to configuring Vertical Pod Autoscaler for automatic resource optimization in Kubernetes deployments
platform: platform
product: simple-container
category: example
subcategory: kubernetes
guides: examples
date: '2024-12-07'
---

# **Vertical Pod Autoscaler (VPA) Configuration**

This example demonstrates how to configure **Vertical Pod Autoscaler (VPA)** in Simple Container for automatic resource optimization in Kubernetes deployments.

## 🎯 **Overview**

VPA automatically adjusts CPU and memory requests for your containers based on actual usage patterns, providing:

- **Cost Optimization**: Prevents over-provisioning of resources
- **Performance Optimization**: Ensures adequate resources during load spikes
- **Operational Efficiency**: Reduces manual resource tuning

## 📁 **Example Structure**

```
kubernetes-vpa/
├── README.md                    # This documentation
├── .sc/
│   └── stacks/
│       ├── infrastructure/      # Parent stack (DevOps managed)
│       │   ├── server.yaml      # GKE Autopilot with Caddy VPA
│       │   └── secrets.yaml     # Authentication and encrypted secrets
│       └── vpa-demo/            # Client stack (Developer managed)
│           └── client.yaml      # Application with VPA configuration
├── docker-compose.yaml          # Local development environment
└── Dockerfile                   # Application container
```

## 🚀 **Quick Start**

1. **Deploy the parent stack** (GKE Autopilot with Caddy VPA):
   ```bash
   sc provision -s infrastructure -e production
   ```

2. **Deploy the application** with VPA:
   ```bash
   sc deploy -s vpa-demo -e production
   ```

3. **Monitor VPA recommendations**:
   ```bash
   kubectl get vpa
   kubectl describe vpa vpa-demo-vpa
   ```

## 📋 **Configuration Examples**

### **Application VPA Configuration** (`client.yaml`)

```yaml
# File: .sc/stacks/vpa-demo/client.yaml
schemaVersion: 1.0

stacks:
  production:
    type: cloud-compose
    parent: infrastructure
    config:
      dockerComposeFile: ./docker-compose.yaml
      uses: [mongodb]
      runs: [web-app]
      
      # VPA Configuration for automatic resource optimization
      cloudExtras:
        vpa:
          enabled: true
          updateMode: "Auto"  # Off, Initial, Auto, InPlaceOrRecreate
          minAllowed:
            cpu: "100m"
            memory: "128Mi"
          maxAllowed:
            cpu: "2"
            memory: "4Gi"
          controlledResources: ["cpu", "memory"]
      
      env:
        DATABASE_URL: "${resource:mongodb.uri}"
        NODE_ENV: "production"
      
      scale:
        min: 2
        max: 10
```

### **Infrastructure with Caddy VPA** (`server.yaml`)

```yaml
# File: .sc/stacks/infrastructure/server.yaml
schemaVersion: 1.0

# Deployment templates
templates:
  gke-autopilot:
    type: gcp-gke-autopilot
    config:
      projectId: "${auth:gcloud.projectId}"
      credentials: "${auth:gcloud}"
      gkeClusterResource: gke-cluster
      artifactRegistryResource: artifact-registry

# Infrastructure resources
resources:
  production:
    resources:
      # GKE Autopilot cluster with Caddy VPA
      gke-cluster:
        type: gcp-gke-autopilot-cluster
        config:
          projectId: "${auth:gcloud.projectId}"
          credentials: "${auth:gcloud}"
          location: "us-central1"
          gkeMinVersion: "1.33.4-gke.1245000"
          caddy:
            enable: true
            namespace: caddy
            replicas: 2
            # VPA Configuration for Caddy ingress controller
            vpa:
              enabled: true
              updateMode: "Auto"  # Recommended for ingress controllers (recreates pods)
              minAllowed:
                cpu: "50m"
                memory: "64Mi"
              maxAllowed:
                cpu: "1"
                memory: "1Gi"
              controlledResources: ["cpu", "memory"]
            # Optional: Manual resource limits alongside VPA
            resources:
              limits:
                cpu: "500m"
                memory: "512Mi"
              requests:
                cpu: "100m"
                memory: "128Mi"
      
      # Artifact Registry
      artifact-registry:
        type: gcp-artifact-registry
        config:
          projectId: "${auth:gcloud.projectId}"
          credentials: "${auth:gcloud}"
          location: "us-central1"
      
      # MongoDB Atlas
      mongodb:
        type: mongodb-atlas-cluster
        config:
          projectId: "${secret:mongodb-atlas-project-id}"
          publicKey: "${secret:mongodb-atlas-public-key}"
          privateKey: "${secret:mongodb-atlas-private-key}"
          clusterName: "vpa-demo-cluster"
          instanceSize: "M10"
          region: "US_CENTRAL_1"
```

## 🔧 **VPA Update Modes**

| Mode | Description | Use Case | Behavior |
|------|-------------|----------|----------|
| **Off** | Only provides recommendations | Testing and analysis | No automatic changes |
| **Initial** | Sets resources only at pod creation | Conservative approach | One-time resource setting |
| **Auto** | Updates by recreating pods | Recommended for stateless apps | Pod restart required |
| **InPlaceOrRecreate** | Updates resources in-place or recreates | Advanced use (preview) | May cause brief interruptions |

## 📊 **VPA Best Practices**

### **1. Resource Boundaries**

```yaml
cloudExtras:
  vpa:
    enabled: true
    # Prevent resource starvation
    minAllowed:
      cpu: "100m"      # Minimum for basic functionality
      memory: "128Mi"   # Minimum memory requirement
    # Control maximum costs
    maxAllowed:
      cpu: "4"          # Reasonable upper limit
      memory: "8Gi"     # Prevent memory exhaustion
```

### **2. Update Mode Selection**

```yaml
# For stateless applications
cloudExtras:
  vpa:
    updateMode: "Auto"        # Fast resource updates

# For critical services (like ingress controllers)
caddy:
  vpa:
    updateMode: "Auto"  # Safer pod recreation
```

### **3. Combining VPA with Manual Limits**

```yaml
# Manual resource specifications
resources:
  limits:
    cpu: "2"
    memory: "4Gi"
  requests:
    cpu: "500m"
    memory: "1Gi"

# VPA will adjust requests within these bounds
vpa:
  enabled: true
  maxAllowed:
    cpu: "2"      # Matches manual limit
    memory: "4Gi" # Matches manual limit
```

## 🔍 **Monitoring VPA**

### **Check VPA Status**
```bash
# List all VPAs
kubectl get vpa

# Get detailed VPA information
kubectl describe vpa <vpa-name>

# View VPA recommendations
kubectl get vpa <vpa-name> -o yaml
```

### **VPA Metrics**
```bash
# Check current resource usage
kubectl top pods

# Compare with VPA recommendations
kubectl get vpa <vpa-name> -o jsonpath='{.status.recommendation}'
```

## 🚨 **Troubleshooting**

### **VPA Not Creating Recommendations**
- Ensure VPA controller is installed in the cluster
- Check if pods have been running long enough (usually 24 hours)
- Verify resource usage patterns exist

### **Pods Not Being Updated**
- Check VPA update mode is not "Off"
- Verify VPA has sufficient permissions
- Ensure resource boundaries allow for changes

### **Resource Recommendations Too High/Low**
- Adjust `minAllowed` and `maxAllowed` boundaries
- Check if workload patterns are representative
- Consider using `controlledResources` to limit scope

## 🔗 **Related Examples**

- [GKE Autopilot Setup](../gke-autopilot/) - Basic GKE Autopilot configuration
- [Kubernetes Affinity](../kubernetes-affinity/) - Advanced pod scheduling
- [Resource Management](../kubernetes-native/) - Manual resource configuration

## 📚 **Additional Resources**

- [Kubernetes VPA Documentation](https://github.com/kubernetes/autoscaler/tree/master/vertical-pod-autoscaler)
- [GKE Autopilot VPA Guide](https://cloud.google.com/kubernetes-engine/docs/concepts/verticalpodautoscaler)
- [Simple Container VPA Reference](../../reference/supported-resources.md#vertical-pod-autoscaler-vpa)
