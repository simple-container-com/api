package pulumi

import (
	"context"

	"api/pkg/api"
	"api/pkg/provisioner/logger"
)

//go:generate ../../../bin/mockery --name Pulumi --output ./mocks --filename pulumi_mock.go --outpkg pulumi_mocks --structname PulumiMock
type Pulumi interface {
	api.Provisioner
}

type pulumi struct {
	logger logger.Logger
}

func InitPulumiProvisioner(opts ...api.ProvisionerOption) (api.Provisioner, error) {
	res := &pulumi{
		logger: logger.New(),
	}
	for _, opt := range opts {
		if err := opt(res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (p *pulumi) CreateStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack) error {
	if err := p.createProject(ctx, cfg, stack); err != nil {
		return err
	}
	return nil
}
