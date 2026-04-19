# GKE Autopilot: Pod Priority and Large Storage Example

This example demonstrates how to configure **pod priority classes** and **generic ephemeral volumes** for GKE Autopilot deployments. These features are essential for production workloads that require:

- **Preemption protection** - Prevent critical workloads from being evicted
- **Large temporary storage** - Support >10GB temp storage (bypasses GKE Autopilot limit)

## Configuration

- **PriorityClassName**: High priority pods to prevent preemption
- **Generic Ephemeral Volumes**: Large temporary storage (up to 64TB)
- **VPA**: Vertical Pod Autoscaler for resource optimization
- **Pod Disruption Budget**: High availability configuration

## Use Cases

### When to Use PriorityClassName

- **Critical production services** - Prevent preemption by system pods
- **Streaming platforms** - Avoid service interruption during node pressure
- **Data processing pipelines** - Ensure job completion
- **ML model serving** - Maintain availability for inference endpoints

### When to Use Generic Ephemeral Volumes

- **N8N workflows** - Binary data processing (>10GB)
- **Container build systems** - Large build artifacts
- **ML model training** - Dataset caching and model checkpoints
- **Data processing** - Large temporary datasets
- **Media transcoding** - Temporary video files

## Prerequisites

### 1. Create PriorityClass (One-Time Setup)

Before deploying, create the PriorityClass in your cluster:

```bash
kubectl apply -f - <<EOF
apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata:
  name: production-high-priority
value: 10000
globalDefault: false
description: "High priority production workloads that should not be preempted"
EOF
```

### 2. Verify PriorityClass

```bash
kubectl get priorityclass
# NAME                      VALUE        GLOBAL-DEFAULT   AGE
# production-high-priority   10000        false             1m
# system-cluster-critical   2000000000   false             12d
# system-node-critical      2000000000   false             12d
```

## Client Configuration

### client.yaml with Priority and Storage

```yaml
---
# File: ".sc/stacks/data-processor/client.yaml"
schemaVersion: 1.0

stacks:
  production:
    type: cloud-compose
    parent: myproject/devops
    config:
      domain: data-processor.myproject.com
      dockerComposeFile: ./docker-compose.yaml
      runs: [data-processor]

      # Pod Priority Configuration
      cloudExtras:
        # High priority to prevent preemption
        priorityClassName: "production-high-priority"

        # Generic ephemeral volumes for large temp storage
        ephemeralVolumes:
          - name: processing-cache
            mountPath: /tmp/cache
            size: 50Gi
            storageClassName: pd-ssd

          - name: data-staging
            mountPath: /tmp/staging
            size: 200Gi
            storageClassName: pd-balanced

        # Vertical Pod Autoscaler
        vpa:
          enabled: true
          updateMode: "Auto"
          minAllowed:
            cpu: "500m"
            memory: "512Mi"
          maxAllowed:
            cpu: "4"
            memory: "8Gi"
          controlledResources: ["cpu", "memory"]

        # Pod disruption budget for high availability
        disruptionBudget:
          minAvailable: 2

        # Rolling update strategy
        rollingUpdate:
          maxSurge: 1
          maxUnavailable: 0
```

## Deployment

Deploy the stack:

```bash
sc deploy -s data-processor -e production
```

## Verification

### Check Pod Priority

```bash
kubectl get pods -o yaml | grep priorityClassName
# Output: priorityClassName: production-high-priority
```

### Check Pod Scheduling Priority

```bash
kubectl get pods -o jsonpath='{.items[0].status.priority}')
# Output: 10000
```

### Check Ephemeral Volumes

```bash
kubectl get pvc
# NAME                                      STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS    AGE
# processing-cache-data-processor-abc123   Bound    pvc-12345678-1234-5678-1234-567812345678   50Gi       RWO            pd-ssd          5m
# data-staging-data-processor-abc123        Bound    pvc-87654321-4321-8765-4321-876543210987   200Gi      RWO            pd-balanced    5m
```

### Verify Pod Mounts

```bash
kubectl exec -it data-processor-xyz123 -- df -h /tmp/cache /tmp/staging
# Filesystem      Size  Used Avail Use% Mounted on
# /dev/xvda       50G   10G   40G  20% /tmp/cache
# /dev/xvdb      200G   50G  150G  25% /tmp/staging
```

