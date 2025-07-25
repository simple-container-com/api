package api

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/clouds/compose"
)

const (
	ClientSchemaVersion = "1.0"

	ClientTypeCloudCompose = "cloud-compose"
	ClientTypeStatic       = "static"
	ClientTypeSingleImage  = "single-image"
)

// ClientDescriptor describes the client schema
type ClientDescriptor struct {
	SchemaVersion string                           `json:"schemaVersion" yaml:"schemaVersion"`
	Stacks        map[string]StackClientDescriptor `json:"stacks" yaml:"stacks"`
}

type StackClientDescriptor struct {
	Type        string `json:"type" yaml:"type"`
	ParentStack string `json:"parent" yaml:"parent"`
	ParentEnv   string `json:"parentEnv" yaml:"parentEnv"`
	Template    string `json:"template" yaml:"template"`
	Config      Config `json:",inline" yaml:",inline"`
}

type ImagePlatform string

const (
	ImagePlatformLinuxAmd64 ImagePlatform = "linux/amd64"
)

const (
	ComposeLabelIngressContainer        = "simple-container.com/ingress"
	ComposeLabelVolumeSize              = "simple-container.com/volume-size"
	ComposeLabelVolumeAccessModes       = "simple-container.com/volume-access-modes"
	ComposeLabelVolumeStorageClass      = "simple-container.com/volume-storage-class"
	ComposeLabelIngressPort             = "simple-container.com/ingress/port"
	ComposeLabelHealthcheckSuccessCodes = "simple-container.com/healthcheck/success-codes"
	ComposeLabelHealthcheckPath         = "simple-container.com/healthcheck/path"
	ComposeLabelHealthcheckPort         = "simple-container.com/healthcheck/port"
)

type RemoteImage struct {
	Name string `json:"name" yaml:"name"`
	Tag  string `json:"tag" yaml:"tag"`
}

type ContainerImage struct {
	Name       string `json:"name" yaml:"name"`
	Dockerfile string `json:"dockerfile" yaml:"dockerfile"`
	Context    string `json:"context" yaml:"context"`

	Build    *ContainerImageBuild `json:"build" yaml:"build"`
	Platform ImagePlatform        `json:"platform" yaml:"platform"`
}

type Container struct {
	ContainerImage `json:",inline" yaml:",inline"`
	Name           string `json:"name" yaml:"name"`
}

type ContainerImageBuild struct {
	Args map[string]string `json:"args" yaml:"args"`
}

type StackConfigSingleImage struct {
	Image               *ContainerImage   `json:"image" yaml:"image"`
	Domain              string            `json:"domain" yaml:"domain"`
	BaseDnsZone         string            `json:"baseDnsZone" yaml:"baseDnsZone"` // only necessary if differs from parent stack
	Env                 map[string]string `json:"env" yaml:"env"`
	Secrets             map[string]string `json:"secrets" yaml:"secrets"`
	Min                 int               `yaml:"min" json:"min"`
	Max                 int               `yaml:"max" json:"max"`
	Version             string            `json:"version" yaml:"version"` // only when need to forcefully redeploy (e.g. aws secrets)
	Timeout             *int              `json:"timeout" yaml:"timeout"`
	BasePath            string            `json:"basePath" yaml:"basePath"`                       // base path where API will listen on (e.g. for aws apigateway -> lambda integration)
	MaxMemory           *int              `json:"maxMemory" yaml:"maxMemory"`                     // max memory to use for container
	MaxEphemeralStorage *int              `json:"maxEphemeralStorage" yaml:"maxEphemeralStorage"` // max ephemeral storage in MB
	Uses                []string          `json:"uses" yaml:"uses"`
	StaticEgressIP      *bool             `json:"staticEgressIP" yaml:"staticEgressIP"` // when need to provision NAT with fixed egress IP address (e.g. AWS Lambda with static IP)
	CloudExtras         *any              `json:"cloudExtras" yaml:"cloudExtras"`       // when need to specify additional extra config for the specific cloud (e.g. AWS extra roles)
}

type TextVolume struct {
	Content   string `json:"content" yaml:"content"`
	Name      string `json:"name" yaml:"name"`
	MountPath string `json:"mountPath" yaml:"mountPath"`
}

type Headers map[string]string

type SimpleContainerLBConfig struct {
	Https        bool     `json:"https" yaml:"https"`
	ExtraHelpers []string `json:"extraHelpers" yaml:"extraHelpers"`
}

