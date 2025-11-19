package mongodb

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-mongodbatlas/sdk/v3/go/mongodbatlas"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/mongodb"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// AdoptCluster imports an existing MongoDB Atlas cluster into Pulumi state without modifying it
func AdoptCluster(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != mongodb.ResourceTypeMongodbAtlas {
		return nil, errors.Errorf("unsupported mongodb-atlas type %q", input.Descriptor.Type)
	}

	atlasCfg, ok := input.Descriptor.Config.Config.(*mongodb.AtlasConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert mongodb atlas config for %q", input.Descriptor.Type)
	}

	if !atlasCfg.Adopt {
		return nil, errors.Errorf("adopt flag not set for resource %q", input.Descriptor.Name)
	}

	if atlasCfg.ClusterName == "" {
		return nil, errors.Errorf("clusterName is required when adopt=true for resource %q", input.Descriptor.Name)
	}

	// Use identical naming functions as provisioning to ensure export compatibility
	projectName := toProjectName(stack.Name, input)
	clusterName := toClusterName(stack.Name, input)

	params.Log.Info(ctx.Context(), "adopting existing MongoDB Atlas cluster %q for project %q", atlasCfg.ClusterName, atlasCfg.ProjectId)

	// Import existing cluster into Pulumi state
	// The cluster resource ID in MongoDB Atlas is: {project_id}-{cluster_name}
	clusterResourceId := fmt.Sprintf("%s-%s", atlasCfg.ProjectId, atlasCfg.ClusterName)

	opts := []sdk.ResourceOption{
		sdk.Provider(params.Provider),
		// Import the existing cluster without creating or modifying it
		sdk.Import(sdk.ID(clusterResourceId)),
	}

	cluster, err := mongodbatlas.NewCluster(ctx, clusterName, &mongodbatlas.ClusterArgs{
		ProjectId: sdk.String(atlasCfg.ProjectId),
		Name:      sdk.String(atlasCfg.ClusterName),
		// Note: We don't need to specify all the cluster configuration since we're importing
		// Pulumi will read the current state from MongoDB Atlas
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to import MongoDB Atlas cluster %q", atlasCfg.ClusterName)
	}

	// Export the same keys as the provisioning function to ensure compute processor compatibility
	ctx.Export(toProjectIdExport(projectName), sdk.String(atlasCfg.ProjectId))
	ctx.Export(toClusterIdExport(clusterName), cluster.ClusterId)
	ctx.Export(toMongoUriExport(clusterName), cluster.MongoUri)
	ctx.Export(toMongoUriWithOptionsExport(clusterName), cluster.MongoUriWithOptions)

	// For adopted clusters, we don't create database users here - that's handled by the compute processor
	// Export empty users list to maintain compatibility
	usersOutput := sdk.ToOutput([]interface{}{})
	ctx.Export(fmt.Sprintf("%s-users", projectName), usersOutput)

	out := &ClusterOutput{
		DbUsers:                    usersOutput,
		Cluster:                    cluster,
		Project:                    nil, // We don't manage the project for adopted clusters
		PrivateLinkEndpointService: nil, // We don't manage private link for adopted clusters
	}

	params.Log.Info(ctx.Context(), "successfully adopted MongoDB Atlas cluster %q", atlasCfg.ClusterName)

	return &api.ResourceOutput{Ref: out}, nil
}
