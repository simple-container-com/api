package aws

import (
	"strconv"
	"time"

	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

const (
	TemplateTypeEcsFargate = "ecs-fargate"
)

const ComposeLabelIngressContainer = "simple-container.com/ingress"

type EcsFargateConfig struct {
	api.Credentials `json:",inline" yaml:",inline"`
	AccountConfig   `json:",inline" yaml:",inline"`
	Cpu             int    `json:"cpu" yaml:"cpu"`
	Memory          int    `json:"memory" yaml:"memory"`
	Version         string `json:"version" yaml:"version"`
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
	Command             []string     `json:"command" yaml:"command"`
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

	Policy *EcsFargateScalePolicy `json:"policy" yaml:"policy"`
	Update FargateRollingUpdate   `json:"update" yaml:"update"`
}

type FargateRollingUpdate struct {
	MinHealthyPercent int `json:"minHealthyPercent" yaml:"minHealthyPercent"`
	MaxPercent        int `json:"maxPercent" yaml:"maxPercent"`
}

type EcsFargateScalePolicyType string

var ScaleCpu EcsFargateScalePolicyType = "cpu"

type EcsFargateScalePolicy struct {
	Type             EcsFargateScalePolicyType `json:"type" yaml:"type"`
	TargetValue      int                       `yaml:"targetValue" json:"targetValue"`
	ScaleInCooldown  int                       `json:"scaleInCooldown" json:"scaleInCooldown"`
	ScaleOutCooldown int                       `json:"scaleOutCooldown" json:"scaleOutCooldown"`
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
	TemplateConfig   `json:"templateConfig" yaml:"templateConfig"`
	Scale            EcsFargateScale       `json:"scale" yaml:"scale"`
	Containers       []EcsFargateContainer `json:"containers" yaml:"containers"`
	IngressContainer EcsFargateContainer   `json:"ingressContainer" yaml:"ingressContainer"`
	Config           EcsFargateConfig      `json:"config" yaml:"config"`
	Domain           string                `json:"domain" yaml:"domain"`
	RefResourceNames []string              `json:"refResourceNames" yaml:"refResourceNames"`
	Secrets          map[string]string     `json:"secrets" yaml:"secrets"`
	BaseDnsZone      string                `yaml:"baseDnsZone" json:"baseDnsZone"`
}

func (i *EcsFargateInput) Uses() []string {
	return i.RefResourceNames
}

func (i *EcsFargateInput) OverriddenBaseZone() string {
	return i.BaseDnsZone
}

func ToEcsFargateConfig(tpl any, composeCfg compose.Config, stackCfg *api.StackConfigCompose) (any, error) {
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
	res := &EcsFargateInput{
		BaseDnsZone:      stackCfg.BaseDnsZone,
		TemplateConfig:   *templateCfg,
		RefResourceNames: stackCfg.Uses,
		Config: EcsFargateConfig{
			Credentials:   templateCfg.Credentials,
			AccountConfig: *accountConfig,
			Version:       stackCfg.Version,
		},
		Secrets: stackCfg.Secrets,
	}
	if stackCfg.Size != nil {
		if res.Config.Cpu, err = strconv.Atoi(stackCfg.Size.Cpu); err != nil {
			return nil, errors.Wrapf(err, "failed to convert cpu size %q to ECS fargate cpu size: must be a number (e.g. 256)", stackCfg.Size.Cpu)
		}
		if res.Config.Memory, err = strconv.Atoi(stackCfg.Size.Memory); err != nil {
			return nil, errors.Wrapf(err, "failed to convert memory size %q to ECS fargate memory size: must be a number (e.g. 512)", stackCfg.Size.Memory)
		}
	}
	if stackCfg.Scale != nil {
		res.Scale = EcsFargateScale{
			Min: lo.If(stackCfg.Scale.Min == 0, 1).Else(stackCfg.Scale.Min),
			Max: lo.If(stackCfg.Scale.Max == 0, 1).Else(stackCfg.Scale.Max),
		}
		if stackCfg.Scale.Policy != nil && stackCfg.Scale.Policy.Cpu != nil {
			res.Scale.Policy = &EcsFargateScalePolicy{
				Type:             ScaleCpu,
				TargetValue:      lo.If(stackCfg.Scale.Policy.Cpu.Max != 0, stackCfg.Scale.Policy.Cpu.Max).Else(70), // Target CPU utilization of 70%
				ScaleInCooldown:  60,                                                                                // Wait 60s between scale-in activities
				ScaleOutCooldown: 60,                                                                                // Wait 60s between scale-out activities
			}
		}
	} else {
		res.Scale = EcsFargateScale{
			Min: 1,
			Max: 2,
		}
	}
	res.Scale.Update = FargateRollingUpdate{
		MinHealthyPercent: 100,
		MaxPercent:        200,
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
			Env:           lo.Assign(toRunEnv(svc.Environment), stackCfg.Env),
			Secrets:       secrets,
			Port:          port,
			LivenessProbe: liveProbe,
			StartupProbe:  startProbe,
			// TODO: cpu, memory
		})
	}

	iContainers := lo.Filter(composeCfg.Project.Services, func(s types.ServiceConfig, _ int) bool {
		v, hasLabel := s.Labels[ComposeLabelIngressContainer]
		return hasLabel && v == "true"
	})
	if len(iContainers) > 1 || len(iContainers) == 0 {
		return nil, errors.Errorf("must have exactly 1 ingress container, but found (%v) in compose files %q,"+
			"did you forget to add label %q to the main container?",
			lo.Map(iContainers, func(item types.ServiceConfig, _ int) string {
				return item.Name
			}), composeCfg.Project.ComposeFiles, ComposeLabelIngressContainer)
	}
	res.IngressContainer, _ = lo.Find(res.Containers, func(item EcsFargateContainer) bool {
		return item.Name == iContainers[0].Name
	})
	res.Domain = stackCfg.Domain

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
	res := EcsFargateProbe{}
	res.FromHealthCheck(svc.HealthCheck, port)
	return res, nil
}

func toLivenessProbe(svc types.ServiceConfig, port int) (EcsFargateProbe, error) {
	res := EcsFargateProbe{}
	res.FromHealthCheck(svc.HealthCheck, port)
	return res, nil
}

func (p *EcsFargateProbe) FromHealthCheck(check *types.HealthCheckConfig, port int) {
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
		if len(check.Test) > 0 {
			p.Command = check.Test
		}
		if len(check.Test) == 0 {
			p.HttpGet = ProbeHttpGet{
				Path: "/",
				Port: port,
			}
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
