package mongodb

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypeMongodbAtlas = "mongodb-atlas"

type AtlasConfig struct {
	Admins        []string     `json:"admins" yaml:"admins"`
	Developers    []string     `json:"developers" yaml:"developers"`
	InstanceSize  string       `json:"instanceSize" yaml:"instanceSize"`
	OrgId         string       `json:"orgId" yaml:"orgId"`
	ProjectId     string       `json:"projectId" yaml:"projectId"`
	ProjectName   string       `json:"projectName" yaml:"projectName"`
	Region        string       `json:"region" yaml:"region"`
	PrivateKey    string       `json:"privateKey" yaml:"privateKey"`
	PublicKey     string       `json:"publicKey" yaml:"publicKey"`
	CloudProvider string       `json:"cloudProvider" yaml:"cloudProvider"`
	Backup        *AtlasBackup `json:"backup" yaml:"backup"`
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

func (r *AtlasConfig) ProviderType() string {
	return ProviderType
}
