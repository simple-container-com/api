package gcloud

import (
	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

const (
	TemplateTypeGcpCloudrun = "cloudrun"
)

type AlertsConfig struct {
	MaxErrors MaxErrorConfig
	Discord   DiscordCfg
	Telegram  TelegramCfg
}

type TelegramCfg struct {
	DefaultChatId string
}

type DiscordCfg struct {
	WebhookId string
}

type MaxErrorConfig struct {
	ErrorLogMessageRegexp string
	MaxErrorCount         int
}

type CloudRunInput struct {
	TemplateConfig `json:"templateConfig" yaml:"templateConfig"`
	Deployment     k8s.DeploymentConfig `json:"deployment" yaml:"deployment"`
}

func (i *CloudRunInput) Uses() []string {
	return i.Deployment.StackConfig.Uses
}

func (i *CloudRunInput) OverriddenBaseZone() string {
	return i.Deployment.StackConfig.BaseDnsZone
}

func ToCloudRunConfig(tpl any, composeCfg compose.Config, stackCfg *api.StackConfigCompose) (any, error) {
	templateCfg, ok := tpl.(*TemplateConfig)
	if !ok {
		return nil, errors.Errorf("template config is not of type *gcloud.TemplateConfig")
	}
	if templateCfg == nil {
		return nil, errors.Errorf("template config is nil")
	}

	res := &CloudRunInput{
		TemplateConfig: *templateCfg,
		Deployment: k8s.DeploymentConfig{
			StackConfig: stackCfg,
		},
	}
	containers, err := k8s.ConvertComposeToContainers(composeCfg, stackCfg)
	if err != nil {
		return nil, err
	}
	res.Deployment.Containers = containers

	iContainer, err := findIngressContainer(composeCfg, containers)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to detect ingress container")
	}
	res.Deployment.IngressContainer = iContainer

	return res, nil
}

func findIngressContainer(composeCfg compose.Config, contaniers []k8s.CloudRunContainer) (*k8s.CloudRunContainer, error) {
	iContainers := lo.Filter(composeCfg.Project.Services, func(s types.ServiceConfig, _ int) bool {
		v, hasLabel := s.Labels[api.ComposeLabelIngressContainer]
		return hasLabel && v == "true"
	})
	if len(iContainers) > 1 {
		return nil, errors.Errorf("must have exactly 1 ingress container, but found (%v) in compose files %q,"+
			"did you forget to add label %q to the main container?",
			lo.Map(iContainers, func(item types.ServiceConfig, _ int) string {
				return item.Name
			}), composeCfg.Project.ComposeFiles, api.ComposeLabelIngressContainer)
	}
	iContainer, found := lo.Find(contaniers, func(item k8s.CloudRunContainer) bool {
		return item.Name == iContainers[0].Name
	})
	if !found {
		return nil, nil
	}
	return &iContainer, nil
}
