// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k3s

import (
	"testing"

	"github.com/compose-spec/compose-go/types"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
	"github.com/simple-container-com/api/pkg/clouds/docker"
)

func TestK3sCloudrunTemplateValidate(t *testing.T) {
	RegisterTestingT(t)

	for _, tc := range []struct {
		name    string
		tpl     K3sCloudrunTemplate
		wantErr string
	}{
		{
			name: "minimal valid",
			tpl: K3sCloudrunTemplate{
				K3sClusterResource: "ru-cluster",
				RegistryCredentials: docker.RegistryCredentials{
					DockerRegistryURL: lo.ToPtr("registry.example.ru/forge"),
				},
			},
		},
		{
			name: "no cluster resource",
			tpl: K3sCloudrunTemplate{
				RegistryCredentials: docker.RegistryCredentials{
					DockerRegistryURL: lo.ToPtr("registry.example.ru/forge"),
				},
			},
			wantErr: "`k3sClusterResource` must be specified",
		},
		{
			name:    "no registry",
			tpl:     K3sCloudrunTemplate{K3sClusterResource: "ru-cluster"},
			wantErr: "`dockerRegistryURL` must be specified",
		},
		{
			name: "empty registry is the same as none",
			tpl: K3sCloudrunTemplate{
				K3sClusterResource:  "ru-cluster",
				RegistryCredentials: docker.RegistryCredentials{DockerRegistryURL: lo.ToPtr("")},
			},
			wantErr: "`dockerRegistryURL` must be specified",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.tpl.Validate()
			if tc.wantErr == "" {
				Expect(err).To(BeNil())
				return
			}
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring(tc.wantErr))
		})
	}
}

// The provider type on BOTH the template and the converted input, because they are
// looked up on different paths — the template on provision, the input on deploy
// (deploy.go:193 → getProvisionParams → initProvider). Reporting "kubernetes" from
// either would hand the config to kubernetes.Provider, which builds a real
// pulumi-kubernetes provider from CredentialsValue(); with no kubeconfig to give it,
// pulumi-kubernetes falls back to the AMBIENT kubeconfig of whoever is running the
// deploy. That is a provider silently pointed at the operator's own cluster, and it
// would not fail — which is exactly why it is asserted rather than left to review.
func TestK3sCloudrunReportsK3sAndNotKubernetes(t *testing.T) {
	RegisterTestingT(t)

	var tpl api.AuthConfig = &K3sCloudrunTemplate{}
	Expect(tpl.ProviderType()).To(Equal(ProviderType))
	Expect(tpl.ProviderType()).NotTo(Equal("kubernetes"))
	// Valid JSON, not "": api.ConvertAuth json.Unmarshals this value.
	Expect(tpl.CredentialsValue()).To(Equal("{}"))

	var in api.AuthConfig = &K3sCloudrunInput{}
	Expect(in.ProviderType()).To(Equal(ProviderType),
		"the embedded k8s.KubeRunInput promotes a ProviderType() of \"kubernetes\" — it must be shadowed")
	Expect(in.CredentialsValue()).To(Equal("{}"),
		"the embedded k8s.KubernetesConfig promotes a CredentialsValue() returning its (empty) kubeconfig")
}

// The converter must delegate to the k8s one rather than reimplement it: the
// compose→containers mapping, cloudExtras handling and ingress detection are identical
// for every Kubernetes flavour, and a second copy drifts into "this cloudExtras field
// works on GKE but is ignored on k3s".
func TestToK3sCloudrunConfigDelegatesToTheK8sConverter(t *testing.T) {
	RegisterTestingT(t)

	tpl := &K3sCloudrunTemplate{
		K3sClusterResource: "ru-cluster",
		RegistryCredentials: docker.RegistryCredentials{
			DockerRegistryURL:      lo.ToPtr("registry.example.ru/forge"),
			DockerRegistryUsername: lo.ToPtr("robot"),
			DockerRegistryPassword: lo.ToPtr("pwd"),
		},
		CaddyResource: lo.ToPtr("caddy"),
		UseSSL:        lo.ToPtr(true),
	}
	composeCfg := compose.Config{
		Project: &types.Project{
			Services: types.Services{
				{
					Name:  "api",
					Image: "nginx:1.27-alpine",
					Ports: []types.ServicePortConfig{{Target: 80, Published: "80"}},
				},
			},
		},
	}
	stackCfg := &api.StackConfigCompose{
		DockerComposeFile: "docker-compose.yaml",
		Domain:            "k3s-smoke.example.ru",
		Runs:              []string{"api"},
	}

	out, err := ToK3sCloudrunConfig(tpl, composeCfg, stackCfg)
	Expect(err).To(BeNil())

	in, ok := out.(*K3sCloudrunInput)
	Expect(ok).To(BeTrue())

	// Carried through for the provisioning side to spell the parent's export names.
	Expect(in.K3sClusterResource).To(Equal("ru-cluster"))
	// Registry + Caddy + SSL survive the synthesised k8s.CloudrunTemplate.
	Expect(lo.FromPtr(in.DockerRegistryURL)).To(Equal("registry.example.ru/forge"))
	Expect(in.RegistryRequiresAuth()).To(BeTrue())
	Expect(lo.FromPtr(in.CaddyResource)).To(Equal("caddy"))
	Expect(lo.FromPtr(in.UseSSL)).To(BeTrue())
	// And the k8s converter's own work is present — containers were mapped.
	Expect(in.Deployment.Containers).NotTo(BeEmpty())
	Expect(in.Deployment.StackConfig).NotTo(BeNil())

	// Empty, and filled in by K3sCloudrun from the parent stack's output. It cannot be
	// known at convert time, which is the entire reason this template type exists
	// instead of reusing kubernetes-cloudrun.
	Expect(in.Kubeconfig).To(BeEmpty())
}

func TestToK3sCloudrunConfigRefusesABadTemplate(t *testing.T) {
	RegisterTestingT(t)

	_, err := ToK3sCloudrunConfig(&K3sCloudrunTemplate{}, compose.Config{}, &api.StackConfigCompose{})
	Expect(err).NotTo(BeNil())
	Expect(err.Error()).To(ContainSubstring("`k3sClusterResource` must be specified"))

	// A template of the wrong type is a registration bug, not an operator one, so it
	// says what it got.
	_, err = ToK3sCloudrunConfig(&K3sClusterConfig{}, compose.Config{}, &api.StackConfigCompose{})
	Expect(err).NotTo(BeNil())
	Expect(err.Error()).To(ContainSubstring("*k3s.K3sClusterConfig"))
}
