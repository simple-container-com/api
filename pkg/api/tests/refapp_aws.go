package tests

import (
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
)

var RefappAwsServerDescriptor = &api.ServerDescriptor{
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
		"stack-per-app-aws": {
			Inherit: api.Inherit{Inherit: "common"},
		},
	},
	Resources: api.PerStackResourcesDescriptor{
		Registrar: api.RegistrarDescriptor{
			Inherit: api.Inherit{Inherit: "common"},
		},
		Resources: map[string]api.PerEnvResourcesDescriptor{
			"staging": {
				Template: "stack-per-app-aws",
			},
			"prod": {
				Template: "stack-per-app-aws",
			},
		},
	},
}

var RefappAwsLambdaServerDescriptor = &api.ServerDescriptor{
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
		"lambda-per-app": {
			Type: aws.TemplateTypeAwsLambda,
			Config: api.Config{Config: &aws.TemplateConfig{
				AccountConfig: aws.AccountConfig{
					Account: "${auth:aws.projectId}",
					Credentials: api.Credentials{
						Credentials: "${auth:aws}",
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
				Template: "lambda-per-app",
			},
			"prod": {
				Template: "lambda-per-app",
			},
		},
	},
}

var ResolvedRefappAwsServerDescriptor = &api.ServerDescriptor{
	SchemaVersion: api.ServerSchemaVersion,
	Provisioner:   ResolvedCommonServerDescriptor.Provisioner,
	Secrets:       ResolvedCommonServerDescriptor.Secrets,
	CiCd:          ResolvedCommonServerDescriptor.CiCd,
	Templates: map[string]api.StackDescriptor{
		"stack-per-app-aws": ResolvedCommonServerDescriptor.Templates["stack-per-app-aws"],
	},
	Variables: map[string]api.VariableDescriptor{},
	Resources: api.PerStackResourcesDescriptor{
		Registrar: ResolvedCommonServerDescriptor.Resources.Registrar,
		Resources: map[string]api.PerEnvResourcesDescriptor{
			"staging": {
				Template:  "stack-per-app-aws",
				Resources: map[string]api.ResourceDescriptor{},
			},
			"prod": {
				Template:  "stack-per-app-aws",
				Resources: map[string]api.ResourceDescriptor{},
			},
		},
	},
}

var RefappAwsLambdaClientDescriptor = &api.ClientDescriptor{
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

var RefappAwsClientDescriptor = &api.ClientDescriptor{
	SchemaVersion: api.ClientSchemaVersion,
	Stacks: map[string]api.StackClientDescriptor{
		"staging": {
			Type:        api.ClientTypeCloudCompose,
			ParentStack: "refapp-aws",
			Config: api.Config{
				Config: RefappClientComposeConfigStaging,
			},
		},
		"prod": {
			Type:        api.ClientTypeCloudCompose,
			ParentStack: "refapp-aws",
			Config: api.Config{
				Config: RefappClientComposeConfigProd,
			},
		},
	},
}
