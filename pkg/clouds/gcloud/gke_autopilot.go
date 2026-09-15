// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcloud

import (
	"net"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

const (
	ResourceTypeGkeAutopilot = "gcp-gke-autopilot-cluster"
	TemplateTypeGkeAutopilot = "gcp-gke-autopilot"
)

// Cloud NAT port-allocation defaults and bounds. Defaults preserve prior
// behaviour; the floor/ceiling match GCP's accepted range for ports per VM.
const (
	DefaultMinPortsPerVm = 64
	DefaultMaxPortsPerVm = 65536
	PortsPerVmFloor      = 32
	PortsPerVmCeiling    = 65536
)

type GkeAutopilotResource struct {
	Credentials   `json:",inline" yaml:",inline"`
	GkeMinVersion string           `json:"gkeMinVersion" yaml:"gkeMinVersion"`
	Location      string           `json:"location" yaml:"location"`
	Zone          string           `json:"zone" yaml:"zone"`
	Timeouts      *Timeouts        `json:"timeouts,omitempty" yaml:"timeouts,omitempty"`
	Caddy         *k8s.CaddyConfig `json:"caddy,omitempty" yaml:"caddy,omitempty"`

	// External Egress IP Configuration
	ExternalEgressIp *ExternalEgressIpConfig `json:"externalEgressIp,omitempty" yaml:"externalEgressIp,omitempty"`

	// Private VPC - creates dedicated VPC for the cluster (avoids CloudNAT conflicts)
	PrivateVpc bool `json:"privateVpc,omitempty" yaml:"privateVpc,omitempty"`

	// ControlPlaneAccess restricts who may reach the Kubernetes API server.
	// Left unset the control plane keeps GKE's default, an IP endpoint that
	// accepts connections from any address on the internet.
	ControlPlaneAccess *ControlPlaneAccessConfig `json:"controlPlaneAccess,omitempty" yaml:"controlPlaneAccess,omitempty"`

	// Resource adoption fields
	Adopt       bool   `json:"adopt,omitempty" yaml:"adopt,omitempty"`
	ClusterName string `json:"clusterName,omitempty" yaml:"clusterName,omitempty"`
}

type Timeouts struct {
	Create string `json:"create" yaml:"create"`
	Update string `json:"update" yaml:"update"`
	Delete string `json:"delete" yaml:"delete"`
}

// ExternalEgressIpConfig provides simple configuration for static egress IP
type ExternalEgressIpConfig struct {
	Enabled  bool   `json:"enabled" yaml:"enabled"`
	Existing string `json:"existing,omitempty" yaml:"existing,omitempty"`

	// Cloud NAT port/mapping tuning. All optional; when unset the previous
	// defaults are preserved (64 min ports, endpoint-independent mapping on,
	// dynamic port allocation off), so existing clusters are unaffected until
	// they opt in. Raising the port budget and enabling dynamic port
	// allocation avoids source-port exhaustion for pods that open many
	// concurrent outbound connections (dropped SYNs surface downstream as
	// dial i/o timeouts).
	MinPortsPerVm *int `json:"minPortsPerVm,omitempty" yaml:"minPortsPerVm,omitempty"`
	MaxPortsPerVm *int `json:"maxPortsPerVm,omitempty" yaml:"maxPortsPerVm,omitempty"`
	// DynamicPortAllocation lets a VM scale its NAT ports between min and max
	// on demand. GCP requires endpoint-independent mapping to be off when it is
	// enabled, and both port bounds to be powers of two.
	DynamicPortAllocation *bool `json:"dynamicPortAllocation,omitempty" yaml:"dynamicPortAllocation,omitempty"`
	// EndpointIndependentMapping toggles NAT EIM (default true). Must be false
	// to use dynamic port allocation.
	EndpointIndependentMapping *bool `json:"endpointIndependentMapping,omitempty" yaml:"endpointIndependentMapping,omitempty"`
}

type GkeAutopilotTemplate struct {
	Credentials              `json:",inline" yaml:",inline"`
	GkeClusterResource       string `json:"gkeClusterResource" yaml:"gkeClusterResource"`
	ArtifactRegistryResource string `json:"artifactRegistryResource" yaml:"artifactRegistryResource"`
}

type GkeAutopilotInput struct {
	GkeAutopilotTemplate `json:"templateConfig" yaml:"templateConfig"`
	Deployment           k8s.DeploymentConfig `json:"deployment" yaml:"deployment"`
}

func (i *GkeAutopilotInput) Uses() []string {
	return i.Deployment.StackConfig.Uses
}

func (i *GkeAutopilotInput) OverriddenBaseZone() string {
	return i.Deployment.StackConfig.BaseDnsZone
}

func (i *GkeAutopilotInput) DependsOnResources() []api.StackConfigDependencyResource {
	return i.Deployment.StackConfig.Dependencies
}

func ReadGkeAutopilotTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &GkeAutopilotTemplate{})
}

func ReadGkeAutopilotResourceConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &GkeAutopilotResource{})
}

func ToGkeAutopilotConfig(tpl any, composeCfg compose.Config, stackCfg *api.StackConfigCompose) (any, error) {
	templateCfg, ok := tpl.(*GkeAutopilotTemplate)
	if !ok {
		return nil, errors.Errorf("template config is not of type *gcloud.GkeAutopilotTemplate")
	}
	if templateCfg == nil {
		return nil, errors.Errorf("template config is nil")
	}
	deployCfg := k8s.DeploymentConfig{
		StackConfig: stackCfg,
		Scale:       k8s.ToScale(stackCfg),
	}

	// Process CloudExtras for affinity rules, node selector, etc.
	if stackCfg.CloudExtras != nil {
		k8sCloudExtras := &k8s.CloudExtras{}
		var err error
		k8sCloudExtras, err = api.ConvertDescriptor(stackCfg.CloudExtras, k8sCloudExtras)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert cloudExtras field to Kubernetes Cloud extras format")
		}

		deployCfg.RollingUpdate = k8sCloudExtras.RollingUpdate
		deployCfg.DisruptionBudget = k8sCloudExtras.DisruptionBudget
		deployCfg.NodeSelector = k8sCloudExtras.NodeSelector
		deployCfg.Tolerations = k8sCloudExtras.Tolerations
		deployCfg.VPA = k8sCloudExtras.VPA                                     // Extract VPA configuration from CloudExtras
		deployCfg.ReadinessProbe = k8sCloudExtras.ReadinessProbe               // Extract global readiness probe configuration
		deployCfg.LivenessProbe = k8sCloudExtras.LivenessProbe                 // Extract global liveness probe configuration
		deployCfg.StartupProbe = k8sCloudExtras.StartupProbe                   // Extract global startup probe configuration
		deployCfg.PriorityClassName = k8sCloudExtras.PriorityClassName         // Extract PriorityClass for pod scheduling and preemption
		deployCfg.ServiceType = k8sCloudExtras.ServiceType                     // Extract Service type override (e.g. LoadBalancer for UDP)
		deployCfg.ExternalTrafficPolicy = k8sCloudExtras.ExternalTrafficPolicy // e.g. Local, required for WebRTC return media
		deployCfg.TopologySpreadConstraints = k8sCloudExtras.TopologySpreadConstraints

		// Process affinity rules and merge with existing NodeSelector if needed
		if k8sCloudExtras.Affinity != nil {
			// Store the full affinity configuration for advanced usage
			deployCfg.Affinity = k8sCloudExtras.Affinity

			// Merge Space Pay style affinity rules with existing NodeSelector
			if deployCfg.NodeSelector == nil {
				deployCfg.NodeSelector = make(map[string]string)
			}

			// GKE Autopilot supports custom nodeSelector labels for workload separation!
			// When you specify a custom nodeSelector + toleration, GKE automatically creates
			// separate nodes with those labels and taints.

			// Handle nodePool as a custom workload separation label
			if k8sCloudExtras.Affinity.NodePool != nil {
				nodePoolValue := *k8sCloudExtras.Affinity.NodePool
				// Use custom label for workload separation (not system labels)
				deployCfg.NodeSelector["workload-group"] = nodePoolValue

				// Automatically add corresponding toleration for GKE Autopilot workload separation
				workloadToleration := k8s.Toleration{
					Key:      "workload-group",
					Operator: "Equal",
					Value:    nodePoolValue,
					Effect:   "NoSchedule",
				}
				deployCfg.Tolerations = append(deployCfg.Tolerations, workloadToleration)
			}

			// Handle computeClass - require exact GKE Autopilot values
			// Valid values: Accelerator, Balanced, Performance, Scale-Out, autopilot, autopilot-spot
			if k8sCloudExtras.Affinity.ComputeClass != nil {
				deployCfg.NodeSelector["cloud.google.com/compute-class"] = *k8sCloudExtras.Affinity.ComputeClass
			}

			// For exclusive node pool, anti-affinity rules are handled in simple_container.go
		}
	}

	res := &GkeAutopilotInput{
		GkeAutopilotTemplate: *templateCfg,
		Deployment:           deployCfg,
	}

	containers, err := k8s.ConvertComposeToContainers(composeCfg, stackCfg)
	if err != nil {
		return nil, err
	}
	iContainer, err := k8s.FindIngressContainer(composeCfg, containers)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to detect ingress container")
	}
	res.Deployment.Containers = containers
	res.Deployment.IngressContainer = iContainer
	res.Deployment.Headers = lo.ToPtr(k8s.ToHeaders(stackCfg.Headers))
	res.Deployment.TextVolumes = k8s.ToSimpleTextVolumes(stackCfg)

	return res, nil
}

