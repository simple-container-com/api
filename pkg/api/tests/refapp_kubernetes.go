package tests

import (
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

var RefappKubernetesServerResources = map[string]api.ResourceDescriptor{
	"caddy": {
		Type: k8s.ResourceTypeCaddy,
		Config: api.Config{Config: &k8s.CaddyResource{
			KubernetesConfig: k8s.KubernetesConfig{
				Kubeconfig: "${auth:kubernetes}",
			},
			CaddyConfig: k8s.CaddyConfig{
				Enable:    lo.ToPtr(true),
				Namespace: lo.ToPtr("caddy"),
				Replicas:  lo.ToPtr(2),
			},
		}},
	},
}

var RefappKubernetesServerDescriptor = &api.ServerDescriptor{
	SchemaVersion: api.ServerSchemaVersion,
	Provisioner: api.ProvisionerDescriptor{
		Inherit: api.Inherit{Inherit: "common"},
	},
	Secrets: api.SecretsConfigDescriptor{
		Inherit: api.Inherit{Inherit: "common"},
	},
	CiCd: api.CiCdDescriptor{
		Inherit: api.Inherit{Inherit: "common"},
	},
	Templates: map[string]api.StackDescriptor{
		"stack-per-app-k8s": {
			Type: k8s.TemplateTypeKubernetes,
			Config: api.Config{
				Config: &k8s.TemplateConfig{
					KubernetesConfig: k8s.KubernetesConfig{
						Kubeconfig: "${auth:kubernetes}",
					},
					DockerRegistryURL:      lo.ToPtr("index.docker.io"),
					DockerRegistryUsername: lo.ToPtr("${secret:docker-registry-username}"),
					DockerRegistryPassword: lo.ToPtr("${secret:docker-registry-password}"),
					CaddyResource:          lo.ToPtr("caddy"),
				},
			},
			Inherit: api.Inherit{},
		},
	},
	Resources: api.PerStackResourcesDescriptor{
		Registrar: api.RegistrarDescriptor{
			Inherit: api.Inherit{Inherit: "common"},
		},
		Resources: map[string]api.PerEnvResourcesDescriptor{
			"staging": {
				Template:  "stack-per-app-k8s",
				Resources: RefappKubernetesServerResources,
			},
		},
	},
}

var ResolvedRefappKubernetesServerResources = map[string]api.ResourceDescriptor{
	"caddy": {
		Type: k8s.ResourceTypeCaddy,
		Config: api.Config{Config: &k8s.CaddyResource{
			KubernetesConfig: k8s.KubernetesConfig{
				Kubeconfig: "<kube-config>",
			},
			CaddyConfig: k8s.CaddyConfig{
				Enable:    lo.ToPtr(true),
				Namespace: lo.ToPtr("caddy"),
				Replicas:  lo.ToPtr(2),
			},
		}},
	},
}

var ResolvedRefappKubernetesServerDescriptor = &api.ServerDescriptor{
	SchemaVersion: api.ServerSchemaVersion,
	Provisioner:   ResolvedCommonServerDescriptor.Provisioner,
	Secrets:       ResolvedCommonServerDescriptor.Secrets,
	CiCd:          ResolvedCommonServerDescriptor.CiCd,
	Templates: map[string]api.StackDescriptor{
		"stack-per-app-k8s": {
			Type: "kubernetes-cloudrun",
			Config: api.Config{
				Config: &k8s.TemplateConfig{
					KubernetesConfig: k8s.KubernetesConfig{
						Kubeconfig: "<kube-config>",
					},
					DockerRegistryURL:      lo.ToPtr("index.docker.io"),
					DockerRegistryUsername: lo.ToPtr("test-user"),
					DockerRegistryPassword: lo.ToPtr("test-pass"),
					CaddyResource:          lo.ToPtr("caddy"),
				},
			},
			Inherit: api.Inherit{},
		},
	},
	Variables: map[string]api.VariableDescriptor{},
	Resources: api.PerStackResourcesDescriptor{
		Registrar: ResolvedCommonServerDescriptor.Resources.Registrar,
		Resources: map[string]api.PerEnvResourcesDescriptor{
			"staging": {
				Template:  "stack-per-app-k8s",
				Resources: ResolvedRefappKubernetesServerResources,
			},
		},
	},
}

var RefappKubernetesClientDescriptor = &api.ClientDescriptor{
	SchemaVersion: api.ClientSchemaVersion,
	Stacks: map[string]api.StackClientDescriptor{
		"staging": {
			Type:        api.ClientTypeCloudCompose,
			ParentStack: "refapp-kubernetes",
			Config: api.Config{
				Config: RefappClientComposeConfigStaging,
			},
		},
		"prod": {
			Type:        api.ClientTypeCloudCompose,
			ParentStack: "refapp-kubernetes",
			Config: api.Config{
				Config: RefappClientComposeConfigProd,
			},
		},
	},
}
