package gcloud

import (
	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

const (
	TemplateTypeGcpCloudrun = "cloudrun"
)

type CloudRunConfig struct {
	ServiceAccountConfig `json:",inline" yaml:",inline"`
	api.Credentials      `json:",inline" yaml:",inline"`
	Name                 string `json:"name,omitempty" yaml:"name"`
	Location             string `json:"location" yaml:"location"`
}

type ImagePlatform string

const (
	ImagePlatformLinuxAmd64 ImagePlatform = "linux/amd64"
)

type CloudRunImage struct {
	Context    string
	Dockerfile string
	Platform   ImagePlatform
}

type CloudRunProbe struct {
	HttpGet             ProbeHttpGet
	InitialDelaySeconds int
}

type CloudRunResources struct {
	Limits   map[string]string
	Requests map[string]string
}

type ProbeHttpGet struct {
	Path string
	Port int
}

type CloudRunContainer struct {
	Name          string
	Image         CloudRunImage
	Env           map[string]string
	Secrets       map[string]string
	Port          int
	LivenessProbe CloudRunProbe
	StartupProbe  CloudRunProbe
	Resources     CloudRunResources
}

type CloudRunScale struct {
	Min int
	Max int
}

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
	Scale          CloudRunScale       `json:"scale" yaml:"scale"`
	Containers     []CloudRunContainer `json:"containers" yaml:"containers"`
	Config         CloudRunConfig      `json:"config" yaml:"config"`
}

func ToCloudRunConfig(tpl any, composeCfg compose.Config, stackCfg api.StackClientDescriptor) (any, error) {
	templateCfg, ok := tpl.(*TemplateConfig)
	if !ok {
		return nil, errors.Errorf("template config is not of type gcloud.TemplateConfig")
	}
	if templateCfg == nil {
		return nil, errors.Errorf("template config is nil")
	}

	res := CloudRunInput{
		TemplateConfig: *templateCfg,
	}

	services := lo.Associate(composeCfg.Project.Services, func(svc types.ServiceConfig) (string, types.ServiceConfig) {
		return svc.Name, svc
	})

	for _, svcName := range stackCfg.Config.Runs {
		svc := services[svcName]
		port, err := toRunPort(svc.Ports)
		if err != nil {
			return nil, errors.Wrapf(err, "error converting service %s to cloudrun", svcName)
		}

		res.Containers = append(res.Containers, CloudRunContainer{
			Name: svcName,
			Image: CloudRunImage{
				Context:    svc.Build.Context,
				Platform:   ImagePlatformLinuxAmd64,
				Dockerfile: svc.Build.Dockerfile,
			},
			Env:           toRunEnv(svc.Environment),
			Secrets:       toRunSecrets(svc.Environment),
			Port:          port,
			LivenessProbe: toLivenessProbe(svc.HealthCheck),
			StartupProbe:  toStartupProbe(svc.HealthCheck),
			Resources:     toResources(svc),
		})
	}

	return res, nil
}

func toRunPort(ports []types.ServicePortConfig) (int, error) {
	if len(ports) == 1 {
		return int(ports[0].Target), nil
	}
	return 0, errors.Errorf("exactly one port must be configured for service, but got: %d", len(ports))
}

func toResources(svc types.ServiceConfig) CloudRunResources {
	return CloudRunResources{}
}

func toStartupProbe(check *types.HealthCheckConfig) CloudRunProbe {
	return CloudRunProbe{}
}

func toLivenessProbe(check *types.HealthCheckConfig) CloudRunProbe {
	return CloudRunProbe{}
}

func toRunSecrets(environment types.MappingWithEquals) map[string]string {
	// TODO: implement secrets with ${secret:blah}
	return map[string]string{}
}

func toRunEnv(environment types.MappingWithEquals) map[string]string {
	res := make(map[string]string)
	for env, envVal := range environment {
		if envVal != nil {
			res[env] = *envVal
		}
	}
	return res
}