// Validate validates the ExternalEgressIpConfig
func (c *ExternalEgressIpConfig) Validate() error {
	if !c.Enabled {
		return nil // No validation needed if disabled
	}

	// Validate existing IP format if specified
	if c.Existing != "" {
		if !strings.HasPrefix(c.Existing, "projects/") {
			return errors.New("'existing' must be a full GCP resource path like 'projects/{project}/regions/{region}/addresses/{name}'")
		}
		parts := strings.Split(c.Existing, "/")
		if len(parts) != 6 || parts[2] != "regions" || parts[4] != "addresses" {
			return errors.New("invalid 'existing' format, expected 'projects/{project}/regions/{region}/addresses/{name}'")
		}
	}

	// Validate the effective port bounds (applying the same defaults as
	// resolveNatPortSettings) so a lone maxPortsPerVm below the default minimum
	// is caught here rather than failing late at the GCP API.
	minPorts, maxPorts := DefaultMinPortsPerVm, DefaultMaxPortsPerVm
	if c.MinPortsPerVm != nil {
		minPorts = *c.MinPortsPerVm
		if minPorts < PortsPerVmFloor || minPorts > PortsPerVmCeiling {
			return errors.Errorf("minPortsPerVm must be between %d and %d, got %d", PortsPerVmFloor, PortsPerVmCeiling, minPorts)
		}
	}
	if c.MaxPortsPerVm != nil {
		maxPorts = *c.MaxPortsPerVm
		if maxPorts < PortsPerVmFloor || maxPorts > PortsPerVmCeiling {
			return errors.Errorf("maxPortsPerVm must be between %d and %d, got %d", PortsPerVmFloor, PortsPerVmCeiling, maxPorts)
		}
	}
	if maxPorts < minPorts {
		return errors.Errorf("effective maxPortsPerVm (%d) must be >= minPortsPerVm (%d)", maxPorts, minPorts)
	}

	if c.DynamicPortAllocation != nil && *c.DynamicPortAllocation {
		// GCP rejects dynamic port allocation together with endpoint-independent
		// mapping, needs both bounds to be powers of two, and needs a real range
		// (max strictly greater than min).
		if c.EndpointIndependentMapping == nil || *c.EndpointIndependentMapping {
			return errors.New("dynamicPortAllocation requires endpointIndependentMapping: false")
		}
		if !isPowerOfTwo(minPorts) {
			return errors.Errorf("minPortsPerVm must be a power of two when dynamicPortAllocation is enabled, got %d", minPorts)
		}
		if !isPowerOfTwo(maxPorts) {
			return errors.Errorf("maxPortsPerVm must be a power of two when dynamicPortAllocation is enabled, got %d", maxPorts)
		}
		if maxPorts <= minPorts {
			return errors.Errorf("effective maxPortsPerVm (%d) must be > minPortsPerVm (%d) when dynamicPortAllocation is enabled", maxPorts, minPorts)
		}
	}

	return nil
}

