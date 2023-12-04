package tests

import (
	"api/pkg/api"
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
			want: CommonServerDescriptor,
		},
		{
			path: "testdata/stacks/refapp/server.yaml",
			want: RefappServerDescriptor,
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
			want: CommonSecretsDescriptor,
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
			want: RefappClientDescriptor,
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
