package gcp

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/container"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"
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
	clusterName := kubernetes.ToClusterName(input, input.Descriptor.Name)

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
		Caddy:   nil, // For adopted clusters, Caddy handling is done in compute processor
	}

	// Export the same keys as the provisioning function to ensure compute processor compatibility
	kubeconfig := generateKubeconfig(cluster, gkeInput)
	ctx.Export(toKubeconfigExport(clusterName), kubeconfig)

	params.Log.Info(ctx.Context(), "successfully adopted GKE Autopilot cluster %q", gkeInput.ClusterName)

	return &api.ResourceOutput{Ref: out}, nil
}
