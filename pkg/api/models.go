package api

import (
	"strings"

	"github.com/samber/lo"
)

type (
	StacksMap      map[string]Stack
	VariableValues map[string]any
)

type Stack struct {
	Name    string            `json:"name" yaml:"name"`
	Secrets SecretsDescriptor `json:"secrets" yaml:"secrets"`
	Server  ServerDescriptor  `json:"server" yaml:"server"`
	Client  ClientDescriptor  `json:"client" yaml:"client"`
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
		if stack.Server.Resources.Registrar.IsInherited() {
			val := current[stackName]
			val.Server.Resources.Registrar = current[stack.Server.Resources.Registrar.Inherit.Inherit].Server.Resources.Registrar
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
			val.Secrets = current[stack.Server.Secrets.Inherit.Inherit].Secrets
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
