package gcloud

import "api/pkg/api"

const ResourceTypePostgresGcpCloudsql = "gcp-cloudsql-postgres"

type PostgresGcpCloudsqlConfig struct {
	Version     string `json:"version" yaml:"version"`
	Project     string `json:"project" yaml:"project"`
	Credentials string `json:"credentials" yaml:"credentials"`
	ProjectId   string `json:"projectId" yaml:"projectId"`
}

func PostgresqlGcpCloudsqlReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &PostgresGcpCloudsqlConfig{})
}
