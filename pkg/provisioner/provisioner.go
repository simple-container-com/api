package provisioner

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"path"

	"api/pkg/api"
)

type Provisioner interface {
	Provision(ctx context.Context, params ProvisionParams) error

	Deploy(ctx context.Context, params DeployParams) error
}

type stacksMap map[string]Stack
type provisioner struct {
	Stacks stacksMap `json:"stacks" yaml:"stacks"`
}

type ProvisionParams struct {
	RootDir string   `json:"rootDir" yaml:"rootDir"`
	Stacks  []string `json:"stacks" yaml:"stacks"`
}

type DeployParams struct {
	Stack       string         `json:"stack" yaml:"stack"`
	Environment string         `json:"environment" yaml:"environment"`
	Vars        VariableValues `json:"vars" yaml:"vars"`
}

type VariableValues map[string]any

type Stack struct {
	Name    string                `json:"name" yaml:"name"`
	Secrets api.SecretsDescriptor `json:"secrets" yaml:"secrets"`
	Server  api.ServerDescriptor  `json:"server" yaml:"server"`
	Client  api.ClientDescriptor  `json:"client" yaml:"client"`
}

func New() Provisioner {
	return &provisioner{}
}

func (p *provisioner) Provision(ctx context.Context, params ProvisionParams) error {
	p.Stacks = make(stacksMap)
	for _, stackName := range params.Stacks {
		stack := Stack{}
		p.Stacks[stackName] = stack

		if serverDesc, err := p.readServerDescriptor(params.RootDir, stackName); err != nil {
			return err
		} else {
			fmt.Println("Successfully read server descriptor:\n", serverDesc)
			stack.Server = *serverDesc
		}

		if clientDesc, err := p.readClientDescriptor(params.RootDir, stackName); err != nil {
			return err
		} else {
			fmt.Println("Successfully read client descriptor:\n", clientDesc)
			stack.Client = *clientDesc
		}

		if secretsDesc, err := p.readSecretsDescriptor(params.RootDir, stackName); err != nil {
			return err
		} else {
			fmt.Println("Successfully read secrets descriptor:\n", secretsDesc)
			stack.Secrets = *secretsDesc
		}
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

func (p *provisioner) readSecretsDescriptor(rootDir string, stackName string) (*api.SecretsDescriptor, error) {
	descFilePath := path.Join(rootDir, stackName, api.ClientDescriptorFileName)
	if desc, err := api.ReadSecretsDescriptor(descFilePath); err != nil {
		return nil, errors.Wrapf(err, "failed to read client descriptor from %q", descFilePath)
	} else {
		return desc, nil
	}
}

func (p *provisioner) readClientDescriptor(rootDir string, stackName string) (*api.ClientDescriptor, error) {
	descFilePath := path.Join(rootDir, stackName, api.ClientDescriptorFileName)
	if desc, err := api.ReadClientDescriptor(descFilePath); err != nil {
		return nil, errors.Wrapf(err, "failed to read client descriptor from %q", descFilePath)
	} else {
		return desc, nil
	}
}

func (p *provisioner) Deploy(ctx context.Context, params DeployParams) error {
	return nil
}
