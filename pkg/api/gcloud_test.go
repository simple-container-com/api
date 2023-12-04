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
					Type:    SecretsTypeGCPSecretsManager,
					Inherit: "",
					Config:  nil,
				},
				Templates: map[string]StackDescriptor{
					"stack-per-app": {
						Type:    TemplateTypeGcpCloudrun,
						Inherit: "",
						Config: &GcloudTemplateConfig{
							Credentials: "",
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
				Variables: nil,
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
