package aws

import (
	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"time"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

const (
	TemplateTypeEcsFargate = "ecs-fargate"
)

type EcsFargateConfig struct {
	api.Credentials `json:",inline" yaml:",inline"`
	AccountConfig   `json:",inline" yaml:",inline"`
	Account         string `json:"account" yaml:"account"`
	Region          string `json:"region" yaml:"region"`
	Cpu             int    `json:"cpu" yaml:"cpu"`
	Memory          int    `json:"memory" yaml:"memory"`
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
	HttpGet             ProbeHttpGet `json:"httpGet" yaml:"httpGet"`
	InitialDelaySeconds int          `json:"initialDelaySeconds" yaml:"initialDelaySeconds"`
	TimeoutSeconds      int          `json:"timeoutSeconds" yaml:"timeoutSeconds"`
	IntervalSeconds     int          `json:"intervalSeconds" yaml:"intervalSeconds"`
	Retries             int          `json:"retries" yaml:"retries"`
}

type EcsFargateResources struct {
	Limits   map[string]string `json:"limits" yaml:"limits"`
	Requests map[string]string `json:"requests" yaml:"requests"`
}

type ProbeHttpGet struct {
	Path string `json:"path" yaml:"path"`
	Port int    `json:"port" yaml:"port"`
}

type EcsFargateContainer struct {
	Name          string            `json:"name" yaml:"name"`
	Image         EcsFargateImage   `json:"image" yaml:"image"`
	Env           map[string]string `json:"env" yaml:"env"`
	Secrets       map[string]string `json:"secrets" yaml:"secrets"`
	Port          int               `json:"port" yaml:"port"`
	LivenessProbe EcsFargateProbe   `json:"livenessProbe" yaml:"livenessProbe"`
	StartupProbe  EcsFargateProbe   `json:"startupProbe" yaml:"startupProbe"`
	Cpu           int               `json:"cpu" yaml:"cpu"`
	Memory        int               `json:"memory" yaml:"memory"`
}

type EcsFargateScale struct {
	Min int `json:"min" yaml:"min"`
	Max int `json:"max" yaml:"max"`
}

type AlertsConfig struct {
	MaxErrors MaxErrorConfig `json:"maxErrors" yaml:"maxErrors"`
	Discord   DiscordCfg     `json:"discord" yaml:"discord"`
	Telegram  TelegramCfg    `json:"telegram" yaml:"telegram"`
}

type TelegramCfg struct {
	DefaultChatId string `json:"defaultChatId" yaml:"defaultChatId"`
}

type DiscordCfg struct {
	WebhookId string `json:"webhookId" yaml:"webhookId"`
}

type MaxErrorConfig struct {
	ErrorLogMessageRegexp string `json:"errorLogMessageRegexp" yaml:"errorLogMessageRegexp"`
	MaxErrorCount         int    `json:"maxErrorCount" yaml:"maxErrorCount"`
}

type EcsFargateInput struct {
	TemplateConfig `json:"templateConfig" yaml:"templateConfig"`
	Scale          EcsFargateScale       `json:"scale" yaml:"scale"`
	Containers     []EcsFargateContainer `json:"containers" yaml:"containers"`
	Config         EcsFargateConfig      `json:"config" yaml:"config"`
}

func ToEcsFargateConfig(tpl any, composeCfg compose.Config, stackCfg *api.StackConfigCompose) (any, error) {
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
			Credentials:   templateCfg.Credentials,
			AccountConfig: templateCfg.AccountConfig,
			Region:        templateCfg.Region,
		},
	}

	if composeCfg.Project == nil {
		return nil, errors.Errorf("compose config is nil")
	}

	services := lo.Associate(composeCfg.Project.Services, func(svc types.ServiceConfig) (string, types.ServiceConfig) {
		return svc.Name, svc
	})

	for _, svcName := range stackCfg.Runs {
		svc := services[svcName]
		port, err := toRunPort(svc.Ports)
		if err != nil {
			return EcsFargateInput{}, errors.Wrapf(err, "service %s", svcName)
		}

		liveProbe, err := toLivenessProbe(svc, port)
		if err != nil {
			return EcsFargateInput{}, errors.Wrapf(err, "service %s", svcName)
		}
		startProbe, err := toStartupProbe(svc, port)
		if err != nil {
			return EcsFargateInput{}, errors.Wrapf(err, "service %s", svcName)
		}
		secrets, err := toRunSecrets(svc.Environment)
		if err != nil {
			return EcsFargateInput{}, errors.Wrapf(err, "service %s", svcName)
		}
		res.Containers = append(res.Containers, EcsFargateContainer{
			Name: svcName,
			Image: EcsFargateImage{
				Context:    composeCfg.Project.RelativePath(svc.Build.Context),
				Platform:   ImagePlatformLinuxAmd64,
				Dockerfile: svc.Build.Dockerfile,
			},
			Env:           toRunEnv(svc.Environment),
			Secrets:       secrets,
			Port:          port,
			LivenessProbe: liveProbe,
			StartupProbe:  startProbe,
			// TODO: cpu, memory
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

func toStartupProbe(svc types.ServiceConfig, port int) (EcsFargateProbe, error) {
	res := EcsFargateProbe{
		HttpGet: ProbeHttpGet{
			Path: "/",
			Port: port,
		},
	}
	res.FromHealthCheck(svc.HealthCheck)
	return res, nil
}

func toLivenessProbe(svc types.ServiceConfig, port int) (EcsFargateProbe, error) {
	res := EcsFargateProbe{
		HttpGet: ProbeHttpGet{
			Path: "/",
			Port: port,
		},
	}
	res.FromHealthCheck(svc.HealthCheck)
	return res, nil
}

func (p *EcsFargateProbe) FromHealthCheck(check *types.HealthCheckConfig) {
	if check != nil {
		if check.Interval != nil {
			p.IntervalSeconds = int(time.Duration(lo.FromPtr(check.Interval)).Seconds())
		}
		if check.Retries != nil {
			p.Retries = int(lo.FromPtr(check.Retries))
		}
		if check.StartPeriod != nil {
			p.InitialDelaySeconds = int(time.Duration(lo.FromPtr(check.StartPeriod)).Seconds())
		}
	}
}

func toRunSecrets(environment types.MappingWithEquals) (map[string]string, error) {
	// TODO: implement secrets with ${secret:blah}
	return map[string]string{}, nil
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
