package gcloud

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypeArtifactRegistry = "gcp-artifact-registry"

type ArtifactRegistryConfig struct {
	Credentials `json:",inline" yaml:",inline"`
	Location    string        `json:"location" yaml:"location"`
	Public      *bool         `json:"public,omitempty" yaml:"public,omitempty"`
	Docker      *DockerConfig `json:"docker,omitempty" yaml:"docker,omitempty"`
	Domain      *string       `json:"domain" yaml:"domain"`
}

type DockerConfig struct {
	ImmutableTags *bool `json:"immutableTags" yaml:"immutableTags"`
}

func ArtifactRegistryConfigReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &ArtifactRegistryConfig{})
}
