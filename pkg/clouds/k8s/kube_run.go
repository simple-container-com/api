// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

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
	NodeSelector      map[string]string        `json:"nodeSelector" yaml:"nodeSelector"`
	DisruptionBudget  *DisruptionBudget        `json:"disruptionBudget" yaml:"disruptionBudget"`
	RollingUpdate     *RollingUpdate           `json:"rollingUpdate" yaml:"rollingUpdate"`
	Affinity          *AffinityRules           `json:"affinity" yaml:"affinity"`
	Tolerations       []Toleration             `json:"tolerations" yaml:"tolerations"`
	VPA               *VPAConfig               `json:"vpa" yaml:"vpa"`
	ReadinessProbe    *CloudRunProbe           `json:"readinessProbe" yaml:"readinessProbe"`
	LivenessProbe     *CloudRunProbe           `json:"livenessProbe" yaml:"livenessProbe"`
	StartupProbe      *CloudRunProbe           `json:"startupProbe" yaml:"startupProbe"`
	EphemeralVolumes  []GenericEphemeralVolume `json:"ephemeralVolumes" yaml:"ephemeralVolumes"`   // Generic ephemeral volumes for large temp storage
	PriorityClassName *string                  `json:"priorityClassName" yaml:"priorityClassName"` // Kubernetes PriorityClass for pod scheduling and preemption

	TopologySpreadConstraints []TopologySpreadConstraint `json:"topologySpreadConstraints" yaml:"topologySpreadConstraints"`
}

// TopologySpreadConstraint spreads pods across nodes without the GKE Autopilot 0.5 vCPU minimum that pod anti-affinity requires.
type TopologySpreadConstraint struct {
	MaxSkew           *int           `json:"maxSkew" yaml:"maxSkew"`
	TopologyKey       string         `json:"topologyKey" yaml:"topologyKey"`
	WhenUnsatisfiable string         `json:"whenUnsatisfiable" yaml:"whenUnsatisfiable"`
	LabelSelector     *LabelSelector `json:"labelSelector" yaml:"labelSelector"`
	MinDomains        *int           `json:"minDomains" yaml:"minDomains"`
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
	// ControlledResources specifies which resources VPA should control.
	// Per the VPA CRD this is a per-container field; SC places it inside each
	// containerPolicy entry, not at resourcePolicy level.
	ControlledResources []string `json:"controlledResources" yaml:"controlledResources"`
	// ControlledValues specifies which resource values VPA should control.
	// One of "RequestsAndLimits" (default) or "RequestsOnly". Use "RequestsOnly"
	// when the underlying deployment template's limits are sized for cold-start
	// bursts (e.g. Django/gunicorn) and you don't want VPA to scale the limit
	// proportionally with a lowered request — the proportional shrink causes
	// CPU-throttle-induced startup probe failures.
	ControlledValues *string `json:"controlledValues" yaml:"controlledValues"`
	// ContainerPolicies specifies per-container VPA overrides. The top-level
	// MinAllowed/MaxAllowed/ControlledResources/ControlledValues render the
	// catch-all "*" policy; each entry here adds a policy for a specific
	// container that takes precedence over "*" (the VPA admission controller
	// matches an exact containerName before the wildcard). The common use is
	// excluding an injected sidecar (e.g. cloudsql-proxy) with mode "Off" so it
	// keeps its small template request instead of being floored at the app
	// container's minAllowed.
	ContainerPolicies []VPAContainerPolicy `json:"containerPolicies,omitempty" yaml:"containerPolicies,omitempty"`
}

// VPAResourceRequirements defines resource requirements for VPA
type VPAResourceRequirements struct {
	CPU              *string `json:"cpu" yaml:"cpu"`
	Memory           *string `json:"memory" yaml:"memory"`
	EphemeralStorage *string `json:"ephemeral-storage" yaml:"ephemeral-storage"`
}

// VPAContainerPolicy is a per-container VPA resource policy. ContainerName is
// the exact container name (e.g. "cloudsql-proxy"); the remaining fields mirror
// the VPA CRD's containerPolicies entry. Mode "Off" disables VPA for that
// container so its requests are left at the deployment template values.
type VPAContainerPolicy struct {
	ContainerName       string                   `json:"containerName" yaml:"containerName"`
	Mode                *string                  `json:"mode,omitempty" yaml:"mode,omitempty"`
	MinAllowed          *VPAResourceRequirements `json:"minAllowed,omitempty" yaml:"minAllowed,omitempty"`
	MaxAllowed          *VPAResourceRequirements `json:"maxAllowed,omitempty" yaml:"maxAllowed,omitempty"`
	ControlledResources []string                 `json:"controlledResources,omitempty" yaml:"controlledResources,omitempty"`
	ControlledValues    *string                  `json:"controlledValues,omitempty" yaml:"controlledValues,omitempty"`
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
		deployCfg.VPA = k8sCloudExtras.VPA                             // Extract VPA configuration from CloudExtras
		deployCfg.ReadinessProbe = k8sCloudExtras.ReadinessProbe       // Extract global readiness probe configuration
		deployCfg.LivenessProbe = k8sCloudExtras.LivenessProbe         // Extract global liveness probe configuration
		deployCfg.EphemeralVolumes = k8sCloudExtras.EphemeralVolumes   // Extract generic ephemeral volumes configuration
		deployCfg.PriorityClassName = k8sCloudExtras.PriorityClassName // Extract PriorityClass for pod scheduling and preemption
		deployCfg.TopologySpreadConstraints = k8sCloudExtras.TopologySpreadConstraints

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
