package yandex

import (
	"github.com/pkg/errors"
	"github.com/simple-container-com/api/pkg/api"
)

const (
	TemplateTypeYandexCloudFunction = "yandex-cloud-function"
)

type CloudFunctionInput struct {
	AccountConfig `json:",inline" yaml:",inline"`
	StackConfig   api.StackConfigSingleImage `json:"stackConfig" yaml:"stackConfig"`
}

func ToCloudFunctionConfig(tpl any, stackCfg *api.StackConfigSingleImage) (any, error) {
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

	res := &CloudFunctionInput{
		AccountConfig: *accountConfig,
		StackConfig:   *stackCfg,
	}

	return res, nil
}
