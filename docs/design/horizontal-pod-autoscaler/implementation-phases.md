# HPA Implementation Phases - Cloud-Agnostic Approach

## Phase 1: Enhanced Scale Configuration Support (Week 1-2)

### 1.1 Kubernetes HPA Detection Logic

**Files to Modify:**
```
pkg/clouds/k8s/types.go
```

**Implementation Steps:**

1. **Update ToScale function**:
   ```go
   func ToScale(stack *api.StackConfigCompose) *Scale {
       if stack.Scale != nil {
           // Detect if autoscaling should be enabled
           shouldAutoscale := stack.Scale.Min != stack.Scale.Max && 
                            (stack.Scale.Policy != nil && 
                             (stack.Scale.Policy.Cpu != nil || stack.Scale.Policy.Memory != nil))
           
           return &Scale{
               Replicas:     stack.Scale.Min, // Use min as base replica count
               EnableHPA:    shouldAutoscale,
               MinReplicas:  stack.Scale.Min,
               MaxReplicas:  stack.Scale.Max,
               CPUTarget:    getCPUTarget(stack.Scale.Policy),
               MemoryTarget: getMemoryTarget(stack.Scale.Policy),
           }
       }
       return nil
   }
   ```

2. **Add helper functions**:
   ```go
   func getCPUTarget(policy *api.StackConfigComposeScalePolicy) *int {
       if policy != nil && policy.Cpu != nil {
           return &policy.Cpu.Max
       }
       return nil
   }
   
   func getMemoryTarget(policy *api.StackConfigComposeScalePolicy) *int {
       if policy != nil && policy.Memory != nil {
           return &policy.Memory.Max
       }
       return nil
   }
   ```

3. **Extend Scale struct**:
   ```go
   type Scale struct {
       Replicas     int  `json:"replicas" yaml:"replicas"`
       EnableHPA    bool `json:"enableHPA" yaml:"enableHPA"`
       MinReplicas  int  `json:"minReplicas" yaml:"minReplicas"`
       MaxReplicas  int  `json:"maxReplicas" yaml:"maxReplicas"`
       CPUTarget    *int `json:"cpuTarget" yaml:"cpuTarget"`
       MemoryTarget *int `json:"memoryTarget" yaml:"memoryTarget"`
   }
   ```

**Testing:**
- Unit tests for scale configuration detection
- Validation of HPA enablement logic
- Backward compatibility tests

### 1.2 HPA Resource Generation

**New File**: `pkg/clouds/pulumi/kubernetes/hpa.go`

**Core Functions:**
```go
type HPAArgs struct {
    Name         string
    Namespace    string
    Deployment   *appsv1.Deployment
    MinReplicas  int
    MaxReplicas  int
    CPUTarget    *int
    MemoryTarget *int
    Labels       map[string]string
}

func CreateHPA(ctx *pulumi.Context, args *HPAArgs, opts ...pulumi.ResourceOption) (*autoscalingv2.HorizontalPodAutoscaler, error) {
    // Build metrics from CPU and Memory targets
    var metrics []autoscalingv2.MetricSpecArgs
    
    if args.CPUTarget != nil {
        metrics = append(metrics, autoscalingv2.MetricSpecArgs{
            Type: pulumi.String("Resource"),
            Resource: &autoscalingv2.ResourceMetricSourceArgs{
                Name: pulumi.String("cpu"),
                Target: &autoscalingv2.MetricTargetArgs{
                    Type:               pulumi.String("Utilization"),
                    AverageUtilization: pulumi.IntPtr(*args.CPUTarget),
                },
            },
        })
    }
    
    if args.MemoryTarget != nil {
        metrics = append(metrics, autoscalingv2.MetricSpecArgs{
            Type: pulumi.String("Resource"),
            Resource: &autoscalingv2.ResourceMetricSourceArgs{
                Name: pulumi.String("memory"),
                Target: &autoscalingv2.MetricTargetArgs{
                    Type:               pulumi.String("Utilization"),
                    AverageUtilization: pulumi.IntPtr(*args.MemoryTarget),
                },
            },
        })
    }
    
    // Create HPA resource
    hpaName := fmt.Sprintf("%s-hpa", args.Name)
    return autoscalingv2.NewHorizontalPodAutoscaler(ctx, hpaName, &autoscalingv2.HorizontalPodAutoscalerArgs{
        Metadata: &metav1.ObjectMetaArgs{
            Name:      pulumi.String(hpaName),
            Namespace: pulumi.String(args.Namespace),
            Labels:    pulumi.ToStringMap(args.Labels),
        },
        Spec: &autoscalingv2.HorizontalPodAutoscalerSpecArgs{
            ScaleTargetRef: &autoscalingv2.CrossVersionObjectReferenceArgs{
                ApiVersion: pulumi.String("apps/v1"),
                Kind:       pulumi.String("Deployment"),
                Name:       args.Deployment.Metadata.Name(),
            },
            MinReplicas: pulumi.Int(args.MinReplicas),
            MaxReplicas: pulumi.Int(args.MaxReplicas),
            Metrics:     autoscalingv2.MetricSpecArray(metrics),
        },
    }, opts...)
}
```

