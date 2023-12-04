package provisioner

import (
	"context"

	"api/pkg/api"
)

type Provisioner interface {
	Provision(ctx context.Context, params ProvisionParams) error

	Deploy(ctx context.Context, params DeployParams) error
}

type provisioner struct {
	Stacks map[string]Stack `json:"stacks" yaml:"stacks"`
}

type ProvisionParams struct {
}

type DeployParams struct {
}

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
	return nil
}

func (p *provisioner) Deploy(ctx context.Context, params DeployParams) error {
	return nil
}