func isPowerOfTwo(n int) bool {
	return n > 0 && n&(n-1) == 0
}

// ControlPlaneAccessConfig configures reachability of the cluster control
// plane. The two endpoints are independent: the IP endpoint is authorised by
// source address, the DNS endpoint by IAM. Enabling the DNS endpoint is what
// makes locking the IP endpoint practical for callers with no stable address,
// such as hosted CI runners.
type ControlPlaneAccessConfig struct {
	// DnsEndpoint opens the DNS-based control plane endpoint to callers outside
	// the cluster VPC. The endpoint itself always exists; this is the flag that
	// makes it usable, and it stays authorised by IAM either way.
	DnsEndpoint *bool `json:"dnsEndpoint,omitempty" yaml:"dnsEndpoint,omitempty"`

	// IpEndpoint toggles the IP-based endpoint. Defaults to enabled.
	IpEndpoint *bool `json:"ipEndpoint,omitempty" yaml:"ipEndpoint,omitempty"`

	// AuthorizedNetworks lists the CIDRs allowed to reach the IP endpoint.
	// Setting this field is what turns the allow list on: with the list on and
	// no entries, only Google internal traffic reaches the IP endpoint.
	AuthorizedNetworks []AuthorizedNetwork `json:"authorizedNetworks,omitempty" yaml:"authorizedNetworks,omitempty"`

	// AllowGcpPublicCidrs keeps every Google Cloud public address authorised.
	// GKE defaults this to true, which admits any Google Cloud tenant, so it
	// defaults to false here whenever an allow list is configured.
	AllowGcpPublicCidrs *bool `json:"allowGcpPublicCidrs,omitempty" yaml:"allowGcpPublicCidrs,omitempty"`
}

// AuthorizedNetwork is one CIDR allowed to reach the control plane IP endpoint.
type AuthorizedNetwork struct {
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	Cidr string `json:"cidr" yaml:"cidr"`
}

// DnsEndpointEnabled reports whether the DNS endpoint accepts external callers.
func (c *ControlPlaneAccessConfig) DnsEndpointEnabled() bool {
	return c != nil && c.DnsEndpoint != nil && *c.DnsEndpoint
}

// IpEndpointEnabled reports whether the IP endpoint is switched on. It is on
// unless explicitly disabled, matching GKE.
func (c *ControlPlaneAccessConfig) IpEndpointEnabled() bool {
	if c == nil || c.IpEndpoint == nil {
		return true
	}
	return *c.IpEndpoint
}

// AuthorizedNetworksEnabled reports whether the IP endpoint allow list applies.
//
// An explicitly empty list is a request to authorise nobody, so it counts as
// enabled. Only an absent list means "no opinion": read the other way round,
// `authorizedNetworks: []` would drop the whole block and leave the control
// plane on GKE's default, which is reachable from anywhere. That is a config
// that reads as a lock-down and silently is not one, so nil is the only value
// that turns the allow list off.
func (c *ControlPlaneAccessConfig) AuthorizedNetworksEnabled() bool {
	return c != nil && (c.AuthorizedNetworks != nil || c.AllowGcpPublicCidrs != nil)
}

