package models

import (
	"api/pkg/api"
	"github.com/samber/lo"
	"strings"
)

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

func (m *StacksMap) ResolveInheritance() *StacksMap {
	current := *m
	iterMap := lo.Assign(current)
	for stackName, stack := range iterMap {
		if stack.Server.Provisioner.IsInherited() {
			val := current[stackName]
			val.Server.Provisioner = current[stack.Server.Provisioner.Inherit.Inherit].Server.Provisioner
			current[stackName] = val
		}
		if stack.Server.Resources.IsInherited() {
			val := current[stackName]
			val.Server.Resources = current[stack.Server.Resources.Inherit.Inherit].Server.Resources
			current[stackName] = val
		}
		if stack.Server.CiCd.IsInherited() {
			val := current[stackName]
			val.Server.CiCd = current[stack.Server.CiCd.Inherit.Inherit].Server.CiCd
			current[stackName] = val
		}
		if stack.Server.Secrets.IsInherited() {
			val := current[stackName]
			val.Server.Secrets = current[stack.Server.Secrets.Inherit.Inherit].Server.Secrets
			current[stackName] = val
		}
		for tplName, tpl := range lo.Assign(stack.Server.Templates) {
			if tpl.IsInherited() {
				parts := strings.SplitN(tpl.Inherit.Inherit, "/", 2)
				var refStack, refTplName string
				if len(parts) > 1 {
					refStack, refTplName = parts[0], parts[1]
				} else {
					refStack, refTplName = parts[0], tplName
				}
				stack.Server.Templates[tplName] = current[refStack].Server.Templates[refTplName]
			}
		}
	}
	return &current
}
