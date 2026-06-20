// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package cloudflare

import "github.com/simple-container-com/api/pkg/api"

const RegistrarType = "cloudflare"

type AuthConfig struct {
	api.Credentials `json:",inline" yaml:",inline"`
	AccountId       string `json:"accountId" yaml:"accountId"`
}

type RegistrarConfig struct {
	AuthConfig `json:",inline" yaml:",inline"`
	ZoneName   string          `json:"zoneName" yaml:"zoneName"`
	Records    []api.DnsRecord `json:"dnsRecords" yaml:"dnsRecords"`
}

func ReadRegistrarConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &RegistrarConfig{})
}

func (r *AuthConfig) CredentialsValue() string {
	return r.Credentials.Credentials
}

func (r *AuthConfig) ProjectIdValue() string {
	return r.AccountId
}

func (r *AuthConfig) ProviderType() string {
	return ProviderType
}

func (r *RegistrarConfig) DnsRecords() []api.DnsRecord {
	return r.Records
}
