package yandex

import (
	"github.com/samber/lo"
	"github.com/simple-container-com/api/pkg/api"
)

type TemplateConfig struct {
	AccountConfig `json:",inline" yaml:",inline"`
}

type AccountConfig struct {
	Account         string `json:"account" yaml:"account"`
	api.Credentials `json:",inline" yaml:",inline"`
}

func (r *AccountConfig) ProviderType() string {
	return ProviderType
}

func (r *AccountConfig) CredentialsValue() string {
	return lo.If(r.Credentials.Credentials == "", api.AuthToString(r)).Else(r.Credentials.Credentials)
}

func (r *AccountConfig) ProjectIdValue() string {
	return r.Account
}

func ReadTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &TemplateConfig{})
}
