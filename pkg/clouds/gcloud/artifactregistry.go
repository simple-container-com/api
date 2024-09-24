package gcloud

import "github.com/simple-container-com/api/pkg/api"

const (
	ResourceTypeArtifactRegistry      = "gcp-artifact-registry"
	ResourceTypeRemoteDockerImagePush = "gcp-docker-image-push"
)

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

type RemoteImagePush struct {
	Credentials              `json:",inline" yaml:",inline"`
	RemoteImage              string  `json:"remoteImage" yaml:"remoteImage"`
	Name                     string  `json:"name" yaml:"name"`
	Tag                      string  `json:"tag" yaml:"tag"`
	ArtifactRegistryResource string  `json:"artifactRegistryResource" yaml:"artifactRegistryResource"`
	RegistryCredentials      *string `json:"registryCredentials" yaml:"registryCredentials"` // TODO: support other registries' creds
}

func (i *RemoteImagePush) DependsOnResources() []api.ParentResourceDependency {
	return []api.ParentResourceDependency{
		{Name: i.ArtifactRegistryResource},
	}
}

func ArtifactRegistryConfigReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &ArtifactRegistryConfig{})
}

func DockerRemoteImagePushReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &RemoteImagePush{})
}