// GcpPublicCidrsAllowed reports the effective value of the Google Cloud public
// range exemption.
func (c *ControlPlaneAccessConfig) GcpPublicCidrsAllowed() bool {
	return c != nil && c.AllowGcpPublicCidrs != nil && *c.AllowGcpPublicCidrs
}

func (c *ControlPlaneAccessConfig) Validate() error {
	if c == nil {
		return nil
	}

	// GKE caps the list. Past the cap the cluster is rejected by the API after
	// the update has already started.
	const maxAuthorizedNetworks = 50
	if len(c.AuthorizedNetworks) > maxAuthorizedNetworks {
		return errors.Errorf("controlPlaneAccess: %d authorized networks exceeds GKE's limit of %d",
			len(c.AuthorizedNetworks), maxAuthorizedNetworks)
	}

	seen := make(map[string]int, len(c.AuthorizedNetworks))
	for i, n := range c.AuthorizedNetworks {
		n.Cidr = strings.TrimSpace(n.Cidr)
		if n.Cidr == "" {
			return errors.Errorf("controlPlaneAccess.authorizedNetworks[%d]: cidr is required", i)
		}
		ip, ipNet, err := net.ParseCIDR(n.Cidr)
		if err != nil {
			if net.ParseIP(n.Cidr) != nil {
				return errors.Errorf("controlPlaneAccess.authorizedNetworks[%d]: %q is a single address, it needs an explicit /32 or /128", i, n.Cidr)
			}
			return errors.Errorf("controlPlaneAccess.authorizedNetworks[%d]: %q is not a CIDR", i, n.Cidr)
		}
		if ones, _ := ipNet.Mask.Size(); ones == 0 {
			return errors.Errorf("controlPlaneAccess.authorizedNetworks[%d]: %q allows every address, which is what the allow list exists to prevent", i, n.Cidr)
		}
		if !ip.Equal(ipNet.IP) {
			return errors.Errorf("controlPlaneAccess.authorizedNetworks[%d]: %q has host bits set, use %s", i, n.Cidr, ipNet.String())
		}
		if first, dup := seen[ipNet.String()]; dup {
			return errors.Errorf("controlPlaneAccess.authorizedNetworks[%d]: %q duplicates entry %d", i, n.Cidr, first)
		}
		seen[ipNet.String()] = i
	}

	// Both endpoints off leaves no way to reach the API server, and GKE accepts
	// it, so it is caught here rather than after the cluster is unreachable.
	if !c.IpEndpointEnabled() && !c.DnsEndpointEnabled() {
		return errors.New("controlPlaneAccess: ipEndpoint is disabled and dnsEndpoint is not enabled, which leaves no way to reach the control plane")
	}

	// An empty allow list on the IP endpoint is the same lockout unless the DNS
	// endpoint is there to take over. allowGcpPublicCidrs does not count as a
	// way in: it authorises Google Cloud's public address space, which is every
	// other tenant and nobody in this organisation.
	if c.IpEndpointEnabled() && c.AuthorizedNetworksEnabled() &&
		len(c.AuthorizedNetworks) == 0 && !c.DnsEndpointEnabled() {
		return errors.New("controlPlaneAccess: the authorized network list is empty and dnsEndpoint is not enabled, which leaves no way to reach the control plane")
	}

	// An allow list on a switched-off endpoint governs nothing, and reads as
	// though it does.
	if !c.IpEndpointEnabled() && len(c.AuthorizedNetworks) > 0 {
		return errors.New("controlPlaneAccess: authorizedNetworks has no effect while ipEndpoint is disabled")
	}

	return nil
}
