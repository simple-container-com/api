package tests

import (
	"fmt"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	"github.com/simple-container-com/api/pkg/clouds/cloudflare"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"github.com/simple-container-com/api/pkg/clouds/github"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	"github.com/simple-container-com/api/pkg/clouds/mongodb"
	"github.com/simple-container-com/api/pkg/clouds/pulumi"
)

var CommonGcpCredentials = gcloud.Credentials{
	Credentials: api.Credentials{
		Credentials: "${auth:gcloud}",
	},
	ServiceAccountConfig: gcloud.ServiceAccountConfig{
		ProjectId: "${auth:gcloud.projectId}",
	},
}

var CommonKubernetesCredentials = k8s.KubernetesConfig{
	Kubeconfig: "${auth:kubernetes}",
}

var awsAccountConfig = aws.AccountConfig{
	Account: "${auth:aws.projectId}",
	Credentials: api.Credentials{
		Credentials: "${auth:aws}",
	},
}

var resolvedAwsAccountConfig = aws.AccountConfig{
	Account: "000",
	Credentials: api.Credentials{
		Credentials: `{"account":"000","accessKey":"\u003caws-access-key\u003e","secretAccessKey":"\u003caws-secret-key\u003e","region":"","credentials":""}`,
	},
}

var CommonServerDescriptor = &api.ServerDescriptor{
	SchemaVersion: api.ServerSchemaVersion,
	Provisioner: api.ProvisionerDescriptor{
		Type: pulumi.ProvisionerTypePulumi,
		Config: api.Config{Config: &pulumi.ProvisionerConfig{
			StateStorage: pulumi.StateStorageConfig{
				Type: pulumi.StateStorageTypeGcpBucket,
				Config: api.Config{Config: &gcloud.StateStorageConfig{
					Credentials: CommonGcpCredentials,
					Provision:   true,
				}},
			},
			SecretsProvider: pulumi.SecretsProviderConfig{
				Type: pulumi.SecretsProviderTypeGcpKms,
				Config: api.Config{Config: &gcloud.SecretsProviderConfig{
					Credentials: CommonGcpCredentials,
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
				AccountConfig: awsAccountConfig,
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

var ResolvedCommonGcpCredentials = gcloud.Credentials{
	Credentials: api.Credentials{
		Credentials: "<gcloud-service-account-email>",
	},
	ServiceAccountConfig: gcloud.ServiceAccountConfig{
		ProjectId: "test-gcp-project",
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
					Credentials: ResolvedCommonGcpCredentials,
					Provision:   true,
				}},
			},
			SecretsProvider: pulumi.SecretsProviderConfig{
				Type: pulumi.SecretsProviderTypeGcpKms,
				Config: api.Config{Config: &gcloud.SecretsProviderConfig{
					Credentials: ResolvedCommonGcpCredentials,
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
			Credentials: ResolvedCommonGcpCredentials,
		}},
	},
	Templates: map[string]api.StackDescriptor{
		"stack-per-app-aws": {
			Type: aws.TemplateTypeEcsFargate,
			Config: api.Config{Config: &aws.TemplateConfig{
				AccountConfig: resolvedAwsAccountConfig,
			}},
		},
		"stack-per-app": {
			Type: gcloud.TemplateTypeGcpCloudrun,
			Config: api.Config{Config: &gcloud.TemplateConfig{
				Credentials: ResolvedCommonGcpCredentials,
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

var mongodbAtlasResourceConfig = api.ResourceDescriptor{
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
		NetworkConfig: &mongodb.AtlasNetworkConfig{
			PrivateLinkEndpoint: &mongodb.PrivateLinkEndpoint{
				ProviderName: "AWS",
			},
		},
		ExtraProviders: map[string]api.AuthDescriptor{
			"AWS": {
				Type: "aws-token",
				Config: api.Config{
					Config: &awsAccountConfig,
				},
				Inherit: api.Inherit{},
			},
		},
	}},
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
					"mongodb": mongodbAtlasResourceConfig,
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
					"mongodb": mongodbAtlasResourceConfig,
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

var resolvedMongodbAtlasResource = api.ResourceDescriptor{
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
		NetworkConfig: &mongodb.AtlasNetworkConfig{
			PrivateLinkEndpoint: &mongodb.PrivateLinkEndpoint{
				ProviderName: "AWS",
			},
		},
		ExtraProviders: map[string]api.AuthDescriptor{
			"AWS": {
				Type: "aws-token",
				Config: api.Config{
					Config: &resolvedAwsAccountConfig,
				},
				Inherit: api.Inherit{},
			},
		},
	}},
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
					"mongodb": resolvedMongodbAtlasResource,
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
					"mongodb": resolvedMongodbAtlasResource,
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
			Config: api.Config{Config: &aws.AccountConfig{
				Account:         "000",
				AccessKey:       "<aws-access-key>",
				SecretAccessKey: "<aws-secret-key>",
			}},
		},
		"pulumi": {
			Type: pulumi.AuthTypePulumiToken,
			Config: api.Config{Config: &pulumi.TokenAuthDescriptor{
				Credentials: "<pulumi-token>",
			}},
		},
		"kubernetes": {
			Type: k8s.AuthTypeKubeconfig,
			Config: api.Config{Config: &k8s.KubernetesConfig{
				Kubeconfig: "<kube-config>",
			}},
		},
	},
	Values: map[string]string{
		"CLOUDFLARE_API_TOKEN":      "<encrypted-secret>",
		"GITHUB_TOKEN":              "<encrypted-secret>",
		"MONGODB_ATLAS_PRIVATE_KEY": "<encrypted-secret>",
		"MONGODB_ATLAS_PUBLIC_KEY":  "<encrypted-secret>",
		"JWT_SECRET":                "<encrypted-secret>",
	},
}

var RefappClientDescriptor = &api.ClientDescriptor{
	SchemaVersion: api.ClientSchemaVersion,
	Stacks: map[string]api.StackClientDescriptor{
		"staging": {
			Type:        api.ClientTypeCloudCompose,
			ParentStack: "refapp",
			Config: api.Config{
				Config: &api.StackConfigCompose{
					Domain:            "staging.sc-refapp.org",
					DockerComposeFile: "${git:root}/docker-compose.yaml",
					Uses: []string{
						"mongodb",
					},
					Runs: []string{
						"api",
						"ui",
					},
					Env: map[string]string{
						"JWT_SECRET": "${secret:JWT_SECRET}",
						"MONGO_URI":  "${resource:mongodb.uri}",
					},
					Secrets:      map[string]string{},
					Dependencies: []api.StackConfigDependencyResource{},
				},
			},
		},
		"prod": {
			Type:        api.ClientTypeCloudCompose,
			ParentStack: "refapp",
			Config: api.Config{
				Config: &api.StackConfigCompose{
					Domain:            "prod.sc-refapp.org",
					DockerComposeFile: "${git:root}/docker-compose.yaml",
					Uses: []string{
						"mongodb",
					},
					Runs: []string{
						"api",
						"ui",
					},
					Env: map[string]string{
						"JWT_SECRET": "${secret:JWT_SECRET}",
						"MONGO_URI":  "${resource:mongodb.uri}",
					},
					Secrets:      map[string]string{},
					Dependencies: []api.StackConfigDependencyResource{},
				},
			},
		},
	},
}

var RefappClientComposeConfigStaging = &api.StackConfigCompose{
	Domain:            "staging.sc-refapp.org",
	DockerComposeFile: "${git:root}/docker-compose.yaml",
	Uses: []string{
		"mongodb",
	},
	Runs: []string{
		"api",
		"ui",
	},
	Env:          map[string]string{},
	Secrets:      map[string]string{},
	Dependencies: []api.StackConfigDependencyResource{},
}

var RefappClientComposeConfigProd = &api.StackConfigCompose{
	Domain:            "prod.sc-refapp.org",
	DockerComposeFile: "${git:root}/docker-compose.yaml",
	Uses: []string{
		"mongodb",
	},
	Runs: []string{
		"api",
		"ui",
	},
	Env:          map[string]string{},
	Secrets:      map[string]string{},
	Dependencies: []api.StackConfigDependencyResource{},
}

func ResolvedRefappCloudClientDescriptor(gitRoot string, desc *api.ClientDescriptor) *api.ClientDescriptor {
	res := desc.Copy()
	staging := res.Stacks["staging"]
	stagingCompose := staging.Config.Config.(*api.StackConfigCompose)
	stagingCompose.DockerComposeFile = fmt.Sprintf("%s/docker-compose.yaml", gitRoot)
	res.Stacks["staging"] = staging
	prod := res.Stacks["prod"]
	prodCompose := prod.Config.Config.(*api.StackConfigCompose)
	prodCompose.DockerComposeFile = fmt.Sprintf("%s/docker-compose.yaml", gitRoot)
	res.Stacks["prod"] = prod
	return &res
}

func ResolvedRefappClientDescriptor(gitRoot string) *api.ClientDescriptor {
	res := RefappClientDescriptor.Copy()
	staging := res.Stacks["staging"]
	stagingCompose := staging.Config.Config.(*api.StackConfigCompose)
	stagingCompose.DockerComposeFile = fmt.Sprintf("%s/docker-compose.yaml", gitRoot)
	stagingCompose.Env["JWT_SECRET"] = "<encrypted-secret>"
	res.Stacks["staging"] = staging
	prod := res.Stacks["prod"]
	prodCompose := prod.Config.Config.(*api.StackConfigCompose)
	prodCompose.DockerComposeFile = fmt.Sprintf("%s/docker-compose.yaml", gitRoot)
	prodCompose.Env["JWT_SECRET"] = "<encrypted-secret>"
	res.Stacks["prod"] = prod
	return &res
}
