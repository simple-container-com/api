package tests

import (
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

var RefappKubernetesServerResources map[string]api.ResourceDescriptor = nil

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

var ResolvedRefappKubernetesServerResources = map[string]api.ResourceDescriptor{}

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
