package api

import (
	"github.com/samber/lo"
	"github.com/simple-container-com/api/pkg/api"
)

type Collector struct {
	Stack   string            `json:"stackName" yaml:"stackName"`
	Env     string            `json:"environment" yaml:"environment"`
	EnvVars map[string]string `json:"envVariables" yaml:"envVariables"`
}

func (c *Collector) EnvVariables() map[string]string {
	return c.EnvVars
}

func (c *Collector) AddEnvVariable(name string, value string) {
	c.EnvVars = lo.Assign(c.EnvVars, map[string]string{
		name: value,
	})
}

func NewComputeContextCollector(stackName string, environment string) api.ComputeContextCollector {
	return &Collector{
		Stack:   stackName,
		Env:     environment,
		EnvVars: make(map[string]string),
	}
}
