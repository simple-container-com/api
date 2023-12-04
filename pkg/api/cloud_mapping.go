package api

import "gopkg.in/yaml.v3"

type configReaderFunc func(any) (any, error)

var cloudMapping = map[string]configReaderFunc{
	// pulumi
	ProvisionerTypePulumi: PulumiReadProvisionerConfig,
	AuthTypePulumiToken:   PulumiReadAuthConfig,

	// gcloud
	SecretsTypeGCPSecretsManager: GcloudReadSecretsConfig,
	TemplateTypeGcpCloudrun:      GcloudReadTemplateConfig,
	AuthTypeGCPServiceAccount:    GcloudReadAuthServiceAccountConfig,

	// github actions
	CiCdTypeGithubActions: GithubACtionsReadCiCdConfig,

	// cloudflare
	RegistrarTypeCloudflare: CloudflareReadRegistrarConfig,

	// mongodb
	ResourceTypeMongodbAtlas: MondodbAtlasReadConfig,

	// postgres
	ResourceTypePostgresGcpCloudsql: PostgresqlGcpCloudsqlReadConfig,
}

func ConvertDescriptor[T any](from any, to *T) (*T, error) {
	if bytes, err := yaml.Marshal(from); err == nil {
		if err = yaml.Unmarshal(bytes, to); err != nil {
			return nil, err
		} else {
			return to, nil
		}
	} else {
		return nil, err
	}
}
