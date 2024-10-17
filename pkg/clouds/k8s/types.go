package k8s

import (
	"fmt"
	"math"
	"strconv"

	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

type DeploymentConfig struct {
	StackConfig      *api.StackConfigCompose `json:"stackConfig" yaml:"stackConfig"`
	Containers       []CloudRunContainer     `json:"containers" yaml:"containers"`
	IngressContainer *CloudRunContainer      `json:"ingressContainer" yaml:"ingressContainer"`
	Scale            *Scale                  `json:"replicas" yaml:"replicas"`
	Headers          *Headers                `json:"headers" yaml:"headers"`
	TextVolumes      []SimpleTextVolume      `json:"textVolumes" yaml:"textVolumes"`
}

type DisruptionBudget struct {
	MaxUnavailable *int `json:"maxUnavailable" yaml:"maxUnavailable"`
	MinAvailable   *int `json:"minAvailable" yaml:"minAvailable"`
}

type Headers = map[string]string

type Resources struct {
	Limits   map[string]string `json:"limits" yaml:"limits"`
	Requests map[string]string `json:"requests" yaml:"requests"`
}

type SimpleTextVolume struct {
	api.TextVolume `json:",inline" yaml:",inline"`
}

type PersistentVolume struct {
	Name      string `json:"name" yaml:"name"`
	MountPath string `json:"mountPath" yaml:"mountPath"`
	Storage   string `json:"storage" yaml:"storage"`
}

type Scale struct {
	Replicas int `json:"replicas" yaml:"replicas"`
	// TODO: support autoscaling
}

type CloudRunProbe struct {
	HttpGet             ProbeHttpGet `json:"httpGet" yaml:"httpGet"`
	InitialDelaySeconds int          `json:"initialDelaySeconds" yaml:"initialDelaySeconds"`
}

type ProbeHttpGet struct {
	Path string `json:"path" yaml:"path"`
	Port int    `json:"port" yaml:"port"`
}

type CloudRunContainer struct {
	Name           string             `json:"name" yaml:"name"`
	Command        []string           `json:"command" yaml:"command"`
	Args           []string           `json:"args" yaml:"args"`
	Image          api.ContainerImage `json:"image" yaml:"image"`
	Env            map[string]string  `json:"env" yaml:"env"`
	Secrets        map[string]string  `json:"secrets" yaml:"secrets"`
	Ports          []int              `json:"ports" yaml:"ports"`
	MainPort       *int               `json:"mainPort" yaml:"mainPort"`
	ReadinessProbe *CloudRunProbe     `json:"readinessProbe" yaml:"readinessProbe"`
	StartupProbe   *CloudRunProbe     `json:"startupProbe" yaml:"startupProbe"`
	ComposeDir     string             `json:"composeDir" yaml:"composeDir"`
	Resources      *Resources         `json:"resources" yaml:"resources"`
	Volumes        []PersistentVolume `json:"volumes" yaml:"volumes"`

	Warnings []string `json:"warnings" yaml:"warnings"` // non-critical errors happened during conversion (should be reported later)
}

type CloudRunScale struct {
	Min int `json:"min" yaml:"min"`
	Max int `json:"max" yaml:"max"`
}

func ToSimpleTextVolumes(cfg *api.StackConfigCompose) []SimpleTextVolume {
	return lo.Map(lo.FromPtr(cfg.TextVolumes), func(v api.TextVolume, _ int) SimpleTextVolume {
		return SimpleTextVolume{
			TextVolume: v,
		}
	})
}

func ToResources(cfg *api.StackConfigCompose, svc types.ServiceConfig) (*Resources, error) {
	cpuInt, err := toCpu(cfg, svc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert CPU limits")
	}
	memInt, err := toMemory(cfg, svc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert memory limits")
	}
	return &Resources{
		// TODO: separate limits from requests
		Limits: map[string]string{
			"memory": bytesSizeToHuman(memInt * 1024 * 1024), // must be in MB
			"cpu":    fmt.Sprintf("%dm", cpuInt),
		},
		Requests: map[string]string{
			"memory": bytesSizeToHuman(memInt * 1024 * 1024), // must be in MB
			"cpu":    fmt.Sprintf("%dm", cpuInt),
		},
	}, nil
}

func toCpu(cfg *api.StackConfigCompose, svc types.ServiceConfig) (int64, error) {
	if len(cfg.Runs) == 1 && cfg.Size != nil {
		if v, err := strconv.Atoi(cfg.Size.Cpu); err != nil {
			return 0, errors.Wrapf(err, "failed to parse cpu value specified for stack: %q", cfg.Size.Cpu)
		} else {
			return int64(v), nil
		}
	}

	if svc.Deploy != nil && svc.Deploy.Resources.Limits != nil {
		if f, err := strconv.ParseFloat(svc.Deploy.Resources.Limits.NanoCPUs, 32); err != nil {
			return 0, errors.Wrapf(err, "failed to parse cpu limit: %q for service %q", svc.Deploy.Resources.Limits.NanoCPUs, svc.Name)
		} else {
			return int64(1024.0 * f), nil
		}
	}
	// TODO: change default if necessary
	return 256, nil
}

func toMemory(cfg *api.StackConfigCompose, svc types.ServiceConfig) (int64, error) {
	if len(cfg.Runs) == 1 && cfg.Size != nil {
		if v, err := strconv.Atoi(cfg.Size.Memory); err != nil {
			return 0, errors.Wrapf(err, "failed to parse memory value specified for stack: %q", cfg.Size.Memory)
		} else {
			return int64(v), nil
		}
	}

	if svc.Deploy != nil && svc.Deploy.Resources.Limits != nil {
		return int64(svc.Deploy.Resources.Limits.MemoryBytes), nil
	}
	// TODO: change default if necessary
	return 512, nil
}

func ToHeaders(headers *api.Headers) Headers {
	if headers == nil {
		return nil
	}
	return lo.Assign(*headers)
}

func ToScale(stack *api.StackConfigCompose) *Scale {
	if lo.FromPtr(stack).Scale != nil {
		return &Scale{
			Replicas: stack.Scale.Min,
		}
	}
	return nil
}

func ToPersistentVolumes(svc types.ServiceConfig) []PersistentVolume {
	var volumes []PersistentVolume
	for _, v := range svc.Volumes {
		pv := PersistentVolume{
			Name:      v.Source,
			MountPath: v.Target,
		}
		if v.Tmpfs != nil {
			pv.Storage = bytesSizeToHuman(int64(v.Tmpfs.Size))
		}
		volumes = append(volumes, pv)
	}
	return volumes
}

func bytesSizeToHuman(size int64) string {
	if size == 0 {
		return "0"
	}

	units := []string{"", "K", "M", "G", "T"}
	i := math.Floor(math.Log(float64(size)) / math.Log(1024))
	humanSize := float64(size) / math.Pow(1024, i)

	return fmt.Sprintf("%d%s", int64(humanSize), units[int(i)])
}

func ConvertComposeToContainers(composeCfg compose.Config, stackCfg *api.StackConfigCompose) ([]CloudRunContainer, error) {
	if composeCfg.Project == nil {
		return nil, errors.Errorf("compose config is nil")
	}

	services := lo.Associate(composeCfg.Project.Services, func(svc types.ServiceConfig) (string, types.ServiceConfig) {
		return svc.Name, svc
	})

	var containers []CloudRunContainer

	for _, svcName := range stackCfg.Runs {
		svc := services[svcName]

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

		resources, err := ToResources(stackCfg, svc)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert stack compose resources for service %s", svcName)
		}

		container := CloudRunContainer{
			Name:    svcName,
			Command: svc.Entrypoint,
			Args:    svc.Command,
			Image: api.ContainerImage{
				Context:    context,
				Platform:   api.ImagePlatformLinuxAmd64,
				Dockerfile: dockerFile,
				Build: &api.ContainerImageBuild{
					Args: buildArgs,
				},
			},
			ComposeDir:     composeCfg.Project.WorkingDir,
			Env:            toRunEnv(svc.Environment),
			Secrets:        toRunSecrets(svc.Environment),
			Ports:          toRunPorts(svc.Ports),
			ReadinessProbe: toLivenessProbe(svc.HealthCheck),
			StartupProbe:   toStartupProbe(svc.HealthCheck),
			Resources:      resources,
			Volumes:        ToPersistentVolumes(svc),
		}
		if container.MainPort == nil && len(container.Ports) > 1 {
			container.Warnings = append(container.Warnings, fmt.Sprintf("container %q has multiple ports and no main port specified", container.Name))
		}
		containers = append(containers, container)
	}
	return containers, nil
}

func toRunPorts(ports []types.ServicePortConfig) []int {
	return lo.Map(ports, func(p types.ServicePortConfig, _ int) int {
		return int(p.Target)
	})
}

func toStartupProbe(check *types.HealthCheckConfig) *CloudRunProbe {
	// TODO
	return nil
}

func toLivenessProbe(check *types.HealthCheckConfig) *CloudRunProbe {
	// TODO
	return nil
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
