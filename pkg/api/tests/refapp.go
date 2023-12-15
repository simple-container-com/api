package tests

import (
	"api/pkg/api"
	"api/pkg/clouds/cloudflare"
	"api/pkg/clouds/gcloud"
	"api/pkg/clouds/github"
	"api/pkg/clouds/mongodb"
	"api/pkg/clouds/pulumi"
)

var CommonServerDescriptor = &api.ServerDescriptor{
	SchemaVersion: api.ServerSchemaVersion,
	Provisioner: api.ProvisionerDescriptor{
		Type: pulumi.ProvisionerTypePulumi,
		Config: api.Config{Config: &pulumi.ProvisionerConfig{
			StateStorage: pulumi.StateStorageConfig{
				Type:        pulumi.StateStorageTypeGcpBucket,
				Credentials: "${auth:gcloud}",
				Provision:   true,
			},
			SecretsProvider: pulumi.SecretsProviderConfig{
				Type:        pulumi.SecretsProviderTypeGcpKms,
				Credentials: "${auth:gcloud}",
				KeyName:     "mypulumi-base-kms-key",
				KeyLocation: "global",
				Provision:   true,
			},
		}},
	},
	CiCd: api.CiCdDescriptor{
		Type: "github-actions",
		Config: api.Config{Config: &github.GithubActionsCiCdConfig{
			AuthToken: "${secret:GITHUB_TOKEN}",
		}},
	},
	Secrets: api.SecretsConfigDescriptor{
		Type: gcloud.SecretsTypeGCPSecretsManager,
		Config: api.Config{Config: &gcloud.SecretsConfig{
			Credentials: "${auth:gcloud}",
		}},
	},
	Templates: map[string]api.StackDescriptor{
		"stack-per-app": {
			Type: gcloud.TemplateTypeGcpCloudrun,
			Config: api.Config{Config: &gcloud.GcloudTemplateConfig{
				Credentials: "${auth:gcloud}",
			}},
		},
	},
	Resources: api.PerStackResourcesDescriptor{
		Registrar: api.RegistrarDescriptor{
			Type: cloudflare.RegistrarTypeCloudflare,
			Config: api.Config{Config: &cloudflare.RegistrarConfig{
				Credentials: "${secret:CLOUDFLARE_API_TOKEN}",
				Project:     "sc-refapp",
				ZoneName:    "sc-refapp.org",
				DnsRecords: []cloudflare.DnsRecord{
					{
						Name:  "@",
						Type:  "TXT",
						Value: "MS=ms83691649",
					},
				},
			}},
		},
	},
}

var ResolvedCommonServerDescriptor = &api.ServerDescriptor{
	SchemaVersion: api.ServerSchemaVersion,
	Provisioner: api.ProvisionerDescriptor{
		Type: pulumi.ProvisionerTypePulumi,
		Config: api.Config{Config: &pulumi.ProvisionerConfig{
			StateStorage: pulumi.StateStorageConfig{
				Type:        pulumi.StateStorageTypeGcpBucket,
				Credentials: "<gcloud-service-account-email>",
				Provision:   true,
			},
			SecretsProvider: pulumi.SecretsProviderConfig{
				Type:        pulumi.SecretsProviderTypeGcpKms,
				Credentials: "<gcloud-service-account-email>",
				KeyName:     "mypulumi-base-kms-key",
				KeyLocation: "global",
				Provision:   true,
			},
		}},
	},
	CiCd: api.CiCdDescriptor{
		Type: "github-actions",
		Config: api.Config{Config: &github.GithubActionsCiCdConfig{
			AuthToken: "<encrypted-secret>",
		}},
	},
	Secrets: api.SecretsConfigDescriptor{
		Type: gcloud.SecretsTypeGCPSecretsManager,
		Config: api.Config{Config: &gcloud.SecretsConfig{
			Credentials: "<gcloud-service-account-email>",
		}},
	},
	Templates: map[string]api.StackDescriptor{
		"stack-per-app": {
			Type: gcloud.TemplateTypeGcpCloudrun,
			Config: api.Config{Config: &gcloud.GcloudTemplateConfig{
				Credentials: "<gcloud-service-account-email>",
			}},
		},
	},
	Resources: api.PerStackResourcesDescriptor{
		Registrar: api.RegistrarDescriptor{
			Type: cloudflare.RegistrarTypeCloudflare,
			Config: api.Config{Config: &cloudflare.RegistrarConfig{
				Credentials: "<encrypted-secret>",
				Project:     "sc-refapp",
				ZoneName:    "sc-refapp.org",
				DnsRecords: []cloudflare.DnsRecord{
					{
						Name:  "@",
						Type:  "TXT",
						Value: "MS=ms83691649",
					},
				},
			}},
		},
		Resources: map[string]api.PerEnvResourcesDescriptor{},
	},
	Variables: map[string]api.VariableDescriptor{},
}

