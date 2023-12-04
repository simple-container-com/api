package api

const ResourceTypePostgresGcpCloudsql = "gcp-cloudsql-postgres"

type PostgresGcpCloudsqlConfig struct {
	Version     string `json:"version" yaml:"version"`
	Project     string `json:"project" yaml:"project"`
	Credentials string `json:"credentials" yaml:"credentials"`
}

func PostgresqlGcpCloudsqlReadConfig(config any) (any, error) {
	return ConvertDescriptor(config, &PostgresGcpCloudsqlConfig{})
}
