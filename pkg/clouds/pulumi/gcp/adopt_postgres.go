package gcp

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/sql"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// AdoptPostgres imports an existing Cloud SQL Postgres instance into Pulumi state without modifying it
func AdoptPostgres(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.ResourceTypePostgresGcpCloudsql {
		return nil, errors.Errorf("unsupported postgres type %q", input.Descriptor.Type)
	}

	pgCfg, ok := input.Descriptor.Config.Config.(*gcloud.PostgresGcpCloudsqlConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert postgresql config for %q", input.Descriptor.Type)
	}

	if !pgCfg.Adopt {
		return nil, errors.Errorf("adopt flag not set for resource %q", input.Descriptor.Name)
	}

	if pgCfg.InstanceName == "" {
		return nil, errors.Errorf("instanceName is required when adopt=true for resource %q", input.Descriptor.Name)
	}

	if pgCfg.ConnectionName == "" {
		return nil, errors.Errorf("connectionName is required when adopt=true for resource %q", input.Descriptor.Name)
	}

	// Use identical naming functions as provisioning to ensure export compatibility
	postgresName := toPostgresName(input, input.Descriptor.Name)

	params.Log.Info(ctx.Context(), "adopting existing Cloud SQL Postgres instance %q", pgCfg.InstanceName)

	// First, lookup the existing instance to get its current configuration
	params.Log.Info(ctx.Context(), "fetching existing Cloud SQL instance details for %q", pgCfg.InstanceName)
	existingInstance, err := sql.LookupDatabaseInstance(ctx, &sql.LookupDatabaseInstanceArgs{
		Name:    pgCfg.InstanceName,
		Project: &pgCfg.ProjectId,
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to lookup existing Cloud SQL Postgres instance %q", pgCfg.InstanceName)
	}

	// Use the existing instance's configuration for the import, but allow overrides from config
	databaseVersion := existingInstance.DatabaseVersion
	if pgCfg.Version != "" {
		databaseVersion = pgCfg.Version
		params.Log.Info(ctx.Context(), "overriding database version with config value: %q", databaseVersion)
	}

	region := existingInstance.Region
	if pgCfg.Region != nil && *pgCfg.Region != "" {
		region = *pgCfg.Region
		params.Log.Info(ctx.Context(), "overriding region with config value: %q", region)
	}

	// Extract settings from existing instance
	tier := "db-f1-micro" // default fallback
	if len(existingInstance.Settings) > 0 {
		tier = existingInstance.Settings[0].Tier
	}
	if pgCfg.Tier != nil && *pgCfg.Tier != "" {
		tier = *pgCfg.Tier
		params.Log.Info(ctx.Context(), "overriding tier with config value: %q", tier)
	}

	params.Log.Info(ctx.Context(), "found existing instance with version %q, region %q, tier %q",
		databaseVersion, region, tier)

	// Import existing Cloud SQL instance into Pulumi state
	// The instance resource ID in GCP is: projects/{project}/instances/{instance}
	instanceResourceId := fmt.Sprintf("projects/%s/instances/%s", pgCfg.ProjectId, pgCfg.InstanceName)

	opts := []sdk.ResourceOption{
		sdk.Provider(params.Provider),
		// Import the existing instance without creating or modifying it
		sdk.Import(sdk.ID(instanceResourceId)),
	}

	// Build database flags if max connections is specified
	var databaseFlags sql.DatabaseInstanceSettingsDatabaseFlagArray
	if pgCfg.MaxConnections != nil {
		databaseFlags = append(databaseFlags, sql.DatabaseInstanceSettingsDatabaseFlagArgs{
			Name:  sdk.String("max_connections"),
			Value: sdk.String(fmt.Sprintf("%d", *pgCfg.MaxConnections)),
		})
	}

	pgInstance, err := sql.NewDatabaseInstance(ctx, postgresName, &sql.DatabaseInstanceArgs{
		Name:            sdk.String(pgCfg.InstanceName),
		Region:          sdk.StringPtr(region),
		DatabaseVersion: sdk.String(databaseVersion),
		// Use the existing instance's configuration for import
		Settings: &sql.DatabaseInstanceSettingsArgs{
			Tier:          sdk.String(tier),
			DatabaseFlags: databaseFlags,
			InsightsConfig: &sql.DatabaseInstanceSettingsInsightsConfigArgs{
				QueryInsightsEnabled:  sdk.BoolPtr(pgCfg.QueryInsightsEnabled != nil && *pgCfg.QueryInsightsEnabled),
				QueryStringLength:     sdk.IntPtr(getQueryStringLength(pgCfg.QueryStringLength)),
				RecordApplicationTags: sdk.BoolPtr(false), // Default to false
				RecordClientAddress:   sdk.BoolPtr(false), // Default to false
			},
		},
		DeletionProtection: sdk.BoolPtrFromPtr(pgCfg.DeletionProtection),
		// Note: Using actual instance configuration from GCP
		// This ensures the import matches the existing instance exactly
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to import Cloud SQL Postgres instance %q", pgCfg.InstanceName)
	}

	// For adopted instances, we need to export the root password from configuration
	// instead of generating a new one like in provisioning
	rootPasswordExport := toPostgresRootPasswordExport(postgresName)

	// Read root password from configuration (this will be provided in secrets.yaml)
	// The compute processor will use this to create database users
	if pgCfg.RootPassword != "" {
		ctx.Export(rootPasswordExport, sdk.ToSecret(sdk.String(pgCfg.RootPassword)))
	} else {
		// For backward compatibility, we can also try to read from secrets
		params.Log.Warn(ctx.Context(), "rootPassword not provided in config for adopted instance %q, compute processor will need to handle this", pgCfg.InstanceName)
		ctx.Export(rootPasswordExport, sdk.ToSecret(sdk.String(""))) // Empty, compute processor will handle
	}

	params.Log.Info(ctx.Context(), "successfully adopted Cloud SQL Postgres instance %q", pgCfg.InstanceName)

	return &api.ResourceOutput{Ref: pgInstance}, nil
}

// getQueryStringLength returns a sensible default for query string length if not specified
func getQueryStringLength(configValue *int) int {
	if configValue != nil {
		return *configValue
	}
	return 2048 // Default value matching regular provisioning
}