type StackConfigCompose struct {
	DockerComposeFile string                          `json:"dockerComposeFile" yaml:"dockerComposeFile"`
	Domain            string                          `json:"domain" yaml:"domain"`
	Prefix            string                          `json:"prefix" yaml:"prefix"`                   // prefix for service under LB (e.g. /<service>) default: empty
	ProxyKeepPrefix   bool                            `json:"proxyKeepPrefix" yaml:"proxyKeepPrefix"` // if prefix is specified, whether we need to keep it when proxying to service
	DomainProxied     *bool                           `json:"domainProxied" yaml:"domainProxied"`
	BaseDnsZone       string                          `json:"baseDnsZone" yaml:"baseDnsZone"` // only necessary if differs from parent stack
	Uses              []string                        `json:"uses" yaml:"uses"`
	Runs              []string                        `json:"runs" yaml:"runs"`
	Env               map[string]string               `json:"env" yaml:"env"`
	Secrets           map[string]string               `json:"secrets" yaml:"secrets"`
	Version           string                          `json:"version" yaml:"version"` // only when need to forcefully redeploy (e.g. aws secrets)
	Size              *StackConfigComposeSize         `json:"size,omitempty" yaml:"size,omitempty"`
	Scale             *StackConfigComposeScale        `json:"scale,omitempty" yaml:"scale,omitempty"`
	Dependencies      []StackConfigDependencyResource `json:"dependencies,omitempty" yaml:"dependencies,omitempty"` // when service wants to use resources from another service
	Alerts            *AlertsConfig                   `json:"alerts,omitempty" yaml:"alerts,omitempty"`
	TextVolumes       *[]TextVolume                   `json:"textVolumes" yaml:"textVolumes"`           // extra text volumes to mount to containers (e.g. for k8s deployments)
	Headers           *Headers                        `json:"headers" yaml:"headers"`                   // extra headers to add when serving requests
	LBConfig          *SimpleContainerLBConfig        `json:"lbConfig" yaml:"lbConfig"`                 // load balancer configuration (so far only applicable for k8s deployments)
	CloudExtras       *any                            `json:"cloudExtras" yaml:"cloudExtras"`           // when need to specify additional extra config for the specific cloud (e.g. AWS extra roles)
	StaticEgressIP    *bool                           `json:"staticEgressIP" yaml:"staticEgressIP"`     // when need to provision NAT with fixed egress IP address (e.g. AWS Lambda with static IP)
	ImagePullPolicy   *string                         `json:"imagePullPolicy" yaml:"imagePullPolicy"`   // applicable only for certain compute types, e.g. Kubernetes
	ClusterIPAddress  *string                         `json:"clusterIPAddress" yaml:"clusterIPAddress"` // applicable only for certain compute types, e.g. Kubernetes
}

// StackConfigDependencyResource when stack depends on resource context of another stack (client configuration)
type StackConfigDependencyResource struct {
	Name     string  `json:"name" yaml:"name"`
	Owner    string  `json:"owner" yaml:"owner"`
	Resource string  `json:"resource" yaml:"resource"`
	Env      *string `json:"env" yaml:"env"`
}

// ParentResourceDependency when a resource depends on resource within the same stack
type ParentResourceDependency struct {
	Name string `json:"name" yaml:"name"`
}

type StackConfigComposeSize struct {
	Name      string `yaml:"name" json:"name"`
	Cpu       string `yaml:"cpu" json:"cpu"`
	Memory    string `yaml:"memory" json:"memory"`
	Ephemeral string `yaml:"ephemeral" json:"ephemeral"`
}
type StackConfigComposeScale struct {
	Min int `yaml:"min" json:"min"`
	Max int `yaml:"max" json:"max"`

	Policy *StackConfigComposeScalePolicy `json:"policy" yaml:"policy"`
}

type StackConfigComposeScalePolicy struct {
	Cpu    *StackConfigComposeScaleCpu    `yaml:"cpu" json:"cpu"`
	Memory *StackConfigComposeScaleMemory `yaml:"memory" json:"memory"`
}

type StackConfigComposeScaleMemory struct {
	Max int `yaml:"max" json:"max"`
}

type StackConfigComposeScaleCpu struct {
	Max int `yaml:"max" json:"max"`
}

