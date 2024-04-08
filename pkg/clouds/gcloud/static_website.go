package gcloud

import (
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

const TemplateTypeStaticWebsite = "gcp-static-website"

type StaticSiteInput struct {
	TemplateConfig         `json:"templateConfig" yaml:"templateConfig"`
	*api.StackConfigStatic `json:",inline" yaml:",inline"`
	StackDir               string `json:"stackDir" yaml:"stackDir"`
	StackName              string `json:"stackName" yaml:"stackName"`
	Location               string `json:"location" yaml:"location"`
}

func ToStaticSiteConfig(tpl any, stackDir, stackName string, stackCfg *api.StackConfigStatic) (any, error) {
	templateCfg, ok := tpl.(*TemplateConfig)
	if !ok {
		return nil, errors.Errorf("template config is not of type aws.TemplateConfig")
	}

	if templateCfg == nil {
		return nil, errors.Errorf("template config is nil")
	}

	res := &StaticSiteInput{
		TemplateConfig:    *templateCfg,
		StackConfigStatic: stackCfg,
		StackDir:          stackDir,
		StackName:         stackName,
	}

	return res, nil
}
