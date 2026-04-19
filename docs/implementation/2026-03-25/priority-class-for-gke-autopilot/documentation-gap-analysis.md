# Kubernetes Features Documentation Gap Analysis

**Date:** 2026-03-25
**Purpose:** Identify undocumented or under-documented Kubernetes features in Simple Container

---

## Summary of Recent Features (from git log)

| Commit | Feature | Status | Documentation Status |
|--------|---------|--------|---------------------|
| bc0de33 | priorityClassName support | ✅ Implemented | ❌ Missing |
| cea218e | Generic Ephemeral Volumes (>10GB) | ✅ Implemented | ❌ Missing |
| a336188 | Health probe HTTP headers | ✅ Implemented | ⚠️ Partial |
| 9aca265 | Ephemeral storage size pass-through | ✅ Implemented | ⚠️ Partial |

---

## Documentation Gaps

### 1. priorityClassName Support

**What:** Allows users to specify Kubernetes PriorityClass to prevent pod preemption
**Status:** Implemented in `pkg/clouds/k8s/`
**Documentation:** Missing from user guides

**Implementation Details:**
- Added `PriorityClassName` field to `CloudExtras` struct
- Mapped to PodSpec's `priorityClassName` field
- Supports custom and system PriorityClasses

**Where to Document:**
- `docs/docs/guides/parent-gcp-gke-autopilot.md` - Add section after VPA
- `docs/docs/examples/gke-autopilot/` - Create example showing priority class usage

**Required Content:**
- What is PriorityClass and why it matters
- How to create a PriorityClass in Kubernetes
- How to configure it in client.yaml
- Use cases for GKE Autopilot (prevention of preemption)
- Available system PriorityClasses

---

### 2. Generic Ephemeral Volumes

**What:** Support for large temporary storage (>10GB) on GKE Autopilot
**Status:** Implemented in `pkg/clouds/k8s/`
**Documentation:** Missing from user guides

**Implementation Details:**
- Type: `GenericEphemeralVolume` in `pkg/clouds/k8s/types.go`
- Supports PVCs up to 64TB with `pd-balanced` storage class
- Automatically created/deleted with pods (truly ephemeral)
- Configured via `ephemeralVolumes` array in cloudExtras

**Where to Document:**
- `docs/docs/guides/parent-gcp-gke-autopilot.md` - Add section to cloudExtras
- `docs/docs/concepts/` - Consider creating a dedicated storage concept doc

**Required Content:**
- What are Generic Ephemeral Volumes
- Why they're needed (GKE Autopilot 10GB limit)
- How to configure ephemeralVolumes in cloudExtras
- Storage class options (standard-rwo, pd-balanced, etc.)
- Use cases (N8N, ML training, data processing, container builds)
- Comparison with regular ephemeral storage

---

### 3. Health Probe HTTP Headers

**What:** Support for custom HTTP headers in readiness/liveness/startup probes
**Status:** Implemented
**Documentation:** Has implementation docs, missing from main guides

**Implementation Details:**
- `httpHeaders` field in `CloudRunProbe` struct
- Supports multiple headers per probe
- Useful for authentication, multi-tenancy, custom routing

**Where to Document:**
- `docs/docs/guides/parent-gcp-gke-autopilot.md` - Add to health probe section
- Update existing probe examples

**Required Content:**
- How to add HTTP headers to health probes
- Use cases (multi-tenant routing, authentication)
- Examples for different header scenarios

---

## Current Documentation Structure

### Existing Files

| File | Current Coverage | What's Missing |
|------|------------------|----------------|
| `docs/docs/guides/parent-gcp-gke-autopilot.md` | VPA, basic cloudExtras | priorityClassName, ephemeralVolumes, httpHeaders |
| `docs/docs/concepts/vertical-pod-autoscaler.md` | VPA concepts | Complete |
| `docs/docs/examples/kubernetes-affinity/` | Affinity examples | Could add priority class examples |

### CloudExtras Reference Table (Current vs. Complete)

**Current documentation (from parent-gcp-gke-autopilot.md):**

| Field | Documented |
|-------|------------|
| nodeSelector | ✅ Yes |
| disruptionBudget | ✅ Yes |
| rollingUpdate | ✅ Yes |
| affinity | ✅ Yes |
| tolerations | ✅ Yes |
| vpa | ✅ Yes |
| readinessProbe | ✅ Yes |
| livenessProbe | ✅ Yes |
| **priorityClassName** | ❌ **Missing** |
| **ephemeralVolumes** | ❌ **Missing** |
| **httpHeaders** (in probes) | ❌ **Missing** |