type StaticSiteConfig struct {
	Domain             string      `json:"domain" yaml:"domain"`
	BaseDnsZone        string      `json:"baseDnsZone" yaml:"baseDnsZone"` // only necessary if differs from parent stack
	IndexDocument      string      `json:"indexDocument" yaml:"indexDocument"`
	ErrorDocument      string      `json:"errorDocument" yaml:"errorDocument"`
	ProvisionWwwDomain bool        `json:"provisionWwwDomain" yaml:"provisionWwwDomain"`
	CorsConfig         *CorsConfig `json:"corsConfig,omitempty" yaml:"corsConfig,omitempty"`
}

type CorsConfig struct {
	AllowedOrigins string   `json:"allowedOrigins" yaml:"allowedOrigins"`
	AllowedMethods []string `json:"allowedMethods,omitempty" yaml:"allowedMethods"`
}

type StackConfigStatic struct {
	BundleDir  string           `json:"bundleDir" yaml:"bundleDir"`
	Site       StaticSiteConfig `json:",inline" yaml:",inline"`
	BucketName string           `json:"bucketName" yaml:"bucketName"` // if necessary to override bucket name, only applicable in some clouds (e.g. gcp)
	Location   string           `json:"location" yaml:"location"`
}

type StackParams struct {
	StacksDir   string   `json:"stacksDir" yaml:"stacksDir"`
	StackDir    string   `json:"stackDir" yaml:"stackDir"`
	Profile     string   `json:"profile" yaml:"profile"`
	StackName   string   `json:"stack" yaml:"stack"`
	Environment string   `json:"environment" yaml:"environment"`
	ParentEnv   string   `json:"parentEnv" yaml:"parentEnv"`
	SkipRefresh bool     `json:"skipRefresh" yaml:"skipRefresh"`
	SkipPreview bool     `json:"skipPreview" yaml:"skipPreview"`
	Version     string   `json:"version" yaml:"version"`
	Timeouts    Timeouts `json:",inline" yaml:",inline"`
	Parent      bool     `json:"parent" yaml:"parent"`
}

type Timeouts struct {
	ExecutionTimeout string `json:"executionTimeout" yaml:"executionTimeout"`
	PreviewTimeout   string `json:"previewTimeout" yaml:"previewTimeout"`
	DeployTimeout    string `json:"deployTimeout" yaml:"deployTimeout"`
}

type DeployParams struct {
	StackParams `json:",inline" yaml:",inline"`
	Vars        VariableValues `json:"vars" yaml:"vars"`
}

type UpdateResult struct {
	StackName  string         `json:"stackName" yaml:"stackName"`
	Summary    string         `json:"summary" yaml:"summary"`
	Operations map[string]int `json:"operations" yaml:"operations"`
}

type PreviewResult struct {
	StackName  string         `json:"stackName" yaml:"stackName"`
	Summary    string         `json:"summary" yaml:"summary"`
	Operations map[string]int `json:"operations" yaml:"operations"`
}

type OutputsResult struct {
	StackName string         `json:"stackName" yaml:"stackName"`
	Outputs   map[string]any `json:"outputs" yaml:"outputs"`
}

func (r *UpdateResult) String() string {
	res, _ := json.Marshal(r)
	return string(res)
}

func (r *PreviewResult) String() string {
	res, _ := json.Marshal(r)
	return string(res)
}

func (r *DestroyResult) String() string {
	res, _ := json.Marshal(r)
	return string(res)
}

func (r *RefreshResult) String() string {
	res, _ := json.Marshal(r)
	return string(res)
}

type DestroyResult struct {
	Operations map[string]int `json:"operations" yaml:"operations"`
}
type RefreshResult struct {
	Operations map[string]int `json:"operations" yaml:"operations"`
}

type DestroyParams struct {
	StackParams         `json:",inline" yaml:",inline"`
	DestroySecretsStack bool `json:"destroySecretsStack" yaml:"destroySecretsStack"`
}

func (p *StackParams) ToProvisionParams() ProvisionParams {
	return ProvisionParams{
		StacksDir:   p.StacksDir,
		Profile:     p.Profile,
		Stacks:      []string{p.StackName},
		SkipRefresh: p.SkipRefresh,
		Timeouts:    p.Timeouts,
	}
}

