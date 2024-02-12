package compose

import (
	"context"
	"testing"

	"github.com/compose-spec/compose-go/types"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
)

func TestComposeLoad(t *testing.T) {
	RegisterTestingT(t)

	cfg, err := ReadDockerCompose(context.Background(), "", "testdata/stacks/refapp/docker-compose.yaml")
	Expect(err).To(BeNil())
	Expect(cfg.Project).NotTo(BeNil())

	Expect(cfg.Project.Services).To(HaveLen(3))
	api, apiFound := lo.Find(cfg.Project.Services, func(svc types.ServiceConfig) bool {
		return svc.Name == "api"
	})
	Expect(apiFound).To(BeTrue())
	Expect(api.ContainerName).To(Equal("refapp-api"))
	ui, uiFound := lo.Find(cfg.Project.Services, func(svc types.ServiceConfig) bool {
		return svc.Name == "ui"
	})
	Expect(uiFound).To(BeTrue())
	Expect(ui.ContainerName).To(Equal("refapp-ui"))
}
