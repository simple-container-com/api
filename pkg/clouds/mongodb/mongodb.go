package mongodb

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypeMongodbAtlas = "mongodb-atlas"

type AtlasConfig struct {
	Admins         []string                      `json:"admins" yaml:"admins"`
	Developers     []string                      `json:"developers" yaml:"developers"`
	InstanceSize   string                        `json:"instanceSize" yaml:"instanceSize"`
	OrgId          string                        `json:"orgId" yaml:"orgId"`
	ProjectId      string                        `json:"projectId" yaml:"projectId"`
	ProjectName    string                        `json:"projectName" yaml:"projectName"`
	Region         string                        `json:"region" yaml:"region"`
	PrivateKey     string                        `json:"privateKey" yaml:"privateKey"`
	PublicKey      string                        `json:"publicKey" yaml:"publicKey"`
	CloudProvider  string                        `json:"cloudProvider" yaml:"cloudProvider"`
	Backup         *AtlasBackup                  `json:"backup,omitempty" yaml:"backup,omitempty"`
	NetworkConfig  *AtlasNetworkConfig           `json:"networkConfig,omitempty" yaml:"networkConfig,omitempty"`
	ExtraProviders map[string]api.AuthDescriptor `json:"extraProviders,omitempty" yaml:"extraProviders,omitempty"`
	DiskSizeGB     *float64                      `json:"diskSizeGB,omitempty" yaml:"diskSizeGB,omitempty"`
	NumShards      *int                          `json:"numShards,omitempty" yaml:"numShards,omitempty"`
	// Resource adoption fields
	Adopt       bool   `json:"adopt,omitempty" yaml:"adopt,omitempty"`
	ClusterName string `json:"clusterName,omitempty" yaml:"clusterName,omitempty"`
	// Deletion protection
	DeletionProtection bool `json:"deletionProtection,omitempty" yaml:"deletionProtection,omitempty"`
	// Naming strategy version control
	NamingStrategyVersion *int `json:"namingStrategyVersion,omitempty" yaml:"namingStrategyVersion,omitempty"` // Default 2 (new), use 1 for legacy
}

type AtlasNetworkConfig struct {
	PrivateLinkEndpoint *PrivateLinkEndpoint `json:"privateLinkEndpoint,omitempty" yaml:"privateLinkEndpoint,omitempty"`
	AllowAllIps         *bool                `json:"allowAllIps,omitempty" yaml:"allowAllIps,omitempty"`
	AllowCidrs          *[]string            `json:"allowCidrs,omitempty" yaml:"allowCidrs,omitempty"` // format "0.0.0.0/0"
}

type PrivateLinkEndpoint struct {
	ProviderName string `json:"providerName" yaml:"providerName"`
}

type AtlasBackup struct {
	Every     string `json:"every" yaml:"every"`         // e.g. 2h
	Retention string `json:"retention" yaml:"retention"` // e.g. 24h
}

func ReadAtlasConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &AtlasConfig{})
}

func (r *AtlasConfig) CredentialsValue() string {
	return r.PrivateKey
}

func (r *AtlasConfig) ProjectIdValue() string {
	return r.ProjectId
}

func (r *AtlasConfig) DependencyProviders() map[string]api.AuthDescriptor {
	return r.ExtraProviders
}

func (r *AtlasConfig) ProviderType() string {
	return ProviderType
}
