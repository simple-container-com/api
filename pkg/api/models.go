// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

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

	// PluginTypesNotNeeded lets a read try the cheap server-descriptor parse
	// that does NOT pre-spawn every declared plugin to discover its types.
	//
	// Only for callers that read a stack and then neither provision, deploy,
	// nor convert a stack config: the cheap parse does not record plugin specs
	// for the ConvertStackConfig dialler, so a conversion afterwards would fail
	// with "no plugin spec recorded". `sc stack secret-get` is the model case —
	// it reads one value out of secrets.yaml and exits.
	//
	// It is a HINT, not a promise. The read falls back to the full
	// plugin-aware parse whenever the cheap one fails, so a server.yaml whose
	// types only a plugin's Schema can describe still works; it just costs
	// what it costs today. Nothing becomes unreadable by setting this.
	//
	// Why it matters (measured 2026-11-09): with the per-version Schema cache
	// cold, `sc stack secret-get` spawned sc-plugin-cloudflare and took
	// 0.59-1.32 s against 0.06-0.09 s warm — and in CI the cache AND the
	// binary are both absent, so the same command downloads a 112 MiB plugin
	// release (sc-plugin-cloudflare-linux-amd64 is 117,547,170 bytes) to read
	// a string out of a YAML file. forge's own provision.yaml does this three
	// times in a row.
	PluginTypesNotNeeded bool
}

var (
	ReadIgnoreNoClientCfg           = ReadOpts{IgnoreClientMissing: true}
	ReadIgnoreNoServerCfg           = ReadOpts{IgnoreServerMissing: true, IgnoreSecretsMissing: true}
	ReadIgnoreNoSecretsAndClientCfg = ReadOpts{IgnoreSecretsMissing: true, IgnoreClientMissing: true}
	ReadIgnoreNoSecretsAndServerCfg = ReadOpts{IgnoreSecretsMissing: true, IgnoreServerMissing: true}
	ReadIgnoreNoAnyCfg              = ReadOpts{IgnoreSecretsMissing: true, IgnoreServerMissing: true, IgnoreClientMissing: true}

	// ReadSecretsOnly is for a command that wants a stack's secret values and
	// nothing else — it tolerates a missing client.yaml and asks the read not
	// to pay the plugin pre-parse. The server descriptor is still read (a
	// scoped secrets store needs its provisioner config), just cheaply when
	// that is possible. See ReadOpts.PluginTypesNotNeeded.
	ReadSecretsOnly = ReadOpts{IgnoreClientMissing: true, PluginTypesNotNeeded: true}
)

// ParentStackName returns the stack name of a client's parent reference, which is
// either the bare name, "org/name" or "org/project/name".
func ParentStackName(ref string) string {
	parts := strings.SplitN(ref, "/", 3)
	return parts[len(parts)-1]
}

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
		parentStackName := ParentStackName(clientDesc.ParentStack)
		if parentStack, ok := current[parentStackName]; ok {
			stack.Server = parentStack.Server.Copy()
			stack.Secrets = parentStack.Secrets.Copy()
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
		for regName, registrar := range lo.Assign(stack.Server.Resources.Registrars) {
			if !registrar.IsInherited() {
				continue
			}
			val := current[stackName]
			parent := current[registrar.Inherit.Inherit].Server.Resources
			inherited, found := parent.Registrars[regName]
			if !found {
				// let a child name a parent that predates `registrars:` and still
				// inherit its single block, under whatever name the child chose
				inherited = parent.Registrar
			}
			val.Server.Resources.Registrars[regName] = inherited
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
