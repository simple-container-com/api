package kubernetes

import (
	"fmt"

	"github.com/pkg/errors"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	autoscalingv2 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/autoscaling/v2"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// HPAArgs contains the arguments for creating an HPA resource
type HPAArgs struct {
	Name         string
	Deployment   *appsv1.Deployment
	MinReplicas  int
	MaxReplicas  int
	CPUTarget    *int
	MemoryTarget *int
	Namespace    *corev1.Namespace
	Labels       map[string]string
	Annotations  map[string]string
	Opts         []sdk.ResourceOption
}

// CreateHPA creates a Horizontal Pod Autoscaler resource
func CreateHPA(ctx *sdk.Context, args *HPAArgs) (*autoscalingv2.HorizontalPodAutoscaler, error) {
	if args == nil {
		return nil, errors.New("HPAArgs cannot be nil")
	}

	if args.CPUTarget == nil && args.MemoryTarget == nil {
		return nil, errors.New("at least one metric target (CPU or Memory) must be specified")
	}

	hpaName := fmt.Sprintf("%s-hpa", args.Name)

	// Build metrics array
	var metrics autoscalingv2.MetricSpecArray

	// Add CPU metric if specified
	if args.CPUTarget != nil {
		cpuMetric := &autoscalingv2.MetricSpecArgs{
			Type: sdk.String("Resource"),
			Resource: &autoscalingv2.ResourceMetricSourceArgs{
				Name: sdk.String("cpu"),
				Target: &autoscalingv2.MetricTargetArgs{
					Type:               sdk.String("Utilization"),
					AverageUtilization: sdk.Int(*args.CPUTarget),
				},
			},
		}
		metrics = append(metrics, cpuMetric)
	}

	// Add Memory metric if specified
	if args.MemoryTarget != nil {
		memoryMetric := &autoscalingv2.MetricSpecArgs{
			Type: sdk.String("Resource"),
			Resource: &autoscalingv2.ResourceMetricSourceArgs{
				Name: sdk.String("memory"),
				Target: &autoscalingv2.MetricTargetArgs{
					Type:               sdk.String("Utilization"),
					AverageUtilization: sdk.Int(*args.MemoryTarget),
				},
			},
		}
		metrics = append(metrics, memoryMetric)
	}

	// Merge common labels with HPA-specific labels
	hpaLabels := make(map[string]string)
	for k, v := range args.Labels {
		hpaLabels[k] = v
	}
	// Add HPA-specific labels
	hpaLabels["app.kubernetes.io/component"] = "hpa"
	hpaLabels["app.kubernetes.io/managed-by"] = "simple-container"

	// Use common annotations (HPA doesn't typically need specific annotations)
	hpaAnnotations := make(map[string]string)
	for k, v := range args.Annotations {
		hpaAnnotations[k] = v
	}

	// Create HPA resource
	hpa, err := autoscalingv2.NewHorizontalPodAutoscaler(ctx, hpaName, &autoscalingv2.HorizontalPodAutoscalerArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:        sdk.String(hpaName),
			Namespace:   args.Namespace.Metadata.Name(),
			Labels:      sdk.ToStringMap(hpaLabels),
			Annotations: sdk.ToStringMap(hpaAnnotations),
		},
		Spec: &autoscalingv2.HorizontalPodAutoscalerSpecArgs{
			MinReplicas: sdk.Int(args.MinReplicas),
			MaxReplicas: sdk.Int(args.MaxReplicas),
			ScaleTargetRef: &autoscalingv2.CrossVersionObjectReferenceArgs{
				ApiVersion: sdk.String("apps/v1"),
				Kind:       sdk.String("Deployment"),
				Name:       args.Deployment.Metadata.Name().Elem(),
			},
			Metrics: metrics,
			Behavior: &autoscalingv2.HorizontalPodAutoscalerBehaviorArgs{
				ScaleUp: &autoscalingv2.HPAScalingRulesArgs{
					StabilizationWindowSeconds: sdk.Int(60), // Wait 60s before scaling up again
					Policies: autoscalingv2.HPAScalingPolicyArray{
						&autoscalingv2.HPAScalingPolicyArgs{
							Type:          sdk.String("Percent"),
							Value:         sdk.Int(50), // Scale up by max 50% of current replicas
							PeriodSeconds: sdk.Int(60), // Over 60 second period
						},
						&autoscalingv2.HPAScalingPolicyArgs{
							Type:          sdk.String("Pods"),
							Value:         sdk.Int(2),  // Or max 2 pods at once
							PeriodSeconds: sdk.Int(60), // Over 60 second period
						},
					},
					SelectPolicy: sdk.String("Min"), // Use the more conservative policy
				},
				ScaleDown: &autoscalingv2.HPAScalingRulesArgs{
					StabilizationWindowSeconds: sdk.Int(300), // Wait 5min before scaling down again
					Policies: autoscalingv2.HPAScalingPolicyArray{
						&autoscalingv2.HPAScalingPolicyArgs{
							Type:          sdk.String("Percent"),
							Value:         sdk.Int(10), // Scale down by max 10% of current replicas
							PeriodSeconds: sdk.Int(60), // Over 60 second period
						},
					},
				},
			},
		},
	}, args.Opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create HPA %s", hpaName)
	}

	return hpa, nil
}

// ValidateHPAConfiguration validates HPA configuration for common issues
func ValidateHPAConfiguration(scale *k8s.Scale, resources *k8s.Resources) error {
	if scale == nil || !scale.EnableHPA {
		return nil // No validation needed if HPA is not enabled
	}

	// Validate min/max replicas
	if scale.MinReplicas <= 0 {
		return errors.New("minReplicas must be greater than 0")
	}
	if scale.MaxReplicas <= scale.MinReplicas {
		return errors.New("maxReplicas must be greater than minReplicas")
	}

	// Validate CPU target
	if scale.CPUTarget != nil {
		if *scale.CPUTarget <= 0 || *scale.CPUTarget > 100 {
			return errors.Errorf("CPU target must be between 1-100%%, got %d", *scale.CPUTarget)
		}
		// Check if CPU resource requests are defined
		if resources == nil || resources.Requests["cpu"] == "" {
			return errors.New("CPU resource requests must be defined when using CPU-based scaling")
		}
	}

	// Validate Memory target
	if scale.MemoryTarget != nil {
		if *scale.MemoryTarget <= 0 || *scale.MemoryTarget > 100 {
			return errors.Errorf("Memory target must be between 1-100%%, got %d", *scale.MemoryTarget)
		}
		// Check if Memory resource requests are defined
		if resources == nil || resources.Requests["memory"] == "" {
			return errors.New("Memory resource requests must be defined when using memory-based scaling")
		}
	}

	// Ensure at least one metric is configured
	if scale.CPUTarget == nil && scale.MemoryTarget == nil {
		return errors.New("at least one scaling metric (CPU or Memory) must be configured")
	}

	return nil
}
