package api

import (
	"encoding/json"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"
)

type OutputsCollector interface {
	StackName() string
	Environment() string
	ExportEnvVariable(name string, value pulumi.StringOutput)
	EnvVariables() map[string]string
	ToJson() string
}

type Collector struct {
	Stack   string            `json:"stackName" yaml:"stackName"`
	Env     string            `json:"environment" yaml:"environment"`
	EnvVars map[string]string `json:"envVariables" yaml:"envVariables"`
}

func (c *Collector) ToJson() string {
	res, _ := json.Marshal(c)
	return string(res)
}

func CollectorFromJson(value string) OutputsCollector {
	res := Collector{}
	_ = json.Unmarshal([]byte(value), &res)
	return &res
}

func (c *Collector) EnvVariables() map[string]string {
	return c.EnvVars
}

func (c *Collector) StackName() string {
	return c.Stack
}

func (c *Collector) Environment() string {
	return c.Env
}

func (c *Collector) ExportEnvVariable(name string, valueOut pulumi.StringOutput) {
	valueOut.ApplyT(func(value string) (any, error) {
		c.EnvVars = lo.Assign(c.EnvVars, map[string]string{
			name: value,
		})
		return nil, nil
	})
}

func NewCollector(stackName string, environment string) OutputsCollector {
	return &Collector{
		Stack:   stackName,
		Env:     environment,
		EnvVars: make(map[string]string),
	}
}
