package tests

import (
	"api/pkg/api"
	"api/pkg/api/clouds/cloudflare"
	"api/pkg/api/clouds/gcloud"
	"api/pkg/api/clouds/github"
	"api/pkg/api/clouds/mongodb"
	"api/pkg/api/clouds/pulumi"
	"testing"

	. "github.com/onsi/gomega"
)

func TestReadServerDescriptor(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		path    string
		want    *api.ServerDescriptor
		wantErr bool
	}{
		{
			path: "testdata/stacks/common/server.yaml",
			want: &api.ServerDescriptor{
				SchemaVersion: api.ServerSchemaVersion,
				Provisioner: api.ProvisionerDescriptor{
					Type: pulumi.ProvisionerTypePulumi,
					Config: &pulumi.PulumiProvisionerConfig{
						StateStorage:    pulumi.PulumiStateStorageConfig{},
						SecretsProvider: pulumi.PulumiSecretsProviderConfig{},
					},
				},
				CiCd: api.CiCdDescriptor{
					Type: "github-actions",
					Config: &github.GithubActionsCiCdConfig{
						AuthToken: "${secret:GITHUB_TOKEN}",
					},
				},
				Secrets: api.SecretsConfigDescriptor{
					Type: gcloud.SecretsTypeGCPSecretsManager,
					Config: &gcloud.GcloudSecretsConfig{
						Credentials: "${auth:gcloud}",
					},
				},
				Templates: map[string]api.StackDescriptor{
					"stack-per-app": {
						Type: gcloud.TemplateTypeGcpCloudrun,
						Config: &gcloud.GcloudTemplateConfig{
							Credentials: "${auth:gcloud}",
						},
					},
				},
				Resources: api.PerStackResourcesDescriptor{
					Registrar: api.RegistrarDescriptor{
						Type: cloudflare.RegistrarTypeCloudflare,
						Config: &cloudflare.CloudflareRegistrarConfig{
							Credentials: "${secret:CLOUDFLARE_API_TOKEN}",
							Project:     "sc-refapp",
							ZoneName:    "sc-refapp.org",
							DnsRecords: []cloudflare.CloudflareDnsRecord{
								{
									Name:  "@",
									Type:  "TXT",
									Value: "MS=ms83691649",
								},
							},
						},
					},
				},
			},
		},
		{
			path: "testdata/stacks/refapp/server.yaml",
			want: &api.ServerDescriptor{
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
									Config: &mongodb.MongodbAtlasConfig{
										Admins:       []string{"smecsia"},
										Developers:   []string{},
										InstanceSize: "${var:atlas-instance-size}",
										OrgId:        "${var:atlas-org-id}",
										ProjectId:    "${var:atlas-project-id}",
										ProjectName:  "${stack:name}",
										Region:       "${var:atlas-region}",
										PrivateKey:   "${secret:MONGODB_ATLAS_PRIVATE_KEY}",
										PublicKey:    "${secret:MONGODB_ATLAS_PUBLIC_KEY}",
									},
								},
								"postgres": {
									Type: "gcp-cloudsql-postgres",
									Config: &gcloud.PostgresGcpCloudsqlConfig{
										Version:     "14.5",
										Project:     "${stack:name}",
										Credentials: "${auth:gcloud}",
									},
								},
							},
						},
						"prod": {
							Template: "stack-per-app",
							Resources: map[string]api.ResourceDescriptor{
								"mongodb": {
									Type: "mongodb-atlas",
									Config: &mongodb.MongodbAtlasConfig{
										Admins:       []string{"smecsia"},
										Developers:   []string{},
										InstanceSize: "${var:atlas-instance-size}",
										OrgId:        "${var:atlas-org-id}",
										ProjectId:    "${var:atlas-project-id}",
										ProjectName:  "${stack:name}",
										Region:       "${var:atlas-region}",
										PrivateKey:   "${secret:MONGODB_ATLAS_PRIVATE_KEY}",
										PublicKey:    "${secret:MONGODB_ATLAS_PUBLIC_KEY}",
									},
								},
								"postgres": {
									Type: "gcp-cloudsql-postgres",
									Config: &gcloud.PostgresGcpCloudsqlConfig{
										Version:     "14.5",
										Project:     "${stack:name}",
										Credentials: "${auth:gcloud}",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := api.ReadServerDescriptor(tt.path)
			Expect(err).To(BeNil())

			Expect(got).To(Equal(tt.want))
		})
	}
}

func TestReadSecretsDescriptor(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		path    string
		want    *api.SecretsDescriptor
		wantErr bool
	}{
		{
			path: "testdata/stacks/common/secrets.yaml",
			want: &api.SecretsDescriptor{
				SchemaVersion: api.SecretsSchemaVersion,
				Auth: map[string]api.AuthDescriptor{
					"gcloud": {
						Type: gcloud.AuthTypeGCPServiceAccount,
						Config: &gcloud.GcloudAuthServiceAccountConfig{
							Account: "<gcloud-service-account-email>",
						},
					},
					"pulumi": {
						Type: pulumi.AuthTypePulumiToken,
						Config: &pulumi.PulumiTokenAuthDescriptor{
							Value: "<pulumi-token>",
						},
					},
				},
				Values: map[string]string{
					"CLOUDFLARE_API_TOKEN":      "<encrypted-secret>",
					"GITHUB_TOKEN":              "<encrypted-secret>",
					"MONGODB_ATLAS_PRIVATE_KEY": "<encrypted-secret>",
					"MONGODB_ATLAS_PUBLIC_KEY":  "<encrypted-secret>",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := api.ReadSecretsDescriptor(tt.path)
			Expect(err).To(BeNil())

			Expect(got).To(Equal(tt.want))
		})
	}
}

func TestReadClientDescriptor(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		path    string
		want    *api.ClientDescriptor
		wantErr bool
	}{
		{
			path: "testdata/stacks/refapp/client.yaml",
			want: &api.ClientDescriptor{
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
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := api.ReadClientDescriptor(tt.path)
			Expect(err).To(BeNil())

			Expect(got).To(Equal(tt.want))
		})
	}
}
