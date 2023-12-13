package cloudflare

import "api/pkg/api"

const RegistrarTypeCloudflare = "cloudflare"

type RegistrarConfig struct {
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
