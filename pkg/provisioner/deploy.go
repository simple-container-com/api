package provisioner

import (
	"context"
	"github.com/pkg/errors"
	"github.com/simple-container-com/api/pkg/api"
	"path"
)

func (p *provisioner) Deploy(ctx context.Context, params api.DeployParams) error {
	stack := api.Stack{
		Name: params.Stack,
	}

	cfg, err := api.ReadConfigFile(params.RootDir, p.profile)
	if err != nil {
		return errors.Wrapf(err, "failed to read config file for profile %q", p.profile)
	}

	descFilePath := path.Join(params.RootDir, params.Stack, api.ClientDescriptorFileName)
	clientDesc, err := p.readClientDescriptor(descFilePath)

	if err != nil {
		return errors.Wrapf(err, "failed to read stacks")
	}
	p.log.Info(ctx, "read client desc from %q", descFilePath)

	stack.Client = *clientDesc
	pv := stack.Server.Provisioner.GetProvisioner()
	if p.overrideProvisioner != nil {
		pv = p.overrideProvisioner
	}
	if pv == nil {
		return errors.Errorf("provisioner is not set for stack %q", stack.Name)
	}
	var pubKey string
	if p.cryptor != nil {
		pubKey = p.cryptor.PublicKey()
	} else {
		p.log.Warn(ctx, "Cryptor is not set, secrets will not be encrypted")
	}
	envDesc, ok := clientDesc.Stacks[params.Environment]
	if !ok {
		return errors.Errorf("environment %q for stack %q is not configured", params.Environment, stack.Name)
	}
	return pv.DeployStack(ctx, cfg, pubKey, envDesc)
}
