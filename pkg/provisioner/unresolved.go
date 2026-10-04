// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package provisioner

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/pkg/api"
)

// A ${secret:} or ${auth:} placeholder the resolver could not fill stays in the
// config as literal text, and the deploy goes on with it: an app gets the string
// "${secret:X}" as an environment variable, or a provider gets it as a key.
var unresolvedRef = regexp.MustCompile(`\$\{(?:secret|auth):[^}]*\}`)

// ErrUnresolvedPlaceholders is returned when a deploy that reads its secrets from
// scopes alone would ship an unresolved ${secret:} or ${auth:} placeholder.
var ErrUnresolvedPlaceholders = errors.New("unresolved secret placeholders")

// checkUnresolvedPlaceholders looks at what a client deploy of the target stack
// consumes: the client config for the environment, the template it selects (its own
// or the parent environment's default), the parent resources it lists in uses and
// dependencies, every registrar (the zone is picked at run time) and the parent's
// provisioner. The parent's other templates and resources, its CI/CD (only workflow
// generation reads it) and other environments are left alone: their secrets are
// legitimately out of reach, and a sibling stack's placeholder must not block this
// deploy.
//
// When the parent's secrets came from scope files alone, any hit fails the deploy:
// nothing else can fill it, and scopes are opt-in, so no existing configuration
// changes behaviour. Otherwise the hit is only logged, by name.
func (p *provisioner) checkUnresolvedPlaceholders(ctx context.Context, params api.StackParams) error {
	stack, ok := p.stacks[params.StackName]
	if !ok {
		return nil
	}
	clientDesc, ok := stack.Client.Stacks[params.Environment]
	if !ok {
		return nil
	}
	parentEnv := lo.Ternary(clientDesc.ParentEnv != "", clientDesc.ParentEnv, params.Environment)
	parentName := api.ParentStackName(clientDesc.ParentStack)
	envResources := stack.Server.Resources.Resources[parentEnv]

	templateName := lo.Ternary(clientDesc.Template != "", clientDesc.Template, envResources.Template)
	uses, deps := clientResourceRefs(clientDesc.Config.Config)
	resNames := append([]string{}, uses...)
	for _, d := range deps {
		resNames = append(resNames, d.Resource)
	}

	parts := []any{
		clientDesc,
		stack.Server.Provisioner,
		stack.Server.Resources.Registrar,
		stack.Server.Resources.Registrars,
	}
	if tpl, ok := stack.Server.Templates[templateName]; ok {
		parts = append(parts, tpl)
	}
	for _, name := range lo.Uniq(resNames) {
		if res, ok := envResources.Resources[name]; ok {
			parts = append(parts, res)
		}
	}

	found := map[string]bool{}
	for _, part := range parts {
		out, err := yaml.Marshal(part)
		if err != nil {
			return errors.Wrap(err, "failed to inspect the stack for unresolved placeholders")
		}
		for _, m := range unresolvedRef.FindAllString(string(out), -1) {
			found[m] = true
		}
	}
	if len(found) == 0 {
		return nil
	}
	names := lo.Keys(found)
	sort.Strings(names)
	if p.scopedOnly[parentName] {
		return errors.Wrapf(ErrUnresolvedPlaceholders,
			"stack %q in %q reads its secrets from scopes only, and none of the scopes this deploy can open fills %s",
			params.StackName, params.Environment, strings.Join(names, ", "))
	}
	p.log.Warn(ctx, "stack %q in %q has unresolved placeholders that will be deployed as literal text: %s",
		params.StackName, params.Environment, strings.Join(names, ", "))
	return nil
}

// clientResourceRefs returns the parent resources a client config names in uses and
// dependencies, as the deploy reads them from the typed config.
func clientResourceRefs(cfg any) (uses []string, deps []api.StackConfigDependencyResource) {
	switch c := cfg.(type) {
	case *api.StackConfigCompose:
		return c.Uses, c.Dependencies
	case *api.StackConfigSingleImage:
		return c.Uses, c.Dependencies
	}
	if a, ok := cfg.(api.ResourceAware); ok {
		uses = a.Uses()
	}
	if d, ok := cfg.(api.WithDependsOnResources); ok {
		deps = d.DependsOnResources()
	}
	return uses, deps
}
