package aws

import (
	"github.com/simple-container-com/api/pkg/api"
)

const (
	ResourceTypeRdsMysql = "aws-rds-mysql"
)

type MysqlConfig struct {
	AccountConfig   `json:",inline" yaml:",inline"`
	Name            string  `json:"name" yaml:"name"`
	InstanceClass   string  `json:"instanceClass" yaml:"instanceClass"`
	AllocateStorage *int    `json:"allocateStorage" yaml:"allocateStorage"`
	EngineVersion   string  `json:"engineVersion" yaml:"engineVersion"`
	Username        string  `json:"username" yaml:"username"`
	Password        string  `json:"password" yaml:"password"`
	DatabaseName    *string `json:"databaseName" yaml:"databaseName"`
	EngineName      *string `json:"engineName,omitempty" yaml:"engineName,omitempty"`
}

func ReadRdsMysqlConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &MysqlConfig{})
}
