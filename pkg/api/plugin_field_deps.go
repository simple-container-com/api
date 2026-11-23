// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import "strings"

// resourceTokenPrefix is the opening of a `${resource:<name>.<key>}`
// placeholder — the vocabulary `Collector.ResolvePlaceholders`
// (pkg/clouds/pulumi/api/compute_context.go) resolves against the
// template extensions a resource published during its own provision.
const resourceTokenPrefix = "${resource:"

// DependsOnResources makes a plugin-owned resource satisfy
// api.WithParentDependencies by reporting every sibling its config
// REFERENCES with a `${resource:<name>.<key>}` placeholder, so
// `resourceProvisionOrder` (pkg/clouds/pulumi/provision_order.go)
// provisions the referenced resource FIRST.
//
// Without this, order is purely ALPHABETICAL. `resourceProvisionOrder`
// sorts the resource-name set and runs Kahn's algorithm over whatever
// `WithParentDependencies` reports — and before this method the only two
// implementations in the tree were `*KubernetesConfig` and
// `*K3sClusterConfig`, neither of which a plugin-owned resource can be.
// Every resource SC core holds as an opaque `PluginFieldConfig` reported
// nothing, so a reference between two of them was ordered by name.
//
// That is not a theoretical gap, and the way it fails is the problem:
// `ResolvePlaceholders`' `resource` extension returns the input string
// unchanged when the name is not (yet) in `resTplExtensions`. A reference
// evaluated too early therefore does not fail — it survives as the
// LITERAL `${resource:my-notifier.self-url}` and is handed to the
// dispatched template as if it were a value. Whether a deploy worked came
// down to whether the referenced resource's name happened to sort first:
//
//	my-backup:   {notify: {url: "${resource:my-notifier.self-url}"}}
//	my-notifier: {...}
//
// `my-backup` < `my-notifier`, so the consumer provisioned first and got
// the literal. Renaming the notifier `alerts` would have fixed it, which
// is the signature of a landmine rather than a constraint.
//
// Scope and limits, both deliberate:
//
//   - Only the OPERATOR's `.Raw` is scanned. Ordering is computed in the
//     provision program over parsed config, long before any plugin's
//     ComponentDefaults are merged in (`plugin_sidecars.go` does that at
//     dispatch), so a reference that exists only in a plugin default
//     cannot influence order. A plugin that wants its default to be
//     order-safe must have the operator name the dependency in config.
//   - `${dependency:…}` is NOT scanned. Its three-part vocabulary names a
//     consumer stack's dependency, not a sibling in this resource set, so
//     there is no sibling to order against.
//   - A malformed token contributes no dependency rather than a garbage
//     one, matching `KubernetesConfig.DependsOnResources`. The refusals
//     for a bad reference belong to the resolver that reads it; a second
//     one here would turn one clear error into two.
//   - A self-reference is returned as-is. `resourceProvisionOrder` already
//     drops `dep.Name == name`, and filtering here would mean teaching
//     this method its own resource name, which the config does not carry.
//
// One behaviour change worth stating plainly: two resources that reference
// each other now produce `resourceProvisionOrder`'s circular-dependency
// refusal, where before they both silently fail-opened to literals. That
// error already names `${resource:...}` and tells the operator to break
// the cycle with a literal value, so the message anticipated this; but a
// config that "worked" by virtue of never resolving will now be refused.
// That is the intended trade — a refusal naming both resources beats a
// placeholder string reaching a service's env.
//
// A second source of dependencies, added with Component `uses:`: every
// name in a top-level `uses:` list. Where a `${resource:…}` token is an
// INCIDENTAL reference (the operator wanted one field's value and the
// ordering edge falls out of it), `uses:` is a DELIBERATE one — it asks
// SC to run that resource's ComputeProcessor for this consumer, which
// can only happen after the resource exists. Both must order the same
// way, so both are reported here rather than only at the point of use.
func (p *PluginFieldConfig) DependsOnResources() []ParentResourceDependency {
	if p == nil || len(p.Raw) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var out []ParentResourceDependency
	collectResourceRefs(p.Raw, seen, &out)
	collectUsesRefs(p.Raw["uses"], seen, &out)
	return out
}

