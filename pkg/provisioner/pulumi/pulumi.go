package pulumi

import (
	"context"

	"api/pkg/api"

	"api/pkg/provisioner/logger"
	"api/pkg/provisioner/models"
)

//go:generate ../../../bin/mockery --name Pulumi --output ./mocks --filename pulumi_mock.go --outpkg pulumi_mocks --structname PulumiMock
type Pulumi interface {
	CreateStacks(ctx context.Context, cfg *api.ConfigFile, stacks models.StacksMap) error
}

type pulumi struct {
	logger logger.Logger
}

type Option func(p *pulumi) error

func New(opts ...Option) (Pulumi, error) {
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

func (p pulumi) CreateStacks(ctx context.Context, cfg *api.ConfigFile, stacks models.StacksMap) error {
	// TODO implement me
	panic("implement me")
}