### 1.3 Integration with Deployment

**File**: `pkg/clouds/pulumi/kubernetes/deployment.go`

**Integration Point:**
```go
func CreateDeployment(ctx *pulumi.Context, args *DeploymentArgs, opts ...pulumi.ResourceOption) (*DeploymentResult, error) {
    // ... existing deployment creation ...
    
    result := &DeploymentResult{
        Deployment: deployment,
        Service:    service,
    }
    
    // Create HPA if autoscaling is enabled
    if args.Deployment.Scale != nil && args.Deployment.Scale.EnableHPA {
        hpaArgs := &HPAArgs{
            Name:         deploymentName,
            Namespace:    args.Namespace,
            Deployment:   deployment,
            MinReplicas:  args.Deployment.Scale.MinReplicas,
            MaxReplicas:  args.Deployment.Scale.MaxReplicas,
            CPUTarget:    args.Deployment.Scale.CPUTarget,
            MemoryTarget: args.Deployment.Scale.MemoryTarget,
            Labels:       args.Labels,
        }
        
        hpa, err := CreateHPA(ctx, hpaArgs, opts...)
        if err != nil {
            return nil, errors.Wrapf(err, "failed to create HPA for deployment %s", deploymentName)
        }
        
        result.HPA = hpa
        ctx.Export(fmt.Sprintf("%s-hpa-name", deploymentName), hpa.Metadata.Name())
    }
    
    return result, nil
}
```

### 1.4 Validation Logic

**New File**: `pkg/clouds/k8s/scale_validation.go`

**Validation Functions:**
```go
func ValidateScaleConfiguration(scale *api.StackConfigComposeScale, resources *Resources) error {
    if scale == nil {
        return nil
    }
    
    // Basic validations
    if scale.Min <= 0 {
        return errors.New("scale.min must be greater than 0")
    }
    
    if scale.Max < scale.Min {
        return errors.New("scale.max must be greater than or equal to scale.min")
    }
    
    // If autoscaling is configured, validate resource requirements
    if scale.Min != scale.Max && scale.Policy != nil {
        return validateAutoscalingRequirements(scale.Policy, resources)
    }
    
    return nil
}

func validateAutoscalingRequirements(policy *api.StackConfigComposeScalePolicy, resources *Resources) error {
    if policy.Cpu != nil {
        if policy.Cpu.Max <= 0 || policy.Cpu.Max > 100 {
            return errors.New("CPU scaling threshold must be between 1 and 100")
        }
        
        if resources == nil || resources.Requests["cpu"] == "" {
            return errors.New("CPU resource requests must be defined when using CPU-based scaling")
        }
    }
    
    if policy.Memory != nil {
        if policy.Memory.Max <= 0 || policy.Memory.Max > 100 {
            return errors.New("Memory scaling threshold must be between 1 and 100")
        }
        
        if resources == nil || resources.Requests["memory"] == "" {
            return errors.New("Memory resource requests must be defined when using memory-based scaling")
        }
    }
    
    return nil
}
```

## Phase 2: Production Ready (Week 3-4)

### 2.1 GKE-Specific Optimizations

**New File**: `pkg/clouds/gcp/gke_hpa_optimizer.go`

