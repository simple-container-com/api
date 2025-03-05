package yandex

import (
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	AuthTypeYandex = "yandex"
)

type TemplateConfig struct {
	AccountConfig `json:",inline" yaml:",inline"`
}

type AccountConfig struct {
	CloudId         string `json:"cloudId" yaml:"cloudId"`
	api.Credentials `json:",inline" yaml:",inline"`
}

func (r *AccountConfig) ProviderType() string {
	return ProviderType
}

func (r *AccountConfig) CredentialsValue() string {
	return lo.If(r.Credentials.Credentials == "", api.AuthToString(r)).Else(r.Credentials.Credentials)
}

func (r *AccountConfig) ProjectIdValue() string {
	return r.CloudId
}

func ReadTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &TemplateConfig{})
}

func ReadYandexAuthConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &AccountConfig{})
}
