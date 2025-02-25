package tests

import (
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/yandex"
)

var RefappYandexServerlessContainerServerDescriptor = &api.ServerDescriptor{
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
			Type: yandex.TemplateTypeYandexServerlessContainer,
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

var RefappYandexServerlessContainerClientDescriptor = &api.ClientDescriptor{
	SchemaVersion: api.ClientSchemaVersion,
	Stacks: map[string]api.StackClientDescriptor{
		"staging": {
			Type:        api.ClientTypeSingleImage,
			ParentStack: "refapp-yandex-serverless-container",
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
			ParentStack: "refapp-yandex-serverless-container",
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
		Credentials: `<yandex-creds>`,
	},
}

var ResolvedRefappYandexServerlessContainerServerDescriptor = &api.ServerDescriptor{
	SchemaVersion: api.ServerSchemaVersion,
	Provisioner:   ResolvedCommonServerDescriptor.Provisioner,
	Secrets:       ResolvedCommonServerDescriptor.Secrets,
	CiCd:          ResolvedCommonServerDescriptor.CiCd,
	Templates: map[string]api.StackDescriptor{
		"func-per-app": {
			Type: yandex.TemplateTypeYandexServerlessContainer,
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

func ResolvedRefappYandexServerlessContainerClientDescriptor() *api.ClientDescriptor {
	res := RefappYandexServerlessContainerClientDescriptor.Copy()
	for name := range res.Stacks {
		stackCfg := res.Stacks[name]
		singleImage := stackCfg.Config.Config.(*api.StackConfigSingleImage)
		singleImage.Uses = []string{}
		singleImage.Secrets = map[string]string{}
		res.Stacks[name] = stackCfg
	}

	return &res
}
