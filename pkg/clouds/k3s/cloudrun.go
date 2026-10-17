// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k3s

import (
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
	"github.com/simple-container-com/api/pkg/clouds/docker"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// TemplateTypeK3sCloudrun deploys a cloud-compose stack onto a k3s-cluster resource of
// the parent stack.
//
// It exists for one reason, and it is not a difference in what gets deployed: the
// existing `kubernetes-cloudrun` template takes its kubeconfig as a LITERAL string in
// the template config (k8s.KubernetesConfig.Kubeconfig, surfaced through
// CredentialsValue() so provision.go's initProvider can build the provider from it).
// A k3s cluster's kubeconfig does not exist until the cluster has been provisioned, so
// it can only arrive as a parent-stack output — the same way gke-autopilot's does.
// Hence a template that names a cluster resource instead of carrying credentials.
//
// Everything below the kubeconfig is reused unchanged: the compose→containers mapping
// is k8s.ToKubernetesRunConfig, and the deploy is kubernetes.KubeRunWithProvider.
const TemplateTypeK3sCloudrun = "k3s-cloudrun"

// K3sCloudrunTemplate is the parent-stack template for deploying onto a k3s cluster.
//
// Deliberately NOT embedding k8s.CloudrunTemplate: that would inherit a `kubeconfig`
// field which must always be empty here, and a config field that must never be set is
// a field someone will eventually set. The provisioning side builds a k8s.KubeRunInput
// from these values instead.
type K3sCloudrunTemplate struct {
	api.Credentials `json:",inline" yaml:",inline"`

	// K3sClusterResource is the name of the k3s-cluster resource in the same parent
	// stack whose kubeconfig this template deploys through. Same role as
	// gke-autopilot's `gkeClusterResource`.
	K3sClusterResource string `json:"k3sClusterResource" yaml:"k3sClusterResource"`

	// The registry images are pushed to and pulled from. Required, and there is no
	// fallback: unlike the vm-compose path, which can ship an image over SSH with
	// `imageDelivery: ssh-tar`, a kubelet can only pull. A k3s stand with no reachable
	// registry is a stand that cannot deploy — which on the RU-market track is a
	// deliberate thing to surface at config time rather than at image-pull time.
	docker.RegistryCredentials `json:",inline" yaml:",inline"`

	CaddyResource *string `json:"caddyResource,omitempty" yaml:"caddyResource,omitempty"` // name of the caddy resource in the parent stack
	UseSSL        *bool   `json:"useSSL,omitempty" yaml:"useSSL,omitempty"`               // whether to assume connections are https-only (default: true)
}

// ProviderType reports k3s for the same reason K3sClusterConfig does, and with a
// sharper consequence here: provision.go's initProvider builds a provider from this
// value on the DEPLOY path too (deploy.go:193 → getProvisionParams → initProvider,
// against the converted client-stack config). Reporting "kubernetes" would send it to
// kubernetes.Provider, which calls CredentialsValue() and builds a real
// pulumi-kubernetes provider out of an empty kubeconfig — and pulumi-kubernetes with
// no kubeconfig falls back to the AMBIENT one. On a laptop with a live ~/.kube/config
// that is a provider pointed at whatever cluster the operator last used.
//
// So: k3s, whose provider is pulumi-command's and is simply unused on this path.
// K3sCloudrun overrides params.Provider with the real Kubernetes provider it builds
// from the parent's kubeconfig export.
func (t *K3sCloudrunTemplate) ProviderType() string {
	return ProviderType
}

func (t *K3sCloudrunTemplate) ProjectIdValue() string {
	return "n/a"
}

// CredentialsValue returns an empty JSON object rather than "": api.ConvertAuth
// json.Unmarshals this, and "" is not valid JSON. There are no credentials to carry —
// reaching the cluster needs the parent's kubeconfig output, and pushing images needs
// the registry fields above.
func (t *K3sCloudrunTemplate) CredentialsValue() string {
	if t.Credentials.Credentials == "" {
		return "{}"
	}
	return t.Credentials.Credentials
}

func (t *K3sCloudrunTemplate) Validate() error {
	if t.K3sClusterResource == "" {
		return errors.Errorf("`k3sClusterResource` must be specified: it names the %s resource in the parent stack whose kubeconfig to deploy through", ResourceTypeK3sCluster)
	}
	if url := t.DockerRegistryURL; url == nil || *url == "" {
		return errors.Errorf("`dockerRegistryURL` must be specified: a kubelet can only pull images, so a k3s deploy has no equivalent of vm-compose's ssh-tar delivery")
	}
	return nil
}

func ReadK3sCloudrunTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &K3sCloudrunTemplate{})
}

