# PriorityClassName Support for GKE/K8s - Analysis and Design

## Executive Summary

This document provides analysis and design for adding `priorityClassName` support to Simple Container's `cloudExtras` configuration. This feature will allow users to specify Kubernetes PriorityClass for their deployments, preventing preemption by system pods on GKE Autopilot and other Kubernetes platforms.

**Status**: Analysis complete. Implementation is feasible through `client.yaml` configuration only.

---

## Table of Contents
1. [Background](#background)
2. [Problem Statement](#problem-statement)
3. [Current State Analysis](#current-state-analysis)
4. [Proposed Solution](#proposed-solution)
5. [Implementation Design](#implementation-design)
6. [Configuration Schema](#configuration-schema)
7. [Code Changes Required](#code-changes-required)
8. [Validation & Testing](#validation--testing)
9. [Documentation Requirements](#documentation-requirements)

---

## Background

### What is Kubernetes PriorityClass?

Kubernetes `PriorityClass` is a mapping from a priority class name to an integer value (priority). Higher priority pods are scheduled before lower priority pods and can preempt lower priority pods when resources are scarce.

**Key Points:**
- Pods without a `priorityClassName` have a default priority of 0
- System critical pods typically have priorities of 2000000000+
- GKE Autopilot uses PriorityClass for workload management
- PriorityClasses are cluster-scoped resources that must exist before use

### GKE Autopilot Context

GKE Autopilot manages node resources dynamically and may preempt pods with default priority (0) to make room for higher-priority workloads. Users experiencing frequent preemption events need a way to signal their workload's importance to the Kubernetes scheduler.

---

## Problem Statement

### User Request
```
Workloads deployed via Simple Container on GKE Autopilot are currently susceptible
to preemption by "balloon pods" or higher-priority system tasks. This is because the
underlying Pods are created with a default priority of 0.
```

### Current Behavior
- Simple Container deployments create Pods with default priority (0)
- No mechanism exists in `client.yaml` to specify a `priorityClassName`
- Users cannot protect critical workloads from preemption

### Desired Behavior
- Users should be able to specify `priorityClassName` via `cloudExtras` in `client.yaml`
- The specified PriorityClass should be applied to the Pod spec
- No changes to server-side infrastructure code should be required

---

## Current State Analysis

### Existing Priority/Node Class Support

**Findings:**
1. **No existing `priorityClassName` support** - Extensive code search found no references to priorityClassName, PriorityClass, or priority-class in the codebase.

2. **Existing `computeClass` support** - The codebase already has `computeClass` in the `AffinityRules` struct (pkg/clouds/k8s/kube_run.go:38):
   ```go
   type AffinityRules struct {
       NodePool         *string `json:"nodePool" yaml:"nodePool"`
       ExclusiveNodePool *bool  `json:"exclusiveNodePool" yaml:"exclusiveNodePool"`
       ComputeClass     *string `json:"computeClass" yaml:"computeClass"`
       // ... other fields
   }
   ```
   This maps to the GKE-specific `cloud.google.com/compute-class` node selector, not to Kubernetes PriorityClass.

### CloudExtras Configuration Flow

The configuration flow for cloudExtras is:

```
client.yaml
    ↓
ReadClientDescriptor() (pkg/api/read.go)
    ↓
ToKubernetesRunConfig() / ToGkeAutopilotConfig()
    ↓
DeploymentConfig (pkg/clouds/k8s/types.go:18)
    ↓
SimpleContainerArgs (pkg/clouds/pulumi/kubernetes/simple_container.go:92)
    ↓
NewSimpleContainer() → PodSpecArgs → Kubernetes Deployment
```

### Current CloudExtras Structure

From `pkg/clouds/k8s/kube_run.go:19-29`:
```go
type CloudExtras struct {
    NodeSelector     map[string]string        `json:"nodeSelector" yaml:"nodeSelector"`
    DisruptionBudget *DisruptionBudget        `json:"disruptionBudget" yaml:"disruptionBudget"`
    RollingUpdate    *RollingUpdate           `json:"rollingUpdate" yaml:"rollingUpdate"`
    Affinity         *AffinityRules           `json:"affinity" yaml:"affinity"`
    Tolerations      []Toleration             `json:"tolerations" yaml:"tolerations"`
    VPA              *VPAConfig               `json:"vpa" yaml:"vpa"`
    ReadinessProbe   *CloudRunProbe           `json:"readinessProbe" yaml:"readinessProbe"`
    LivenessProbe    *CloudRunProbe           `json:"livenessProbe" yaml:"livenessProbe"`
    EphemeralVolumes []GenericEphemeralVolume `json:"ephemeralVolumes" yaml:"ephemeralVolumes"`
}
```

### PodSpec Creation Point

The Kubernetes Pod spec is created in `pkg/clouds/pulumi/kubernetes/simple_container.go:518-541`:
```go
podSpecArgs := &corev1.PodSpecArgs{
    NodeSelector: sdk.ToStringMap(args.NodeSelector),
    Affinity:     convertedAffinity,
    // ... other fields
    SecurityContext:    args.SecurityContext,
    ServiceAccountName: args.ServiceAccountName,
}
```

The `corev1.PodSpecArgs` struct from Pulumi's Kubernetes SDK already has a `PriorityClassName` field, so adding support only requires passing the value through.

---

## Proposed Solution

### Approach: Add `priorityClassName` to CloudExtras

Add a new field `priorityClassName` to the `CloudExtras` struct in `client.yaml`. This will:

1. **Allow client-side configuration only** - No server-side changes needed
2. **Follow existing patterns** - Similar to how `computeClass`, `nodeSelector`, etc. are handled
3. **Maintain simplicity** - A single string field that references an existing PriorityClass
4. **Be cloud-agnostic** - Works for any Kubernetes deployment, not just GKE Autopilot

### Why This Approach Works

| Criteria                  | Status                                      |
|---------------------------|---------------------------------------------|
| Client.yaml only          | ✅ Yes - all changes in config parsing       |
| No server changes         | ✅ Yes - no API modifications needed         |
| Follows existing patterns | ✅ Yes - similar to other cloudExtras fields |
| Cloud-agnostic            | ✅ Yes - Kubernetes native feature           |
| Simple implementation     | ✅ Yes - ~5 file changes                     |

---

## Implementation Design

### File Changes Required

#### 1. Add PriorityClassName Field to CloudExtras

**File:** `pkg/clouds/k8s/kube_run.go`

```go
type CloudExtras struct {
    NodeSelector     map[string]string        `json:"nodeSelector" yaml:"nodeSelector"`
    DisruptionBudget *DisruptionBudget        `json:"disruptionBudget" yaml:"disruptionBudget"`
    RollingUpdate    *RollingUpdate           `json:"rollingUpdate" yaml:"rollingUpdate"`
    Affinity         *AffinityRules           `json:"affinity" yaml:"affinity"`
    Tolerations      []Toleration             `json:"tolerations" yaml:"tolerations"`
    VPA              *VPAConfig               `json:"vpa" yaml:"vpa"`
    ReadinessProbe   *CloudRunProbe           `json:"readinessProbe" yaml:"readinessProbe"`
    LivenessProbe    *CloudRunProbe           `json:"livenessProbe" yaml:"livenessProbe"`
    EphemeralVolumes []GenericEphemeralVolume `json:"ephemeralVolumes" yaml:"ephemeralVolumes"`
    PriorityClassName *string                 `json:"priorityClassName" yaml:"priorityClassName"` // NEW
}
```

#### 2. Add PriorityClassName to DeploymentConfig

**File:** `pkg/clouds/k8s/types.go`

```go
type DeploymentConfig struct {
    StackConfig      *api.StackConfigCompose  `json:"stackConfig" yaml:"stackConfig"`
    Containers       []CloudRunContainer      `json:"containers" yaml:"containers"`
    IngressContainer *CloudRunContainer       `json:"ingressContainer" yaml:"ingressContainer"`
    Scale            *Scale                   `json:"scale" yaml:"scale"`
    Headers          *Headers                 `json:"headers" yaml:"headers"`
    TextVolumes      []SimpleTextVolume       `json:"textVolumes" yaml:"textVolumes"`
    DisruptionBudget *DisruptionBudget        `json:"disruptionBudget" yaml:"disruptionBudget"`
    RollingUpdate    *RollingUpdate           `json:"rollingUpdate" yaml:"rollingUpdate"`
    NodeSelector     map[string]string        `json:"nodeSelector" yaml:"nodeSelector"`
    Affinity         *AffinityRules           `json:"affinity" yaml:"affinity"`
    Tolerations      []Toleration             `json:"tolerations" yaml:"tolerations"`
    VPA              *VPAConfig               `json:"vpa" yaml:"vpa"`
    ReadinessProbe   *CloudRunProbe           `json:"readinessProbe" yaml:"readinessProbe"`
    LivenessProbe    *CloudRunProbe           `json:"livenessProbe" yaml:"livenessProbe"`
    EphemeralVolumes []GenericEphemeralVolume `json:"ephemeralVolumes" yaml:"ephemeralVolumes"`
    PriorityClassName *string                 `json:"priorityClassName" yaml:"priorityClassName"` // NEW
}
```

#### 3. Extract PriorityClassName from CloudExtras

**File:** `pkg/clouds/k8s/kube_run.go` - In `ToKubernetesRunConfig()` function around line 166:

```go
if stackCfg.CloudExtras != nil {
    k8sCloudExtras := &k8s.CloudExtras{}
    var err error
    k8sCloudExtras, err = api.ConvertDescriptor(stackCfg.CloudExtras, k8sCloudExtras)
    if err != nil {
        return nil, errors.Wrapf(err, "failed to convert cloudExtras field to Kubernetes Cloud extras format")
    }

    deployCfg.RollingUpdate = k8sCloudExtras.RollingUpdate
    deployCfg.DisruptionBudget = k8sCloudExtras.DisruptionBudget
    deployCfg.NodeSelector = k8sCloudExtras.NodeSelector
    deployCfg.VPA = k8sCloudExtras.VPA
    deployCfg.ReadinessProbe = k8sCloudExtras.ReadinessProbe
    deployCfg.LivenessProbe = k8sCloudExtras.LivenessProbe
    deployCfg.EphemeralVolumes = k8sCloudExtras.EphemeralVolumes
    deployCfg.PriorityClassName = k8sCloudExtras.PriorityClassName // NEW
    // ... rest of function
}
```

**File:** `pkg/clouds/gcloud/gke_autopilot.go` - In `ToGkeAutopilotConfig()` function around line 110:

```go
if stackCfg.CloudExtras != nil {
    k8sCloudExtras := &k8s.CloudExtras{}
    var err error
    k8sCloudExtras, err = api.ConvertDescriptor(stackCfg.CloudExtras, k8sCloudExtras)
    if err != nil {
        return nil, errors.Wrapf(err, "failed to convert cloudExtras field to Kubernetes Cloud extras format")
    }

    deployCfg.RollingUpdate = k8sCloudExtras.RollingUpdate
    deployCfg.DisruptionBudget = k8sCloudExtras.DisruptionBudget
    deployCfg.NodeSelector = k8sCloudExtras.NodeSelector
    deployCfg.Tolerations = k8sCloudExtras.Tolerations
    deployCfg.VPA = k8sCloudExtras.VPA
    deployCfg.ReadinessProbe = k8sCloudExtras.ReadinessProbe
    deployCfg.LivenessProbe = k8sCloudExtras.LivenessProbe
    deployCfg.PriorityClassName = k8sCloudExtras.PriorityClassName // NEW
    // ... rest of function
}
```

#### 4. Add PriorityClassName to SimpleContainerArgs

**File:** `pkg/clouds/pulumi/kubernetes/simple_container.go`

```go
type SimpleContainerArgs struct {
    // ... existing fields
    NodeSelector      map[string]string            `json:"nodeSelector" yaml:"nodeSelector"`
    Affinity          *k8s.AffinityRules           `json:"affinity" yaml:"affinity"`
    PriorityClassName *string                      `json:"priorityClassName" yaml:"priorityClassName"` // NEW
    // ... rest of fields
}
```

#### 5. Map to PodSpec PriorityClassName

**File:** `pkg/clouds/pulumi/kubernetes/simple_container.go` - In `NewSimpleContainer()` function around line 518:

```go
podSpecArgs := &corev1.PodSpecArgs{
    NodeSelector:       sdk.ToStringMap(args.NodeSelector),
    Affinity:           convertedAffinity,
    PriorityClassName:  sdk.ToStringPtr(args.PriorityClassName), // NEW
    // ... rest of fields
}
```

#### 6. Update JSON Schema

**File:** `docs/schemas/kubernetes/kubernetescloudextras.json`

Add the following property to the root of the schema properties:

```json
"priorityClassName": {
  "description": "Kubernetes PriorityClass to assign to pods. This affects pod scheduling and preemption behavior. Higher priority pods are scheduled before lower priority pods and can preempt them. The PriorityClass must already exist in the cluster. Common values include 'system-cluster-critical' (priority 2000000000), 'system-node-critical' (priority 2000000000), or custom PriorityClasses created by cluster administrators.",
  "type": "string",
  "examples": [
    "high-priority",
    "system-cluster-critical",
    "workload-production"
  ]
}
```

---

## Configuration Schema

### client.yaml Example

```yaml
stacks:
  production:
    type: cloud-compose
    config:
      runs: [streams]
      cloudExtras:
        priorityClassName: "high-priority-apps"
        # Optional: Combine with computeClass for Guaranteed QoS
        affinity:
          computeClass: "Balanced"
```

### Schema Validation Rules

1. **Type:** String
2. **Optional:** Yes (defaults to Kubernetes default priority of 0)
3. **Validation:**
   - Must be a valid Kubernetes resource name (RFC 1123)
   - Must reference an existing PriorityClass in the target cluster
   - Kubernetes will validate existence during apply

---

## Validation & Testing

### Unit Tests

Add tests in `pkg/clouds/k8s/kube_run_test.go`:

```go
func TestPriorityClassNameExtraction(t *testing.T) {
    cloudExtras := map[string]any{
        "priorityClassName": "high-priority",
    }

    result := &CloudExtras{}
    err := ConvertDescriptor(cloudExtras, result)

    assert.NoError(t, err)
    assert.Equal(t, "high-priority", *result.PriorityClassName)
}
```

### Integration Test Scenarios

1. **Basic PriorityClass Assignment**
   - Deploy with custom PriorityClass
   - Verify Pod spec contains `spec.priorityClassName`

2. **No PriorityClass (Default)**
   - Deploy without specifying priorityClassName
   - Verify Pod spec has no priorityClassName field (Kubernetes defaults to 0)

3. **System Critical PriorityClass**
   - Deploy with `system-cluster-critical`
   - Verify Pod is scheduled correctly

4. **Invalid PriorityClass**
   - Deploy with non-existent PriorityClass name
   - Verify Kubernetes returns appropriate error

### Example Test Command

```bash
# Create a test PriorityClass
kubectl apply -f - <<EOF
apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata:
  name: high-priority-apps
value: 1000
globalDefault: false
description: "High priority applications"
EOF

# Deploy Simple Container stack
sc deploy production

# Verify the priorityClassName is applied
kubectl get deployment -o yaml | grep priorityClassName
# Expected output: priorityClassName: high-priority-apps
```

---

## Documentation Requirements

### User Documentation Updates

1. **GKE Autopilot Guide** (`docs/gke-autopilot.md`)
   - Add section on using PriorityClass to prevent preemption
   - Include example configuration
   - Explain how to create custom PriorityClasses

2. **CloudExtras Reference** (`docs/cloud-extras-reference.md`)
   - Document the new `priorityClassName` field
   - Provide examples for different scenarios
   - Link to Kubernetes PriorityClass documentation

### Example Documentation Content

```markdown
### PriorityClassName

The `priorityClassName` field allows you to specify a Kubernetes PriorityClass for your pods.
This affects how the Kubernetes scheduler handles pod preemption and scheduling priority.

#### Usage

\`\`\`yaml
stacks:
  production:
    type: cloud-compose
    config:
      runs: [api]
      cloudExtras:
        priorityClassName: "high-priority-apps"
\`\`\`

#### Creating a PriorityClass

Before using a custom PriorityClass, it must exist in your cluster:

\`\`\`bash
kubectl apply -f - <<EOF
apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata:
  name: high-priority-apps
value: 1000
globalDefault: false
description: "High priority production applications"
EOF
\`\`\`

#### Common PriorityClasses

- `system-cluster-critical` (priority: 2000000000) - Cluster critical components
- `system-node-critical` (priority: 2000000000) - Node critical components
- Custom PriorityClasses with any integer value

#### Notes

- The PriorityClass must already exist in the cluster
- Higher priority pods can preempt lower priority pods
- Use with caution on GKE Autopilot to avoid affecting system workloads
\`\`\`
```

---

## Migration Notes

### For Existing Users

No migration required. The `priorityClassName` field is optional:
- Existing deployments will continue to work with default priority (0)
- Users can add the field when needed
- No breaking changes to existing configurations

### Rollout Strategy

1. **Phase 1:** Add support to `cloudExtras`
2. **Phase 2:** Deploy to test environment
3. **Phase 3:** Update documentation
4. **Phase 4:** Announce feature to users

---

## Security Considerations

### Risks

1. **Privilege Escalation:** Users could assign system-critical priorities to non-critical workloads
2. **Resource Starvation:** High-priority workloads could prevent system pods from running
3. **Denial of Service:** Malicious actors could monopolize cluster resources

### Mitigations

1. **RBAC:** Ensure only authorized users can modify `client.yaml`
2. **PriorityClass Management:** Cluster administrators should control which PriorityClasses exist
3. **Documentation:** Clearly communicate the impact of high-priority assignments
4. **Validation:** Consider adding validation for allowed PriorityClass names (optional)

---

## Alternatives Considered

### Alternative 1: Use computeClass for priority mapping
**Rejected:** `computeClass` is specifically for GKE's compute class selection (Balanced, Performance, etc.) and maps to a node selector, not pod priority.

### Alternative 2: Hardcode specific PriorityClasses
**Rejected:** Too restrictive. Users may have custom PriorityClasses with different names and values.

### Alternative 3: Server-side priority class management
**Rejected:** Adds unnecessary complexity. Kubernetes already handles PriorityClass creation. Users can create them manually or via Helm.

---

## Summary

This implementation adds `priorityClassName` support to Simple Container through `client.yaml` configuration only. The design:

- ✅ Requires only client-side changes
- ✅ Follows existing `cloudExtras` patterns
- ✅ Is cloud-agnostic (works with any Kubernetes deployment)
- ✅ Is simple and maintainable (~5 files to modify)
- ✅ Has clear testing and documentation requirements
- ✅ Maintains backward compatibility

**Estimated Implementation Effort:** 4-6 hours
**Risk Level:** Low (optional field, no breaking changes)
**Recommended Priority:** Medium (feature request from user experiencing production issues)

---

## References

- [Kubernetes Documentation: PriorityClass](https://kubernetes.io/docs/concepts/scheduling-eviction/pod-priority-preemption/#priorityclass)
- [GKE Autopilot Documentation](https://cloud.google.com/kubernetes-engine/docs/concepts/autopilot-overview)
- [Pulumi Kubernetes SDK: PodSpecArgs](https://www.pulumi.com/registry/packages/kubernetes/api-docs/core/v1/podspec/)
- Existing CloudExtras implementation in `pkg/clouds/k8s/`
