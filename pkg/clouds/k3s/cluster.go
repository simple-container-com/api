// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k3s

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
)

// ResourceTypeK3sCluster is the `type:` a parent stack's resource declares.
const ResourceTypeK3sCluster = "k3s-cluster"

// DefaultVersion is the k3s release SC installs when a resource does not pin one.
//
// A literal version and NOT a channel ("stable", "latest"), deliberately, and the
// same reasoning refuses a channel in Validate: a channel is resolved by the install
// script at install time, so the same SC config provisioned twice a month apart
// stands up two different control planes, and the "is the installed version the
// wanted one?" guard that keeps a re-provision from restarting the API server cannot
// be evaluated at all. Bump this by hand, from
// `curl -s https://update.k3s.io/v1-release/channels` (the `stable` entry), and only
// together with a smoke — a control-plane upgrade is not a dependency bump.
const DefaultVersion = "v1.36.5+k3s1"

// DefaultInstallScriptURL is Rancher's installer. It resolves the binary from GitHub
// releases, and both hosts were measured reachable from inside Russia on 2026-10-17
// at 6–8 MB/s (roadmap item sc-ru-market-readiness, slice B0) — which is why this is
// the default rather than a bundled binary. InstallScriptURL exists for the networks
// where that stops being true.
const DefaultInstallScriptURL = "https://get.k3s.io"

// APIPort is the port the k3s API server listens on. Not configurable: it is baked
// into the kubeconfig k3s itself writes, into `--tls-san` handling, and into every
// firewall note in the design doc. A flag here would have to be threaded through all
// three to mean anything.
const APIPort = 6443

// The NodePort range a Kubernetes apiserver allocates `type: NodePort` services
// from, and the range k3s uses because SC never passes
// `--service-node-port-range`. Both ends inclusive.
//
// They live here rather than in the firewall code that needs them because they are
// a property of the control plane, not of a cloud: `pkg/clouds/vm` imports this
// package for the `k3s` entry of a vm resource's `ingressPorts` preset table. That
// import direction is the reason this package must keep importing nothing from
// `pkg/clouds/vm` — the name of a vm resource (K3sClusterConfig.VmResource) is all
// it has ever needed, and widening it to the type would close a cycle.
const (
	NodePortRangeStart = 30000
	NodePortRangeEnd   = 32767
)

// The ports k3s nodes use to talk to EACH OTHER. Node-to-node only — see
// vm.PortRange.Internal for why the distinction is per-port and not per-machine.
//
// Taken from k3s's own "Inbound Rules for K3s Nodes" and narrowed to the backends
// SC actually runs: flannel's default VXLAN backend (so no 51820/51821, which are
// the wireguard-native ones), and embedded etcd, which is what `--cluster-init`
// gives an HA control plane.
const (
	// FlannelVxlanPort carries every packet between pods on different nodes. UDP.
	FlannelVxlanPort = 8472
	// KubeletPort serves metrics and `kubectl logs`/`exec`/`port-forward` — which
	// is also why it must never be public: it reaches into any pod on the node.
	KubeletPort = 10250
	// EtcdClientPort and EtcdPeerPort are the embedded etcd of a multi-server
	// cluster. Contiguous, and used as a range.
	EtcdClientPort = 2379
	EtcdPeerPort   = 2380
)

// The k3s packaged components SC is willing to turn off. k3s accepts `--disable` for
// each of these; anything else is refused rather than passed through, because a
// misspelled component name leaves the component RUNNING and k3s says nothing — the
// operator believes traefik is gone while it holds port 80.
var DisableableComponents = []string{
	"traefik",
	"servicelb",
	"local-storage",
	"metrics-server",
}

// The server flags SC sets itself. An operator-supplied duplicate in
// `extraServerArgs` is refused instead of appended: k3s takes the last occurrence of
// a repeated flag, so a duplicate silently overrides SC's value and the result
// (a kubeconfig pointing nowhere, a missing SAN, a cluster joined to itself) is
// diagnosed nowhere near its cause.
var managedServerArgs = []string{
	"--tls-san",
	"--disable",
	"--write-kubeconfig",
	"--write-kubeconfig-mode",
	"--token",
	"--cluster-init",
	"--server",
	"--https-listen-port",
}