---

## Documentation Plan

### Phase 1: Update GKE Autopilot Guide

**File:** `docs/docs/guides/parent-gcp-gke-autopilot.md`

**Add Section:** `# **🔟 Advanced Configuration: Pod Priority and Preemption Control``

Insert after the VPA section (around line 400):

```markdown
# **🔟 Advanced Configuration: Pod Priority and Preemption Control**

## **What is PriorityClass?**

Kubernetes **PriorityClass** allows you to specify the importance of pods relative to other pods. When resources are scarce, higher priority pods are:
- Scheduled before lower priority pods
- Able to preempt lower priority pods if necessary

On **GKE Autopilot**, this is critical for preventing your workloads from being preempted by system pods or other cluster tasks.

## **Default Behavior**

Without a PriorityClass, pods are created with **priority 0** (the default). This means:
- System critical pods (priority: 2000000000) will preempt your pods
- Your pods may be evicted during node pressure
- "Balloon pods" can displace your workloads

## **Creating a PriorityClass**

Before using priorityClassName, create a PriorityClass in your cluster:

```bash
kubectl apply -f - <<EOF
apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata:
  name: high-priority-apps
value: 1000
globalDefault: false
description: "High priority production applications"
EOF
```

## **Configuring priorityClassName in client.yaml**

```yaml
stacks:
  production:
    type: cloud-compose
    parent: myproject/devops
    config:
      dockerComposeFile: ./docker-compose.yaml
      runs: [streams]

      cloudExtras:
        priorityClassName: "high-priority-apps"
```

## **System PriorityClasses**

GKE provides built-in PriorityClasses:

| PriorityClass | Value | Use Case |
|--------------|-------|----------|
| `system-cluster-critical` | 2000000000 | Cluster-critical components (use with caution) |
| `system-node-critical` | 2000000000 | Node-critical components (use with caution) |

⚠️ **Warning:** Only use system-critical PriorityClasses if your workload is truly critical to cluster operation.

## **Priority Value Guidelines**

| Priority Range | Use Case |
|---------------|----------|
| 1000000000+ | System critical (avoid using) |
| 100000 - 999999999 | High priority production workloads |
| 1000 - 99999 | Important production services |
| 1 - 999 | Regular production workloads |
| 0 (default) | Development/testing environments |

## **Example: Preventing Preemption on GKE Autopilot**

```yaml
stacks:
  production:
    config:
      cloudExtras:
        # Prevent preemption with high priority
        priorityClassName: "production-high-priority"

        # Combine with other settings for robust workloads
        vpa:
          enabled: true
          updateMode: "Auto"
        disruptionBudget:
          minAvailable: 2
```
```

**Add Section:** `# **1️⃣1️⃣ Advanced Configuration: Large Temporary Storage`**

Insert after the priorityClassName section:

```markdown
# **1️⃣1️⃣ Advanced Configuration: Large Temporary Storage**

## **What are Generic Ephemeral Volumes?**

**Generic Ephemeral Volumes** provide **truly temporary storage** that:
- Supports sizes up to **64TB** (vs 10GB limit for regular ephemeral storage)
- Creates a PersistentVolumeClaim **automatically for each pod**
- **Deletes the PVC when the pod is deleted** (truly ephemeral)
- Is fully compatible with **GKE Autopilot** constraints

## **Why You Need This**

GKE Autopilot **hard-limits** regular ephemeral storage to **10GB maximum**. This limitation:
- Cannot be increased through configuration
- Cannot be bypassed with VPA
- Creates bottlenecks for applications needing more temp storage

## **Use Cases**

- **N8N** - Binary data processing workflows
- **Container build systems** - Intermediate build artifacts
- **ML model training** - Dataset caching and model checkpoints
- **Data processing pipelines** - Large temporary datasets
- **Media transcoding** - Temporary video processing files

## **Configuring Ephemeral Volumes**

Add `ephemeralVolumes` to your `cloudExtras`:

```yaml
stacks:
  production:
    config:
      dockerComposeFile: ./docker-compose.yaml
      runs: [data-processor]

      cloudExtras:
        # Large temporary storage for data processing
        ephemeralVolumes:
          - name: temp-data
            mountPath: /tmp/data
            size: 100Gi                # Up to 64TB supported!
            storageClassName: pd-balanced  # Optional: defaults to cluster default
