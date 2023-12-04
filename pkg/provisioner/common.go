package provisioner

import (
	"api/pkg/api"
	"api/pkg/provisioner/logger"
	"context"
)

type Provisioner interface {
	Provision(ctx context.Context, params ProvisionParams) error

	Deploy(ctx context.Context, params DeployParams) error

	Stacks() StacksMap
}

type StacksMap map[string]Stack
type provisioner struct {
	stacks StacksMap
	log    logger.Logger
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
	return &provisioner{
		stacks: make(StacksMap),
		log:    logger.New(),
	}
}

func (p *provisioner) Stacks() StacksMap {
	return p.stacks
}