// versionPattern is k3s's own release spelling: an upstream Kubernetes version plus
// the k3s build suffix, e.g. `v1.36.5+k3s1`.
var versionPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+\+k3s\d+$`)

// endpointPattern is a bare host or IP — what goes into a kubeconfig `server:` URL
// and into `--tls-san`, both of which take a host without a scheme or port.
var endpointPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9.\-]*[a-zA-Z0-9])?$`)

// K3sClusterConfig is the resource config of `type: k3s-cluster`.
//
// It describes the CLUSTER only. The machines it runs on are described by a `vm`
// resource in the same parent stack and referenced by name through VmResource — the
// same split as `gcp-gke-autopilot` referencing its own resources, and the reason
// the k3s path inherits cloud-init, SSH readiness, key handling and every future
// cloud driver from pkg/clouds/vm without a line of its own.
type K3sClusterConfig struct {
	// VmResource is the name of the `type: vm` resource, in the same parent stack and
	// the same environment, whose hosts become this cluster's nodes.
	VmResource string `json:"vmResource" yaml:"vmResource"`

	// Version pins the k3s release. Defaults to DefaultVersion; a channel name is
	// refused (see DefaultVersion).
	Version string `json:"version,omitempty" yaml:"version,omitempty"`

	// Endpoint is the host or IP that the EXPORTED kubeconfig points at, and the
	// first `--tls-san`.
	//
	// It defaults to the node's own address, which is correct for the single-node
	// stand K.0 builds. Set it when the API server is reached through a name or a
	// load balancer rather than directly — an endpoint that is not a SAN on the
	// serving certificate fails every request with an x509 hostname error, which is
	// why this field and the SAN list are the same field by default.
	Endpoint string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`

	// TlsSans are additional names to put on the API server certificate, beyond
	// Endpoint and the node address. Spend them freely: adding a SAN later means
	// re-issuing the certificate and restarting the API server, while an unused SAN
	// costs nothing (k3s-all-clouds.md §4).
	TlsSans []string `json:"tlsSans,omitempty" yaml:"tlsSans,omitempty"`

	// Disable overrides which packaged components are turned off. Leave it unset:
	// the default is derived from the host (see pkg/clouds/pulumi/k3s/cluster.go),
	// because whether klipper-lb can have ports 80 and 443 depends on whether the
	// machine's own compose ingress already holds them.
	Disable []string `json:"disable,omitempty" yaml:"disable,omitempty"`

	// ExtraServerArgs is passed verbatim to the k3s server. The flags SC manages
	// itself are refused (managedServerArgs).
	ExtraServerArgs []string `json:"extraServerArgs,omitempty" yaml:"extraServerArgs,omitempty"`

	// InstallScriptURL overrides the installer location for a network that cannot
	// reach Rancher's (see DefaultInstallScriptURL).
	InstallScriptURL string `json:"installScriptUrl,omitempty" yaml:"installScriptUrl,omitempty"`

	api.Credentials `json:",inline" yaml:",inline"`
}

func (r *K3sClusterConfig) ProviderType() string {
	return ProviderType
}

// ProjectIdValue has no meaning for a cluster SC installs over SSH: there is no
// cloud project behind it. The k8s-style "n/a" keeps `${resource:...projectId}` from
// interpolating to something that looks real (same choice as vm.VmConfig).
func (r *K3sClusterConfig) ProjectIdValue() string {
	return "n/a"
}

// CredentialsValue returns the resolved `${auth:...}` blob verbatim, "{}" when none
// was given. A k3s cluster authenticates to nothing: it rides the SSH key of the vm
// resource it names, and that key is never copied into this config.
func (r *K3sClusterConfig) CredentialsValue() string {
	return lo.If(r.Credentials.Credentials == "", "{}").Else(r.Credentials.Credentials)
}

// DependsOnResources tells provision.go that this resource must be provisioned
// AFTER the vm resource it names, and makes the vm resource's output available as
// params.ResourceOutputs[VmResource] (pkg/clouds/pulumi/provision.go:145-172).
//
// This is the whole mechanism by which a cluster learns its nodes' addresses and
// readiness gates without a StackReference back into the stack currently being
// provisioned — which is not merely slow, it is a cycle.
func (r *K3sClusterConfig) DependsOnResources() []api.ParentResourceDependency {
	if r.VmResource == "" {
		return nil
	}
	return []api.ParentResourceDependency{{Name: r.VmResource}}
}

func (r *K3sClusterConfig) EffectiveVersion() string {
	return lo.If(r.Version == "", DefaultVersion).Else(r.Version)
}

func (r *K3sClusterConfig) EffectiveInstallScriptURL() string {
	return lo.If(r.InstallScriptURL == "", DefaultInstallScriptURL).Else(r.InstallScriptURL)
}

// Validate enforces the same rule as the rest of the VM path: either support a field
// or refuse it. It is called from the provisioning tier rather than from
// ReadK3sClusterConfig, because the config readers are also driven with an empty
// config by cmd/schema-gen, and a reader that rejects an empty config drops the whole
// resource out of the generated schema.
func (r *K3sClusterConfig) Validate() error {
	if r.VmResource == "" {
		return errors.Errorf(
			"`%s` resource requires `vmResource`: the name of the `type: vm` resource in this parent stack "+
				"whose hosts become the cluster's nodes", ResourceTypeK3sCluster)
	}

	if r.Version != "" && !versionPattern.MatchString(r.Version) {
		hint := ""
		if lo.Contains([]string{"stable", "latest", "testing"}, r.Version) || strings.HasPrefix(r.Version, "v1.") && !strings.Contains(r.Version, "+k3s") {
			hint = "; a release channel resolves to a different release over time, so the same config would " +
				"install a different control plane on a later provision — pin the version a channel currently " +
				"points at (`curl -s https://update.k3s.io/v1-release/channels`)"
		}
		return errors.Errorf("`version` %q is not a k3s release: expected the form %q%s",
			r.Version, DefaultVersion, hint)
	}

	if r.Endpoint != "" {
		if !endpointPattern.MatchString(r.Endpoint) {
			return errors.Errorf(
				"`endpoint` %q must be a bare host name or IP address — no scheme and no port. SC builds the "+
					"kubeconfig URL (https://<endpoint>:%d) and the `--tls-san` entry from it, and both take a "+
					"host alone", r.Endpoint, APIPort)
		}
	}

	for i, san := range r.TlsSans {
		if !endpointPattern.MatchString(san) {
			return errors.Errorf("`tlsSans[%d]` %q must be a bare host name or IP address", i, san)
		}
	}

	for i, c := range r.Disable {
		if c == "coredns" {
			return errors.Errorf(
				"`disable` must not contain %q: without cluster DNS no workload can resolve a Service, and "+
					"every deploy onto this cluster fails in a way that looks unrelated to this setting", c)
		}
		if !lo.Contains(DisableableComponents, c) {
			return errors.Errorf("`disable[%d]` %q is not a k3s packaged component; one of: %s",
				i, c, strings.Join(sortedComponents(), ", "))
		}
	}

	for i, arg := range r.ExtraServerArgs {
		flag := strings.SplitN(strings.TrimSpace(arg), "=", 2)[0]
		if lo.Contains(managedServerArgs, flag) {
			return errors.Errorf(
				"`extraServerArgs[%d]` sets %s, which SC sets itself: k3s takes the LAST occurrence of a "+
					"repeated flag, so this would silently override SC's value. Use the dedicated field "+
					"instead (%s)", i, flag, fieldFor(flag))
		}
	}

	return nil
}

// fieldFor names the supported way to influence a flag SC manages, so that the
// refusal above is actionable rather than just a refusal.
func fieldFor(flag string) string {
	switch flag {
	case "--tls-san":
		return "`endpoint` / `tlsSans`"
	case "--disable":
		return "`disable`"
	default:
		return "not configurable: it is part of how SC reaches and exports this cluster"
	}
}

func sortedComponents() []string {
	out := append([]string{}, DisableableComponents...)
	sort.Strings(out)
	return out
}

// KubeconfigURL is the `server:` URL the exported kubeconfig must carry.
func KubeconfigURL(endpoint string) string {
	return fmt.Sprintf("https://%s:%d", endpoint, APIPort)
}

func ReadK3sClusterConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &K3sClusterConfig{})
}
