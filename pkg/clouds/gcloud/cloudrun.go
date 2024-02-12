package gcloud

import (
	"github.com/compose-spec/compose-go/types"
	"github.com/samber/lo"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

const (
	TemplateTypeGcpCloudrun = "cloudrun"
)

type CloudRunConfig struct {
	api.AuthConfig
	Name        string `json:"name,omitempty" yaml:"name"`
	Credentials string `json:"credentials" yaml:"credentials"`
	ProjectId   string `json:"projectId" yaml:"projectId"`
	Location    string `json:"location" yaml:"location"`
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
	Scale      CloudRunScale
	Containers []CloudRunContainer
	Config     CloudRunConfig
}

func CloudRunConfigFromCompose(crCfg CloudRunConfig, composeCfg compose.Config, stackCfg api.StackConfig) (CloudRunInput, error) {
	var res CloudRunInput

	services := lo.Associate(composeCfg.Project.Services, func(svc types.ServiceConfig) (string, types.ServiceConfig) {
		return svc.Name, svc
	})
	res.Containers = lo.Map(stackCfg.Runs, func(svcName string, _ int) CloudRunContainer {
		svc := services[svcName]
		return CloudRunContainer{
			Name: svcName,
			Image: CloudRunImage{
				Context:    svc.Build.Context,
				Platform:   ImagePlatformLinuxAmd64,
				Dockerfile: svc.Build.Dockerfile,
			},
			Env:           toRunEnv(svc.Environment),
			Secrets:       toRunSecrets(svc.Environment),
			Port:          toRunPort(svc.Ports),
			LivenessProbe: toLivenessProbe(svc.HealthCheck),
			StartupProbe:  toStartupProbe(svc.HealthCheck),
			Resources:     toResources(svc),
		}
	})

	return res, nil
}

func toRunPort(ports []types.ServicePortConfig) int {
	return 0
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
	return map[string]string{}
}

func toRunEnv(environment types.MappingWithEquals) map[string]string {
	return map[string]string{}
}

func (r *CloudRunConfig) CredentialsValue() string {
	return r.Credentials
}

func (r *CloudRunConfig) ProjectIdValue() string {
	return r.ProjectId
}
