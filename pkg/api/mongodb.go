package api

const ResourceTypeMongodbAtlas = "mongodb-atlas"

type MongodbAtlasConfig struct {
	Admins       []string `json:"admins" yaml:"admins"`
	Developers   []string `json:"developers" yaml:"developers"`
	InstanceSize string   `json:"instanceSize" yaml:"instanceSize"`
	OrgId        string   `json:"orgId" yaml:"orgId"`
	ProjectId    string   `json:"projectId" yaml:"projectId"`
	ProjectName  string   `json:"projectName" yaml:"projectName"`
	Region       string   `json:"region" yaml:"region"`
	PrivateKey   string   `json:"privateKey" yaml:"privateKey"`
	PublicKey    string   `json:"publicKey" yaml:"publicKey"`
}

func MondodbAtlasReadConfig(config any) (any, error) {
	return ConvertDescriptor(config, &MongodbAtlasConfig{})
}
