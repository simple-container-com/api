package gcloud

import "api/pkg/api"

const ResourceTypePostgresGcpCloudsql = "gcp-cloudsql-postgres"

type PostgresGcpCloudsqlConfig struct {
	Version     string `json:"version" yaml:"version"`
	Project     string `json:"project" yaml:"project"`
	Credentials string `json:"credentials" yaml:"credentials"`
}

func PostgresqlGcpCloudsqlReadConfig(config any) (any, error) {
	return api.ConvertDescriptor(config, &PostgresGcpCloudsqlConfig{})
}
