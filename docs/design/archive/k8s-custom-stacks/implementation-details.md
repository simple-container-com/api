# Implementation Details

> **Note**: The `ParentEnv` field already exists in Simple Container's codebase (`pkg/api/client.go`). This design builds upon the existing functionality to add proper Kubernetes resource isolation and naming for custom stacks.

## Core Logic Changes

### Environment Resolution

## Focus Areas

This implementation focuses on enhancing existing `ParentEnv` support specifically in:

1. **`pkg/clouds/pulumi/gcp/gke_autopilot.go`** - GKE Autopilot deployments
2. **`pkg/clouds/pulumi/kubernetes/kube_run.go`** - Direct Kubernetes deployments

The existing `ParentInfo` struct in `pkg/clouds/pulumi/api/cloud_provider.go` already provides the necessary context:

```go
// pkg/clouds/pulumi/api/cloud_provider.go - EXISTING
type ParentInfo struct {
    StackName         string
    ParentEnv         string // parent stack env - WE USE THIS
    StackEnv          string // current stack env
    ResourceEnv       string // environment where resource should be consumed
    FullReference     string
    DependsOnResource string
    UsesResource      string
}
```

### Resource Name Generation Logic

```go
// Enhanced resource naming using existing ParentInfo
func generateResourceName(serviceName, stackEnv, parentEnv string, resourceType string) string {
    baseName := serviceName
    
    // Add stack environment suffix for custom stacks (when parentEnv differs from stackEnv)
    if parentEnv != "" && parentEnv != stackEnv {
        baseName = fmt.Sprintf("%s-%s", serviceName, stackEnv)
    }
    
    // Add resource type suffix if specified
    if resourceType != "" {
        return fmt.Sprintf("%s-%s", baseName, resourceType)
    }
    
    return baseName
}

// Helper functions for specific resource types
func generateDeploymentName(serviceName, stackEnv, parentEnv string) string {
    return generateResourceName(serviceName, stackEnv, parentEnv, "")
}

func generateServiceName(serviceName, stackEnv, parentEnv string) string {
    return generateResourceName(serviceName, stackEnv, parentEnv, "")
}

func generateConfigMapName(serviceName, stackEnv, parentEnv string) string {
    return generateResourceName(serviceName, stackEnv, parentEnv, "config")
}

func generateSecretName(serviceName, stackEnv, parentEnv string) string {
    return generateResourceName(serviceName, stackEnv, parentEnv, "secrets")
}

func generateHPAName(serviceName, stackEnv, parentEnv string) string {
    return generateResourceName(serviceName, stackEnv, parentEnv, "hpa")
}
```

### Label Strategy

```go
// pkg/clouds/pulumi/kubernetes/simple_container.go - Add new constants
const (
    // Existing constants...
    LabelAppType = "appType"
    LabelAppName = "appName"
    LabelScEnv   = "appEnv"
    
    // NEW: ParentEnv support constants
    LabelParentEnv    = "simplecontainer.com/parent-env"
    LabelCustomStack  = "simplecontainer.com/custom-stack"
)

// Use existing appLabels from simple_container.go and enhance for ParentEnv
func enhanceLabelsForParentEnv(appLabels map[string]string, stackEnv, parentEnv string) map[string]string {
    // Start with existing appLabels (LabelAppType, LabelAppName, LabelScEnv)
    enhancedLabels := make(map[string]string)
    for k, v := range appLabels {
        enhancedLabels[k] = v
    }
    
    // Add parent environment label for custom stacks using constants
    if parentEnv != "" && parentEnv != stackEnv {
        enhancedLabels[LabelParentEnv] = parentEnv
        enhancedLabels[LabelCustomStack] = "true"
    }
    
    return enhancedLabels
}

// Selectors use existing label constants for consistency
func generateSelectors(serviceName, stackEnv string) map[string]string {
    return map[string]string{
        LabelAppName: serviceName,  // Use existing constant
        LabelScEnv:   stackEnv,     // This ensures unique selection per environment
    }
}
```

## Implementation in Target Files

### 1. GKE Autopilot Integration

