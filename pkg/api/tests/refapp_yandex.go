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
					Account: "${auth:yandex.projectId}",
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
			ParentStack: "refapp-aws-lambda",
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
			ParentStack: "refapp-aws-lambda",
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
