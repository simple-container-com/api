package provisioner

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
)

func Test_Provision(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		params  ProvisionParams
		wantErr string
	}{
		{
			name:   "happy path",
			params: ProvisionParams{},
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
			}
		})
	}
}
