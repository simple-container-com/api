package k8s

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

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
	Scale            *Scale                  `json:"scale" yaml:"scale"`
	Headers          *Headers                `json:"headers" yaml:"headers"`
	TextVolumes      []SimpleTextVolume      `json:"textVolumes" yaml:"textVolumes"`
	DisruptionBudget *DisruptionBudget       `json:"disruptionBudget" yaml:"disruptionBudget"`
	RollingUpdate    *RollingUpdate          `json:"rollingUpdate" yaml:"rollingUpdate"`
	NodeSelector     map[string]string       `json:"nodeSelector" yaml:"nodeSelector"`
	Affinity         *AffinityRules          `json:"affinity" yaml:"affinity"`
	Tolerations      []Toleration            `json:"tolerations" yaml:"tolerations"`
	VPA              *VPAConfig              `json:"vpa" yaml:"vpa"`                       // Vertical Pod Autoscaler configuration
	ReadinessProbe   *CloudRunProbe          `json:"readinessProbe" yaml:"readinessProbe"` // Global readiness probe configuration
	LivenessProbe    *CloudRunProbe          `json:"livenessProbe" yaml:"livenessProbe"`   // Global liveness probe configuration
}

type CaddyConfig struct {
	Enable           *bool      `json:"enable,omitempty" yaml:"enable,omitempty"`
	Caddyfile        *string    `json:"caddyfile,omitempty" yaml:"caddyfile,omitempty"`             // TODO: support overwriting
	CaddyfilePrefix  *string    `json:"caddyfilePrefix,omitempty" yaml:"caddyfilePrefix,omitempty"` // custom content to inject at the top of Caddyfile (e.g., storage configuration)
	Namespace        *string    `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Image            *string    `json:"image,omitempty" yaml:"image,omitempty"`
	Replicas         *int       `json:"replicas,omitempty" yaml:"replicas,omitempty"`
	Resources        *Resources `json:"resources,omitempty" yaml:"resources,omitempty"`               // CPU and memory limits/requests for Caddy container
	VPA              *VPAConfig `json:"vpa,omitempty" yaml:"vpa,omitempty"`                           // Vertical Pod Autoscaler configuration for Caddy
	UsePrefixes      bool       `json:"usePrefixes,omitempty" yaml:"usePrefixes,omitempty"`           // whether to use prefixes instead of domains (default: false)
	ServiceType      *string    `json:"serviceType,omitempty" yaml:"serviceType,omitempty"`           // whether to use custom service type instead of LoadBalancer (default: LoadBalancer)
	ProvisionIngress bool       `json:"provisionIngress,omitempty" yaml:"provisionIngress,omitempty"` // whether to provision ingress for caddy (default: false)
	UseSSL           *bool      `json:"useSSL,omitempty" yaml:"useSSL,omitempty"`                     // whether to use ssl by default (default: true)
	// Deployment name override for existing Caddy deployments (used when adopting clusters)
	DeploymentName *string `json:"deploymentName,omitempty" yaml:"deploymentName,omitempty"` // override deployment name when adopting existing Caddy
}

type DisruptionBudget struct {
	MaxUnavailable *int `json:"maxUnavailable" yaml:"maxUnavailable"`
	MinAvailable   *int `json:"minAvailable" yaml:"minAvailable"`
}

type RollingUpdate struct {
	MaxSurge       *int `json:"maxSurge" yaml:"maxSurge"`
	MaxUnavailable *int `json:"maxUnavailable" yaml:"maxUnavailable"`
}

// Toleration represents a Kubernetes toleration for pod scheduling
type Toleration struct {
	Key      string `json:"key" yaml:"key"`
	Operator string `json:"operator,omitempty" yaml:"operator,omitempty"` // Equal or Exists
	Value    string `json:"value,omitempty" yaml:"value,omitempty"`
	Effect   string `json:"effect,omitempty" yaml:"effect,omitempty"` // NoSchedule, PreferNoSchedule, NoExecute
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
	Name             string   `json:"name" yaml:"name"`
	MountPath        string   `json:"mountPath" yaml:"mountPath"`
	Storage          string   `json:"storage" yaml:"storage"`
	AccessModes      []string `json:"accessModes" yaml:"accessModes"`
	StorageClassName *string  `json:"storageClassName" yaml:"storageClassName"`
}

type Scale struct {
	Replicas int `json:"replicas" yaml:"replicas"`

	// HPA Configuration
	EnableHPA    bool `json:"enableHPA" yaml:"enableHPA"`
	MinReplicas  int  `json:"minReplicas,omitempty" yaml:"minReplicas,omitempty"`
	MaxReplicas  int  `json:"maxReplicas,omitempty" yaml:"maxReplicas,omitempty"`
	CPUTarget    *int `json:"cpuTarget,omitempty" yaml:"cpuTarget,omitempty"`       // CPU utilization percentage (e.g., 70)
	MemoryTarget *int `json:"memoryTarget,omitempty" yaml:"memoryTarget,omitempty"` // Memory utilization percentage (e.g., 80)
}

type CloudRunProbe struct {
	HttpGet             ProbeHttpGet   `json:"httpGet" yaml:"httpGet"`
	Interval            *time.Duration `json:"interval" yaml:"interval"`
	InitialDelaySeconds *int           `json:"initialDelaySeconds" yaml:"initialDelaySeconds"`
	IntervaSeconds      *int           `json:"intervaSeconds" yaml:"intervaSeconds"`
	FailureThreshold    *int           `json:"failureThreshold" yaml:"failureThreshold"`
	SuccessThreshold    *int           `json:"successThreshold" yaml:"successThreshold"`
	TimeoutSeconds      *int           `json:"timeoutSeconds" yaml:"timeoutSeconds"`
}

// HTTPHeader represents an HTTP header name-value pair for health probe requests.
// This allows customizing HTTP headers sent in readiness, liveness, and startup probes.
//
// Example:
//
//	HTTPHeader{
//		Name:  "Authorization",
//		Value: "Bearer token123",
//	}
//
// Kubernetes Reference: https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
type HTTPHeader struct {
	// Name is the header field name (case-insensitive per HTTP spec)
	Name string `json:"name" yaml:"name"`
	// Value is the header field value
	Value string `json:"value" yaml:"value"`
}

type ProbeHttpGet struct {
	Path        string       `json:"path" yaml:"path"`
	Port        int          `json:"port" yaml:"port"`
	HTTPHeaders []HTTPHeader `json:"httpHeaders,omitempty" yaml:"httpHeaders,omitempty"`
}

type CloudRunContainer struct {
	Name            string             `json:"name" yaml:"name"`
	Command         []string           `json:"command" yaml:"command"`
	Args            []string           `json:"args" yaml:"args"`
	Image           api.ContainerImage `json:"image" yaml:"image"`
	Env             map[string]string  `json:"env" yaml:"env"`
	Secrets         map[string]string  `json:"secrets" yaml:"secrets"`
	Ports           []int              `json:"ports" yaml:"ports"`
	MainPort        *int               `json:"mainPort" yaml:"mainPort"`
	ReadinessProbe  *CloudRunProbe     `json:"readinessProbe" yaml:"readinessProbe"`
	LivenessProbe   *CloudRunProbe     `json:"livenessProbe" yaml:"livenessProbe"`
	StartupProbe    *CloudRunProbe     `json:"startupProbe" yaml:"startupProbe"`
	ComposeDir      string             `json:"composeDir" yaml:"composeDir"`
	Resources       *Resources         `json:"resources" yaml:"resources"`
	Volumes         []PersistentVolume `json:"volumes" yaml:"volumes"`
	ImagePullPolicy *string            `json:"imagePullPolicy" yaml:"imagePullPolicy"`

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
	// Get limits
	cpuLimitInt, err := toCpuLimit(cfg, svc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert CPU limits")
	}
	memLimitInBytesInt, err := toMemoryLimit(cfg, svc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert memory limits")
	}

	// Get requests (with fallback to limits if not specified)
	cpuRequestInt, err := toCpuRequest(cfg, svc, cpuLimitInt)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert CPU requests")
	}
	memRequestInBytesInt, err := toMemoryRequest(cfg, svc, memLimitInBytesInt)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert memory requests")
	}

	return &Resources{
		Limits: map[string]string{
			"memory": bytesSizeToHuman(memLimitInBytesInt), // must be in MB
			"cpu":    fmt.Sprintf("%dm", cpuLimitInt),
		},
		Requests: map[string]string{
			"memory": bytesSizeToHuman(memRequestInBytesInt), // must be in MB
			"cpu":    fmt.Sprintf("%dm", cpuRequestInt),
		},
	}, nil
}

// toCpuLimit extracts CPU limits from configuration
func toCpuLimit(cfg *api.StackConfigCompose, svc types.ServiceConfig) (int64, error) {
	// Priority 1: Explicit limits in size configuration
	if len(cfg.Runs) == 1 && cfg.Size != nil && cfg.Size.Limits != nil && cfg.Size.Limits.Cpu != "" {
		if v, err := strconv.Atoi(cfg.Size.Limits.Cpu); err != nil {
			return 0, errors.Wrapf(err, "failed to parse cpu limit specified for stack: %q", cfg.Size.Limits.Cpu)
		} else {
			return int64(v), nil
		}
	}

	// Priority 2: Legacy size.cpu field (used as limit)
	if len(cfg.Runs) == 1 && cfg.Size != nil && cfg.Size.Cpu != "" {
		if v, err := strconv.Atoi(cfg.Size.Cpu); err != nil {
			return 0, errors.Wrapf(err, "failed to parse cpu value specified for stack: %q", cfg.Size.Cpu)
		} else {
			return int64(v), nil
		}
	}

	// Priority 3: Docker compose deploy resources
	if svc.Deploy != nil && svc.Deploy.Resources.Limits != nil {
		if f, err := strconv.ParseFloat(svc.Deploy.Resources.Limits.NanoCPUs, 32); err != nil {
			return 0, errors.Wrapf(err, "failed to parse cpu limit: %q for service %q", svc.Deploy.Resources.Limits.NanoCPUs, svc.Name)
		} else {
			return int64(1024.0 * f), nil
		}
	}

	// Default CPU limit
	return 256, nil
}

// toCpuRequest extracts CPU requests from configuration, with fallback to limits
func toCpuRequest(cfg *api.StackConfigCompose, svc types.ServiceConfig, cpuLimit int64) (int64, error) {
	// Priority 1: Explicit requests in size configuration
	if len(cfg.Runs) == 1 && cfg.Size != nil && cfg.Size.Requests != nil && cfg.Size.Requests.Cpu != "" {
		if v, err := strconv.Atoi(cfg.Size.Requests.Cpu); err != nil {
			return 0, errors.Wrapf(err, "failed to parse cpu request specified for stack: %q", cfg.Size.Requests.Cpu)
		} else {
			return int64(v), nil
		}
	}

	// Priority 2: Docker compose deploy resources
	if svc.Deploy != nil && svc.Deploy.Resources.Reservations != nil {
		if f, err := strconv.ParseFloat(svc.Deploy.Resources.Reservations.NanoCPUs, 32); err != nil {
			return 0, errors.Wrapf(err, "failed to parse cpu request: %q for service %q", svc.Deploy.Resources.Reservations.NanoCPUs, svc.Name)
		} else {
			return int64(1024.0 * f), nil
		}
	}

	// Fallback: Use 50% of limit as request (Kubernetes best practice)
	return cpuLimit / 2, nil
}

// toMemoryLimit extracts memory limits from configuration
func toMemoryLimit(cfg *api.StackConfigCompose, svc types.ServiceConfig) (int64, error) {
	// Priority 1: Explicit limits in size configuration
	if len(cfg.Runs) == 1 && cfg.Size != nil && cfg.Size.Limits != nil && cfg.Size.Limits.Memory != "" {
		if v, err := strconv.Atoi(cfg.Size.Limits.Memory); err != nil {
			return 0, errors.Wrapf(err, "failed to parse memory limit specified for stack: %q", cfg.Size.Limits.Memory)
		} else {
			return int64(v) * 1024 * 1024, nil // Convert MB to bytes
		}
	}

	// Priority 2: Legacy size.memory field (used as limit)
	if len(cfg.Runs) == 1 && cfg.Size != nil && cfg.Size.Memory != "" {
		if v, err := strconv.Atoi(cfg.Size.Memory); err != nil {
			return 0, errors.Wrapf(err, "failed to parse memory value specified for stack: %q", cfg.Size.Memory)
		} else {
			return int64(v) * 1024 * 1024, nil // Convert MB to bytes (consistent with ECS Fargate)
		}
	}

	// Priority 3: Docker compose deploy resources
	if svc.Deploy != nil && svc.Deploy.Resources.Limits != nil {
		return int64(svc.Deploy.Resources.Limits.MemoryBytes), nil
	}

	// Default memory limit (512MB in bytes)
	return 512 * 1024 * 1024, nil
}

// toMemoryRequest extracts memory requests from configuration, with fallback to limits
func toMemoryRequest(cfg *api.StackConfigCompose, svc types.ServiceConfig, memoryLimit int64) (int64, error) {
	// Priority 1: Explicit requests in size configuration
	if len(cfg.Runs) == 1 && cfg.Size != nil && cfg.Size.Requests != nil && cfg.Size.Requests.Memory != "" {
		if v, err := strconv.Atoi(cfg.Size.Requests.Memory); err != nil {
			return 0, errors.Wrapf(err, "failed to parse memory request specified for stack: %q", cfg.Size.Requests.Memory)
		} else {
			return int64(v) * 1024 * 1024, nil // Convert MB to bytes
		}
	}

	// Priority 2: Docker compose deploy resources
	if svc.Deploy != nil && svc.Deploy.Resources.Reservations != nil {
		return int64(svc.Deploy.Resources.Reservations.MemoryBytes), nil
	}

	// Fallback: Use 50% of limit as request (Kubernetes best practice)
	return memoryLimit / 2, nil
}

func ToHeaders(headers *api.Headers) Headers {
	if headers == nil {
		return nil
	}
	return lo.Assign(*headers)
}

func ToScale(stack *api.StackConfigCompose) *Scale {
	if stack == nil || stack.Scale == nil {
		return nil
	}

	scaleConfig := stack.Scale

	// Detect if autoscaling should be enabled
	// HPA is enabled when: min != max AND policy is defined with CPU or Memory targets
	shouldAutoscale := scaleConfig.Min != scaleConfig.Max &&
		scaleConfig.Policy != nil &&
		(scaleConfig.Policy.Cpu != nil || scaleConfig.Policy.Memory != nil)

	scale := &Scale{
		Replicas:    scaleConfig.Min, // Use min as base replica count
		EnableHPA:   shouldAutoscale,
		MinReplicas: scaleConfig.Min,
		MaxReplicas: scaleConfig.Max,
	}

	// Set CPU target if configured
	if scaleConfig.Policy != nil && scaleConfig.Policy.Cpu != nil {
		scale.CPUTarget = &scaleConfig.Policy.Cpu.Max
	}

	// Set Memory target if configured
	if scaleConfig.Policy != nil && scaleConfig.Policy.Memory != nil {
		scale.MemoryTarget = &scaleConfig.Policy.Memory.Max
	}

	return scale
}

func ToPersistentVolumes(svc types.ServiceConfig, cfg compose.Config) []PersistentVolume {
	var volumes []PersistentVolume
	for _, v := range svc.Volumes {
		pv := PersistentVolume{
			Name:      v.Source,
			MountPath: v.Target,
		}
		if v.Tmpfs != nil {
			pv.Storage = bytesSizeToHuman(int64(v.Tmpfs.Size))
		}
		if volCfg, ok := cfg.Project.Volumes[v.Source]; ok {
			if size, ok := volCfg.Labels[api.ComposeLabelVolumeSize]; ok {
				pv.Storage = size
			}
			if accessModes, ok := volCfg.Labels[api.ComposeLabelVolumeAccessModes]; ok {
				pv.AccessModes = strings.Split(accessModes, ",")
			}
			if storageClass, ok := volCfg.Labels[api.ComposeLabelVolumeStorageClass]; ok {
				pv.StorageClassName = lo.ToPtr(storageClass)
			}
		}
		volumes = append(volumes, pv)
	}
	return volumes
}

func bytesSizeToHuman(size int64) string {
	if size == 0 {
		return "0"
	}

	units := []string{"", "Ki", "Mi", "Gi", "Ti"}
	i := math.Floor(math.Log(float64(size)) / math.Log(1024))

	// Ensure index doesn't exceed available units array bounds
	maxIndex := len(units) - 1
	if int(i) > maxIndex {
		i = float64(maxIndex)
	}

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

		if svc.Name == "" {
			return nil, errors.Errorf("service %s not found in docker-compose config", svcName)
		}

		context := ""
		dockerFile := ""
		buildArgs := make(map[string]string)
		if svc.Build != nil {
			context = svc.Build.Context
			dockerFile = svc.Build.Dockerfile
			// Apply docker-compose defaults: when context is set but dockerfile is empty, default to "Dockerfile"
			if context != "" && dockerFile == "" {
				dockerFile = "Dockerfile"
			}
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
			ComposeDir:      composeCfg.Project.WorkingDir,
			Env:             toRunEnv(svc.Environment),
			Secrets:         toRunSecrets(svc.Environment),
			Ports:           toRunPorts(svc.Ports),
			ReadinessProbe:  toReadinessProbe(svc.HealthCheck),
			StartupProbe:    toStartupProbe(svc.HealthCheck),
			Resources:       resources,
			Volumes:         ToPersistentVolumes(svc, composeCfg),
			ImagePullPolicy: stackCfg.ImagePullPolicy,
		}
		if container.MainPort == nil && len(container.Ports) > 1 {
			container.Warnings = append(container.Warnings, fmt.Sprintf("container %q has multiple ports and no main port specified", container.Name))
		} else if len(container.Ports) > 0 {
			container.MainPort = lo.ToPtr(container.Ports[0])
		}
		containers = append(containers, container)
	}
	return containers, nil
}

func FindIngressContainer(composeCfg compose.Config, contaniers []CloudRunContainer) (*CloudRunContainer, error) {
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
	iContainer, found := lo.Find(contaniers, func(item CloudRunContainer) bool {
		return len(iContainers) > 0 && item.Name == iContainers[0].Name
	})
	if !found && len(contaniers) == 1 && len(contaniers[0].Ports) == 1 {
		iContainer = contaniers[0]
		iContainer.MainPort = lo.ToPtr(iContainer.Ports[0])
		found = true
	}
	if !found {
		return nil, nil
	}
	if len(iContainers) == 1 {
		if portLabel, ok := iContainers[0].Labels[api.ComposeLabelIngressPort]; ok {
			if mainPort, err := strconv.Atoi(portLabel); err != nil {
				iContainer.Warnings = append(iContainer.Warnings, fmt.Sprintf("%q label is specified for container, but failed to convert to int: %v", api.ComposeLabelIngressPort, err.Error()))
			} else {
				iContainer.MainPort = lo.ToPtr(mainPort)
			}
		}
	}
	if iContainer.MainPort == nil && len(iContainer.Ports) == 1 {
		iContainer.MainPort = lo.ToPtr(iContainer.Ports[0])
	}
	return &iContainer, nil
}

func toRunPorts(ports []types.ServicePortConfig) []int {
	return lo.Map(ports, func(p types.ServicePortConfig, _ int) int {
		return int(p.Target)
	})
}

func toStartupProbe(check *types.HealthCheckConfig) *CloudRunProbe {
	if check == nil {
		return nil
	}
	return &CloudRunProbe{
		Interval:            lo.If(check.Interval != nil, lo.ToPtr(time.Duration(lo.FromPtr(check.Interval)))).Else(nil),
		InitialDelaySeconds: lo.If(check.StartInterval != nil, lo.ToPtr(int(time.Duration(lo.FromPtr(check.StartPeriod)).Seconds()))).Else(nil),
		FailureThreshold:    lo.If(check.Retries != nil, lo.ToPtr(int(lo.FromPtr(check.Retries)))).Else(nil),
		TimeoutSeconds:      lo.If(check.Timeout != nil, lo.ToPtr(int(time.Duration(lo.FromPtr(check.Timeout)).Seconds()))).Else(nil),
	}
}

func toReadinessProbe(check *types.HealthCheckConfig) *CloudRunProbe {
	if check == nil {
		return nil
	}
	return &CloudRunProbe{
		Interval:            lo.If(check.Interval != nil, lo.ToPtr(time.Duration(lo.FromPtr(check.Interval)))).Else(nil),
		InitialDelaySeconds: lo.If(check.StartInterval != nil, lo.ToPtr(int(time.Duration(lo.FromPtr(check.StartPeriod)).Seconds()))).Else(nil),
		FailureThreshold:    lo.If(check.Retries != nil, lo.ToPtr(int(lo.FromPtr(check.Retries)))).Else(nil),
		TimeoutSeconds:      lo.If(check.Timeout != nil, lo.ToPtr(int(time.Duration(lo.FromPtr(check.Timeout)).Seconds()))).Else(nil),
	}
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
