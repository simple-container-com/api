package api

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestReadServerDescriptor(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		path    string
		want    *ServerDescriptor
		wantErr bool
	}{
		{
			path: "testdata/stacks/common/server.yaml",
			want: &ServerDescriptor{
				SchemaVersion: ServerSchemaVersion,
				Provisioner: ProvisionerDescriptor{
					Type: ProvisionerTypePulumi,
					Config: &PulumiProvisionerConfig{
						StateStorage:    PulumiStateStorageConfig{},
						SecretsProvider: PulumiSecretsProviderConfig{},
					},
				},
				CiCd: CiCdDescriptor{
					Type: "github-actions",
					Config: &GithubActionsCiCdConfig{
						AuthToken: "${secret:GITHUB_TOKEN}",
					},
				},
				Secrets: SecretsConfigDescriptor{
					Type: SecretsTypeGCPSecretsManager,
					Config: &GcloudSecretsConfig{
						Credentials: "${auth:gcloud}",
					},
				},
				Templates: map[string]StackDescriptor{
					"stack-per-app": {
						Type: TemplateTypeGcpCloudrun,
						Config: &GcloudTemplateConfig{
							Credentials: "${auth:gcloud}",
						},
					},
				},
				Resources: PerStackResourcesDescriptor{
					Registrar: RegistrarDescriptor{
						Type: RegistrarTypeCloudflare,
						Config: &CloudflareRegistrarConfig{
							Credentials: "${secret:CLOUDFLARE_API_TOKEN}",
							Project:     "sc-refapp",
							ZoneName:    "sc-refapp.org",
							DnsRecords: []CloudflareDnsRecord{
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
			want: &ServerDescriptor{
				SchemaVersion: ServerSchemaVersion,
				Provisioner: ProvisionerDescriptor{
					Inherit: Inherit{Inherit: "common"},
				},
				Secrets: SecretsConfigDescriptor{
					Inherit: Inherit{Inherit: "common"},
				},
				CiCd: CiCdDescriptor{
					Inherit: Inherit{Inherit: "common"},
				},
				Templates: map[string]StackDescriptor{
					"stack-per-app": {
						Inherit: Inherit{Inherit: "common"},
					},
				},
				Variables: map[string]VariableDescriptor{
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
				Resources: PerStackResourcesDescriptor{
					Registrar: RegistrarDescriptor{
						Inherit: Inherit{Inherit: "common"},
					},
					Resources: map[string]PerEnvResourcesDescriptor{
						"staging": {
							Template: "stack-per-app",
							Resources: map[string]ResourceDescriptor{
								"mongodb": {
									Type: "mongodb-atlas",
									Config: &MongodbAtlasConfig{
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
									Config: &PostgresGcpCloudsqlConfig{
										Version:     "14.5",
										Project:     "${stack:name}",
										Credentials: "${auth:gcloud}",
									},
								},
							},
						},
						"prod": {
							Template: "stack-per-app",
							Resources: map[string]ResourceDescriptor{
								"mongodb": {
									Type: "mongodb-atlas",
									Config: &MongodbAtlasConfig{
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
									Config: &PostgresGcpCloudsqlConfig{
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
			got, err := ReadServerDescriptor(tt.path)
			Expect(err).To(BeNil())

			Expect(got).To(Equal(tt.want))
		})
	}
}

func TestReadSecretsDescriptor(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		path    string
		want    *SecretsDescriptor
		wantErr bool
	}{
		{
			path: "testdata/stacks/common/secrets.yaml",
			want: &SecretsDescriptor{
				SchemaVersion: SecretsSchemaVersion,
				Auth: map[string]AuthDescriptor{
					"gcloud": {
						Type: AuthTypeGCPServiceAccount,
						Config: &GcloudAuthServiceAccountConfig{
							Account: "<gcloud-service-account-email>",
						},
					},
					"pulumi": {
						Type: AuthTypePulumiToken,
						Config: &PulumiTokenAuthDescriptor{
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
			got, err := ReadSecretsDescriptor(tt.path)
			Expect(err).To(BeNil())

			Expect(got).To(Equal(tt.want))
		})
	}
}

func TestReadClientDescriptor(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		path    string
		want    *ClientDescriptor
		wantErr bool
	}{
		{
			path: "testdata/stacks/refapp/client.yaml",
			want: &ClientDescriptor{
				SchemaVersion: ClientSchemaVersion,
				Stacks: map[string]StackClientDescriptor{
					"staging": {
						Stack:  "refapp/staging",
						Domain: "staging.sc-refapp.org",
						Config: StackConfig{
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
						Config: StackConfig{
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
			got, err := ReadClientDescriptor(tt.path)
			Expect(err).To(BeNil())

			Expect(got).To(Equal(tt.want))
		})
	}
}
