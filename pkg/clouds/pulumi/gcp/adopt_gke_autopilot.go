package gcp

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/container"
	k8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	pulumiKubernetes "github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"
)

// AdoptGkeAutopilot imports an existing GKE Autopilot cluster into Pulumi state without modifying it
func AdoptGkeAutopilot(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.ResourceTypeGkeAutopilot {
		return nil, errors.Errorf("unsupported resource type %q", input.Descriptor.Type)
	}

	gkeInput, ok := input.Descriptor.Config.Config.(*gcloud.GkeAutopilotResource)
	if !ok {
		return nil, errors.Errorf("failed to convert gke autopilot config for %q", input.Descriptor.Type)
	}

	if !gkeInput.Adopt {
		return nil, errors.Errorf("adopt flag not set for resource %q", input.Descriptor.Name)
	}

	if gkeInput.ClusterName == "" {
		return nil, errors.Errorf("clusterName is required when adopt=true for resource %q", input.Descriptor.Name)
	}

	if gkeInput.Location == "" {
		return nil, errors.Errorf("location is required when adopt=true for resource %q", input.Descriptor.Name)
	}

	// Use identical naming functions as provisioning to ensure export compatibility
	clusterName := pulumiKubernetes.ToClusterName(input, input.Descriptor.Name)

	params.Log.Info(ctx.Context(), "adopting existing GKE Autopilot cluster %q in location %q", gkeInput.ClusterName, gkeInput.Location)

	// First, lookup the existing cluster to get its current configuration
	params.Log.Info(ctx.Context(), "fetching existing GKE cluster details for %q", gkeInput.ClusterName)
	existingCluster, err := container.LookupCluster(ctx, &container.LookupClusterArgs{
		Name:     gkeInput.ClusterName,
		Location: &gkeInput.Location,
		Project:  &gkeInput.ProjectId,
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to lookup existing GKE Autopilot cluster %q", gkeInput.ClusterName)
	}

	// Validate that this is actually an Autopilot cluster
	if !existingCluster.EnableAutopilot {
		params.Log.Warn(ctx.Context(), "cluster %q is not an Autopilot cluster (EnableAutopilot: %v)", gkeInput.ClusterName, existingCluster.EnableAutopilot)
	}

	// Use the existing cluster's configuration for the import, but allow overrides from config
	enableAutopilot := existingCluster.EnableAutopilot

	minMasterVersion := existingCluster.MinMasterVersion
	if gkeInput.GkeMinVersion != "" {
		minMasterVersion = gkeInput.GkeMinVersion
		params.Log.Info(ctx.Context(), "overriding GKE min version with config value: %q", minMasterVersion)
	}

	location := existingCluster.Location
	if gkeInput.Location != "" && gkeInput.Location != *location {
		location = &gkeInput.Location
		params.Log.Info(ctx.Context(), "overriding location with config value: %q", gkeInput.Location)
	}

	params.Log.Info(ctx.Context(), "found existing GKE cluster with autopilot=%v, version=%q, location=%q",
		enableAutopilot, minMasterVersion, location)

	// Import existing GKE cluster into Pulumi state
	// The cluster resource ID in GCP is: projects/{project}/locations/{location}/clusters/{cluster}
	clusterResourceId := fmt.Sprintf("projects/%s/locations/%s/clusters/%s",
		gkeInput.ProjectId,
		gkeInput.Location,
		gkeInput.ClusterName)

	opts := []sdk.ResourceOption{
		sdk.Provider(params.Provider),
		// Import the existing cluster without creating or modifying it
		sdk.Import(sdk.ID(clusterResourceId)),
	}

	// For GKE Autopilot adoption, use minimal configuration to avoid conflicts
	// Only specify essential fields that are compatible with Autopilot mode
	clusterArgs := &container.ClusterArgs{
		Name:            sdk.String(gkeInput.ClusterName),
		Location:        sdk.StringPtrFromPtr(location),
		EnableAutopilot: sdk.BoolPtr(enableAutopilot),
	}

	// Only set MinMasterVersion if it's provided and different from existing
	if gkeInput.GkeMinVersion != "" && gkeInput.GkeMinVersion != minMasterVersion {
		clusterArgs.MinMasterVersion = sdk.StringPtr(gkeInput.GkeMinVersion)
		params.Log.Info(ctx.Context(), "setting MinMasterVersion to %q for adoption", gkeInput.GkeMinVersion)
	}

	// Add resource options to ignore changes that might cause conflicts during adoption
	adoptionOpts := append(opts,
		// Ignore node pool related changes since Autopilot manages these automatically
		sdk.IgnoreChanges([]string{
			"nodePools",
			"nodePool",
			"defaultMaxPodsPerNode",
			"ipAllocationPolicy",
			"networkPolicy",
			"enableIntranodeVisibility",
			"loggingService",
			"monitoringService",
			"verticalPodAutoscaling",
			"clusterTelemetry",
			"nodeConfig",
		}),
	)

	cluster, err := container.NewCluster(ctx, clusterName, clusterArgs, adoptionOpts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to import GKE Autopilot cluster %q", gkeInput.ClusterName)
	}

	out := &GkeAutopilotOut{
		Cluster: cluster,
		Caddy:   nil, // Will be set below if Caddy is configured
	}

	// Export the same keys as the provisioning function to ensure compute processor compatibility
	kubeconfig := generateKubeconfig(cluster, gkeInput)
	ctx.Export(toKubeconfigExport(clusterName), kubeconfig)

	// Export Caddy configuration if specified (for child stack compatibility)
	// Note: For adopted clusters, we don't deploy Caddy but still need to export config
	if gkeInput.Caddy != nil {
		params.Log.Info(ctx.Context(), "exporting Caddy config for adopted GKE Autopilot cluster %q (no deployment)", gkeInput.ClusterName)

		// Export a minimal Caddy config that child stacks can read
		// This matches the structure that DeployCaddyService would export
		caddyConfigJson, err := json.Marshal(gkeInput.Caddy)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to marshal caddy config for adopted cluster %q", clusterName)
		}

		// Export the Caddy config using the same export key as regular provisioning
		ctx.Export(pulumiKubernetes.ToCaddyConfigExport(clusterName), sdk.String(string(caddyConfigJson)))

		params.Log.Info(ctx.Context(), "✅ Caddy config exported for child stack compatibility")

		// Detect and export existing load balancer IP for DNS record creation
		if err := exportExistingLoadBalancerIP(ctx, cluster, gkeInput, clusterName, params); err != nil {
			params.Log.Warn(ctx.Context(), "⚠️ Failed to detect existing load balancer IP: %v", err)
			// Don't fail the adoption, just warn - DNS records might need manual setup
		}
	}

	params.Log.Info(ctx.Context(), "successfully adopted GKE Autopilot cluster %q", gkeInput.ClusterName)

	return &api.ResourceOutput{Ref: out}, nil
}