// K3sCloudrunInput is what a converted client stack carries into provisioning. The
// embedded k8s.KubeRunInput is the point: it is byte-for-byte what
// kubernetes.KubeRunWithProvider consumes, so the deploy below the kubeconfig is the
// existing code path and not a second copy of it.
type K3sCloudrunInput struct {
	k8s.KubeRunInput `json:",inline" yaml:",inline"`

	// K3sClusterResource is carried through from the template because the
	// provisioning side needs it to spell the parent's export names, and the
	// embedded KubeRunInput has nowhere to put it.
	K3sClusterResource string `json:"k3sClusterResource" yaml:"k3sClusterResource"`
}

// ProviderType/ProjectIdValue/CredentialsValue are re-declared on the input rather
// than inherited: the embedded k8s.KubeRunInput promotes k8s.KubernetesConfig's
// versions, which report "kubernetes" and hand back an empty kubeconfig as
// credentials. See K3sCloudrunTemplate.ProviderType for what that would cost.
func (i *K3sCloudrunInput) ProviderType() string {
	return ProviderType
}

func (i *K3sCloudrunInput) ProjectIdValue() string {
	return "n/a"
}

func (i *K3sCloudrunInput) CredentialsValue() string {
	return "{}"
}

// ToK3sCloudrunConfig converts a cloud-compose client stack for the k3s path.
//
// It delegates the entire mapping to k8s.ToKubernetesRunConfig against a synthesised
// k8s.CloudrunTemplate. That is not laziness: ConvertComposeToContainers, the
// cloudExtras handling (probes, VPA, topology spread, security context) and ingress
// container detection are a few hundred lines that are identical for every Kubernetes
// flavour, and a second copy would drift silently — the symptom being a cloudExtras
// field that works on GKE and is ignored on k3s.
func ToK3sCloudrunConfig(tpl any, composeCfg compose.Config, stackCfg *api.StackConfigCompose) (any, error) {
	templateCfg, ok := tpl.(*K3sCloudrunTemplate)
	if !ok {
		return nil, errors.Errorf("template config is not of type *k3s.K3sCloudrunTemplate (given %T)", tpl)
	}
	if templateCfg == nil {
		return nil, errors.Errorf("template config is nil")
	}
	if err := templateCfg.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid %s template config", TemplateTypeK3sCloudrun)
	}

	// Kubeconfig is left empty on purpose and filled in by K3sCloudrun from the
	// parent stack's output. It cannot be known here.
	kubeTpl := &k8s.CloudrunTemplate{
		RegistryCredentials: templateCfg.RegistryCredentials,
		CaddyResource:       templateCfg.CaddyResource,
		UseSSL:              templateCfg.UseSSL,
	}

	converted, err := k8s.ToKubernetesRunConfig(kubeTpl, composeCfg, stackCfg)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert compose config for %s", TemplateTypeK3sCloudrun)
	}
	kubeRunInput, ok := converted.(*k8s.KubeRunInput)
	if !ok {
		return nil, errors.Errorf("expected *k8s.KubeRunInput from k8s converter, got %T", converted)
	}

	return &K3sCloudrunInput{
		KubeRunInput:       *kubeRunInput,
		K3sClusterResource: templateCfg.K3sClusterResource,
	}, nil
}
