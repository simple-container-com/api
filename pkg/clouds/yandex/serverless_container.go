package yandex

import (
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	TemplateTypeYandexServerlessContainer = "yandex-serverless-container"
)

type ServerlessContainerInput struct {
	AccountConfig `json:",inline" yaml:",inline"`
	StackConfig   api.StackConfigSingleImage `json:"stackConfig" yaml:"stackConfig"`
}

func ToServerlessContainerConfig(tpl any, stackCfg *api.StackConfigSingleImage) (any, error) {
	templateCfg, ok := tpl.(*TemplateConfig)
	if !ok {
		return nil, errors.Errorf("template config is not of type yandex.TemplateConfig")
	}

	if templateCfg == nil {
		return nil, errors.Errorf("template config is nil")
	}

	accountConfig := &AccountConfig{
		CloudId:     templateCfg.AccountConfig.CloudId,
		Credentials: templateCfg.AccountConfig.Credentials,
	}

	res := &ServerlessContainerInput{
		AccountConfig: *accountConfig,
		StackConfig:   *stackCfg,
	}

	return res, nil
}
