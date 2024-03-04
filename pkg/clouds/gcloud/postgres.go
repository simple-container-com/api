package gcloud

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypePostgresGcpCloudsql = "gcp-cloudsql-postgres"

type PostgresGcpCloudsqlConfig struct {
	api.AuthConfig
	api.Credentials `json:",inline" yaml:",inline"`
	Version         string `json:"version" yaml:"version"`
	Project         string `json:"project" yaml:"project"`
	ProjectId       string `json:"projectId" yaml:"projectId"`
}

func PostgresqlGcpCloudsqlReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &PostgresGcpCloudsqlConfig{})
}

func (r *PostgresGcpCloudsqlConfig) CredentialsValue() string {
	return r.Credentials.Credentials
}

func (r *PostgresGcpCloudsqlConfig) ProjectIdValue() string {
	return r.ProjectId
}
