package provisioner

import (
	"api/pkg/provisioner/models"
	"context"
	"github.com/pkg/errors"
	"os"
	"path"

	"api/pkg/api"
)

func (p *provisioner) Provision(ctx context.Context, params ProvisionParams) error {
	err := p.ReadStacks(ctx, params)
	if err != nil {
		return err
	}

	return nil
}

func (p *provisioner) ReadStacks(ctx context.Context, params ProvisionParams) error {
	for _, stackName := range params.Stacks {
		stack := models.Stack{
			Name: stackName,
		}

		if serverDesc, err := p.readServerDescriptor(params.RootDir, stackName); err != nil {
			return err
		} else {
			p.log.Debug(ctx, "Successfully read server descriptor: %q", serverDesc)
			stack.Server = *serverDesc
		}

		if clientDesc, err := p.optionallyReadClientDescriptor(params.RootDir, stackName); err != nil {
			return err
		} else if clientDesc != nil {
			p.log.Debug(ctx, "Successfully read client descriptor: %q", clientDesc)
			stack.Client = *clientDesc
		} else {
			p.log.Debug(ctx, "Secrets descriptor not found for %s", stackName)
		}

		if secretsDesc, err := p.optionallyReadSecretsDescriptor(params.RootDir, stackName); err != nil {
			return err
		} else if secretsDesc != nil {
			p.log.Debug(ctx, "Successfully read secrets descriptor: %q", secretsDesc)
			stack.Secrets = *secretsDesc
		} else {
			p.log.Debug(ctx, "Secrets descriptor not found for %s", stackName)
		}

		p.stacks[stackName] = stack
	}
	return nil
}

func (p *provisioner) readServerDescriptor(rootDir string, stackName string) (*api.ServerDescriptor, error) {
	descFilePath := path.Join(rootDir, stackName, api.ServerDescriptorFileName)
	if desc, err := api.ReadServerDescriptor(descFilePath); err != nil {
		return nil, errors.Wrapf(err, "failed to read server descriptor from %q", descFilePath)
	} else {
		return desc, nil
	}
}

func (p *provisioner) optionallyReadSecretsDescriptor(rootDir string, stackName string) (*api.SecretsDescriptor, error) {
	descFilePath := path.Join(rootDir, stackName, api.SecretsDescriptorFileName)
	if _, err := os.Stat(descFilePath); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return p.readSecretsDescriptor(descFilePath)
}

func (p *provisioner) readSecretsDescriptor(descFilePath string) (*api.SecretsDescriptor, error) {
	if desc, err := api.ReadSecretsDescriptor(descFilePath); err != nil {
		return nil, errors.Wrapf(err, "failed to read client descriptor from %q", descFilePath)
	} else {
		return desc, nil
	}
}

func (p *provisioner) optionallyReadClientDescriptor(rootDir string, stackName string) (*api.ClientDescriptor, error) {
	descFilePath := path.Join(rootDir, stackName, api.ClientDescriptorFileName)
	if _, err := os.Stat(descFilePath); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return p.readClientDescriptor(descFilePath)
}

func (p *provisioner) readClientDescriptor(path string) (*api.ClientDescriptor, error) {
	if desc, err := api.ReadClientDescriptor(path); err != nil {
		return nil, errors.Wrapf(err, "failed to read client descriptor from %q", path)
	} else {
		return desc, nil
	}
}
