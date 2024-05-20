package aws

import (
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	TemplateTypeAwsLambda = "aws-lambda"
)

type LambdaInput struct {
	AccountConfig `json:",inline" yaml:",inline"`
	StackConfig   api.StackConfigSingleImage `json:"stackConfig" yaml:"stackConfig"`
}

func (l *LambdaInput) Uses() []string {
	return l.StackConfig.Uses
}

func ToAwsLambdaConfig(tpl any, stackCfg *api.StackConfigSingleImage) (any, error) {
	templateCfg, ok := tpl.(*TemplateConfig)
	if !ok {
		return nil, errors.Errorf("template config is not of type aws.TemplateConfig")
	}

	if templateCfg == nil {
		return nil, errors.Errorf("template config is nil")
	}

	accountConfig := &AccountConfig{}
	err := api.ConvertAuth(&templateCfg.AccountConfig, accountConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert aws account config")
	}
	if stackCfg == nil {
		return nil, errors.Errorf("stack config cannot be nil")
	}

	res := &LambdaInput{
		AccountConfig: *accountConfig,
		StackConfig:   *stackCfg,
	}

	return res, nil
}
