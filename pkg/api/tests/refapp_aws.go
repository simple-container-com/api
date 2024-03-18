package tests

import (
	"github.com/simple-container-com/api/pkg/api"
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

var RefappAwsClientDescriptor = &api.ClientDescriptor{
	SchemaVersion: api.ClientSchemaVersion,
	Stacks: map[string]api.StackClientDescriptor{
		"staging": {
			ParentStack: "refapp-aws",
			Environment: "staging",
			Domain:      "staging.sc-refapp.org",
			Config: api.Config{
				Config: &api.StackConfigCompose{
					DockerComposeFile: "./docker-compose.yaml",
					Uses: []string{
						"mongodb",
					},
					Runs: []string{
						"api",
						"ui",
					},
				},
			},
		},
		"prod": {
			ParentStack: "refapp-aws",
			Environment: "prod",
			Domain:      "prod.sc-refapp.org",
			Config: api.Config{
				Config: &api.StackConfigCompose{
					DockerComposeFile: "./docker-compose.yaml",
					Uses: []string{
						"mongodb",
					},
					Runs: []string{
						"api",
						"ui",
					},
				},
			},
		},
	},
}
