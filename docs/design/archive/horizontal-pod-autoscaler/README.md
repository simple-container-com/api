# Horizontal Pod Autoscaler (HPA) Support Design

## Overview

This design document outlines the implementation plan for adding Horizontal Pod Autoscaler (HPA) support to Simple Container's Kubernetes deployments. HPA will enable automatic scaling of pods based on CPU utilization, memory usage, and custom metrics for both plain Kubernetes and Google Kubernetes Engine (GKE) deployments.

## Current State Analysis

### Existing Scaling Infrastructure

Simple Container currently supports:

1. **Static Scaling**: Fixed replica count via `Scale` struct
   ```go
   type Scale struct {
       Replicas int `json:"replicas" yaml:"replicas"`
       // TODO: support autoscaling
   }
   ```

2. **Vertical Pod Autoscaler (VPA)**: Already implemented
   ```go
   type VPAConfig struct {
       Enabled             bool                     `json:"enabled" yaml:"enabled"`
       UpdateMode          *string                  `json:"updateMode" yaml:"updateMode"`
       MinAllowed          *VPAResourceRequirements `json:"minAllowed" yaml:"minAllowed"`
       MaxAllowed          *VPAResourceRequirements `json:"maxAllowed" yaml:"maxAllowed"`
       ControlledResources []string                 `json:"controlledResources" yaml:"controlledResources"`
   }
   ```

3. **ECS Fargate Autoscaling**: Already implemented for AWS
   - CPU-based scaling policies
   - Memory-based scaling policies (partially)
   - Target tracking scaling configuration

### Gaps Identified

1. **No HPA Support**: Kubernetes deployments lack horizontal autoscaling
2. **Static Replica Management**: Only fixed replica counts supported
3. **Missing Metrics Integration**: No support for custom metrics scaling
4. **Configuration Schema**: No HPA configuration structure defined

## Design Goals

### Primary Objectives

1. **Seamless Integration**: HPA should integrate naturally with existing Simple Container configuration patterns
2. **Multi-Environment Support**: Work consistently across plain K8s and GKE
3. **Metric Flexibility**: Support CPU, memory, and custom metrics
4. **Backward Compatibility**: Existing static scaling configurations must continue to work
5. **Production Ready**: Include proper validation, error handling, and monitoring

### Secondary Objectives

1. **Advanced Metrics**: Support for custom and external metrics (Prometheus, etc.)
2. **Behavior Configuration**: Fine-grained control over scaling policies
3. **Integration with VPA**: Proper coordination between HPA and VPA
4. **Observability**: Rich logging and monitoring of scaling decisions

## Proposed Architecture

### Cloud-Agnostic Configuration Schema

Simple Container's existing scale configuration already supports the foundation for horizontal autoscaling. We'll enhance the existing `StackConfigComposeScale` to enable HPA without breaking cloud-agnostic principles.

#### Current Scale Configuration (Already Exists)
```go
// pkg/api/client.go - Already implemented
type StackConfigComposeScale struct {
    Min int `yaml:"min" json:"min"`
    Max int `yaml:"max" json:"max"`
    
    Policy *StackConfigComposeScalePolicy `json:"policy" yaml:"policy"`
}

type StackConfigComposeScalePolicy struct {
    Cpu    *StackConfigComposeScaleCpu    `yaml:"cpu" json:"cpu"`
    Memory *StackConfigComposeScaleMemory `yaml:"memory" json:"memory"`
}

type StackConfigComposeScaleCpu struct {
    Max int `yaml:"max" json:"max"`  // CPU utilization percentage threshold
}

type StackConfigComposeScaleMemory struct {
    Max int `yaml:"max" json:"max"`  // Memory utilization percentage threshold
}
```

#### Enhanced Implementation (Cloud-Agnostic)
The existing configuration already provides everything needed for HPA:
- **`min`/`max`**: Maps directly to HPA minReplicas/maxReplicas
- **`policy.cpu.max`**: CPU utilization threshold (e.g., 70%)
- **`policy.memory.max`**: Memory utilization threshold (e.g., 80%)

**No new configuration fields needed!** The cloud provider implementation will automatically:
- **Kubernetes/GKE**: Create HPA resources when `min != max` and policy is defined
- **AWS ECS**: Use existing autoscaling target configuration
- **Static deployment**: Use `max` value as fixed replica count when `min == max`

### User Configuration Examples

