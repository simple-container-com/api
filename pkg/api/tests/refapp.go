// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package tests

import (
	"fmt"

	"github.com/simple-container-com/sc-plugin-aws/api"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"github.com/simple-container-com/api/pkg/clouds/github"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
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
		Config: api.Config{Config: &github.GitHubActionsCiCdConfig{
			Organization: "simple-container-org",
			Environments: map[string]github.GitHubEnvironmentConfig{
				"staging": {
					Type:        "staging",
					Runner:      "",
					Protection:  false,
					Reviewers:   nil,
					Secrets:     nil,
					Variables:   nil,
					DeployFlags: nil,
					AutoDeploy:  false,
				},
				"production": {
					Type:        "production",
					Runner:      "",
					Protection:  false,
					Reviewers:   nil,
					Secrets:     nil,
					Variables:   nil,
					DeployFlags: nil,
					AutoDeploy:  false,
				},
			},
			Notifications: github.GitHubNotificationConfig{
				Slack: github.SlackNotificationConfig{
					WebhookURL: "",
					Enabled:    false,
				},
				Discord: github.DiscordNotificationConfig{
					WebhookURL: "",
					Enabled:    false,
				},
				Telegram: github.TelegramNotificationConfig{
					BotToken: "",
					ChatID:   "",
					Enabled:  false,
				},
			},
			WorkflowGeneration: github.GitHubWorkflowConfig{
				Enabled:       false,
				OutputPath:    "",
				Templates:     []string{"deploy", "destroy"},
				AutoUpdate:    false,
				CustomActions: map[string]string{},
				SCVersion:     "",
			},
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
			Type: "cloudflare",
			// Post-A3 the cloudflare registrar config decodes opaquely
			// via api/plugin_bootstrap.go's wrap-cover. accountId in the
			// yaml is unquoted (12345) → yaml.v3 unmarshal keeps it as int.
			// Credentials is extracted into the typed slot by the
			// wrap-cover decoder.
			Config: api.Config{Config: &api.PluginFieldConfig{
				TypeName:    "cloudflare",
				Credentials: "${secret:CLOUDFLARE_API_TOKEN}",
				Raw: map[string]any{
					"credentials": "${secret:CLOUDFLARE_API_TOKEN}",
					"accountId":   12345,
					"zoneName":    "sc-refapp.org",
					"dnsRecords": []any{
						map[string]any{
							"name":  "@",
							"type":  "TXT",
							"value": "MS=ms83691649",
						},
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
		Config: api.Config{Config: &github.GitHubActionsCiCdConfig{
			Organization: "simple-container-org",
			// Unset stays unset: placeholder resolution no longer rewrites a nil
			// slice/map to an empty-but-present one, so these match the
			// unresolved fixture above field for field.
			Environments: map[string]github.GitHubEnvironmentConfig{
				"staging": {
					Type:        "staging",
					Runner:      "",
					Protection:  false,
					Reviewers:   nil,
					Secrets:     nil,
					Variables:   nil,
					DeployFlags: nil,
					AutoDeploy:  false,
				},
				"production": {
					Type:        "production",
					Runner:      "",
					Protection:  false,
					Reviewers:   nil,
					Secrets:     nil,
					Variables:   nil,
					DeployFlags: nil,
					AutoDeploy:  false,
				},
			},
			Notifications: github.GitHubNotificationConfig{
				Slack: github.SlackNotificationConfig{
					WebhookURL: "",
					Enabled:    false,
				},
				Discord: github.DiscordNotificationConfig{
					WebhookURL: "",
					Enabled:    false,
				},
				Telegram: github.TelegramNotificationConfig{
					BotToken: "",
					ChatID:   "",
					Enabled:  false,
				},
			},
			WorkflowGeneration: github.GitHubWorkflowConfig{
				Enabled:       false,
				OutputPath:    "",
				Templates:     []string{"deploy", "destroy"},
				AutoUpdate:    false,
				CustomActions: map[string]string{},
				SCVersion:     "",
			},
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
			Type: "cloudflare",
			// Post-resolve: ${secret:CLOUDFLARE_API_TOKEN} → <encrypted-secret>.
			Config: api.Config{Config: &api.PluginFieldConfig{
				TypeName:    "cloudflare",
				Credentials: "<encrypted-secret>",
				Raw: map[string]any{
					"credentials": "<encrypted-secret>",
					"accountId":   12345,
					"zoneName":    "sc-refapp.org",
					"dnsRecords": []any{
						map[string]any{
							"name":  "@",
							"type":  "TXT",
							"value": "MS=ms83691649",
						},
					},
				},
			}},
		},
	},
}

// mongodbAtlasResourceConfig is the opaque-decoded form of the
// mongodb-atlas resource block in testdata/stacks/refapp/server.yaml.
// Before api #543's A3 strip, sc-plugin-mongodb/api's init() registered
// a typed ReadAtlasConfig decoder that produced *mongodb.AtlasConfig;
// after the strip, api/plugin_bootstrap.go seeds the wrap-cover opaque
// decoder that produces *api.PluginFieldConfig with the raw yaml map
// in Raw. Nested maps/lists are the yaml.v3 default shapes.
var mongodbAtlasResourceConfig = api.ResourceDescriptor{
	Type: "mongodb-atlas",
	Config: api.Config{Config: &api.PluginFieldConfig{
		TypeName:  "mongodb-atlas",
		ProjectID: "${var:atlas-project-id}",
		Raw: map[string]any{
			"admins":       []any{"smecsia"},
			"developers":   []any{},
			"instanceSize": "${var:atlas-instance-size}",
			"orgId":        "${var:atlas-org-id}",
			"projectId":    "${var:atlas-project-id}",
			"projectName":  "${stack:name}",
			"region":       "${var:atlas-region}",
			"privateKey":   "${secret:MONGODB_ATLAS_PRIVATE_KEY}",
			"publicKey":    "${secret:MONGODB_ATLAS_PUBLIC_KEY}",
			"networkConfig": map[string]any{
				"privateLinkEndpoint": map[string]any{
					"providerName": "AWS",
				},
			},
			"extraProviders": map[string]any{
				"AWS": map[string]any{
					"type": "aws-token",
					"config": map[string]any{
						"credentials": "${auth:aws}",
						"account":     "${auth:aws.projectId}",
					},
				},
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

// resolvedMongodbAtlasResource mirrors mongodbAtlasResourceConfig after
// the placeholder resolver has walked over it (`${var:...}` → fixture
// value, `${secret:...}` → `<encrypted-secret>` sentinel, etc.). The
// shape stays opaque — same *api.PluginFieldConfig wrapper, same Raw
// map structure.
var resolvedMongodbAtlasResource = api.ResourceDescriptor{
	Type: "mongodb-atlas",
	Config: api.Config{Config: &api.PluginFieldConfig{
		TypeName:  "mongodb-atlas",
		ProjectID: "5b89110a4e6581562623c59c",
		Raw: map[string]any{
			"admins":       []any{"smecsia"},
			"developers":   []any{},
			"instanceSize": "M10",
			"orgId":        "5b89110a4e6581562623c59c",
			"projectId":    "5b89110a4e6581562623c59c",
			"projectName":  "refapp",
			"region":       "US_SOUTH_1",
			"privateKey":   "<encrypted-secret>",
			"publicKey":    "<encrypted-secret>",
			"networkConfig": map[string]any{
				"privateLinkEndpoint": map[string]any{
					"providerName": "AWS",
				},
			},
			"extraProviders": map[string]any{
				"AWS": map[string]any{
					"type": "aws-token",
					"config": map[string]any{
						// Pre-resolved aws-token credentials match resolvedAwsAccountConfig.Credentials.Credentials
						// verbatim — Go's json.Marshal HTML-escapes `<` to `<`, so a `<aws-access-key>`
						// placeholder round-trips as `<aws-access-key>` through the secret-reveal
						// path. Keep in lockstep with the typed const at refapp.go:41.
						"credentials": resolvedAwsAccountConfig.Credentials.Credentials,
						"account":     "000",
					},
				},
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
		"docker-registry-password":  "test-pass",
		"docker-registry-username":  "test-user",
	},
}

var RefappClientDescriptor = &api.ClientDescriptor{
	SchemaVersion: api.ClientSchemaVersion,
	Defaults:      map[string]interface{}{},
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
