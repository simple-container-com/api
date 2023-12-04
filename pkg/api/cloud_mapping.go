package api

type configReaderFunc func(any) (any, error)

var cloudMapping = map[string]configReaderFunc{
	// pulumi
	ProvisionerTypePulumi: PulumiReadProvisionerConfig,

	// gcloud
	SecretsTypeGCPSecretsManager: GcloudReadSecretsConfig,
	TemplateTypeGcpCloudrun:      GcloudReadTemplateConfig,

	// github actions
	CiCdTypeGithubActions: GithubACtionsReadCiCdConfig,

	// cloudflare
	RegistrarTypeCloudflare: CloudflareReadRegistrarConfig,

	// mongodb
	ResourceTypeMongodbAtlas: MondodbAtlasReadConfig,

	// postgres
	ResourceTypePostgresGcpCloudsql: PostgresqlGcpCloudsqlReadConfig,
}
