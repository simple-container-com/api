package api

import (
	. "github.com/onsi/gomega"
	"testing"
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
				SchemaVersion: "1.0",
				Provisioner: ProvisionerDescriptor{
					Type: ProvisionerTypePulumi,
					Config: &PulumiProvisionerConfig{
						StateStorage:    PulumiStateStorageConfig{},
						SecretsProvider: PulumiSecretsProviderConfig{},
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
				SchemaVersion: "1.0",
				Provisioner: ProvisionerDescriptor{
					Inherit: Inherit{Inherit: "common"},
				},
				Secrets: SecretsConfigDescriptor{
					Inherit: Inherit{Inherit: "common"},
				},
				CiCd: CiCdDescriptor{
					Type: "github-actions",
					Config: &GithubActionsCiCdConfig{
						AuthToken: "${secret:GITHUB_TOKEN}",
					},
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
