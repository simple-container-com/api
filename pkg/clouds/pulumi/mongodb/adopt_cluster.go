package mongodb

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

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

	// CRITICAL SAFETY WARNING for production environments
	// Use flexible environment detection instead of hardcoded names
	pApi.LogAdoptionWarnings(ctx, input, params, "MongoDB cluster", atlasCfg.ClusterName)

	params.Log.Info(ctx.Context(), "adopting existing MongoDB Atlas cluster %q for project %q", atlasCfg.ClusterName, atlasCfg.ProjectId)

	// First, lookup the existing cluster to get its current configuration
	params.Log.Info(ctx.Context(), "fetching existing cluster details for %q", atlasCfg.ClusterName)
	existingCluster, err := mongodbatlas.LookupCluster(ctx, &mongodbatlas.LookupClusterArgs{
		ProjectId: atlasCfg.ProjectId,
		Name:      atlasCfg.ClusterName,
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to lookup existing MongoDB Atlas cluster %q", atlasCfg.ClusterName)
	}

	// Use the existing cluster's configuration for the import, but allow overrides from config
	instanceSize := existingCluster.ProviderInstanceSizeName
	if atlasCfg.InstanceSize != "" {
		instanceSize = atlasCfg.InstanceSize
		params.Log.Info(ctx.Context(), "overriding instance size with config value: %q", instanceSize)
	}

	region := existingCluster.ProviderRegionName
	if atlasCfg.Region != "" {
		region = atlasCfg.Region
		params.Log.Info(ctx.Context(), "overriding region with config value: %q", region)
	}

	clusterType := existingCluster.ClusterType
	providerName := existingCluster.ProviderName
	backingProviderName := existingCluster.BackingProviderName

	// Override provider settings if cloud provider is explicitly specified
	if atlasCfg.CloudProvider != "" {
		params.Log.Info(ctx.Context(), "overriding cloud provider with config value: %q", atlasCfg.CloudProvider)
		// Apply the same shared instance size logic as regular provisioning
		sharedInstanceSizes := []string{"M0", "M2", "M5"}
		_, isSharedInstanceSize := lo.Find(sharedInstanceSizes, func(size string) bool {
			return size == instanceSize
		})

		if isSharedInstanceSize {
			providerName = "TENANT"
			backingProviderName = atlasCfg.CloudProvider
		} else {
			providerName = atlasCfg.CloudProvider
			backingProviderName = ""
		}
	}

	params.Log.Info(ctx.Context(), "found existing cluster with instance size %q, region %q, provider %q",
		instanceSize, region, providerName)

	// Import existing cluster into Pulumi state
	// The cluster resource ID in MongoDB Atlas is: {project_id}-{cluster_name}
	clusterResourceId := fmt.Sprintf("%s-%s", atlasCfg.ProjectId, atlasCfg.ClusterName)

	// Use standardized adoption protection options
	adoptionOpts := pApi.AdoptionProtectionOptions([]string{
		// Core cluster configuration that might drift
		"diskSizeGb", "numShards", "cloudBackup",
		// Provider-specific settings that might vary
		"providerAutoScalingComputeEnabled", "providerAutoScalingComputeScaleDownEnabled",
		"providerDiskIops", "providerEncryptEbsVolume", "providerVolumeType",
		// Network and security settings
		"mongoDbMajorVersion", "pitEnabled", "rootCertType",
		// Advanced settings that might be managed outside of Pulumi
		"advancedConfiguration", "labels", "tags",
		// Backup and maintenance settings
		"backupEnabled", "mongoDbVersion", "paused",
	})

	opts := append([]sdk.ResourceOption{
		sdk.Provider(params.Provider),
		// Import the existing cluster without creating or modifying it
		sdk.Import(sdk.ID(clusterResourceId)),
	}, adoptionOpts...)

	cluster, err := mongodbatlas.NewCluster(ctx, clusterName, &mongodbatlas.ClusterArgs{
		ProjectId: sdk.String(atlasCfg.ProjectId),
		Name:      sdk.String(atlasCfg.ClusterName),
		// Use the existing cluster's configuration for import
		ProviderInstanceSizeName: sdk.String(instanceSize),
		ProviderRegionName:       sdk.StringPtr(region),
		ClusterType:              sdk.StringPtr(clusterType),
		ProviderName:             sdk.String(providerName),
		BackingProviderName:      sdk.StringPtrFromPtr(lo.If(backingProviderName != "", &backingProviderName).Else(nil)),
		// Note: Using actual cluster configuration from MongoDB Atlas
		// This ensures the import matches the existing cluster exactly
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
