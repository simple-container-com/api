package gcloud

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypeRedis = "gcp-redis"

type RedisConfig struct {
	Credentials  `json:",inline" yaml:",inline"`
	Version      string            `json:"version" yaml:"version"`
	Project      string            `json:"project" yaml:"project"`
	MemorySizeGb int               `json:"memorySizeGb" yaml:"memorySizeGb"`
	RedisConfig  map[string]string `json:"redisConfig" yaml:"redisConfig"`
	Region       *string           `json:"region" yaml:"region"`

	// VPC Network Configuration
	AuthorizedNetwork *string `json:"authorizedNetwork,omitempty" yaml:"authorizedNetwork,omitempty"` // VPC network for Redis connectivity

	// Resource adoption fields
	Adopt      bool   `json:"adopt,omitempty" yaml:"adopt,omitempty"`
	InstanceId string `json:"instanceId,omitempty" yaml:"instanceId,omitempty"`
}

func RedisReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &RedisConfig{})
}