func (p *StackParams) CopyForParentEnv(env string) *StackParams {
	return &StackParams{
		StacksDir:   p.StackDir,
		StackDir:    p.StackDir,
		Profile:     p.Profile,
		StackName:   p.StackName,
		Environment: p.Environment,
		SkipRefresh: p.SkipRefresh,
		SkipPreview: p.SkipPreview,
		Version:     p.Version,
		Timeouts:    p.Timeouts,
		Parent:      p.Parent,
		ParentEnv:   env,
	}
}

func PrepareCloudSingleImageForDeploy(ctx context.Context, stackDir, stackName string, tpl StackDescriptor, clientConfig *StackConfigSingleImage, parentStack string) (*StackDescriptor, error) {
	stackDesc, err := DetectTemplateType(tpl)
	if err != nil {
		return nil, err
	}
	if tplFun, found := cloudSingleImageConverterMapping[stackDesc.Type]; !found {
		return nil, errors.Errorf("incompatible server template type %q for %q", stackDesc.Type, stackName)
	} else if input, err := tplFun(stackDesc.Config.Config, clientConfig); err != nil {
		return nil, errors.Wrapf(err, "failed to convert single image for type %q in stack %q", stackDesc.Type, stackName)
	} else {
		return &StackDescriptor{
			Type:        stackDesc.Type,
			ParentStack: parentStack,
			Config: Config{
				Config: input,
			},
		}, nil
	}
}

func PrepareCloudComposeForDeploy(ctx context.Context, stackDir, stackName string, tpl StackDescriptor, clientConfig *StackConfigCompose, parentStack string) (*StackDescriptor, error) {
	stackDesc, err := DetectTemplateType(tpl)
	if err != nil {
		return nil, err
	}

	composeCfg, err := compose.ReadDockerCompose(ctx, stackDir, clientConfig.DockerComposeFile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read docker-compose config from %q/%q", stackDir, clientConfig.DockerComposeFile)
	}

	if tplFun, found := cloudComposeConverterMapping[stackDesc.Type]; !found {
		return nil, errors.Errorf("incompatible server template type %q for %q", stackDesc.Type, stackName)
	} else if input, err := tplFun(stackDesc.Config.Config, composeCfg, clientConfig); err != nil {
		return nil, errors.Wrapf(err, "failed to convert cloud compose for type %q in stack %q", stackDesc.Type, stackName)
	} else {
		return &StackDescriptor{
			Type:        stackDesc.Type,
			ParentStack: parentStack,
			Config: Config{
				Config: input,
			},
		}, nil
	}
}

func PrepareStaticForDeploy(ctx context.Context, stackDir, stackName string, tpl StackDescriptor, clientConfig *StackConfigStatic, parentStack string) (*StackDescriptor, error) {
	stackDesc, err := DetectTemplateType(tpl)
	if err != nil {
		return nil, err
	}

	if tplFun, found := cloudStaticSiteConverterMapping[stackDesc.Type]; !found {
		return nil, errors.Errorf("incompatible server template type %q for %q", stackDesc.Type, stackName)
	} else if input, err := tplFun(stackDesc.Config.Config, stackDir, stackName, clientConfig); err != nil {
		return nil, errors.Wrapf(err, "failed to convert cloud static site for type %q in stack %q", stackDesc.Type, stackName)
	} else {
		return &StackDescriptor{
			Type:        stackDesc.Type,
			ParentStack: parentStack,
			Config: Config{
				Config: input,
			},
		}, nil
	}
}

func PrepareClientConfigForDeploy(ctx context.Context, stackDir, stackName string, tpl StackDescriptor, clientDesc StackClientDescriptor) (*StackDescriptor, error) {
	if fnc, found := clientConfigsPrepareMap[clientDesc.Type]; !found {
		return nil, errors.Errorf("unsupported client type %q", tpl.Type)
	} else if sDesc, err := fnc(ctx, stackDir, stackName, tpl, clientDesc); err != nil {
		return nil, errors.Wrapf(err, "failed to prepare config for deploy")
	} else {
		return sDesc, nil
	}
}

func ConvertClientConfig(clientDesc StackClientDescriptor) (*StackClientDescriptor, error) {
	if fnc, found := clientConfigsConvertMap[clientDesc.Type]; !found {
		return nil, errors.Errorf("unsupported client config type %q", clientDesc.Type)
	} else if converted, err := fnc(&clientDesc.Config); err != nil {
		return nil, errors.Wrapf(err, "failed to convert client config")
	} else {
		clientDesc.Config = converted
		return &clientDesc, nil
	}
}
