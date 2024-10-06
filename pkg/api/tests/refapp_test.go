package tests

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"

	"github.com/simple-container-com/api/pkg/api"
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
			want: CommonServerDescriptor,
		},
		{
			path: "testdata/stacks/refapp/server.yaml",
			want: RefappServerDescriptor,
		},
		{
			path: "testdata/stacks/refapp-aws/server.yaml",
			want: RefappAwsServerDescriptor,
		},
		{
			path: "testdata/stacks/refapp-aws-lambda/server.yaml",
			want: RefappAwsLambdaServerDescriptor,
		},
		{
			path: "testdata/stacks/refapp-gke-autopilot/server.yaml",
			want: RefappGkeAutopilotServerDescriptor,
		},
		{
			path: "testdata/stacks/refapp-yc-cloud-function/server.yaml",
			want: RefappYandexCloudFunctionServerDescriptor,
		},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := api.ReadServerDescriptor(tt.path)
			Expect(err).To(BeNil())
			actual := got.ValuesOnly()

			assert.EqualValuesf(t, tt.want.CiCd, actual.CiCd, "%v cicd failed", tt.path)
			assert.EqualValuesf(t, tt.want.Provisioner, actual.Provisioner, "%v provisioner failed", tt.path)
			assert.EqualValuesf(t, tt.want.Secrets, actual.Secrets, "%v server secrets failed", tt.path)
			assert.EqualValuesf(t, tt.want.Templates, actual.Templates, "%v server templates failed", tt.path)
			assert.EqualValuesf(t, tt.want.Variables, actual.Variables, "%v server variables failed", tt.path)
			assert.EqualValuesf(t, tt.want.Resources.Registrar, actual.Resources.Registrar, "%v registrar failed", tt.path)
			for env := range tt.want.Resources.Resources {
				assert.EqualValuesf(t, tt.want.Resources.Resources[env], actual.Resources.Resources[env], "%v/%v env resources failed", tt.path, env)
			}
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
			want: CommonSecretsDescriptor,
		},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := api.ReadSecretsDescriptor(tt.path)
			Expect(err).To(BeNil())

			assert.EqualValuesf(t, tt.want, got, "%v failed", tt.path)
			// Expect(got).To(Equal(tt.want))
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
			want: RefappClientDescriptor,
		},
		{
			path: "testdata/stacks/refapp-aws-lambda/client.yaml",
			want: RefappAwsLambdaClientDescriptor,
		},
		{
			path: "testdata/stacks/refapp-gke-autopilot/client.yaml",
			want: RefappGkeAutopilotClientDescriptor,
		},
		{
			path: "testdata/stacks/refapp-yc-cloud-function/client.yaml",
			want: RefappYandexCloudFunctionClientDescriptor,
		},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := api.ReadClientDescriptor(tt.path)
			Expect(err).To(BeNil())

			assert.EqualValuesf(t, tt.want.Copy(), got.Copy(), "%v failed", tt.path)
		})
	}
}