## Storage Class Options

| Storage Class | Performance | Cost | Use Case |
|--------------|-------------|------|----------|
| `standard-rwo` | Standard SSD | Low | General purpose, cost-sensitive |
| `pd-balanced` | Balanced | Medium | **Recommended for most workloads** |
| `pd-ssd` | High performance SSD | High | I/O intensive, caching |
| `pd-extreme` | Ultra high performance | Very High | Latency-critical applications |

## Priority Value Guidelines

| Priority Range | Use Case | Example |
|---------------|----------|---------|
| 1000000000+ | System critical | Avoid using |
| 100000 - 999999999 | Very high priority | Payment processing, critical infrastructure |
| 10000 - 99999 | High priority | Production APIs, streaming platforms |
| 1000 - 9999 | Important production | Data processing, batch jobs |
| 1 - 999 | Regular production | Development, testing |

## Complete Example: Multi-Configuration

Here's a complete example showing different configurations for different environments:

```yaml
stacks:
  # Production - High priority + large storage
  production:
    config:
      cloudExtras:
        priorityClassName: "production-high-priority"
        ephemeralVolumes:
          - name: cache
            mountPath: /tmp/cache
            size: 100Gi
            storageClassName: pd-balanced
        vpa:
          enabled: true
          updateMode: "Auto"

  # Staging - Medium priority + smaller storage
  staging:
    config:
      cloudExtras:
        priorityClassName: "staging-medium-priority"
        ephemeralVolumes:
          - name: cache
            mountPath: /tmp/cache
            size: 20Gi
            storageClassName: standard-rwo
        vpa:
          enabled: true
          updateMode: "Initial"

  # Development - No priority + minimal storage
  development:
    config:
      cloudExtras:
        ephemeralVolumes:
          - name: cache
            mountPath: /tmp/cache
            size: 5Gi
            storageClassName: standard-rwo
```

## Best Practices

### PriorityClassName

✅ **Do:**
- Use priority classes for critical production workloads
- Create separate priority classes for different service tiers
- Document priority class values in your runbook
- Test with lower priorities before using system-critical

❌ **Don't:**
- Use system-critical priority classes for non-system workloads
- Set all workloads to high priority (defeats the purpose)
- Use extremely high priority values unnecessarily

### Ephemeral Volumes

✅ **Do:**
- Delete pods promptly when not needed to free storage
- Use appropriate storage classes for your workload
- Monitor PVC usage and cleanup
- Set size limits based on actual needs

❌ **Don't:**
- Use ephemeral volumes for permanent data
- Oversize volumes unnecessarily (costs money)
- Use pd-extreme unless latency is truly critical

## Troubleshooting

### PriorityClass Not Working

**Problem:** Pods still getting preempted despite priorityClassName

**Solutions:**
1. Verify PriorityClass exists: `kubectl get priorityclass`
2. Check pod spec: `kubectl get pod -o yaml | grep priorityClassName`
3. Verify priority value: `kubectl get pod -o jsonpath='{.status.priority}'`
4. Check for conflicting priority settings

### Ephemeral Volume Issues

**Problem:** PVC not being created

**Solutions:**
1. Check storage class exists: `kubectl get storageclass`
2. Verify quota allows PVC creation
3. Check GKE Autopilot resource limits
4. Review pod events: `kubectl describe pod`

**Problem:** High storage costs

**Solutions:**
1. Delete unused pods promptly
2. Use smaller volume sizes
3. Use standard-rwo instead of pd-ssd
4. Consider compression for data at rest

## Related Documentation

- [GKE Autopilot Guide](../../../guides/parent-gcp-gke-autopilot.md)
- [PriorityClass Documentation](https://kubernetes.io/docs/concepts/scheduling-eviction/pod-priority-preemption/)
- [Generic Ephemeral Volumes](https://kubernetes.io/docs/concepts/storage/generic-ephemeral-volumes/)
- [GKE Storage Classes](https://cloud.google.com/kubernetes-engine/docs/concepts/persistent-volumes#storageclasses)
