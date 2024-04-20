package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := api.ReadServerDescriptor(tt.path)
			Expect(err).To(BeNil())

			assert.EqualValuesf(t, tt.want, got.ValuesOnly(), "%v failed", tt.path)
			// Expect(got.ValuesOnly()).To(Equal(tt.want))
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
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := api.ReadClientDescriptor(tt.path)
			Expect(err).To(BeNil())

			assert.EqualValuesf(t, tt.want.Copy(), got.Copy(), "%v failed", tt.path)
		})
	}
}
