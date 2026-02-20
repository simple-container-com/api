package tests

import (
	"testing"

	. "github.com/onsi/gomega"

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
			path: "testdata/stacks/refapp-kubernetes/server.yaml",
			want: RefappKubernetesServerDescriptor,
		},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := api.ReadServerDescriptor(tt.path)
			Expect(err).To(BeNil())
			actual := got.ValuesOnly()

			Expect(actual.CiCd).To(Equal(tt.want.CiCd), "%v cicd failed", tt.path)
			Expect(actual.Provisioner).To(Equal(tt.want.Provisioner), "%v provisioner failed", tt.path)
			Expect(actual.Secrets).To(Equal(tt.want.Secrets), "%v server secrets failed", tt.path)
			Expect(actual.Templates).To(Equal(tt.want.Templates), "%v server templates failed", tt.path)
			Expect(actual.Variables).To(Equal(tt.want.Variables), "%v server variables failed", tt.path)
			Expect(actual.Resources.Registrar).To(Equal(tt.want.Resources.Registrar), "%v registrar failed", tt.path)
			for env := range tt.want.Resources.Resources {
				Expect(actual.Resources.Resources[env]).To(Equal(tt.want.Resources.Resources[env]), "%v/%v env resources failed", tt.path, env)
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

			Expect(got).To(Equal(tt.want), "%v failed", tt.path)
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
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := api.ReadClientDescriptor(tt.path)
			Expect(err).To(BeNil())

			Expect(got.Copy()).To(Equal(tt.want.Copy()), "%v failed", tt.path)
		})
	}
}
