// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k8s

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

// The dependency edge is the whole point: it is what makes the provisioner run
// the cluster first and hand its outputs to the Caddy edge, instead of the two
// racing and the operator pasting a kubeconfig between two `sc provision` runs.
func TestKubernetesConfig_DependsOnResources(t *testing.T) {
	RegisterTestingT(t)

	Expect((&KubernetesConfig{Kubeconfig: "${resource:my-k3s.kubeconfig}"}).DependsOnResources()).
		To(Equal([]api.ParentResourceDependency{{Name: "my-k3s"}}))

	// A literal kubeconfig declares no dependency. The resource still satisfies
	// api.WithParentDependencies — the type assertion cannot be conditional —
	// so the EMPTY list is what tells the ordering there is no constraint.
	Expect((&KubernetesConfig{Kubeconfig: kubeconfigYAML}).DependsOnResources()).To(BeEmpty())
	Expect((&KubernetesConfig{Kubeconfig: "${secret:K3S_KUBECONFIG}"}).DependsOnResources()).To(BeEmpty())

	// Nil receiver: KubernetesConfig is embedded as a POINTER by every resource
	// that takes a kubeconfig, and a config that omitted the inline block leaves
	// it nil. Reached through promotion on CaddyResource, so a panic here is a
	// panic inside the provisioner's ordering pass.
	var absent *KubernetesConfig
	Expect(absent.DependsOnResources()).To(BeEmpty())
	ref, isRef, err := absent.KubeconfigRef()
	Expect(err).ToNot(HaveOccurred())
	Expect(isRef).To(BeFalse())
	Expect(ref).To(Equal(api.ResourceRef{}))
}

// Every resource type that embeds *KubernetesConfig gets the fix, not just
// Caddy: the two-phase-provision problem belongs to the kubeconfig field. Pinned
// because the promotion is invisible at the use site — nothing in CaddyResource
// or the helm operator types mentions DependsOnResources.
func TestKubeconfigConsumers_SatisfyWithParentDependencies(t *testing.T) {
	RegisterTestingT(t)

	const ref = "${resource:my-k3s.kubeconfig}"
	for name, cfg := range map[string]any{
		"kubernetes-caddy":       &CaddyResource{KubernetesConfig: &KubernetesConfig{Kubeconfig: ref}},
		"helm-postgres-operator": &HelmPostgresOperator{KubernetesConfig: &KubernetesConfig{Kubeconfig: ref}},
		"helm-mongodb-operator":  &HelmMongodbOperator{KubernetesConfig: &KubernetesConfig{Kubeconfig: ref}},
	} {
		withDeps, ok := cfg.(api.WithParentDependencies)
		Expect(ok).To(BeTrue(), "%s must satisfy api.WithParentDependencies so the provisioner orders it "+
			"after the cluster its kubeconfig names", name)
		Expect(withDeps.DependsOnResources()).To(Equal([]api.ParentResourceDependency{{Name: "my-k3s"}}),
			"%s did not pick up the kubeconfig's dependency", name)
	}
}

// A malformed reference must be refused where the operator can still see their
// own file. Left to the provisioning tier it becomes a connection error naming
// the cluster — the one place nobody looks for a typo.
func TestReaders_RefuseMalformedKubeconfigRef(t *testing.T) {
	RegisterTestingT(t)

	const bad = "${resource:my-k3s}"

	_, err := ReadKubernetesConfig(&api.Config{Config: &KubernetesConfig{Kubeconfig: bad}})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("${resource:<resource-name>.<output>}"))

	// CaddyReadConfig is a SEPARATE reader — the Caddy edge is the first and
	// most common consumer of the reference, and it never goes through
	// ReadKubernetesConfig.
	_, err = CaddyReadConfig(&api.Config{Config: &CaddyResource{KubernetesConfig: &KubernetesConfig{Kubeconfig: bad}}})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("${resource:<resource-name>.<output>}"))

	// And a valid one passes both.
	_, err = ReadKubernetesConfig(&api.Config{Config: &KubernetesConfig{Kubeconfig: "${resource:my-k3s.kubeconfig}"}})
	Expect(err).ToNot(HaveOccurred())
	_, err = CaddyReadConfig(&api.Config{Config: &CaddyResource{
		KubernetesConfig: &KubernetesConfig{Kubeconfig: "${resource:my-k3s.kubeconfig}"},
	}})
	Expect(err).ToNot(HaveOccurred())
}
