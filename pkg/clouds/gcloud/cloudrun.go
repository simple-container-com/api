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
	Image         api.ContainerImage
	Env           map[string]string
	Secrets       map[string]string
	Port          int
	LivenessProbe CloudRunProbe
	StartupProbe  CloudRunProbe
	ComposeDir    string
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
	TemplateConfig   `json:"templateConfig" yaml:"templateConfig"`
	Scale            CloudRunScale       `json:"scale" yaml:"scale"`
	Containers       []CloudRunContainer `json:"containers" yaml:"containers"`
	RefResourceNames []string            `json:"refResourceNames" yaml:"refResourceNames"`
	BaseDnsZone      string              `json:"baseDnsZOne" yaml:"baseDnsZOne"`
}

func (i *CloudRunInput) Uses() []string {
	return i.RefResourceNames
}

func (i *CloudRunInput) OverriddenBaseZone() string {
	return i.BaseDnsZone
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
		TemplateConfig:   *templateCfg,
		RefResourceNames: stackCfg.Uses,
		BaseDnsZone:      stackCfg.BaseDnsZone,
	}
	containers, err := convertComposeToContainers(composeCfg, stackCfg)
	if err != nil {
		return nil, err
	}
	res.Containers = containers

	return res, nil
}

func convertComposeToContainers(composeCfg compose.Config, stackCfg *api.StackConfigCompose) ([]CloudRunContainer, error) {
	if composeCfg.Project == nil {
		return nil, errors.Errorf("compose config is nil")
	}

	services := lo.Associate(composeCfg.Project.Services, func(svc types.ServiceConfig) (string, types.ServiceConfig) {
		return svc.Name, svc
	})

	var containers []CloudRunContainer

	for _, svcName := range stackCfg.Runs {
		svc := services[svcName]
		port, err := toRunPort(svc.Ports)
		if err != nil {
			return nil, errors.Wrapf(err, "error converting service %s to cloud container", svcName)
		}

		context := ""
		dockerFile := ""
		buildArgs := make(map[string]string)
		if svc.Build != nil {
			context = svc.Build.Context
			dockerFile = svc.Build.Dockerfile
			buildArgs = lo.MapValues(svc.Build.Args, func(value *string, _ string) string {
				return lo.FromPtr(value)
			})
		}

		containers = append(containers, CloudRunContainer{
			Name: svcName,
			Image: api.ContainerImage{
				Context:    context,
				Platform:   api.ImagePlatformLinuxAmd64,
				Dockerfile: dockerFile,
				Build: &api.ContainerImageBuild{
					Args: buildArgs,
				},
			},
			ComposeDir:    composeCfg.Project.WorkingDir,
			Env:           toRunEnv(svc.Environment),
			Secrets:       toRunSecrets(svc.Environment),
			Port:          port,
			LivenessProbe: toLivenessProbe(svc.HealthCheck),
			StartupProbe:  toStartupProbe(svc.HealthCheck),
			Resources:     toResources(svc),
		})
	}
	return containers, nil
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
