package api

import (
	"strings"

	"github.com/pkg/errors"
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

type ReadOpts struct {
	IgnoreServerMissing  bool
	IgnoreClientMissing  bool
	IgnoreSecretsMissing bool
	RequireClientConfigs []string
	RequireServerConfigs []string
	RequireSecretConfigs []string
}

var (
	ReadIgnoreNoClientCfg           = ReadOpts{IgnoreClientMissing: true}
	ReadIgnoreNoServerCfg           = ReadOpts{IgnoreServerMissing: true, IgnoreSecretsMissing: true}
	ReadIgnoreNoSecretsAndClientCfg = ReadOpts{IgnoreSecretsMissing: true, IgnoreClientMissing: true}
	ReadIgnoreNoSecretsAndServerCfg = ReadOpts{IgnoreSecretsMissing: true, IgnoreServerMissing: true}
	ReadIgnoreNoAnyCfg              = ReadOpts{IgnoreSecretsMissing: true, IgnoreServerMissing: true, IgnoreClientMissing: true}
)

func (m *StacksMap) ReconcileForDeploy(params StackParams) (*StacksMap, error) {
	current := *m
	iterMap := lo.Assign(current)
	for stackName, stack := range iterMap {
		if len(stack.Client.Stacks) == 0 {
			// skip server-only stack
			continue
		}
		clientDesc, ok := stack.Client.Stacks[params.Environment]
		if !ok && stackName != params.StackName {
			// skip non-target stacks if they are not configured for env
			continue
		}
		if !ok {
			return nil, errors.Errorf("client stack %q is not configured for %q", stackName, params.Environment)
		}
		parentStackParts := strings.SplitN(clientDesc.ParentStack, "/", 3)
		parentStackName := parentStackParts[len(parentStackParts)-1]
		if parentStack, ok := current[parentStackName]; ok {
			stack.Server = parentStack.Server.Copy()
			stack.Secrets = parentStack.Secrets.Copy()

			// Apply environment-specific secret filtering for child stacks
			if stack.Server.Secrets.SecretsConfig != nil {
				resolver := NewSecretResolver(stack.Server.Secrets.SecretsConfig)
				env := params.Environment
				if clientDesc.ParentEnv != "" {
					env = clientDesc.ParentEnv
				}
				filteredSecrets, err := resolver.ResolveSecrets(stack.Secrets.Values, env)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to resolve secrets for stack %q in environment %q", stackName, env)
				}
				stack.Secrets.Values = filteredSecrets
			}
		} else {
			return nil, errors.Errorf("parent stack %q is not configured for %q in %q", clientDesc.ParentStack, stackName, params.Environment)
		}
		current[stackName] = stack
	}
	return &current, nil
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

func (s *Stack) ValuesOnly() Stack {
	return Stack{
		Name:    s.Name,
		Secrets: s.Secrets.Copy(),
		Server:  *s.Server.ValuesOnly(),
		Client:  s.Client.Copy(),
	}
}
