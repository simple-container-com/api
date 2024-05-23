package aws

import (
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

const TemplateTypeStaticWebsite = "aws-static-website"

type StaticSiteInput struct {
	TemplateConfig        `json:"templateConfig" yaml:"templateConfig"`
	api.StackConfigStatic `json:",inline" yaml:",inline"`
	StackDir              string `json:"stackDir" yaml:"stackDir"`
	StackName             string `json:"stackName" yaml:"stackName"`
}

func ToStaticSiteConfig(tpl any, stackDir, stackName string, stackCfg *api.StackConfigStatic) (any, error) {
	templateCfg, ok := tpl.(*TemplateConfig)
	if !ok {
		return nil, errors.Errorf("template config is not of type aws.TemplateConfig")
	}

	if templateCfg == nil {
		return nil, errors.Errorf("template config is nil")
	}

	if stackCfg == nil {
		return nil, errors.Errorf("stack config is nil")
	}

	accountConfig := &AccountConfig{}
	err := api.ConvertAuth(&templateCfg.AccountConfig, accountConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert aws account config")
	}
	templateCfg.AccountConfig = *accountConfig

	res := &StaticSiteInput{
		TemplateConfig:    *templateCfg,
		StackConfigStatic: *stackCfg,
		StackDir:          stackDir,
		StackName:         stackName,
	}

	return res, nil
}
