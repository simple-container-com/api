package pulumi

import (
	"context"
	"github.com/pulumi/pulumi/pkg/v3/backend"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/pkg/errors"

	"api/pkg/api"
	"api/pkg/api/logger"
)

//go:generate ../../../bin/mockery --name Pulumi --output ./mocks --filename pulumi_mock.go --outpkg pulumi_mocks --structname PulumiMock
type Pulumi interface {
	api.Provisioner
}

type pulumi struct {
	logger logger.Logger
	pubKey string

	initialProvisionProgram func(ctx *sdk.Context) error
	stack                   *auto.Stack
	backend                 backend.Backend
	stackRef                backend.StackReference
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

func (p *pulumi) ProvisionStack(ctx context.Context, cfg *api.ConfigFile, pubKey string, stack api.Stack) error {
	p.pubKey = pubKey // find better way of setting this
	if err := p.createStackIfNotExists(ctx, cfg, stack); err != nil {
		return errors.Wrapf(err, "failed to create stack %q if not exists", stack.Name)
	}
	if err := p.provisionStack(ctx, cfg, stack); err != nil {
		return err
	}
	return nil
}

func (p *pulumi) Provision(ctx context.Context, rd *api.ResourceDescriptor) error {
	// TODO implement
	return nil
}