// collectUsesRefs appends a dependency for each resource name in a
// top-level `uses:` value, over the same three shapes
// `(*ComponentConfig).Uses` accepts (bare string, []string, []any).
//
// Deliberately silent on a shape it cannot read: `Uses()` is the
// accessor that refuses a malformed `uses:` with a message naming the
// offending type, and parse-time validation runs it. Returning a
// garbage ordering edge from here — or a second error for the same
// typo — would make one clear refusal into two confusing ones, the
// same reasoning the `${resource:…}` scan above records for a
// malformed token.
func collectUsesRefs(node any, seen map[string]struct{}, out *[]ParentResourceDependency) {
	var names []string
	switch u := node.(type) {
	case nil:
		return
	case string:
		names = []string{u}
	case []string:
		names = u
	case []any:
		for _, el := range u {
			if s, ok := el.(string); ok {
				names = append(names, s)
			}
		}
	default:
		return
	}
	for _, name := range names {
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		*out = append(*out, ParentResourceDependency{Name: name})
	}
}

// collectResourceRefs walks an arbitrary parsed-YAML node and appends a
// dependency for each distinct resource name referenced by a
// `${resource:…}` token in any string leaf, at any depth.
//
// Depth and container shape are both load-bearing: a reference can sit
// under a nested list (`channels[].url`) as readily as at the top level,
// and the placeholder resolver itself walks the whole object graph
// reflectively, so anything shallower here would order a deploy the
// resolver then resolves.
//
// `out` is appended through a pointer rather than returned so the
// recursion carries no per-node allocation, and `seen` keeps the result
// stable: one dependency per name however many times it is referenced.
func collectResourceRefs(node any, seen map[string]struct{}, out *[]ParentResourceDependency) {
	switch v := node.(type) {
	case map[string]any:
		for _, el := range v {
			collectResourceRefs(el, seen, out)
		}
	case map[any]any:
		// yaml.v2 unmarshals a nested mapping into map[any]any. A
		// PluginFieldConfig's Raw reaches us through whichever decoder
		// the caller used, so both shapes have to be walked or the
		// ordering silently depends on the YAML library version.
		for _, el := range v {
			collectResourceRefs(el, seen, out)
		}
	case []any:
		for _, el := range v {
			collectResourceRefs(el, seen, out)
		}
	case string:
		for _, name := range resourceRefNames(v) {
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			*out = append(*out, ParentResourceDependency{Name: name})
		}
	}
}

// resourceRefNames returns the resource name of every
// `${resource:<name>.<key>}` token in s, in order of appearance.
//
// Parser is deliberately small and mirrors `substituteParentTokens`
// (pkg/clouds/pulumi/api/defaults_interpolation.go): scan for the
// prefix, take the first `}`, split on the first `.`. One string may
// carry several tokens — a URL built from two references is legal — so
// the scan continues past each match.
//
// A token with no `.`, an empty name, or an empty key yields nothing:
// `ResolvePlaceholders` refuses that shape outright, so treating it as
// an ordering constraint would order a deploy that is about to be
// rejected anyway.
func resourceRefNames(s string) []string {
	var names []string
	i := 0
	for {
		rel := strings.Index(s[i:], resourceTokenPrefix)
		if rel < 0 {
			return names
		}
		start := i + rel + len(resourceTokenPrefix)
		end := strings.Index(s[start:], "}")
		if end < 0 {
			// Unterminated token — nothing further can parse.
			return names
		}
		body := s[start : start+end]
		i = start + end + 1
		dot := strings.Index(body, ".")
		if dot <= 0 || dot == len(body)-1 {
			continue
		}
		if strings.Contains(body[:dot], "${") {
			// A nested `${…}` inside the name means this token is itself
			// the product of an earlier substitution pass that has not
			// run yet. There is no name to order against.
			continue
		}
		names = append(names, body[:dot])
	}
}
