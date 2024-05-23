package aws

import (
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	ResourceTypeRdsPostgres = "aws-rds-postgres"
)

type PostgresConfig struct {
	AccountConfig   `json:",inline" yaml:",inline"`
	Name            string `json:"name,omitempty" yaml:"name"`
	InstanceClass   string `json:"instanceClass" yaml:"instanceClass"`
	AllocateStorage *int   `json:"allocateStorage" yaml:"allocateStorage"`
	EngineVersion   string `json:"engineVersion" yaml:"engineVersion"`
	Username        string `json:"username" yaml:"username"`
	Password        string `json:"password" yaml:"password"`
}

func ReadRdsPostgresConfig(config *api.Config) (api.Config, error) {
	pgCfg := &PostgresConfig{}
	res, err := api.ConvertConfig(config, pgCfg)
	if err != nil {
		return *config, errors.Wrapf(err, "failed to convert rds postgres config")
	}
	accountConfig := &AccountConfig{}
	err = api.ConvertAuth(&pgCfg.AccountConfig, accountConfig)
	if err != nil {
		return *config, errors.Wrapf(err, "failed to convert aws account config")
	}
	pgCfg.AccountConfig = *accountConfig
	return res, nil
}
