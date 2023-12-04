package provisioner

import (
	"api/pkg/api/tests"
	"context"
	"testing"

	. "github.com/onsi/gomega"
)

func Test_Provision(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name         string
		params       ProvisionParams
		expectStacks StacksMap
		wantErr      string
	}{
		{
			name: "happy path",
			params: ProvisionParams{
				RootDir: "testdata/stacks",
				Stacks: []string{
					"common",
					"refapp",
				},
			},
			expectStacks: map[string]Stack{
				"common": {
					Name:    "common",
					Secrets: *tests.CommonSecretsDescriptor,
					Server:  *tests.CommonServerDescriptor,
				},
				"refapp": {
					Name:   "refapp",
					Server: *tests.RefappServerDescriptor,
					Client: *tests.RefappClientDescriptor,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			p := New()

			err := p.Provision(ctx, tt.params)

			if err != nil && tt.wantErr != "" {
				Expect(err).To(MatchRegexp(tt.wantErr))
			} else {
				Expect(err).To(BeNil())
				if tt.expectStacks != nil {
					Expect(p.Stacks()).To(Equal(tt.expectStacks))
				}
			}
		})
	}
}
