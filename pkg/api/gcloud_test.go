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
					Type:    "",
					Inherit: "",
					Config:  nil,
				},
				Templates: nil,
				Resources: nil,
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
