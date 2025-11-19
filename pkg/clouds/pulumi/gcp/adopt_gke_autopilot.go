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

	cluster, err := container.NewCluster(ctx, clusterName, &container.ClusterArgs{
		Name:     sdk.String(gkeInput.ClusterName),
		Location: sdk.String(gkeInput.Location),
		// Note: We don't need to specify all the cluster configuration since we're importing
		// Pulumi will read the current state from GCP
	}, opts...)
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