// exportExistingLoadBalancerIP detects the existing Caddy load balancer service in the adopted cluster
// and exports its IP address for DNS record creation
func exportExistingLoadBalancerIP(ctx *sdk.Context, cluster *container.Cluster, gkeInput *gcloud.GkeAutopilotResource, clusterName string, params pApi.ProvisionParams) error {
	// Create a Kubernetes provider for the adopted cluster
	kubeconfig := generateKubeconfig(cluster, gkeInput)
	kubeProvider, err := k8s.NewProvider(ctx, fmt.Sprintf("%s-adoption-kube-provider", clusterName), &k8s.ProviderArgs{
		Kubeconfig: kubeconfig,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to create Kubernetes provider for adopted cluster %q", gkeInput.ClusterName)
	}

	// Determine the service name and namespace based on Caddy configuration
	serviceName := "caddy"
	namespace := "caddy"

	if gkeInput.Caddy.DeploymentName != nil {
		serviceName = *gkeInput.Caddy.DeploymentName
	}
	if gkeInput.Caddy.Namespace != nil {
		namespace = *gkeInput.Caddy.Namespace
	}

	params.Log.Info(ctx.Context(), "🔍 Looking for existing Caddy service %q in namespace %q", serviceName, namespace)

	// Look up the existing Caddy service to get its load balancer IP
	// Use GetService with the proper resource ID format: namespace/serviceName
	serviceId := fmt.Sprintf("%s/%s", namespace, serviceName)
	service, err := corev1.GetService(ctx, fmt.Sprintf("%s-adoption-service-lookup", clusterName), sdk.ID(serviceId), nil, sdk.Provider(kubeProvider))
	if err != nil {
		return errors.Wrapf(err, "failed to lookup existing Caddy service %q in namespace %q", serviceName, namespace)
	}

	// Extract the load balancer IP and export it
	loadBalancerIP := service.Status.ApplyT(func(status *corev1.ServiceStatus) string {
		if status.LoadBalancer == nil || len(status.LoadBalancer.Ingress) == 0 {
			params.Log.Warn(ctx.Context(), "⚠️ No load balancer ingress found for service %q in namespace %q", serviceName, namespace)
			return ""
		}

		ingress := status.LoadBalancer.Ingress[0]
		ip := ""
		if ingress.Ip != nil {
			ip = *ingress.Ip
		} else if ingress.Hostname != nil {
			// Some load balancers provide hostname instead of IP
			ip = *ingress.Hostname
		}

		if ip != "" {
			params.Log.Info(ctx.Context(), "✅ Found existing load balancer IP: %s", ip)
		}
		return ip
	}).(sdk.StringOutput)

	// Export the IP using the same key that child stacks expect
	ctx.Export(pulumiKubernetes.ToIngressIpExport(clusterName), loadBalancerIP)
	params.Log.Info(ctx.Context(), "✅ Exported existing load balancer IP for DNS record creation")

	return nil
}
