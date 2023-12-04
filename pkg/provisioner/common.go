package provisioner

import (
	"api/pkg/api"
	"api/pkg/provisioner/git"
	"api/pkg/provisioner/logger"
	"context"
)

type Provisioner interface {
	Init() error

	Provision(ctx context.Context, params ProvisionParams) error

	Deploy(ctx context.Context, params DeployParams) error

	Stacks() StacksMap
}

type StacksMap map[string]Stack
type provisioner struct {
	stacks StacksMap

	gitRepo git.Repo
	log     logger.Logger
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

type Option func(p *provisioner) error

func New(opts ...Option) (Provisioner, error) {
	res := &provisioner{
		stacks: make(StacksMap),
		log:    logger.New(),
	}

	for _, opt := range opts {
		if err := opt(res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (p *provisioner) Stacks() StacksMap {
	return p.stacks
}

func (p *provisioner) Init() error {
	//TODO implement me
	panic("implement me")
}