#### Basic CPU-based Autoscaling (Simple Container Style)
```yaml
# client.yaml - Uses existing SC pattern
stacks:
  web-app:
    type: single-image
    config:
      scale:
        min: 2          # Minimum replicas (HPA minReplicas)
        max: 10         # Maximum replicas (HPA maxReplicas)
        policy:
          cpu:
            max: 70     # Scale up when CPU > 70%
```

#### Memory-based Autoscaling
```yaml
# client.yaml
stacks:
  data-processor:
    type: single-image
    config:
      scale:
        min: 1
        max: 8
        policy:
          memory:
            max: 80     # Scale up when Memory > 80%
```

#### Multi-Metric Scaling (CPU + Memory)
```yaml
# client.yaml
stacks:
  api-server:
    type: single-image
    config:
      scale:
        min: 3
        max: 20
        policy:
          cpu:
            max: 60     # Scale up when CPU > 60%
          memory:
            max: 75     # OR when Memory > 75%
```

#### Static Scaling (No Autoscaling)
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

## Implementation Plan

### Phase 1: Enhanced Scale Configuration Support

#### 1.1 Kubernetes HPA Logic Implementation
- **File**: `pkg/clouds/k8s/types.go`
- **Action**: Update `ToScale` function to detect autoscaling configuration
- **Logic**: 
  ```go
  func ToScale(stack *api.StackConfigCompose) *Scale {
      if stack.Scale != nil {
          // Detect if autoscaling should be enabled
          shouldAutoscale := stack.Scale.Min != stack.Scale.Max && 
                           (stack.Scale.Policy != nil && 
                            (stack.Scale.Policy.Cpu != nil || stack.Scale.Policy.Memory != nil))
          
          return &Scale{
              Replicas:    stack.Scale.Min, // Use min as base replica count
              EnableHPA:   shouldAutoscale,
              MinReplicas: stack.Scale.Min,
              MaxReplicas: stack.Scale.Max,
              CPUTarget:   getCPUTarget(stack.Scale.Policy),
              MemoryTarget: getMemoryTarget(stack.Scale.Policy),
          }
      }
      return nil
  }
  ```

#### 1.2 HPA Resource Generation
- **File**: `pkg/clouds/pulumi/kubernetes/hpa.go` (new)
- **Action**: Create HPA Pulumi resource generation using existing scale config
- **Features**:
  - CPU utilization metrics from `policy.cpu.max`
  - Memory utilization metrics from `policy.memory.max`
  - Min/Max replicas from existing `min`/`max` fields

#### 1.3 Integration with Deployment
- **File**: `pkg/clouds/pulumi/kubernetes/deployment.go`
- **Action**: Integrate HPA creation when autoscaling is detected
- **Logic**:
  ```go
  // Create HPA if min != max and policy is defined
  if args.Deployment.Scale != nil && args.Deployment.Scale.EnableHPA {
      hpaArgs := &HPAArgs{
          Name:         deploymentName,
          Deployment:   deployment,
          MinReplicas:  args.Deployment.Scale.MinReplicas,
          MaxReplicas:  args.Deployment.Scale.MaxReplicas,
          CPUTarget:    args.Deployment.Scale.CPUTarget,
          MemoryTarget: args.Deployment.Scale.MemoryTarget,
          Namespace:    args.Namespace,
      }
      hpa, err := CreateHPA(ctx, hpaArgs, opts...)
      // ... error handling
  }
  ```

#### 1.4 Validation Logic
- **File**: `pkg/clouds/k8s/validation.go` (new)
- **Action**: Add scale configuration validation
- **Validations**:
  - `min` < `max` when policy is defined
  - CPU/Memory thresholds are reasonable (1-100%)
  - Resource requests defined when using resource-based scaling

### Phase 2: Advanced Features

#### 2.1 Custom Metrics Support
- **Files**: 
  - `pkg/clouds/k8s/types.go` (extend HPAMetric types)
  - `pkg/clouds/pulumi/kubernetes/hpa.go` (add custom metric generation)
- **Features**:
  - Pods metrics
  - Object metrics  
  - External metrics (Prometheus, GCP monitoring)

#### 2.2 Scaling Behavior Configuration
- **File**: `pkg/clouds/pulumi/kubernetes/hpa.go`
- **Action**: Add scaling behavior support
- **Features**:
  - Stabilization windows
  - Scaling policies
  - Policy selection strategies

