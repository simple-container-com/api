package k8s

import (
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

type KubeRunInput struct {
	CloudrunTemplate `json:"templateConfig" yaml:"templateConfig"`
	Deployment       DeploymentConfig `json:"deployment" yaml:"deployment"`
}

func (i *KubeRunInput) OverriddenBaseZone() string {
	return i.Deployment.StackConfig.BaseDnsZone
}

type CloudExtras struct {
	NodeSelector     map[string]string `json:"nodeSelector" yaml:"nodeSelector"`
	DisruptionBudget *DisruptionBudget `json:"disruptionBudget" yaml:"disruptionBudget"`
	RollingUpdate    *RollingUpdate    `json:"rollingUpdate" yaml:"rollingUpdate"`
	Affinity         *AffinityRules    `json:"affinity" yaml:"affinity"`
	Tolerations      []Toleration      `json:"tolerations" yaml:"tolerations"`
	VPA              *VPAConfig        `json:"vpa" yaml:"vpa"`
	ReadinessProbe   *CloudRunProbe    `json:"readinessProbe" yaml:"readinessProbe"`
	LivenessProbe    *CloudRunProbe    `json:"livenessProbe" yaml:"livenessProbe"`
}

// AffinityRules defines pod affinity and anti-affinity rules for node pool isolation
type AffinityRules struct {
	// NodePool specifies the target node pool for pod scheduling
	NodePool *string `json:"nodePool" yaml:"nodePool"`
	// ExclusiveNodePool ensures pods only run on the specified node pool
	ExclusiveNodePool *bool `json:"exclusiveNodePool" yaml:"exclusiveNodePool"`
	// ComputeClass specifies the compute class (Performance, Scale-Out, general-purpose)
	ComputeClass *string `json:"computeClass" yaml:"computeClass"`
	// NodeAffinity provides direct Kubernetes node affinity configuration
	NodeAffinity *NodeAffinity `json:"nodeAffinity" yaml:"nodeAffinity"`
	// PodAffinity provides pod affinity rules
	PodAffinity *PodAffinity `json:"podAffinity" yaml:"podAffinity"`
	// PodAntiAffinity provides pod anti-affinity rules
	PodAntiAffinity *PodAffinity `json:"podAntiAffinity" yaml:"podAntiAffinity"`
}

// NodeAffinity defines node affinity rules
type NodeAffinity struct {
	RequiredDuringSchedulingIgnoredDuringExecution  *NodeSelector             `json:"requiredDuringSchedulingIgnoredDuringExecution" yaml:"requiredDuringSchedulingIgnoredDuringExecution"`
	PreferredDuringSchedulingIgnoredDuringExecution []PreferredSchedulingTerm `json:"preferredDuringSchedulingIgnoredDuringExecution" yaml:"preferredDuringSchedulingIgnoredDuringExecution"`
}

// NodeSelector defines node selector requirements
type NodeSelector struct {
	NodeSelectorTerms []NodeSelectorTerm `json:"nodeSelectorTerms" yaml:"nodeSelectorTerms"`
}

// NodeSelectorTerm defines a node selector term
type NodeSelectorTerm struct {
	MatchExpressions []NodeSelectorRequirement `json:"matchExpressions" yaml:"matchExpressions"`
	MatchFields      []NodeSelectorRequirement `json:"matchFields" yaml:"matchFields"`
}

// NodeSelectorRequirement defines a node selector requirement
type NodeSelectorRequirement struct {
	Key      string   `json:"key" yaml:"key"`
	Operator string   `json:"operator" yaml:"operator"`
	Values   []string `json:"values" yaml:"values"`
}

// PreferredSchedulingTerm defines a preferred scheduling term
type PreferredSchedulingTerm struct {
	Weight     int32            `json:"weight" yaml:"weight"`
	Preference NodeSelectorTerm `json:"preference" yaml:"preference"`
}

// PodAffinity defines pod affinity rules
type PodAffinity struct {
	RequiredDuringSchedulingIgnoredDuringExecution  []PodAffinityTerm         `json:"requiredDuringSchedulingIgnoredDuringExecution" yaml:"requiredDuringSchedulingIgnoredDuringExecution"`
	PreferredDuringSchedulingIgnoredDuringExecution []WeightedPodAffinityTerm `json:"preferredDuringSchedulingIgnoredDuringExecution" yaml:"preferredDuringSchedulingIgnoredDuringExecution"`
}

// PodAffinityTerm defines a pod affinity term
type PodAffinityTerm struct {
	LabelSelector *LabelSelector `json:"labelSelector" yaml:"labelSelector"`
	Namespaces    []string       `json:"namespaces" yaml:"namespaces"`
	TopologyKey   string         `json:"topologyKey" yaml:"topologyKey"`
}

// WeightedPodAffinityTerm defines a weighted pod affinity term
type WeightedPodAffinityTerm struct {
	Weight          int32           `json:"weight" yaml:"weight"`
	PodAffinityTerm PodAffinityTerm `json:"podAffinityTerm" yaml:"podAffinityTerm"`
}

// LabelSelector defines a label selector
type LabelSelector struct {
	MatchLabels      map[string]string          `json:"matchLabels" yaml:"matchLabels"`
	MatchExpressions []LabelSelectorRequirement `json:"matchExpressions" yaml:"matchExpressions"`
}

// LabelSelectorRequirement defines a label selector requirement
type LabelSelectorRequirement struct {
	Key      string   `json:"key" yaml:"key"`
	Operator string   `json:"operator" yaml:"operator"`
	Values   []string `json:"values" yaml:"values"`
}

// VPAConfig defines Vertical Pod Autoscaler configuration
type VPAConfig struct {
	// Enabled controls whether VPA should be created for the deployment
	Enabled bool `json:"enabled" yaml:"enabled"`
	// UpdateMode specifies how VPA should update pods (Off, Initial, Recreation, Auto)
	UpdateMode *string `json:"updateMode" yaml:"updateMode"`
	// MinAllowed specifies minimum allowed resources
	MinAllowed *VPAResourceRequirements `json:"minAllowed" yaml:"minAllowed"`
	// MaxAllowed specifies maximum allowed resources
	MaxAllowed *VPAResourceRequirements `json:"maxAllowed" yaml:"maxAllowed"`
	// ControlledResources specifies which resources VPA should control
	ControlledResources []string `json:"controlledResources" yaml:"controlledResources"`
}

// VPAResourceRequirements defines resource requirements for VPA
type VPAResourceRequirements struct {
	CPU    *string `json:"cpu" yaml:"cpu"`
	Memory *string `json:"memory" yaml:"memory"`
}

func (i *KubeRunInput) DependsOnResources() []api.StackConfigDependencyResource {
	return i.Deployment.StackConfig.Dependencies
}

func (i *KubeRunInput) Uses() []string {
	return i.Deployment.StackConfig.Uses
}

func ToKubernetesRunConfig(tpl any, composeCfg compose.Config, stackCfg *api.StackConfigCompose) (any, error) {
	templateCfg, ok := tpl.(*CloudrunTemplate)
	if !ok {
		return nil, errors.Errorf("template config is not of type *gcloud.TemplateConfig")
	}
	if templateCfg == nil {
		return nil, errors.Errorf("template config is nil")
	}

	deployCfg := DeploymentConfig{
		StackConfig: stackCfg,
		Scale:       ToScale(stackCfg),
	}

	if stackCfg.CloudExtras != nil {
		k8sCloudExtras := &CloudExtras{}
		var err error
		k8sCloudExtras, err = api.ConvertDescriptor(stackCfg.CloudExtras, k8sCloudExtras)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert cloudExtras field to Kubernetes Cloud extras format")
		}

		deployCfg.RollingUpdate = k8sCloudExtras.RollingUpdate
		deployCfg.DisruptionBudget = k8sCloudExtras.DisruptionBudget
		deployCfg.NodeSelector = k8sCloudExtras.NodeSelector
		deployCfg.VPA = k8sCloudExtras.VPA                       // Extract VPA configuration from CloudExtras
		deployCfg.ReadinessProbe = k8sCloudExtras.ReadinessProbe // Extract global readiness probe configuration
		deployCfg.LivenessProbe = k8sCloudExtras.LivenessProbe   // Extract global liveness probe configuration

		// Process affinity rules and merge with existing NodeSelector if needed
		if k8sCloudExtras.Affinity != nil {
			// Store the full affinity configuration for advanced usage
			deployCfg.Affinity = k8sCloudExtras.Affinity

			// Merge Space Pay style affinity rules with existing NodeSelector
			if deployCfg.NodeSelector == nil {
				deployCfg.NodeSelector = make(map[string]string)
			}

			// Apply nodePool and computeClass to NodeSelector for GKE compatibility
			if k8sCloudExtras.Affinity.NodePool != nil {
				deployCfg.NodeSelector["cloud.google.com/gke-nodepool"] = *k8sCloudExtras.Affinity.NodePool
			}
			if k8sCloudExtras.Affinity.ComputeClass != nil {
				deployCfg.NodeSelector["node.kubernetes.io/instance-type"] = *k8sCloudExtras.Affinity.ComputeClass
			}

			// For exclusive node pool, anti-affinity rules are handled in simple_container.go
		}
	}
	res := &KubeRunInput{
		CloudrunTemplate: *templateCfg,
		Deployment:       deployCfg,
	}
	containers, err := ConvertComposeToContainers(composeCfg, stackCfg)
	if err != nil {
		return nil, err
	}
	res.Deployment.Containers = containers

	iContainer, err := FindIngressContainer(composeCfg, containers)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to detect ingress container")
	}
	res.Deployment.IngressContainer = iContainer

	return res, nil
}
