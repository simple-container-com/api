package cloudflare

import "github.com/simple-container-com/api/pkg/api"

const RegistrarTypeCloudflare = "cloudflare"

type AuthConfig struct {
	api.Credentials `json:",inline" yaml:",inline"`
	Project         string `json:"project" yaml:"project"`
}

type RegistrarConfig struct {
	AuthConfig `json:",inline" yaml:",inline"`
	ZoneName   string      `json:"zoneName" yaml:"zoneName"`
	DnsRecords []DnsRecord `json:"dnsRecords" yaml:"dnsRecords"`
}

type DnsRecord struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

func ReadRegistrarConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &RegistrarConfig{})
}

func (r *AuthConfig) CredentialsValue() string {
	return r.Credentials.Credentials
}

func (r *AuthConfig) ProjectIdValue() string {
	return r.Project
}