#### 2.3 GKE-Specific Optimizations
- **File**: `pkg/clouds/gcp/gke_hpa.go` (new)
- **Action**: GKE-specific HPA enhancements
- **Features**:
  - GKE Autopilot compatibility
  - Google Cloud Monitoring integration
  - Workload Identity support for custom metrics

### Phase 3: Production Readiness

#### 3.1 Monitoring and Observability
- **File**: `pkg/clouds/pulumi/kubernetes/hpa_monitoring.go` (new)
- **Action**: Add HPA monitoring resources
- **Features**:
  - ServiceMonitor for Prometheus
  - Grafana dashboard templates
  - Alert rules for scaling events

#### 3.2 Documentation and Examples
- **Files**:
  - `docs/docs/examples/kubernetes-hpa/` (new directory)
  - `docs/docs/guides/autoscaling-kubernetes.md` (new)
  - `docs/docs/reference/configuration-reference.md` (update)

#### 3.3 Testing and Validation
- **Files**:
  - `pkg/clouds/k8s/hpa_test.go` (new)
  - `pkg/clouds/pulumi/kubernetes/hpa_test.go` (new)
- **Tests**:
  - Configuration validation tests
  - Resource generation tests
  - Integration tests with mock Kubernetes API

## Technical Considerations

### HPA and VPA Coordination

**Challenge**: HPA and VPA can conflict when both are enabled
**Solution**: Implement coordination logic

```go
func ValidateAutoscalingConfiguration(scale *Scale, vpa *VPAConfig) error {
    if scale != nil && scale.HPA != nil && scale.HPA.Enabled && vpa != nil && vpa.Enabled {
        // Check if VPA is configured to not control CPU/Memory when HPA is using them
        if vpa.ControlledResources == nil {
            return errors.New("when HPA is enabled, VPA must specify controlledResources to avoid conflicts")
        }
        
        // Check for metric conflicts
        for _, metric := range scale.HPA.Metrics {
            if metric.Resource != nil {
                resourceName := metric.Resource.Name
                if contains(vpa.ControlledResources, resourceName) {
                    return errors.Errorf("HPA and VPA both configured to control %s resource", resourceName)
                }
            }
        }
    }
    return nil
}
```

### Resource Requirements

**Requirement**: HPA requires resource requests to be defined for resource-based metrics
**Solution**: Automatic validation of scale configuration

```go
func ValidateScaleConfiguration(scale *api.StackConfigComposeScale, resources *Resources) error {
    // If autoscaling is configured, validate resource requirements
    if scale.Min != scale.Max && scale.Policy != nil {
        if scale.Policy.Cpu != nil && (resources == nil || resources.Requests["cpu"] == "") {
            return errors.New("CPU resource requests must be defined when using CPU-based scaling")
        }
        if scale.Policy.Memory != nil && (resources == nil || resources.Requests["memory"] == "") {
            return errors.New("Memory resource requests must be defined when using memory-based scaling")
        }
    }
    return nil
}
```

### Backward Compatibility

**Strategy**: Graceful migration from static scaling
- Existing `replicas` field continues to work
- When HPA is enabled, `replicas` becomes `minReplicas` if not specified
- Clear migration path and documentation

```go
func NormalizeScaleConfiguration(scale *Scale) *Scale {
    if scale == nil {
        return nil
    }
    
    // If HPA is enabled but minReplicas not set, use static replicas as minimum
    if scale.HPA != nil && scale.HPA.Enabled && scale.HPA.MinReplicas == 0 {
        scale.HPA.MinReplicas = max(scale.Replicas, 1)
    }
    
    return scale
}
```

## Platform-Specific Considerations

### Plain Kubernetes

**Requirements**:
- Metrics Server must be installed
- RBAC permissions for HPA controller
- Custom metrics API for advanced metrics

**Implementation**:
```go
func ValidateKubernetesHPARequirements(ctx context.Context, kubeClient kubernetes.Interface) error {
    // Check if Metrics Server is available
    _, err := kubeClient.AppsV1().Deployments("kube-system").Get(ctx, "metrics-server", metav1.GetOptions{})
    if err != nil {
        return errors.Wrap(err, "Metrics Server is required for HPA but not found in kube-system namespace")
    }
    
    // Check custom metrics API availability for advanced metrics
    discoveryClient := kubeClient.Discovery()
    apiGroups, err := discoveryClient.ServerGroups()
    if err != nil {
        return errors.Wrap(err, "failed to discover API groups")
    }
    
    hasCustomMetrics := false
    for _, group := range apiGroups.Groups {
        if group.Name == "custom.metrics.k8s.io" {
            hasCustomMetrics = true
            break
        }
    }
    
    // Log warning if custom metrics not available
    if !hasCustomMetrics {
        log.Warn("Custom metrics API not available - only CPU and memory metrics will work")
    }
    
    return nil
}
```

