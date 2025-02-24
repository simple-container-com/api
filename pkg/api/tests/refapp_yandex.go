package tests

import (
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/yandex"
)

var RefappYandexCloudFunctionServerDescriptor = &api.ServerDescriptor{
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
		"func-per-app": {
			Type: yandex.TemplateTypeYandexCloudFunction,
			Config: api.Config{Config: &yandex.TemplateConfig{
				AccountConfig: yandex.AccountConfig{
					CloudId: "${auth:yandex.projectId}",
					Credentials: api.Credentials{
						Credentials: "${auth:yandex}",
					},
				},
			}},
		},
	},
	Resources: api.PerStackResourcesDescriptor{
		Registrar: api.RegistrarDescriptor{
			Inherit: api.Inherit{Inherit: "common"},
		},
		Resources: map[string]api.PerEnvResourcesDescriptor{
			"staging": {
				Template: "func-per-app",
			},
			"prod": {
				Template: "func-per-app",
			},
		},
	},
}

var RefappYandexCloudFunctionClientDescriptor = &api.ClientDescriptor{
	SchemaVersion: api.ClientSchemaVersion,
	Stacks: map[string]api.StackClientDescriptor{
		"staging": {
			Type:        api.ClientTypeSingleImage,
			ParentStack: "refapp-yandex-cloud-function",
			Config: api.Config{
				Config: &api.StackConfigSingleImage{
					Domain: "staging.sc-refapp.org",
					Image: &api.ContainerImage{
						Dockerfile: "Dockerfile",
					},
					Env: map[string]string{
						"ENV": "staging",
					},
				},
			},
		},
		"prod": {
			Type:        api.ClientTypeSingleImage,
			ParentStack: "refapp-yandex-cloud-function",
			Config: api.Config{
				Config: &api.StackConfigSingleImage{
					Domain: "prod.sc-refapp.org",
					Image: &api.ContainerImage{
						Dockerfile: "Dockerfile",
					},
					Env: map[string]string{
						"ENV": "prod",
					},
				},
			},
		},
	},
}

var resolvedYandexAccountConfig = yandex.AccountConfig{
	CloudId: "000",
	Credentials: api.Credentials{
		Credentials: `{"account":"123","accessKey":"\u003cyandex-access-key\u003e","secretAccessKey":"\u003cyandex-secret-key\u003e","credentials":""}`,
	},
}

var ResolvedRefappYandexCloudFunctionServerDescriptor = &api.ServerDescriptor{
	SchemaVersion: api.ServerSchemaVersion,
	Provisioner:   ResolvedCommonServerDescriptor.Provisioner,
	Secrets:       ResolvedCommonServerDescriptor.Secrets,
	CiCd:          ResolvedCommonServerDescriptor.CiCd,
	Templates: map[string]api.StackDescriptor{
		"func-per-app": {
			Type: yandex.TemplateTypeYandexCloudFunction,
			Config: api.Config{Config: &yandex.TemplateConfig{
				AccountConfig: resolvedYandexAccountConfig,
			}},
		},
	},
	Variables: map[string]api.VariableDescriptor{},
	Resources: api.PerStackResourcesDescriptor{
		Registrar: ResolvedCommonServerDescriptor.Resources.Registrar,
		Resources: map[string]api.PerEnvResourcesDescriptor{
			"staging": {
				Template:  "func-per-app",
				Resources: map[string]api.ResourceDescriptor{},
			},
			"prod": {
				Template:  "func-per-app",
				Resources: map[string]api.ResourceDescriptor{},
			},
		},
	},
}

func ResolvedRefappYandexCloudFunctionClientDescriptor() *api.ClientDescriptor {
	res := RefappClientDescriptor.Copy()

	return &res
}
