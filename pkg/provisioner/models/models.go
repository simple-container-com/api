package models

import "api/pkg/api"

type (
	StacksMap      map[string]Stack
	VariableValues map[string]any
)

type Stack struct {
	Name    string                `json:"name" yaml:"name"`
	Secrets api.SecretsDescriptor `json:"secrets" yaml:"secrets"`
	Server  api.ServerDescriptor  `json:"server" yaml:"server"`
	Client  api.ClientDescriptor  `json:"client" yaml:"client"`
}
