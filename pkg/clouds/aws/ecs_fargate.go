package aws

import (
	"fmt"

	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

const (
	TemplateTypeEcsFargate = "ecs-fargate"
)

type EcsFargateConfig struct {
	api.Credentials  `json:",inline" yaml:",inline"`
	AwsAccountConfig `json:",inline" yaml:",inline"`
	Name             string `json:"name,omitempty" yaml:"name"`
	Account          string `json:"account" yaml:"account"`
	Region           string `json:"region" yaml:"region"`
}

type ImagePlatform string

const (
	ImagePlatformLinuxAmd64 ImagePlatform = "linux/amd64"
)

type EcsFargateImage struct {
	Context    string
	Dockerfile string
	Platform   ImagePlatform
}

type EcsFargateProbe struct {
	HttpGet             ProbeHttpGet
	InitialDelaySeconds int
}

type EcsFargateResources struct {
	Limits   map[string]string
	Requests map[string]string
}

type ProbeHttpGet struct {
	Path string
	Port int
}

type EcsFargateContainer struct {
	Name          string
	Image         EcsFargateImage
	Env           map[string]string
	Secrets       map[string]string
	Port          int
	LivenessProbe EcsFargateProbe
	StartupProbe  EcsFargateProbe
	Resources     EcsFargateResources
}

type EcsFargateScale struct {
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

type EcsFargateInput struct {
	TemplateConfig `json:"templateConfig" yaml:"templateConfig"`
	Scale          EcsFargateScale       `json:"scale" yaml:"scale"`
	Containers     []EcsFargateContainer `json:"containers" yaml:"containers"`
	Config         EcsFargateConfig      `json:"config" yaml:"config"`
}

func ToEcsFargateConfig(tpl any, composeCfg compose.Config, stackCfg api.StackClientDescriptor) (any, error) {
	templateCfg, ok := tpl.(*TemplateConfig)
	if !ok {
		return nil, errors.Errorf("template config is not of type aws.TemplateConfig")
	}

	if templateCfg == nil {
		return nil, errors.Errorf("template config is nil")
	}

	res := &EcsFargateInput{
		TemplateConfig: *templateCfg,
		Config: EcsFargateConfig{
			Credentials:      templateCfg.Credentials,
			AwsAccountConfig: templateCfg.AwsAccountConfig,
			Name:             fmt.Sprintf("%s-%s", stackCfg.ParentStack, stackCfg.Environment),
			Region:           templateCfg.Region,
		},
	}

	if composeCfg.Project == nil {
		return nil, errors.Errorf("compose config is nil")
	}

	services := lo.Associate(composeCfg.Project.Services, func(svc types.ServiceConfig) (string, types.ServiceConfig) {
		return svc.Name, svc
	})

	for _, svcName := range stackCfg.Config.Runs {
		svc := services[svcName]
		port, err := toRunPort(svc.Ports)
		if err != nil {
			return EcsFargateInput{}, errors.Wrapf(err, "service %s", svcName)
		}

		res.Containers = append(res.Containers, EcsFargateContainer{
			Name: svcName,
			Image: EcsFargateImage{
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
	return 0, errors.Errorf("expected 1 port, got %d", len(ports))
}

func toResources(svc types.ServiceConfig) EcsFargateResources {
	return EcsFargateResources{}
}

func toStartupProbe(check *types.HealthCheckConfig) EcsFargateProbe {
	return EcsFargateProbe{}
}

func toLivenessProbe(check *types.HealthCheckConfig) EcsFargateProbe {
	return EcsFargateProbe{}
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

func (r *EcsFargateConfig) CredentialsValue() string {
	return api.AuthToString(r)
}

func (r *EcsFargateConfig) ProjectIdValue() string {
	return r.Account
}