var RefappServerDescriptor = &api.ServerDescriptor{
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
		"stack-per-app": {
			Inherit: api.Inherit{Inherit: "common"},
		},
	},
	Variables: map[string]api.VariableDescriptor{
		"atlas-region": {
			Type:  "string",
			Value: "US_SOUTH_1",
		},
		"atlas-project-id": {
			Type:  "string",
			Value: "5b89110a4e6581562623c59c",
		},
		"atlas-org-id": {
			Type:  "string",
			Value: "5b89110a4e6581562623c59c",
		},
		"atlas-instance-size": {
			Type:  "string",
			Value: "M10",
		},
	},
	Resources: api.PerStackResourcesDescriptor{
		Registrar: api.RegistrarDescriptor{
			Inherit: api.Inherit{Inherit: "common"},
		},
		Resources: map[string]api.PerEnvResourcesDescriptor{
			"staging": {
				Template: "stack-per-app",
				Resources: map[string]api.ResourceDescriptor{
					"mongodb": {
						Type: "mongodb-atlas",
						Config: api.Config{Config: &mongodb.AtlasConfig{
							Admins:       []string{"smecsia"},
							Developers:   []string{},
							InstanceSize: "${var:atlas-instance-size}",
							OrgId:        "${var:atlas-org-id}",
							ProjectId:    "${var:atlas-project-id}",
							ProjectName:  "${stack:name}",
							Region:       "${var:atlas-region}",
							PrivateKey:   "${secret:MONGODB_ATLAS_PRIVATE_KEY}",
							PublicKey:    "${secret:MONGODB_ATLAS_PUBLIC_KEY}",
						}},
					},
					"postgres": {
						Type: "gcp-cloudsql-postgres",
						Config: api.Config{Config: &gcloud.PostgresGcpCloudsqlConfig{
							Version:     "14.5",
							Project:     "${stack:name}",
							Credentials: "${auth:gcloud}",
						}},
					},
				},
			},
			"prod": {
				Template: "stack-per-app",
				Resources: map[string]api.ResourceDescriptor{
					"mongodb": {
						Type: "mongodb-atlas",
						Config: api.Config{Config: &mongodb.AtlasConfig{
							Admins:       []string{"smecsia"},
							Developers:   []string{},
							InstanceSize: "${var:atlas-instance-size}",
							OrgId:        "${var:atlas-org-id}",
							ProjectId:    "${var:atlas-project-id}",
							ProjectName:  "${stack:name}",
							Region:       "${var:atlas-region}",
							PrivateKey:   "${secret:MONGODB_ATLAS_PRIVATE_KEY}",
							PublicKey:    "${secret:MONGODB_ATLAS_PUBLIC_KEY}",
						}},
					},
					"postgres": {
						Type: "gcp-cloudsql-postgres",
						Config: api.Config{Config: &gcloud.PostgresGcpCloudsqlConfig{
							Version:     "14.5",
							Project:     "${stack:name}",
							Credentials: "${auth:gcloud}",
						}},
					},
				},
			},
		},
	},
}

