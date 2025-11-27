# Configuration Examples

## Basic Configuration

### Minimal Setup with Auto-Created Static IP

```yaml
# server.yaml
resources:
  production-cluster:
    type: gcp-gke-autopilot-cluster
    config:
      projectId: "my-production-project"
      location: "us-central1"
      gkeMinVersion: "1.28"
      
      # Enable external egress IP - Simple Container creates everything automatically!
      externalEgressIp:
        enabled: true
```

### Using Existing Static IP

```yaml
# server.yaml  
resources:
  staging-cluster:
    type: gcp-gke-autopilot-cluster
    config:
      projectId: "my-staging-project"
      location: "us-west1"
      gkeMinVersion: "1.28"
      
      # Use existing static IP
      externalEgressIp:
        enabled: true
        existing: "projects/my-staging-project/regions/us-west1/addresses/shared-egress-ip"
```

## Real-World Examples

### API Integration Cluster

```yaml
# server.yaml - Perfect for clusters that call external APIs
resources:
  api-integration-cluster:
    type: gcp-gke-autopilot-cluster
    config:
      projectId: "integration-project"
      location: "us-west2"
      gkeMinVersion: "1.28"
      
      # Enable static egress IP for API allowlisting
      externalEgressIp:
        enabled: true
        # Simple Container creates: static IP, router, NAT gateway automatically
```

### Database Access Cluster

```yaml
# server.yaml - For clusters accessing external databases with IP restrictions
resources:
  database-client-cluster:
    type: gcp-gke-autopilot-cluster
    config:
      projectId: "database-project"
      location: "us-east4"
      gkeMinVersion: "1.28"
      
      # All database connections will use this static IP
      externalEgressIp:
        enabled: true
```

## Multi-Environment Setup

### Development Environment

```yaml
# server.yaml (development)
resources:
  dev-cluster:
    type: gcp-gke-autopilot-cluster
    config:
      projectId: "dev-project"
      location: "us-central1-a"
      gkeMinVersion: "1.28"
      
      # Simple egress IP for development
      externalEgressIp:
        enabled: true
```

### Production Environment

```yaml
# server.yaml (production)
resources:
  prod-cluster:
    type: gcp-gke-autopilot-cluster
    config:
      projectId: "prod-project"
      location: "us-central1"
      gkeMinVersion: "1.28"
      
      # Same simple config - Simple Container optimizes for production automatically
      externalEgressIp:
        enabled: true
```

## Shared Infrastructure Examples

### Multiple Clusters with Shared Static IP

```yaml
# server.yaml - First cluster creates the static IP
resources:
  cluster-1:
    type: gcp-gke-autopilot-cluster
    config:
      projectId: "shared-project"
      location: "us-central1"
      gkeMinVersion: "1.28"
      
      # Creates new static IP automatically
      externalEgressIp:
        enabled: true

  # Second cluster uses the same static IP  
  cluster-2:
    type: gcp-gke-autopilot-cluster
    config:
      projectId: "shared-project"
      location: "us-central1"
      gkeMinVersion: "1.28"
      
      # References the static IP created by cluster-1
      externalEgressIp:
        enabled: true
        existing: "projects/shared-project/regions/us-central1/addresses/cluster-1-egress-ip"
```

## Migration Examples

### Adding External Egress IP to Existing Cluster

```yaml
# Before (existing cluster without external egress IP)
resources:
  existing-cluster:
    type: gcp-gke-autopilot-cluster
    config:
      projectId: "migration-project"
      location: "us-west1"
      gkeMinVersion: "1.28"
      # No externalEgressIp configuration

# After (add external egress IP to existing cluster)
resources:
  existing-cluster:
    type: gcp-gke-autopilot-cluster
    config:
      projectId: "migration-project"
      location: "us-west1"
      gkeMinVersion: "1.28"
      
      # Add external egress IP - zero downtime migration!
      externalEgressIp:
        enabled: true
```

## Validation Examples

### Valid Configurations

```yaml
# ✅ VALID: Enable with auto-created static IP
externalEgressIp:
  enabled: true

# ✅ VALID: Enable with existing static IP
externalEgressIp:
  enabled: true
  existing: "projects/my-project/regions/us-central1/addresses/existing-ip"

# ✅ VALID: Disabled (default behavior)
# Simply omit the externalEgressIp section entirely
```

### Invalid Configurations

```yaml
# ❌ INVALID: Missing enabled field
externalEgressIp:
  existing: "projects/my-project/regions/us-central1/addresses/existing-ip"

# ❌ INVALID: Invalid existing IP format
externalEgressIp:
  enabled: true
  existing: "just-an-ip-name"  # Must be full GCP resource path
```

## How It Works

### Automatic Integration with Client.yaml

```yaml
# client.yaml - No changes needed!
stacks:
  web-app:
    type: single-image
    config:
      image: "gcr.io/my-project/web-app:latest"
      port: 8080
      
      # All outbound traffic from this app automatically uses 
      # the static egress IP configured in server.yaml
```

The external egress IP configured in `server.yaml` automatically applies to **all workloads** deployed to the cluster, including applications defined in `client.yaml`. No additional configuration needed!

### What Simple Container Creates Automatically

When you enable `externalEgressIp`, Simple Container automatically creates:

1. **Static IP Address** - Reserved external IP for egress traffic
2. **Cloud Router** - Manages routing with production-ready defaults
3. **Cloud NAT Gateway** - Provides NAT with optimized port allocation
4. **Proper Dependencies** - Ensures resources are created in correct order

All with **zero additional configuration** required!
