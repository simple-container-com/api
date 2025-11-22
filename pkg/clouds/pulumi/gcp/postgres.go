package gcp

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/sql"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func Postgres(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.ResourceTypePostgresGcpCloudsql {
		return nil, errors.Errorf("unsupported postgres type %q", input.Descriptor.Type)
	}

	pgCfg, ok := input.Descriptor.Config.Config.(*gcloud.PostgresGcpCloudsqlConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert postgresql config for %q", input.Descriptor.Type)
	}

	// Handle resource adoption - exit early if adopting
	if pgCfg.Adopt {
		return AdoptPostgres(ctx, stack, input, params)
	}

	containerServiceName := fmt.Sprintf("projects/%s/services/sqladmin.googleapis.com", pgCfg.ProjectId)
	if err := enableServicesAPI(ctx.Context(), input.Descriptor.Config.Config, containerServiceName); err != nil {
		return nil, errors.Wrapf(err, "failed to enable %s", containerServiceName)
	}

	postgresName := toPostgresName(input, input.Descriptor.Name)
	rootPasswordExport := toPostgresRootPasswordExport(postgresName)
	rootPassword, err := random.NewRandomPassword(ctx, rootPasswordExport, &random.RandomPasswordArgs{
		Length:          sdk.Int(16),
		OverrideSpecial: sdk.String("-_"),
		Special:         sdk.Bool(true),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate root postgres password")
	}
	ctx.Export(rootPasswordExport, rootPassword.Result)

	var databaseFlags sql.DatabaseInstanceSettingsDatabaseFlagArray

	if pgCfg.MaxConnections != nil {
		databaseFlags = append(databaseFlags, sql.DatabaseInstanceSettingsDatabaseFlagArgs{
			Name:  sdk.String("max_connections"),
			Value: sdk.String(fmt.Sprintf("%d", *pgCfg.MaxConnections)),
		})
	}

	pgInstance, err := sql.NewDatabaseInstance(ctx, postgresName, &sql.DatabaseInstanceArgs{
		Name:            sdk.String(postgresName),
		Region:          sdk.StringPtrFromPtr(lo.If(pgCfg.Region != nil, pgCfg.Region).Else(nil)),
		DatabaseVersion: sdk.String(pgCfg.Version),
		RootPassword:    rootPassword.Result,
		Settings: &sql.DatabaseInstanceSettingsArgs{
			Tier:          sdk.String(lo.If(pgCfg.Tier != nil, lo.FromPtr(pgCfg.Tier)).Else("db-f1-micro")),
			DatabaseFlags: databaseFlags,
			InsightsConfig: &sql.DatabaseInstanceSettingsInsightsConfigArgs{
				QueryInsightsEnabled: sdk.Bool(
					lo.If(pgCfg.QueryInsightsEnabled != nil, lo.FromPtr(pgCfg.QueryInsightsEnabled)).Else(true),
				),
				QueryStringLength: sdk.Int(
					lo.If(pgCfg.QueryStringLength != nil, lo.FromPtr(pgCfg.QueryStringLength)).Else(2048),
				),
			},
		},
		DeletionProtection: sdk.Bool(pgCfg.DeletionProtection != nil && *pgCfg.DeletionProtection),
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision postgres instance %q", postgresName)
	}

	return &api.ResourceOutput{Ref: pgInstance}, nil
}

func toPostgresRootPasswordExport(resName string) string {
	return fmt.Sprintf("%s-root-password", resName)
}

func toPostgresName(input api.ResourceInput, resName string) string {
	return input.ToResName(resName)
}
