// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k8s

import (
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	AuthTypeKubeconfig = "kubernetes"
)

// KubeconfigRefKey is the output name a kubeconfig reference reads. A cluster
// resource that wants to be referenceable this way publishes its admin kubeconfig
// under this key (k3s's K3sOutput does; see pkg/clouds/pulumi/k3s/cluster.go).
const KubeconfigRefKey = "kubeconfig"

type KubernetesConfig struct {
	// Kubeconfig is either the kubeconfig itself — typically via
	// `${secret:...}` — or a reference to a sibling resource's output in the
	// same parent stack and environment, `${resource:<name>.kubeconfig}`.
	//
	// The reference form is what lets a cluster and the things deployed into it
	// live in one server.yaml and provision in one pass; see api.ResourceRef for
	// why it exists and KubeconfigRef below for how it is honoured. It is
	// resolved at the provisioning tier, not by the placeholder engine, because
	// the value does not exist until the sibling has been provisioned in this
	// same update.
	Kubeconfig string `json:"kubeconfig" yaml:"kubeconfig"`
}

func (r *KubernetesConfig) ProviderType() string {
	return ProviderType
}

func (r *KubernetesConfig) ProjectIdValue() string {
	return "n/a"
}

// CredentialsValue returns the field verbatim — INCLUDING an unresolved
// `${resource:...}` reference.
//
// Deliberately not resolved here: this method is a pure string accessor called
// from the generic provider-init path, which has no Pulumi context and no sibling
// outputs to resolve against. The one consumer that can resolve it —
// kubernetes.Provider — checks KubeconfigRef first and never reaches this value
// in the reference case. Anything else that treats the result as a credential
// without that check will fail loudly (an invalid kubeconfig), not silently.
func (r *KubernetesConfig) CredentialsValue() string {
	return r.Kubeconfig
}

// KubeconfigRef reports whether the kubeconfig is a sibling-resource reference,
// and which resource and output it names.
//
// Nil-safe: KubernetesConfig is embedded as a pointer by every resource that
// takes a kubeconfig (CaddyResource, the postgres resources), and a config that
// omitted the whole inline block leaves it nil.
func (r *KubernetesConfig) KubeconfigRef() (api.ResourceRef, bool, error) {
	if r == nil {
		return api.ResourceRef{}, false, nil
	}
	return api.ParseResourceRef(r.Kubeconfig)
}

// DependsOnResources makes every kubeconfig consumer satisfy
// api.WithParentDependencies, so the provisioner orders it AFTER the cluster it
// references rather than at the same time.
//
// Promoted into CaddyResource and the kubernetes-postgres resources through the
// embedded *KubernetesConfig, on purpose: the two-phase-provision problem is the
// kubeconfig's, not Caddy's, and fixing it on the field fixes it for every
// consumer at once instead of once per resource type.
//
// Returns nil for a literal kubeconfig. The type assertion in
// pkg/clouds/pulumi/provision.go succeeds either way; an empty dependency list is
// what tells the ordering there is no constraint.
//
// A malformed reference contributes no dependency rather than a garbage one — the
// refusal belongs to ReadKubernetesConfig, which has already run by then, and
// duplicating it here would only turn one clear error into two.
func (r *KubernetesConfig) DependsOnResources() []api.ParentResourceDependency {
	ref, isRef, err := r.KubeconfigRef()
	if err != nil || !isRef {
		return nil
	}
	return []api.ParentResourceDependency{{Name: ref.ResourceName}}
}

func ReadKubernetesConfig(config *api.Config) (api.Config, error) {
	cfg, err := api.ConvertConfig(config, &KubernetesConfig{})
	if err != nil {
		return cfg, err
	}
	// Refuse a malformed reference at parse time. The alternative is a string
	// that looks like a reference, is treated as a literal kubeconfig, and
	// surfaces as a connection failure from inside a Pulumi apply — an error
	// about the cluster, pointing nowhere near the typo.
	if kcfg, ok := cfg.Config.(*KubernetesConfig); ok {
		if _, _, err := kcfg.KubeconfigRef(); err != nil {
			return cfg, errors.Wrap(err, "invalid kubeconfig")
		}
	}
	return cfg, nil
}
