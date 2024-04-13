package api

import (
	"context"

	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
	"github.com/simple-container-com/welder/pkg/template"
)

type Collector struct {
	Stack   string               `json:"stackName" yaml:"stackName"`
	Env     string               `json:"environment" yaml:"environment"`
	EnvVars []ComputeEnvVariable `json:"envVariables" yaml:"envVariables"`

	dependencies  []sdk.Resource
	outputs       []sdk.Output
	tplExtensions map[string]template.Extension
	log           logger.Logger
	ctx           context.Context
}

func (c *Collector) ResolvePlaceholders(obj any) error {
	return placeholders.New().Apply(obj, placeholders.WithExtensions(c.tplExtensions))
}

func (c *Collector) AddTplExtensions(m map[string]template.Extension) {
	c.tplExtensions = lo.Assign(c.tplExtensions, m)
}

func (c *Collector) AddOutput(o sdk.Output) {
	c.outputs = append(c.outputs, o)
}

func (c *Collector) Outputs() []sdk.Output {
	return c.outputs
}

func (c *Collector) EnvVariables() []ComputeEnvVariable {
	return c.EnvVars
}

func (c *Collector) AddEnvVariableIfNotExist(name, value, resType, resName, stackName string) {
	if _, found := lo.Find(c.EnvVars, func(v ComputeEnvVariable) bool {
		return v.Name == name
	}); found {
		c.log.Warn(c.ctx, "env variable %q already exists, skipping", name)
		return
	}
	c.EnvVars = append(c.EnvVars, ComputeEnvVariable{
		Name:         name,
		Value:        value,
		ResourceName: resName,
		ResourceType: resType,
		StackName:    stackName,
	})
}

func (c *Collector) AddDependency(res sdk.Resource) {
	c.dependencies = append(c.dependencies, res)
}

func (c *Collector) Dependencies() []sdk.Resource {
	return c.dependencies
}

func NewComputeContextCollector(ctx context.Context, log logger.Logger, stackName string, environment string) ComputeContextCollector {
	return &Collector{
		Stack:         stackName,
		Env:           environment,
		EnvVars:       make([]ComputeEnvVariable, 0),
		tplExtensions: make(map[string]template.Extension),

		log: log,
		ctx: ctx,
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