**GKE-Specific Features:**
```go
type GKEHPAOptimizer struct {
    clusterInfo *GKEClusterInfo
}

type GKEClusterInfo struct {
    IsAutopilot bool
    Region      string
    ProjectID   string
}

func (g *GKEHPAOptimizer) OptimizeScaleConfig(scale *Scale) *Scale {
    if !scale.EnableHPA {
        return scale
    }
    
    optimized := *scale
    
    if g.clusterInfo.IsAutopilot {
        // Autopilot-specific optimizations
        // - More conservative scaling for node provisioning delays
        // - Adjust thresholds for Autopilot characteristics
        optimized = g.optimizeForAutopilot(optimized)
    }
    
    return &optimized
}

func (g *GKEHPAOptimizer) optimizeForAutopilot(scale Scale) Scale {
    // Autopilot nodes take longer to provision, so be more conservative
    if scale.CPUTarget != nil && *scale.CPUTarget < 60 {
        newTarget := 60
        scale.CPUTarget = &newTarget
    }
    
    if scale.MemoryTarget != nil && *scale.MemoryTarget < 70 {
        newTarget := 70
        scale.MemoryTarget = &newTarget
    }
    
    return scale
}
```

### 2.2 Comprehensive Testing

**Test Files:**
- `pkg/clouds/k8s/scale_test.go`
- `pkg/clouds/pulumi/kubernetes/hpa_test.go`
- `pkg/clouds/k8s/scale_validation_test.go`

**Test Categories:**
1. **Configuration Detection Tests**
2. **HPA Resource Generation Tests**
3. **Validation Tests**
4. **Integration Tests**
5. **Platform-Specific Tests**

### 2.3 Documentation and Examples

**Files to Create:**
```
docs/docs/examples/kubernetes-scaling/
├── README.md
├── basic-cpu-scaling/
│   ├── README.md
│   ├── client.yaml
│   └── docker-compose.yaml
├── memory-scaling/
│   ├── README.md
│   ├── client.yaml
│   └── docker-compose.yaml
├── multi-metric-scaling/
│   ├── README.md
│   ├── client.yaml
│   └── docker-compose.yaml
└── production-ready/
    ├── README.md
    ├── client.yaml
    └── server.yaml
```

**Guide Updates:**
- `docs/docs/guides/kubernetes-autoscaling.md`
- `docs/docs/reference/configuration-reference.md`
- Update existing scaling documentation

## Implementation Timeline

### Week 1: Core Implementation
- [ ] Update `ToScale` function with HPA detection logic
- [ ] Create HPA resource generation function
- [ ] Integrate HPA creation with deployment
- [ ] Basic validation logic
- [ ] Unit tests for core functionality

### Week 2: Integration and Testing
- [ ] Integration tests with mock Kubernetes API
- [ ] Validation tests for edge cases
- [ ] Backward compatibility verification
- [ ] Performance testing
- [ ] Documentation updates

### Week 3: Production Features
- [ ] GKE-specific optimizations
- [ ] Comprehensive test suite
- [ ] Example configurations
- [ ] Migration guides
- [ ] Error handling improvements

### Week 4: Polish and Release
- [ ] Performance optimization
- [ ] Final documentation review
- [ ] Integration testing with real clusters
- [ ] Release preparation
- [ ] User feedback incorporation

## Success Criteria

### Phase 1 Success Criteria
- [ ] HPA automatically created when `min != max` and policy defined
- [ ] CPU and Memory metrics work correctly
- [ ] Existing configurations continue working unchanged
- [ ] Unit tests achieve 90%+ coverage

### Phase 2 Success Criteria
- [ ] GKE Autopilot optimizations functional
- [ ] Complete test coverage
- [ ] Documentation complete and accurate
- [ ] Performance benchmarks met
- [ ] Zero breaking changes confirmed

## Key Benefits of This Approach

### ✅ **Zero Configuration Overhead**
- Uses existing Simple Container scale configuration
- No new fields or HPA-specific knowledge required
- Automatic detection based on configuration patterns

### ✅ **Cloud-Agnostic Design**
- Same configuration works for Kubernetes HPA and ECS autoscaling
- Maintains Simple Container's platform independence
- No Kubernetes-specific configuration exposed to users

### ✅ **Backward Compatibility**
- All existing scale configurations continue working
- Gradual migration path available
- No breaking changes to existing APIs

### ✅ **Production Ready**
- Comprehensive validation and error handling
- Platform-specific optimizations where beneficial
- Follows Simple Container's reliability standards

This implementation approach transforms Simple Container's existing scale configuration into a powerful, cloud-agnostic autoscaling system while maintaining the platform's core principles of simplicity and compatibility.