```

## **Storage Class Options**

| Storage Class | Description | Best For |
|--------------|-------------|----------|
| `standard-rwo` | Standard SSD | General purpose |
| `pd-balanced` | Balanced performance/performance | Most workloads (recommended) |
| `pd-ssd` | High performance SSD | I/O intensive workloads |
| `pd-extreme` | Ultra high performance | Latency-critical applications |

## **Multiple Volumes**

You can specify multiple ephemeral volumes:

```yaml
cloudExtras:
  ephemeralVolumes:
    - name: build-cache
      mountPath: /tmp/build
      size: 50Gi
      storageClassName: pd-ssd
    - name: data-staging
      mountPath: /tmp/staging
      size: 200Gi
      storageClassName: pd-balanced
```

## **Comparison: Ephemeral Storage Options**

| Feature | Regular Ephemeral | Generic Ephemeral Volumes |
|---------|------------------|---------------------------|
| Max Size (GKE Autopilot) | 10GB | 64TB |
| PVC Management | N/A | Automatic |
| Cleanup on Pod Delete | Yes | Yes |
| Storage Class Selection | No | Yes |
| Use Case | Small temp files | Large temp datasets |

## **Cost Considerations**

- PVCs are billed per GB-month regardless of usage
- Delete pods promptly when not needed to free storage
- Consider using smaller sizes with autoscaling for cost optimization
```

---

### Phase 2: Update Health Probe Section

**File:** `docs/docs/guides/parent-gcp-gke-autopilot.md`

**Add to existing health probe section (around line 572):**

```markdown
### **HTTP Headers in Health Probes**

Health probes support custom HTTP headers for advanced scenarios:

```yaml
cloudExtras:
  readinessProbe:
    httpGet:
      path: "/health"
      port: 8080
      httpHeaders:
        - name: "X-Health-Check-Token"
          value: "secret-token-123"
        - name: "X-Tenant-ID"
          value: "tenant-abc"
    initialDelaySeconds: 10
```

**Use Cases for HTTP Headers:**
- **Multi-tenant routing** - Route health checks to correct tenant
- **Authentication** - Bypass auth for health check endpoints
- **Custom routing** - Direct health checks through proxies/load balancers
```

---

### Phase 3: Create New Example

**File:** `docs/docs/examples/gke-autopilot/priority-and-storage/README.md`

Create a complete example showing:
- PriorityClass creation
- Generic ephemeral volumes usage
- Combined configuration for production workloads

---

## Implementation Documentation Files

Existing implementation docs that should be referenced:

| Implementation Doc | Feature | Location |
|--------------------|---------|----------|
| `priority-class-for-gke-autopilot/` | priorityClassName | `docs/implementation/2026-03-25/` |
| `health-probe-http-headers/` | httpHeaders | `docs/implementation/2026-02-26/` |

**Note:** No implementation docs found for ephemeral volumes in docs/implementation/*

---

## Summary of Required Changes

### Files to Update

1. **`docs/docs/guides/parent-gcp-gke-autopilot.md`** (PRIMARY)
   - Add priorityClassName section
   - Add ephemeralVolumes section
   - Update health probes with httpHeaders

2. **`docs/docs/examples/gke-autopilot/`** (NEW EXAMPLE)
   - Create priority-and-storage example

### Content Structure

Each new section should include:
- **What is it?** - Clear explanation
- **Why use it?** - Problem/solution context
- **How to configure?** - YAML examples
- **Use cases** - Practical scenarios
- **Best practices** - Guidelines and warnings

---

## Validation Checklist

After documentation updates:

- [ ] All cloudExtras fields documented
- [ ] Code examples are tested and working
- [ ] YAML formatting is consistent
- [ ] Cross-references between sections
- [ ] Links to Kubernetes official docs where relevant
- [ ] GKE Autopilot specific guidance included

---

## Next Steps

1. Review this plan with stakeholders
2. Prioritize documentation updates (priorityClassName is highest priority based on recent implementation)
3. Create draft content for each section
4. Technical review of examples
5. Merge and publish documentation
