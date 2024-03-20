package gcloud

import (
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

const TemplateTypeStaticWebsite = "gcp-static-website"

type StaticSiteInput struct {
	TemplateConfig         `json:"templateConfig" yaml:"templateConfig"`
	*api.StackConfigStatic `json:",inline" yaml:",inline"`
	RootDir                string `json:"rootDir" yaml:"rootDir"`
	StackName              string `json:"stackName" yaml:"stackName"`
}

func ToStaticSiteConfig(tpl any, rootDir, stackName string, stackCfg *api.StackConfigStatic) (any, error) {
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
		RootDir:           rootDir,
		StackName:         stackName,
	}

	return res, nil
}
