package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
	"github.com/simple-container-com/welder/pkg/template"
)

type (
	perResTplValues map[string]map[string]string
	Collector       struct {
		Stack   string               `json:"stackName" yaml:"stackName"`
		Env     string               `json:"environment" yaml:"environment"`
		EnvVars []ComputeEnvVariable `json:"envVariables" yaml:"envVariables"`

		dependencies        []sdk.Resource
		outputs             []sdk.Output
		resTplExtensions    perResTplValues
		dependTplExtensions perResTplValues
		log                 logger.Logger
		ctx                 context.Context
	}
)

func (c *Collector) ResolvePlaceholders(obj any) error {
	return placeholders.New().Apply(obj, placeholders.WithExtensions(map[string]template.Extension{
		"dependency": func(noSubs string, path string, defaultValue *string) (string, error) {
			// e.g. ${dependency:<name>.<resource>.uri}
			pathParts := strings.SplitN(path, ".", 3)
			depName := pathParts[0]
			refResName := pathParts[1]
			refValue := pathParts[2]
			if values, ok := c.dependTplExtensions[fmt.Sprintf("%s.%s", depName, refResName)]; ok {
				if value, ok := values[refValue]; ok {
					return value, nil
				}
			}
			return noSubs, nil
		},
		"resource": func(noSubs string, path string, defaultValue *string) (string, error) {
			// e.g. ${resource:<resource>.uri}
			pathParts := strings.SplitN(path, ".", 2)
			refResName := pathParts[0]
			refValue := pathParts[1]
			if values, ok := c.resTplExtensions[refResName]; ok {
				if value, ok := values[refValue]; ok {
					return value, nil
				}
			}
			return noSubs, nil
		},
	}))
}

func (c *Collector) AddDependencyTplExtension(depName string, resName string, values map[string]string) {
	c.dependTplExtensions[fmt.Sprintf("%s.%s", depName, resName)] = values
}

func (c *Collector) AddResourceTplExtension(resName string, values map[string]string) {
	c.resTplExtensions[resName] = values
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
		c.log.Info(c.ctx, "env variable %q already exists, skipping", name)
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
		Stack:            stackName,
		Env:              environment,
		EnvVars:          make([]ComputeEnvVariable, 0),
		resTplExtensions: make(perResTplValues),

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
