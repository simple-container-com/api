package tests

import (
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	"github.com/simple-container-com/api/pkg/clouds/cloudflare"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"github.com/simple-container-com/api/pkg/clouds/github"
	"github.com/simple-container-com/api/pkg/clouds/mongodb"
	"github.com/simple-container-com/api/pkg/clouds/pulumi"
)

var CommonServerDescriptor = &api.ServerDescriptor{
	SchemaVersion: api.ServerSchemaVersion,
	Provisioner: api.ProvisionerDescriptor{
		Type: pulumi.ProvisionerTypePulumi,
		Config: api.Config{Config: &pulumi.ProvisionerConfig{
			StateStorage: pulumi.StateStorageConfig{
				Type: pulumi.StateStorageTypeGcpBucket,
				Config: api.Config{Config: &gcloud.StateStorageConfig{
					Credentials: gcloud.Credentials{
						Credentials: api.Credentials{
							Credentials: "${auth:gcloud}",
						},
						ServiceAccountConfig: gcloud.ServiceAccountConfig{
							ProjectId: "${auth:gcloud.projectId}",
						},
					},
					Provision: true,
				}},
			},
			SecretsProvider: pulumi.SecretsProviderConfig{
				Type: pulumi.SecretsProviderTypeGcpKms,
				Config: api.Config{Config: &gcloud.SecretsProviderConfig{
					Credentials: gcloud.Credentials{
						Credentials: api.Credentials{
							Credentials: "${auth:gcloud}",
						},
						ServiceAccountConfig: gcloud.ServiceAccountConfig{
							ProjectId: "${auth:gcloud.projectId}",
						},
					},
					Provision:   true,
					KeyName:     "mypulumi-base-kms-key",
					KeyLocation: "global",
				}},
			},
		}},
	},
	CiCd: api.CiCdDescriptor{
		Type: "github-actions",
		Config: api.Config{Config: &github.ActionsCiCdConfig{
			AuthToken: "${secret:GITHUB_TOKEN}",
		}},
	},
	Secrets: api.SecretsConfigDescriptor{
		Type: gcloud.SecretsTypeGCPSecretsManager,
		Config: api.Config{Config: &gcloud.SecretsProviderConfig{
			Credentials: gcloud.Credentials{
				Credentials: api.Credentials{
					Credentials: "${auth:gcloud}",
				},
				ServiceAccountConfig: gcloud.ServiceAccountConfig{
					ProjectId: "${auth:gcloud.projectId}",
				},
			},
		}},
	},
	Templates: map[string]api.StackDescriptor{
		"stack-per-app-aws": {
			Type: aws.TemplateTypeEcsFargate,
			Config: api.Config{Config: &aws.TemplateConfig{
				AwsAccountConfig: aws.AwsAccountConfig{
					Account: "${auth:aws.projectId}",
					Credentials: api.Credentials{
						Credentials: "${auth:aws}",
					},
				},
			}},
		},
		"stack-per-app": {
			Type: gcloud.TemplateTypeGcpCloudrun,
			Config: api.Config{Config: &gcloud.TemplateConfig{
				Credentials: gcloud.Credentials{
					Credentials: api.Credentials{
						Credentials: "${auth:gcloud}",
					},
					ServiceAccountConfig: gcloud.ServiceAccountConfig{
						ProjectId: "${auth:gcloud.projectId}",
					},
				},
			}},
		},
	},
	Resources: api.PerStackResourcesDescriptor{
		Registrar: api.RegistrarDescriptor{
			Type: cloudflare.RegistrarType,
			Config: api.Config{Config: &cloudflare.RegistrarConfig{
				AuthConfig: cloudflare.AuthConfig{
					Credentials: api.Credentials{
						Credentials: "${secret:CLOUDFLARE_API_TOKEN}",
					},
					AccountId: "12345",
				},
				ZoneName: "sc-refapp.org",
				Records: []api.DnsRecord{
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
				Type: pulumi.StateStorageTypeGcpBucket,
				Config: api.Config{Config: &gcloud.StateStorageConfig{
					Credentials: gcloud.Credentials{
						Credentials: api.Credentials{
							Credentials: "<gcloud-service-account-email>",
						},
						ServiceAccountConfig: gcloud.ServiceAccountConfig{
							ProjectId: "test-gcp-project",
						},
					},
					Provision: true,
				}},
			},
			SecretsProvider: pulumi.SecretsProviderConfig{
				Type: pulumi.SecretsProviderTypeGcpKms,
				Config: api.Config{Config: &gcloud.SecretsProviderConfig{
					Credentials: gcloud.Credentials{
						Credentials: api.Credentials{
							Credentials: "<gcloud-service-account-email>",
						},
						ServiceAccountConfig: gcloud.ServiceAccountConfig{
							ProjectId: "test-gcp-project",
						},
					},
					KeyName:     "mypulumi-base-kms-key",
					KeyLocation: "global",
					Provision:   true,
				}},
			},
		}},
	},
	CiCd: api.CiCdDescriptor{
		Type: "github-actions",
		Config: api.Config{Config: &github.ActionsCiCdConfig{
			AuthToken: "<encrypted-secret>",
		}},
	},
	Secrets: api.SecretsConfigDescriptor{
		Type: gcloud.SecretsTypeGCPSecretsManager,
		Config: api.Config{Config: &gcloud.SecretsProviderConfig{
			Credentials: gcloud.Credentials{
				Credentials: api.Credentials{
					Credentials: "<gcloud-service-account-email>",
				},
				ServiceAccountConfig: gcloud.ServiceAccountConfig{
					ProjectId: "test-gcp-project",
				},
			},
		}},
	},
	Templates: map[string]api.StackDescriptor{
		"stack-per-app-aws": {
			Type: aws.TemplateTypeEcsFargate,
			Config: api.Config{Config: &aws.TemplateConfig{
				AwsAccountConfig: aws.AwsAccountConfig{
					Account: "000",
					Credentials: api.Credentials{
						Credentials: `{"account":"000","accessKey":"\u003caws-access-key\u003e","secretAccessKey":"\u003caws-secret-key\u003e","credentials":""}`,
					},
				},
			}},
		},
		"stack-per-app": {
			Type: gcloud.TemplateTypeGcpCloudrun,
			Config: api.Config{Config: &gcloud.TemplateConfig{
				Credentials: gcloud.Credentials{
					Credentials: api.Credentials{
						Credentials: "<gcloud-service-account-email>",
					},
					ServiceAccountConfig: gcloud.ServiceAccountConfig{
						ProjectId: "test-gcp-project",
					},
				},
			}},
		},
	},
	Resources: api.PerStackResourcesDescriptor{
		Registrar: api.RegistrarDescriptor{
			Type: cloudflare.RegistrarType,
			Config: api.Config{Config: &cloudflare.RegistrarConfig{
				AuthConfig: cloudflare.AuthConfig{
					Credentials: api.Credentials{
						Credentials: "<encrypted-secret>",
					},
					AccountId: "12345",
				},
				ZoneName: "sc-refapp.org",
				Records: []api.DnsRecord{
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
						Type: mongodb.ResourceTypeMongodbAtlas,
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
						Type: gcloud.ResourceTypePostgresGcpCloudsql,
						Config: api.Config{Config: &gcloud.PostgresGcpCloudsqlConfig{
							Version: "14.5",
							Project: "${stack:name}",
							Credentials: gcloud.Credentials{
								Credentials: api.Credentials{
									Credentials: "${auth:gcloud}",
								},
								ServiceAccountConfig: gcloud.ServiceAccountConfig{
									ProjectId: "${auth:gcloud.projectId}",
								},
							},
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
							Version: "14.5",
							Project: "${stack:name}",
							Credentials: gcloud.Credentials{
								Credentials: api.Credentials{
									Credentials: "${auth:gcloud}",
								},
								ServiceAccountConfig: gcloud.ServiceAccountConfig{
									ProjectId: "${auth:gcloud.projectId}",
								},
							},
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
	Templates: map[string]api.StackDescriptor{
		"stack-per-app": ResolvedCommonServerDescriptor.Templates["stack-per-app"],
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
		Registrar: ResolvedCommonServerDescriptor.Resources.Registrar,
		Resources: map[string]api.PerEnvResourcesDescriptor{
			"staging": {
				Template: "stack-per-app",
				Resources: map[string]api.ResourceDescriptor{
					"mongodb": {
						Type: mongodb.ResourceTypeMongodbAtlas,
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
						Type: gcloud.ResourceTypePostgresGcpCloudsql,
						Config: api.Config{Config: &gcloud.PostgresGcpCloudsqlConfig{
							Version: "14.5",
							Project: "refapp",
							Credentials: gcloud.Credentials{
								Credentials: api.Credentials{
									Credentials: "<gcloud-service-account-email>",
								},
								ServiceAccountConfig: gcloud.ServiceAccountConfig{
									ProjectId: "test-gcp-project",
								},
							},
						}},
					},
				},
			},
			"prod": {
				Template: "stack-per-app",
				Resources: map[string]api.ResourceDescriptor{
					"mongodb": {
						Type: mongodb.ResourceTypeMongodbAtlas,
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
						Type: gcloud.ResourceTypePostgresGcpCloudsql,
						Config: api.Config{Config: &gcloud.PostgresGcpCloudsqlConfig{
							Version: "14.5",
							Project: "refapp",
							Credentials: gcloud.Credentials{
								Credentials: api.Credentials{
									Credentials: "<gcloud-service-account-email>",
								},
								ServiceAccountConfig: gcloud.ServiceAccountConfig{
									ProjectId: "test-gcp-project",
								},
							},
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
			Config: api.Config{Config: &gcloud.Credentials{
				Credentials: api.Credentials{
					Credentials: "<gcloud-service-account-email>",
				},
				ServiceAccountConfig: gcloud.ServiceAccountConfig{
					ProjectId: "test-gcp-project",
				},
			}},
		},
		"aws": {
			Type: aws.AuthTypeAWSToken,
			Config: api.Config{Config: &aws.AuthAccessKeyConfig{
				AwsAccountConfig: aws.AwsAccountConfig{
					Account:         "000",
					AccessKey:       "<aws-access-key>",
					SecretAccessKey: "<aws-secret-key>",
				},
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
			Type:        api.ClientTypeCompose,
			ParentStack: "refapp",
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
			Type:        api.ClientTypeCompose,
			ParentStack: "refapp",
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