var ResolvedRefappServerDescriptor = &api.ServerDescriptor{
	SchemaVersion: api.ServerSchemaVersion,
	Provisioner:   ResolvedCommonServerDescriptor.Provisioner,
	Secrets:       ResolvedCommonServerDescriptor.Secrets,
	CiCd:          ResolvedCommonServerDescriptor.CiCd,
	Templates:     ResolvedCommonServerDescriptor.Templates,
	Variables: map[string]api.VariableDescriptor{
		"atlas-region": {
			Type:  "string",
			Value: "US_SOUTH_1",
		},
		"atlas-project-id": {
			Type:  "string",
			Value: "5b89110a4e6581562623c59c",
		},
		"atlas-org-id": {
			Type:  "string",
			Value: "5b89110a4e6581562623c59c",
		},
		"atlas-instance-size": {
			Type:  "string",
			Value: "M10",
		},
	},
	Resources: api.PerStackResourcesDescriptor{
		Registrar: ResolvedCommonServerDescriptor.Resources.Registrar,
		Resources: map[string]api.PerEnvResourcesDescriptor{
			"staging": {
				Template: "stack-per-app",
				Resources: map[string]api.ResourceDescriptor{
					"mongodb": {
						Type: "mongodb-atlas",
						Config: api.Config{Config: &mongodb.AtlasConfig{
							Admins:       []string{"smecsia"},
							Developers:   []string{},
							InstanceSize: "M10",
							OrgId:        "5b89110a4e6581562623c59c",
							ProjectId:    "5b89110a4e6581562623c59c",
							ProjectName:  "refapp",
							Region:       "US_SOUTH_1",
							PrivateKey:   "<encrypted-secret>",
							PublicKey:    "<encrypted-secret>",
						}},
					},
					"postgres": {
						Type: "gcp-cloudsql-postgres",
						Config: api.Config{Config: &gcloud.PostgresGcpCloudsqlConfig{
							Version:     "14.5",
							Project:     "refapp",
							Credentials: "<gcloud-service-account-email>",
						}},
					},
				},
			},
			"prod": {
				Template: "stack-per-app",
				Resources: map[string]api.ResourceDescriptor{
					"mongodb": {
						Type: "mongodb-atlas",
						Config: api.Config{Config: &mongodb.AtlasConfig{
							Admins:       []string{"smecsia"},
							Developers:   []string{},
							InstanceSize: "M10",
							OrgId:        "5b89110a4e6581562623c59c",
							ProjectId:    "5b89110a4e6581562623c59c",
							ProjectName:  "refapp",
							Region:       "US_SOUTH_1",
							PrivateKey:   "<encrypted-secret>",
							PublicKey:    "<encrypted-secret>",
						}},
					},
					"postgres": {
						Type: "gcp-cloudsql-postgres",
						Config: api.Config{Config: &gcloud.PostgresGcpCloudsqlConfig{
							Version:     "14.5",
							Project:     "refapp",
							Credentials: "<gcloud-service-account-email>",
						}},
					},
				},
			},
		},
	},
}

var CommonSecretsDescriptor = &api.SecretsDescriptor{
	SchemaVersion: api.SecretsSchemaVersion,
	Auth: map[string]api.AuthDescriptor{
		"gcloud": {
			Type: gcloud.AuthTypeGCPServiceAccount,
			Config: api.Config{Config: &gcloud.AuthServiceAccountConfig{
				Account: "<gcloud-service-account-email>",
			}},
		},
		"pulumi": {
			Type: pulumi.AuthTypePulumiToken,
			Config: api.Config{Config: &pulumi.TokenAuthDescriptor{
				Value: "<pulumi-token>",
			}},
		},
	},
	Values: map[string]string{
		"CLOUDFLARE_API_TOKEN":      "<encrypted-secret>",
		"GITHUB_TOKEN":              "<encrypted-secret>",
		"MONGODB_ATLAS_PRIVATE_KEY": "<encrypted-secret>",
		"MONGODB_ATLAS_PUBLIC_KEY":  "<encrypted-secret>",
	},
}

var RefappClientDescriptor = &api.ClientDescriptor{
	SchemaVersion: api.ClientSchemaVersion,
	Stacks: map[string]api.StackClientDescriptor{
		"staging": {
			Stack:  "refapp/staging",
			Domain: "staging.sc-refapp.org",
			Config: api.StackConfig{
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
		"prod": {
			Stack:  "refapp/prod",
			Domain: "prod.sc-refapp.org",
			Config: api.StackConfig{
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
}
