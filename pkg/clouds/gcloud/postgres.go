package gcloud

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypePostgresGcpCloudsql = "gcp-cloudsql-postgres"

type PostgresGcpCloudsqlConfig struct {
	Credentials        `json:",inline" yaml:",inline"`
	Version            string  `json:"version" yaml:"version"`
	Project            string  `json:"project" yaml:"project"`
	Tier               *string `json:"tier" yaml:"tier"`
	Region             *string `json:"region" yaml:"region"`
	MaxConnections     *int    `json:"maxConnections" yaml:"maxConnections"`
	DeletionProtection *bool   `json:"deletionProtection" yaml:"deletionProtection"`
}

func PostgresqlGcpCloudsqlReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &PostgresGcpCloudsqlConfig{})
}
