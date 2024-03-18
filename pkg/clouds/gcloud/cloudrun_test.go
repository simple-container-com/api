package gcloud

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

func TestToCloudRunConfig(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name     string
		tpl      any
		stackCfg *api.StackConfigCompose
		check    func(t *testing.T, got any, err error)
	}{
		{
			name:     "happy path",
			tpl:      &TemplateConfig{},
			stackCfg: &api.StackConfigCompose{},
			check: func(t *testing.T, got any, err error) {
				Expect(err).To(BeNil())
				_, ok := got.(*CloudRunInput)
				Expect(ok).To(BeTrue())
				_, ok = got.(api.AuthConfig)
				Expect(ok).To(BeTrue())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			cfg, err := compose.ReadDockerCompose(ctx, "", "testdata/stacks/refapp/docker-compose.yaml")
			Expect(err).To(BeNil())
			Expect(cfg.Project).NotTo(BeNil())

			got, err := ToCloudRunConfig(tt.tpl, cfg, tt.stackCfg)

			if tt.check != nil {
				tt.check(t, got, err)
			}
		})
	}
}