### Google Kubernetes Engine (GKE)

**Advantages**:
- Metrics Server pre-installed
- Google Cloud Monitoring integration
- Autopilot optimizations

**GKE-Specific Optimizations**:
```go
type GKEScaleOptimizer struct {
    clusterInfo *GKEClusterInfo
}

func (g *GKEScaleOptimizer) OptimizeScale(scale *Scale) *Scale {
    if g.clusterInfo.IsAutopilot {
        // Autopilot nodes take longer to provision - use conservative thresholds
        if scale.CPUTarget != nil && *scale.CPUTarget < 60 {
            *scale.CPUTarget = 60
        }
        if scale.MemoryTarget != nil && *scale.MemoryTarget < 70 {
            *scale.MemoryTarget = 70
        }
    }
    return scale
}
```

## Migration Strategy

### Existing Users

1. **No Breaking Changes**: Existing static scaling continues to work
2. **Opt-in HPA**: Users must explicitly enable HPA
3. **Clear Documentation**: Migration guide with examples
4. **Validation Warnings**: Helpful messages for common misconfigurations

### Migration Path

```yaml
# Step 1: Current static scaling (continues to work)
scale:
  min: 3
  max: 3  # Same as min = static scaling

# Step 2: Enable autoscaling (change max and add policy)
scale:
  min: 3          # Keep same minimum
  max: 10         # Allow scaling up
  policy:
    cpu:
      max: 70     # Scale when CPU > 70%
```

## Success Metrics

### Technical Metrics

1. **Resource Utilization**: Improved CPU/Memory efficiency
2. **Response Time**: Faster scaling response to load changes
3. **Cost Optimization**: Reduced over-provisioning
4. **Reliability**: Maintained service availability during scaling

### User Experience Metrics

1. **Configuration Simplicity**: Easy HPA setup with sensible defaults
2. **Documentation Quality**: Clear examples and migration guides
3. **Error Messages**: Helpful validation and troubleshooting guidance
4. **Integration Smoothness**: Seamless integration with existing SC workflows

## Risk Mitigation

### Technical Risks

1. **HPA/VPA Conflicts**: Comprehensive validation and documentation
2. **Metrics Availability**: Clear requirements and validation checks
3. **Scaling Thrashing**: Proper stabilization window defaults
4. **Resource Constraints**: Validation of resource requests/limits

### Operational Risks

1. **Breaking Changes**: Extensive backward compatibility testing
2. **Performance Impact**: Monitoring of HPA controller overhead
3. **Security Concerns**: Proper RBAC and service account configuration
4. **Complexity**: Progressive disclosure of advanced features

## Future Enhancements

### Advanced Metrics Integration

1. **Prometheus Integration**: Native support for Prometheus metrics
2. **Application Metrics**: Custom application-specific scaling metrics
3. **Multi-Dimensional Scaling**: Complex metric combinations
4. **Predictive Scaling**: ML-based scaling predictions

### Cross-Cloud Consistency

1. **AWS ECS Integration**: Align HPA concepts with ECS autoscaling
2. **Azure Container Instances**: Extend to Azure deployments
3. **Unified Configuration**: Consistent autoscaling across all platforms

### Operational Excellence

1. **Scaling Analytics**: Detailed scaling behavior analysis
2. **Cost Optimization**: Integration with cloud cost management
3. **Capacity Planning**: Predictive capacity recommendations
4. **SLO Integration**: Scaling based on SLO compliance

## Conclusion

This design provides a comprehensive approach to implementing HPA support in Simple Container while maintaining backward compatibility and following established patterns. The phased implementation allows for iterative development and validation, ensuring a robust and user-friendly autoscaling solution for Kubernetes deployments.

The design emphasizes:
- **Simplicity**: Easy configuration for common use cases
- **Flexibility**: Advanced options for complex scenarios  
- **Reliability**: Proper validation and error handling
- **Consistency**: Aligned with existing Simple Container patterns
- **Future-Proof**: Extensible architecture for future enhancements
