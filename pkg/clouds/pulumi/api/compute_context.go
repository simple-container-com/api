package api

import (
	"github.com/pkg/errors"
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

func (c *Collector) AddOutput(o sdk.Output) {
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

func GetParentOutput(ref *sdk.StackReference, outName string, parentRefString string, secret bool) (string, error) {
	parentOutput, err := ref.GetOutputDetails(outName)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get output %q from %q", outName, parentRefString)
	}
	value := parentOutput.Value
	if secret {
		value = parentOutput.SecretValue
	}
	if value == nil {
		return "", errors.Wrapf(err, "no secret value for output %q from %q", outName, parentRefString)
	}
	if s, ok := value.(string); ok {
		return s, nil
	} else {
		return "", errors.Wrapf(err, "parent output %q is not of type string (%T)", s, value)
	}
}
