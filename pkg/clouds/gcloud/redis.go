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
}

func RedisReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &RedisConfig{})
}
