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

	// Extract all settings from existing instance to preserve them
	var existingSettings *sql.GetDatabaseInstanceSetting
	if len(existingInstance.Settings) > 0 {
		existingSettings = &existingInstance.Settings[0]
	} else {
		return nil, errors.Errorf("existing instance %q has no settings - cannot adopt", pgCfg.InstanceName)
	}

	// Use existing tier unless overridden in config
	tier := existingSettings.Tier
	if pgCfg.Tier != nil && *pgCfg.Tier != "" {
		tier = *pgCfg.Tier
		params.Log.Info(ctx.Context(), "overriding tier with config value: %q", tier)
	}

	params.Log.Info(ctx.Context(), "found existing instance with version %q, region %q, tier %q",
		databaseVersion, region, tier)
	params.Log.Info(ctx.Context(), "preserving existing settings: %d database flags, insights enabled: %t, maintenance windows: %d",
		len(existingSettings.DatabaseFlags), len(existingSettings.InsightsConfigs) > 0 && existingSettings.InsightsConfigs[0].QueryInsightsEnabled, len(existingSettings.MaintenanceWindows))

	// Import existing Cloud SQL instance into Pulumi state
	// The instance resource ID in GCP is: projects/{project}/instances/{instance}
	instanceResourceId := fmt.Sprintf("projects/%s/instances/%s", pgCfg.ProjectId, pgCfg.InstanceName)

	opts := []sdk.ResourceOption{
		sdk.Provider(params.Provider),
		// Import the existing instance without creating or modifying it
		sdk.Import(sdk.ID(instanceResourceId)),
	}

	// Preserve existing database flags and optionally override max_connections
	var databaseFlags sql.DatabaseInstanceSettingsDatabaseFlagArray

	// First, copy all existing database flags
	for _, flag := range existingSettings.DatabaseFlags {
		// Skip max_connections if we're overriding it from config
		if flag.Name == "max_connections" && pgCfg.MaxConnections != nil {
			continue
		}
		databaseFlags = append(databaseFlags, sql.DatabaseInstanceSettingsDatabaseFlagArgs{
			Name:  sdk.String(flag.Name),
			Value: sdk.String(flag.Value),
		})
	}

	// Add max_connections override if specified in config
	if pgCfg.MaxConnections != nil {
		databaseFlags = append(databaseFlags, sql.DatabaseInstanceSettingsDatabaseFlagArgs{
			Name:  sdk.String("max_connections"),
			Value: sdk.String(fmt.Sprintf("%d", *pgCfg.MaxConnections)),
		})
		params.Log.Info(ctx.Context(), "overriding max_connections with config value: %d", *pgCfg.MaxConnections)
	}

	pgInstance, err := sql.NewDatabaseInstance(ctx, postgresName, &sql.DatabaseInstanceArgs{
		Name:            sdk.String(pgCfg.InstanceName),
		Region:          sdk.StringPtr(region),
		DatabaseVersion: sdk.String(databaseVersion),
		// Use the existing instance's configuration for import, preserving all settings
		Settings:           buildSettingsFromExisting(ctx, existingSettings, tier, databaseFlags, pgCfg, params),
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

// buildSettingsFromExisting creates database instance settings that preserve all existing configuration
// while allowing selective overrides from the Simple Container configuration
func buildSettingsFromExisting(ctx *sdk.Context, existingSettings *sql.GetDatabaseInstanceSetting, tier string, databaseFlags sql.DatabaseInstanceSettingsDatabaseFlagArray, pgCfg *gcloud.PostgresGcpCloudsqlConfig, params pApi.ProvisionParams) *sql.DatabaseInstanceSettingsArgs {
	// Build insights config - preserve existing unless explicitly overridden
	var insightsConfig *sql.DatabaseInstanceSettingsInsightsConfigArgs
	if len(existingSettings.InsightsConfigs) > 0 {
		existing := existingSettings.InsightsConfigs[0]
		insightsConfig = &sql.DatabaseInstanceSettingsInsightsConfigArgs{
			QueryInsightsEnabled:  sdk.Bool(existing.QueryInsightsEnabled),
			QueryStringLength:     sdk.Int(existing.QueryStringLength),
			RecordApplicationTags: sdk.Bool(existing.RecordApplicationTags),
			RecordClientAddress:   sdk.Bool(existing.RecordClientAddress),
		}

		// Override with config values if specified
		if pgCfg.QueryInsightsEnabled != nil {
			insightsConfig.QueryInsightsEnabled = sdk.Bool(*pgCfg.QueryInsightsEnabled)
		}
		if pgCfg.QueryStringLength != nil {
			insightsConfig.QueryStringLength = sdk.Int(*pgCfg.QueryStringLength)
		}
	}

	// Build backup configuration - preserve existing (use first one if multiple)
	var backupConfig *sql.DatabaseInstanceSettingsBackupConfigurationArgs
	if len(existingSettings.BackupConfigurations) > 0 {
		backup := existingSettings.BackupConfigurations[0]
		backupConfig = &sql.DatabaseInstanceSettingsBackupConfigurationArgs{
			Enabled:                     sdk.Bool(backup.Enabled),
			StartTime:                   sdk.String(backup.StartTime),
			Location:                    sdk.String(backup.Location),
			BinaryLogEnabled:            sdk.Bool(backup.BinaryLogEnabled),
			PointInTimeRecoveryEnabled:  sdk.Bool(backup.PointInTimeRecoveryEnabled),
			TransactionLogRetentionDays: sdk.Int(backup.TransactionLogRetentionDays),
			BackupRetentionSettings: func() *sql.DatabaseInstanceSettingsBackupConfigurationBackupRetentionSettingsArgs {
				if len(backup.BackupRetentionSettings) > 0 {
					retention := backup.BackupRetentionSettings[0]
					return &sql.DatabaseInstanceSettingsBackupConfigurationBackupRetentionSettingsArgs{
						RetainedBackups: sdk.Int(retention.RetainedBackups),
						RetentionUnit:   sdk.String(retention.RetentionUnit),
					}
				}
				return nil
			}(),
		}
	}

	// Build IP configuration - preserve existing (use first one if multiple)
	var ipConfig *sql.DatabaseInstanceSettingsIpConfigurationArgs
	if len(existingSettings.IpConfigurations) > 0 {
		ip := existingSettings.IpConfigurations[0]
		var authorizedNetworks sql.DatabaseInstanceSettingsIpConfigurationAuthorizedNetworkArray
		for _, network := range ip.AuthorizedNetworks {
			authorizedNetworks = append(authorizedNetworks, sql.DatabaseInstanceSettingsIpConfigurationAuthorizedNetworkArgs{
				Name:           sdk.String(network.Name),
				Value:          sdk.String(network.Value),
				ExpirationTime: sdk.String(network.ExpirationTime),
			})
		}

		ipConfig = &sql.DatabaseInstanceSettingsIpConfigurationArgs{
			Ipv4Enabled:                             sdk.Bool(ip.Ipv4Enabled),
			PrivateNetwork:                          sdk.String(ip.PrivateNetwork),
			AllocatedIpRange:                        sdk.String(ip.AllocatedIpRange),
			AuthorizedNetworks:                      authorizedNetworks,
			EnablePrivatePathForGoogleCloudServices: sdk.Bool(ip.EnablePrivatePathForGoogleCloudServices),
		}
	}

	// Build maintenance window - preserve existing (use first one if multiple)
	var maintenanceWindow *sql.DatabaseInstanceSettingsMaintenanceWindowArgs
	if len(existingSettings.MaintenanceWindows) > 0 {
		window := existingSettings.MaintenanceWindows[0]
		// Only create maintenance window if day is valid (1-7). Day 0 means no maintenance window configured.
		if window.Day >= 1 && window.Day <= 7 {
			maintenanceWindow = &sql.DatabaseInstanceSettingsMaintenanceWindowArgs{
				Day:         sdk.Int(window.Day),
				Hour:        sdk.Int(window.Hour),
				UpdateTrack: sdk.String(window.UpdateTrack),
			}
		} else {
			params.Log.Info(ctx.Context(), "skipping maintenance window with invalid day value: %d (expected 1-7)", window.Day)
		}
	}

	// Build location preference - preserve existing (use first one if multiple)
	var locationPref *sql.DatabaseInstanceSettingsLocationPreferenceArgs
	if len(existingSettings.LocationPreferences) > 0 {
		pref := existingSettings.LocationPreferences[0]
		locationPref = &sql.DatabaseInstanceSettingsLocationPreferenceArgs{
			FollowGaeApplication: sdk.String(pref.FollowGaeApplication),
			Zone:                 sdk.String(pref.Zone),
			SecondaryZone:        sdk.String(pref.SecondaryZone),
		}
	}

	return &sql.DatabaseInstanceSettingsArgs{
		Tier:                      sdk.String(tier),
		DatabaseFlags:             databaseFlags,
		InsightsConfig:            insightsConfig,
		BackupConfiguration:       backupConfig,
		IpConfiguration:           ipConfig,
		MaintenanceWindow:         maintenanceWindow,
		LocationPreference:        locationPref,
		ActivationPolicy:          sdk.String(existingSettings.ActivationPolicy),
		AvailabilityType:          sdk.String(existingSettings.AvailabilityType),
		Collation:                 sdk.String(existingSettings.Collation),
		ConnectorEnforcement:      sdk.String(existingSettings.ConnectorEnforcement),
		DeletionProtectionEnabled: sdk.Bool(existingSettings.DeletionProtectionEnabled),
		DiskAutoresize:            sdk.Bool(existingSettings.DiskAutoresize),
		DiskAutoresizeLimit:       sdk.Int(existingSettings.DiskAutoresizeLimit),
		DiskSize:                  sdk.Int(existingSettings.DiskSize),
		DiskType:                  sdk.String(existingSettings.DiskType),
		Edition:                   sdk.String(existingSettings.Edition),
		EnableDataplexIntegration: sdk.Bool(existingSettings.EnableDataplexIntegration),
		EnableGoogleMlIntegration: sdk.Bool(existingSettings.EnableGoogleMlIntegration),
		PricingPlan:               sdk.String(existingSettings.PricingPlan),
		TimeZone:                  sdk.String(existingSettings.TimeZone),
		UserLabels:                sdk.ToStringMap(existingSettings.UserLabels),
		// Note: Version is automatically managed by GCP and cannot be explicitly set
		// Other auto-managed fields are excluded to prevent configuration errors
	}
}
