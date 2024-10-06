package yandex

import (
	"github.com/pkg/errors"
	"github.com/simple-container-com/api/pkg/api"
)

const (
	TemplateTypeYandexCloudFunction = "yc-cloud-function"
)

type CloudFunctionInput struct {
	AccountConfig `json:",inline" yaml:",inline"`
	StackConfig   api.StackConfigSingleImage `json:"stackConfig" yaml:"stackConfig"`
}

func ToCloudFunctionConfig(tpl any, stackCfg *api.StackConfigSingleImage) (any, error) {
	templateCfg, ok := tpl.(*TemplateConfig)
	if !ok {
		return nil, errors.Errorf("template config is not of type yc.TemplateConfig")
	}

	if templateCfg == nil {
		return nil, errors.Errorf("template config is nil")
	}

	accountConfig := &AccountConfig{}
	err := api.ConvertAuth(&templateCfg.AccountConfig, accountConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert yc account config")
	}
	if stackCfg == nil {
		return nil, errors.Errorf("stack config cannot be nil")
	}

	res := &CloudFunctionInput{
		AccountConfig: *accountConfig,
		StackConfig:   *stackCfg,
	}

	return res, nil
}
