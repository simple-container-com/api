package cloudflare

import "github.com/simple-container-com/api/pkg/api"

const RegistrarTypeCloudflare = "cloudflare"

type RegistrarConfig struct {
	api.AuthConfig
	Credentials string      `json:"credentials" yaml:"credentials"`
	Project     string      `json:"project" yaml:"project"`
	ZoneName    string      `json:"zoneName" yaml:"zoneName"`
	DnsRecords  []DnsRecord `json:"dnsRecords" yaml:"dnsRecords"`
}

type DnsRecord struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

func ReadRegistrarConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &RegistrarConfig{})
}

func (r *RegistrarConfig) CredentialsValue() string {
	return r.Credentials
}

func (r *RegistrarConfig) ProjectIdValue() string {
	return r.Project
}
