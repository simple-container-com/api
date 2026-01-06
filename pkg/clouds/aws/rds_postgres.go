package aws

import (
	"github.com/simple-container-com/api/pkg/api"
)

const (
	ResourceTypeRdsPostgres = "aws-rds-postgres"
)

type PostgresConfig struct {
	AccountConfig   `json:",inline" yaml:",inline"`
	Name            string  `json:"name" yaml:"name"`
	InstanceClass   string  `json:"instanceClass" yaml:"instanceClass"`
	AllocateStorage *int    `json:"allocateStorage" yaml:"allocateStorage"`
	EngineVersion   string  `json:"engineVersion" yaml:"engineVersion"`
	Username        string  `json:"username" yaml:"username"`
	Password        string  `json:"password" yaml:"password"`
	DatabaseName    *string `json:"databaseName" yaml:"databaseName"`
	InitSQL         *string `json:"initSQL,omitempty" yaml:"initSQL,omitempty"`
}

func ReadRdsPostgresConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &PostgresConfig{})
}