```go
// pkg/clouds/pulumi/gcp/gke_autopilot.go - Enhanced for ParentEnv support

func GkeAutopilot(
    ctx *sdk.Context,
    gkeInput *gcloud.GkeAutopilotResource,
    params pApi.ProvisionParams,
) (*GkeAutopilotOut, error) {
    
    // Extract ParentEnv information from params
    var parentEnv string
    if params.ParentStack != nil {
        parentEnv = params.ParentStack.ParentEnv
    }
    stackEnv := params.StackParams.Environment
    
    // Determine namespace - use parentEnv if it's a custom stack
    namespace := stackEnv
    if parentEnv != "" && parentEnv != stackEnv {
        namespace = parentEnv  // Deploy to parent's namespace
    }
    
    // Generate resource names with environment-specific suffixes
    serviceName := params.StackParams.StackName // or extract from config
    deploymentName := generateDeploymentName(serviceName, stackEnv, parentEnv)
    
    // When calling SimpleContainer deployment, pass enhanced context:
    // 1. Use calculated namespace
    // 2. Use environment-specific deployment name
    // 3. The existing appLabels/appAnnotations will be enhanced automatically
    //    in the SimpleContainer function for ParentEnv support
    
    // Example integration point:
    simpleContainerArgs := &SimpleContainerArgs{
        Deployment: deploymentName,  // Environment-specific name
        Service:    serviceName,
        ScEnv:      stackEnv,        // Current stack environment
        // ... other existing args
    }
    
    // The SimpleContainer function will need to be enhanced to:
    // 1. Detect ParentEnv from params.ParentStack
    // 2. Use parentEnv namespace instead of stackEnv namespace
    // 3. Enhance appLabels with parent environment info using new constants:
    //    - LabelParentEnv: parentEnv
    //    - LabelCustomStack: "true"
}
```

### 2. Kubernetes Direct Deployment Integration

```go
// pkg/clouds/pulumi/kubernetes/kube_run.go - Enhanced for ParentEnv support

func KubeRun(
    ctx *sdk.Context,
    kubeInput *k8s.KubeRunResource,
    params pApi.ProvisionParams,
) (*KubeRunOut, error) {
    
    // Extract ParentEnv information from params
    var parentEnv string
    if params.ParentStack != nil {
        parentEnv = params.ParentStack.ParentEnv
    }
    stackEnv := params.StackParams.Environment
    
    // Determine namespace - use parentEnv if it's a custom stack
    namespace := stackEnv
    if parentEnv != "" && parentEnv != stackEnv {
        namespace = parentEnv  // Deploy to parent's namespace
    }
    
    // Generate resource names with environment-specific suffixes
    serviceName := params.StackParams.StackName
    deploymentName := generateDeploymentName(serviceName, stackEnv, parentEnv)
    serviceName := generateServiceName(serviceName, stackEnv, parentEnv)
    
    // Pass enhanced naming to existing SimpleContainer deployment
    // The key changes are:
    // 1. Use calculated namespace
    // 2. Use environment-specific resource names
    // 3. Pass parentEnv context for labeling
    
    // Rest of existing KubeRun logic remains the same...
}
```

## Key Implementation Points

### Changes Required

1. **In `simple_container.go`**:
   - Add new label constants: `LabelParentEnv` and `LabelCustomStack`
   - Enhance `appLabels` generation to include ParentEnv info when applicable
   - Modify namespace logic to use parent's namespace for custom stacks

2. **In `gke_autopilot.go`**:
   - Extract `ParentEnv` from `params.ParentStack.ParentEnv`
   - Calculate target namespace (parent's namespace for custom stacks)
   - Generate environment-specific resource names
   - Pass enhanced context to existing deployment functions

3. **In `kube_run.go`**:
   - Same ParentEnv extraction logic
   - Same namespace calculation
   - Same resource naming enhancements
   - Integration with existing SimpleContainer deployment logic

### No New Types Needed

The implementation leverages:
- Existing `ParentInfo` struct
- Existing `StackParams` structure  
- Existing deployment functions
- Simple helper functions for naming and labeling

### Minimal Changes

This approach requires minimal changes to existing code:
- Add ParentEnv-aware naming logic
- Enhance namespace determination
- Pass additional context to existing functions
- No breaking changes to existing functionality
