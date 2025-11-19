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

	// Import existing Cloud SQL instance into Pulumi state
	// The instance resource ID in GCP is: projects/{project}/instances/{instance}
	instanceResourceId := fmt.Sprintf("projects/%s/instances/%s", pgCfg.ProjectId, pgCfg.InstanceName)

	opts := []sdk.ResourceOption{
		sdk.Provider(params.Provider),
		// Import the existing instance without creating or modifying it
		sdk.Import(sdk.ID(instanceResourceId)),
	}

	pgInstance, err := sql.NewDatabaseInstance(ctx, postgresName, &sql.DatabaseInstanceArgs{
		Name:            sdk.String(pgCfg.InstanceName),
		DatabaseVersion: sdk.String(pgCfg.Version),
		// Note: We don't need to specify all the instance configuration since we're importing
		// Pulumi will read the current state from GCP
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
		ctx.Export(rootPasswordExport, sdk.String(pgCfg.RootPassword))
	} else {
		// For backward compatibility, we can also try to read from secrets
		params.Log.Warn(ctx.Context(), "rootPassword not provided in config for adopted instance %q, compute processor will need to handle this", pgCfg.InstanceName)
		ctx.Export(rootPasswordExport, sdk.String("")) // Empty, compute processor will handle
	}

	params.Log.Info(ctx.Context(), "successfully adopted Cloud SQL Postgres instance %q", pgCfg.InstanceName)

	return &api.ResourceOutput{Ref: pgInstance}, nil
}
