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

// checkUnresolvedPlaceholders looks at the parts of the target stack this deploy
// uses: the client config for the environment and the parent's provisioner,
// templates, registrar, CI/CD and the resources of the parent environment. Other
// environments are left alone; their secrets are legitimately out of reach.
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
	parentParts := strings.Split(clientDesc.ParentStack, "/")
	parentName := parentParts[len(parentParts)-1]

	parts := []any{
		clientDesc,
		stack.Server.Provisioner,
		stack.Server.Templates,
		stack.Server.Resources.Registrar,
		stack.Server.Resources.Resources[parentEnv],
		stack.Server.CiCd,
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
