package provisioner

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	"api/pkg/api/tests"
)

func Test_Provision(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		name         string
		params       ProvisionParams
		opts         []Option
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
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			p, err := New(tt.opts...)

			if err != nil && tt.wantErr != "" {
				Expect(err).To(MatchRegexp(tt.wantErr))
			} else {
				Expect(err).To(BeNil())
			}

			err = p.Provision(ctx, tt.params)

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
