# PriorityClassName Data Flow Diagram

## Configuration Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           client.yaml                                        │
│                                                                             │
│  stacks:                                                                    │
│    production:                                                              │
│      type: cloud-compose                                                    │
│      config:                                                                │
│        runs: [streams]                                                      │
│        cloudExtras:                                                         │
│          priorityClassName: "high-priority-apps"   ───┐                     │
│          affinity:                                   │                     │
│            computeClass: "Balanced"                  │                     │
└──────────────────────────────────────────────────────│─────────────────────┘
                                                       │
                                                       │ (priorityClassName)
                                                       ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                     ReadClientDescriptor()                                  │
│                   (pkg/api/read.go)                                         │
│                                                                             │
│  Parses YAML → StackClientDescriptor                                       │
└─────────────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                ToGkeAutopilotConfig() / ToKubernetesRunConfig()            │
│          (pkg/clouds/gcloud/gke_autopilot.go)                               │
│          (pkg/clouds/k8s/kube_run.go)                                       │
│                                                                             │
│  Extracts cloudExtras → CloudExtras struct                                  │
│                                                                             │
│  deployCfg.PriorityClassName = k8sCloudExtras.PriorityClassName            │
└─────────────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                       DeploymentConfig                                      │
│                     (pkg/clouds/k8s/types.go)                               │
│                                                                             │
│  type DeploymentConfig struct {                                             │
│      ...                                                                    │
│      PriorityClassName *string  // ← NEW FIELD                              │
│  }                                                                          │
└─────────────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                       SimpleContainerArgs                                   │
│        (pkg/clouds/pulumi/kubernetes/simple_container.go)                   │
│                                                                             │
│  type SimpleContainerArgs struct {                                          │
│      ...                                                                    │
│      PriorityClassName *string  // ← NEW FIELD                              │
│  }                                                                          │
└─────────────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                      NewSimpleContainer()                                    │
│        (pkg/clouds/pulumi/kubernetes/simple_container.go)                   │
│                                                                             │
│  podSpecArgs := &corev1.PodSpecArgs{                                        │
│      ...                                                                    │
│      PriorityClassName: sdk.ToStringPtr(args.PriorityClassName)  ← NEW     │
│  }                                                                          │
└─────────────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Kubernetes Deployment                                    │
│                                                                             │
│  apiVersion: apps/v1                                                        │
│  kind: Deployment                                                           │
│  spec:                                                                      │
│    template:                                                                │
│      spec:                                                                  │
│        priorityClassName: high-priority-apps  ← MAPPED TO POD SPEC          │
│        containers: [...]                                                    │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Modified Files Summary

```
pkg/clouds/k8s/kube_run.go          + CloudExtras.PriorityClassName field
                                   + Extract priorityClassName from cloudExtras

pkg/clouds/k8s/types.go            + DeploymentConfig.PriorityClassName field

pkg/clouds/gcloud/gke_autopilot.go + Extract priorityClassName from cloudExtras
                                   (for GKE Autopilot deployments)

pkg/clouds/pulumi/kubernetes/
  simple_container.go              + SimpleContainerArgs.PriorityClassName field
                                   + Map to PodSpecArgs.PriorityClassName

docs/schemas/kubernetes/
  kubernetescloudextras.json       + priorityClassName property definition
```

## Key Implementation Points

### 1. Field Addition Pattern
Following the exact same pattern as other cloudExtras fields like `VPA`, `ReadinessProbe`, `LivenessProbe`:

```go
// In CloudExtras struct
PriorityClassName *string `json:"priorityClassName" yaml:"priorityClassName"`

// In ToKubernetesRunConfig()
deployCfg.PriorityClassName = k8sCloudExtras.PriorityClassName

// In SimpleContainerArgs
PriorityClassName *string `json:"priorityClassName" yaml:"priorityClassName"`

// In NewSimpleContainer()
PriorityClassName: sdk.ToStringPtr(args.PriorityClassName),
```

### 2. Pulumi SDK Compatibility
The Pulumi Kubernetes SDK's `corev1.PodSpecArgs` already has the `PriorityClassName` field:

```go
// From Pulumi's corev1.PodSpecArgs
type PodSpecArgs struct {
    ...
    PriorityClassName *string `pulumi:"priorityClassName"`
    ...
}
```

This means no adapter code is needed - we just pass the string through directly.

### 3. Pointer Usage
Using `*string` (pointer) instead of `string` allows the field to be optional:
- `nil` → No priorityClassName set (Kubernetes default priority = 0)
- `"high-priority"` → Sets the specified PriorityClass

## Testing Strategy

```
┌─────────────────────────────────────────────────────────────────┐
│                       Unit Tests                                │
│                                                                  │
│  TestPriorityClassNameExtraction()                              │
│  TestPriorityClassNameNilHandling()                             │
│  TestPriorityClassNameMappingToPodSpec()                        │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Integration Tests                            │
│                                                                  │
│  Deploy with priorityClassName → Verify Pod spec                │
│  Deploy without priorityClassName → Verify default behavior     │
│  Deploy with invalid PriorityClass → Verify error handling      │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                   E2E Tests (Optional)                          │
│                                                                  │
│  Create PriorityClass → Deploy → Verify scheduling priority     │
│  Test preemption scenario with mixed priorities                  │
└─────────────────────────────────────────────────────────────────┘
```

## Validation Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          Validation Stages                                  │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────┐     ┌─────────────────┐     ┌─────────────────────────┐
│  Config Parsing │────▶│  Schema Check   │────▶│   Kubernetes Apply      │
│                 │     │                 │     │                         │
│ • Valid YAML    │     │ • Type: string  │     │ • PriorityClass exists │
│ • Optional      │     │ • Optional      │     │ • Permission to use     │
└─────────────────┘     └─────────────────┘     └─────────────────────────┘
                                                       │
                                                       ▼
                                              ┌─────────────────┐
                                              │   Runtime       │
                                              │   Validation    │
                                              │                 │
                                              │ • K8s Scheduler │
                                              │   checks value  │
                                              └─────────────────┘
```

## Error Scenarios

| Scenario | Where Detected | Error Message |
|----------|---------------|---------------|
| Invalid YAML | Config Parsing | YAML parse error |
| Non-string value | Schema Check | type mismatch error |
| Non-existent PriorityClass | Kubernetes Apply | `priorityClassName not found` |
| Missing RBAC permission | Kubernetes Apply | `forbidden: attempted to grant privileges` |
