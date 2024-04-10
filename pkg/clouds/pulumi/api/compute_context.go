package api

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"
)

type Collector struct {
	Stack   string            `json:"stackName" yaml:"stackName"`
	Env     string            `json:"environment" yaml:"environment"`
	EnvVars map[string]string `json:"envVariables" yaml:"envVariables"`

	dependencies []sdk.Resource
	outputs      []sdk.Output
}

func (c *Collector) AddOutputs(o sdk.Output) {
	c.outputs = append(c.outputs, o)
}

func (c *Collector) Outputs() []sdk.Output {
	return c.outputs
}

func (c *Collector) EnvVariables() map[string]string {
	return c.EnvVars
}

func (c *Collector) AddEnvVariable(name string, value string) {
	c.EnvVars = lo.Assign(c.EnvVars, map[string]string{
		name: value,
	})
}

func (c *Collector) AddDependency(res sdk.Resource) {
	c.dependencies = append(c.dependencies, res)
}

func (c *Collector) Dependencies() []sdk.Resource {
	return c.dependencies
}

func NewComputeContextCollector(stackName string, environment string) ComputeContextCollector {
	return &Collector{
		Stack:   stackName,
		Env:     environment,
		EnvVars: make(map[string]string),
	}
}
