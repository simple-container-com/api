// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// scInjectedEnvNamePattern is the namespace SC reserves for env vars it
// injects into a workload on the operator's behalf.
//
// The guard it backs is the property that makes auto-injection safe
// rather than merely convenient: a provider plugin can publish
// `SC_NOTIFY_URL`, but not `NOTIFY_URL`, `DATABASE_URL` or `PATH`. An
// unprefixed name would let a plugin version bump silently shadow an
// application's own variable — the application keeps starting, reads
// the plugin's value, and nothing in the diff says why. Refusing the
// name at provision time turns that into an error naming the plugin.
var scInjectedEnvNamePattern = regexp.MustCompile(`^SC_[A-Z0-9_]+$`)

// componentUsesKey is the Component config key that declares which
// sibling resources the component consumes.
const componentUsesKey = "uses"

// processComponentUses runs the ComputeProcessor of every sibling named
// in a resource's `uses:` list, against a collector private to THIS
// consumer, and returns that collector so the component's dispatch can
// read what the providers published for it.
//
// This is the Component-side half of a mechanism that already existed
// for client stacks: `deploy.go` reads `uses:` off the client descriptor
// and fans out to `processResourceDependency`. A Component could not be
// a compute consumer at all — `provisionProgram` never fanned out — so
// wiring one sibling component to another cost the operator a hostname
// reference, a hand-minted credential and an `sc secrets add`, per
// consumer, forever. One line of config replaces all three.
//
// Deliberately generic, and the generality is the point: nothing here
// knows what a notifier is. Any resource type with a registered
// ComputeProcessor can be consumed by a sibling component, which is
// what the metrics / logs / compute-status providers coming for
// ECS/GCP/VPS will ride.
//
// Returns (nil, nil) — not an error — for the overwhelmingly common
// case of a resource with no `uses:`. Callers pass the nil through to
// ProvisionParams.ConsumerContext, where nil means "no consumer
// wiring" and every pre-existing path stays byte-identical.
//
// # The consumer's identity
//
// Each used resource's processor is told who is asking via
// `StackParams.StackName`, and providers scope per-consumer credentials
// to it (the notifier mints a `tenant` claim from it). On the client
// path that is naturally unique — it is the client stack's name. In a
// parent stack it is NOT: every component lives in the one parent
// stack, so passing `stack.Name` would hand every component in the
// fleet the same identity, i.e. one shared tenant, one shared token,
// and no way to tell two publishers apart in an alert. Hence the
// composite `<parentStack>--<resourceName>`:
//
//   - unique per consumer, and across parent stacks;
//   - identifier-safe — no `/`, so a provider that derives a scoped
//     database username or a bucket prefix from it stays valid;
//   - readable in an alert, which is where an operator meets it
//     ("infra--my-backup").
//
// Two alternatives rejected: the bare resource name collides with a
// same-named client stack (`my-backup` the component and `my-backup`
// the deployed service would share a tenant), and a new explicit
// consumer-identity field on ParentInfo fails silently-wrong — a
// provider that has not adopted the field keeps reading StackName and
// shares one identity across every component, with nothing to see.
func (p *pulumi) processComponentUses(
	ctx *sdk.Context,
	stack api.Stack,
	cfg *api.ConfigFile,
	env string,
	resName string,
	res api.ResourceDescriptor,
	shared pApi.ComputeContextCollector,
	outs pApi.ResourcesOutputs,
) (pApi.ComputeContextCollector, error) {
	usedNames, err := resourceUsesNames(res)
	if err != nil {
		return nil, errors.Wrapf(err, "resource %q of env %q", resName, env)
	}
	if len(usedNames) == 0 {
		return nil, nil
	}

	consumerName := componentConsumerIdentity(stack.Name, resName)
	consumer := pApi.NewComputeContextCollector(ctx.Context(), p.logger, consumerName, env)
	consumerParams := api.StackParams{
		StackName:   consumerName,
		Environment: env,
		Parent:      true,
	}
	parentRef := pApi.ExpandStackReference(stack.Name, p.provisionerCfg.Organization, cfg.ProjectName)

	used, err := resolveUsedSiblings(stack.Server.Resources.Resources[env].Resources, env, resName, usedNames)
	if err != nil {
		return nil, err
	}
	for _, sibling := range used {
		usedName := sibling.name
		if _, hasProcessor := pApi.ComputeProcessorFuncByType[sibling.res.Type]; !hasProcessor {
			// Matches what the client-stack path does with the same
			// shape (deploy.go logs and moves on): a resource type with
			// no ComputeProcessor has nothing to publish to a consumer.
			// Warned rather than silent, because the operator asked for
			// wiring and is getting none — the one outcome worth being
			// loud about even though it is not fatal.
			p.logger.Warn(ctx.Context(),
				"resource %q of env %q declares `uses: [%s]`, but resource type %q registers no compute processor — "+
					"nothing will be injected for this dependency",
				resName, env, usedName, sibling.res.Type)
			continue
		}
		p.logger.Info(ctx.Context(), "resource %q of env %q uses resource %q", resName, env, usedName)
		if err := p.processResourceDependency(ctx, stack, consumerParams, dependencyResourceParams{
			resName: usedName,
			resEnv:  env,

			collector:       consumer,
			parentStackName: stack.Name,
			stackEnv:        env,
			parentStackRef:  parentRef,
			usesResource:    true,
			// The sibling outputs the provider reads to find the
			// resource it is wiring — the notifier's deployed endpoint
			// URL arrives this way. The client path has no equivalent
			// (it passes nil and providers fall back to the declared
			// ingress hostname); here the real output exists, because
			// ordering guarantees the used resource provisioned first.
			resourceOutputs: outs,
		}); err != nil {
			return nil, errors.Wrapf(err, "resource %q of env %q uses resource %q", resName, env, usedName)
		}
	}

	// Pulumi wait-ordering stays on the shared collector, where every
	// resource in the env already contributes. Dependencies are URNs to
	// wait on, not values, so a superset is exactly today's behaviour
	// and carries nothing consumer-specific.
	for _, dep := range consumer.Dependencies() {
		shared.AddDependency(dep)
	}
	return consumer, nil
}

// usedSibling is one entry of a resolved `uses:` list.
type usedSibling struct {
	name string
	res  api.ResourceDescriptor
}

// resolveUsedSiblings validates a `uses:` list against the resource set
// of its own environment, preserving declaration order.
//
// Both refusals are loud on purpose. A name with no matching resource
// is a typo the operator cannot see any other way: nothing would be
// injected, the component would deploy, and the failure would surface
// later as a service missing an env var it never knew to expect. The
// available names are listed because the usual cause is a near-miss
// (`my-notify` for `my-notifier`) or the resource living in a different
// environment.
func resolveUsedSiblings(
	siblings map[string]api.ResourceDescriptor,
	env, resName string,
	usedNames []string,
) ([]usedSibling, error) {
	out := make([]usedSibling, 0, len(usedNames))
	for _, usedName := range usedNames {
		if usedName == resName {
			return nil, errors.Errorf(
				"resource %q of env %q declares `uses: [%s]` — a resource cannot consume itself",
				resName, env, usedName,
			)
		}
		usedRes, found := siblings[usedName]
		if !found {
			return nil, errors.Errorf(
				"resource %q of env %q declares `uses: [%s]` but no resource named %q is declared in env %q (available: %s)",
				resName, env, usedName, usedName, env, strings.Join(sortedKeys(siblings), ", "),
			)
		}
		if usedRes.Name == "" {
			usedRes.Name = usedName
		}
		out = append(out, usedSibling{name: usedName, res: usedRes})
	}
	return out, nil
}

// resourceUsesNames reads a resource's `uses:` list.
//
// Scoped to plugin-owned config (`*api.PluginFieldConfig`, which is how
// a Component reaches core) because that is the only shape where `uses:`
// is a Component declaration. An in-tree resource type carries a typed
// config struct and expresses dependencies through its own
// `DependsOnResources`.
func resourceUsesNames(res api.ResourceDescriptor) ([]string, error) {
	pfc, ok := res.Config.Config.(*api.PluginFieldConfig)
	if !ok || pfc == nil {
		return nil, nil
	}
	if _, declared := pfc.Raw[componentUsesKey]; !declared {
		return nil, nil
	}
	cc, err := api.NewComponentConfig(pfc)
	if err != nil {
		return nil, err
	}
	return cc.Uses()
}

// componentConsumerIdentity builds the per-consumer identity described
// on processComponentUses.
func componentConsumerIdentity(stackName, resName string) string {
	return fmt.Sprintf("%s--%s", stackName, resName)
}

func sortedKeys(m map[string]api.ResourceDescriptor) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// mergeConsumerEnvIntoComponentRaw folds what the used resources
// published for this consumer into the Component's merged `.Raw`, so
// the dispatched template carries them to the deployed workload.
//
// Plain values land in `env:`; secret ones in `secrets:`, which the
// Lambda / compose provisioner wires to a per-function SecretsManager
// entry rather than a plain env var at rest — the same split
// `applyComponentSecretEnvMap` relies on, and the reason a minted
// publisher token never appears in a function's visible configuration.
//
// Operator-written keys win. A `uses:`-injected value is a default the
// operator never typed; silently overwriting a line they did type would
// make the config file stop describing the deployment.
//
// Refuses an injected name outside the `SC_` namespace (D8), naming the
// resource that published it. Returns the error rather than logging:
// a plugin shadowing an application's env var is a contract violation
// by the plugin, and the operator who must report it needs it to fail
// where it happened.
func mergeConsumerEnvIntoComponentRaw(merged map[string]any, consumer pApi.ComputeContext) error {
	if consumer == nil {
		return nil
	}
	for _, spec := range []struct {
		key  string
		vars []pApi.ComputeEnvVariable
	}{
		{key: "env", vars: consumer.EnvVariables()},
		{key: "secrets", vars: consumer.SecretEnvVariables()},
	} {
		for _, v := range spec.vars {
			if !scInjectedEnvNamePattern.MatchString(v.Name) {
				return errors.Errorf(
					"resource %q (type %q) tried to inject env variable %q into a consumer, "+
						"but SC only injects names matching %s. An unprefixed name would shadow "+
						"an application's own variable; rename it to SC_%s in the plugin",
					v.ResourceName, v.ResourceType, v.Name,
					scInjectedEnvNamePattern.String(), strings.ToUpper(v.Name),
				)
			}
			dst, ok := merged[spec.key].(map[string]any)
			if !ok {
				dst = map[string]any{}
				merged[spec.key] = dst
			}
			if _, written := dst[v.Name]; written {
				continue
			}
			dst[v.Name] = v.Value
		}
	}
	return nil
}

// refuseUnresolvedUsesPlaceholders refuses a `${resource:<used>.<key>}`
// reference to a resource this component declared in `uses:` that is
// still a literal after both resolve passes.
//
// Why this needs its own refusal: `ResolvePlaceholders` fails OPEN. An
// unresolved token survives as the literal string
// `${resource:my-notifier.token}` and is deployed into the workload's
// env, where it reads as a value — a publisher authenticating with the
// nine characters `${resource` and failing in a way whose error message
// names nothing relevant. The operator wrote a reference; they get
// either the value or an error.
//
// Deliberately narrow — only names the component itself declared in
// `uses:`. A blanket scan of `merged` would break the legitimate
// deferred case the dispatch documents (a ComponentDefaults
// `${resource:<self>.<key>}` meant to resolve later, at a consumer's
// own deploy), and a token naming an undeclared sibling is already
// reported by the `uses:`-missing path with better advice.
func refuseUnresolvedUsesPlaceholders(merged map[string]any, resName string, usedNames []string) error {
	if len(usedNames) == 0 {
		return nil
	}
	var offenders []string
	for _, usedName := range usedNames {
		prefix := fmt.Sprintf("${resource:%s.", usedName)
		collectLiteralTokens(merged, prefix, &offenders)
	}
	if len(offenders) == 0 {
		return nil
	}
	sort.Strings(offenders)
	return errors.Errorf(
		"component %q references %s, which did not resolve — the used resource's compute processor "+
			"published no such key. `${resource:…}` resolution fails open, so leaving this would deploy "+
			"the literal placeholder string as the value. Check the key name against what the "+
			"resource's plugin publishes",
		resName, strings.Join(offenders, ", "),
	)
}

// collectLiteralTokens appends every distinct `${resource:…}` token
// starting with prefix found in any string leaf of node, at any depth.
//
// Walks both map shapes for the same reason `collectResourceRefs`
// (pkg/api/plugin_field_deps.go) does: a `.Raw` map reaches core
// through whichever YAML decoder the caller used, and yaml.v2 produces
// `map[any]any` for a nested mapping. Missing one shape would make the
// guard silently depend on the decoder version.
func collectLiteralTokens(node any, prefix string, out *[]string) {
	switch v := node.(type) {
	case map[string]any:
		for _, el := range v {
			collectLiteralTokens(el, prefix, out)
		}
	case map[any]any:
		for _, el := range v {
			collectLiteralTokens(el, prefix, out)
		}
	case []any:
		for _, el := range v {
			collectLiteralTokens(el, prefix, out)
		}
	case string:
		idx := 0
		for {
			rel := strings.Index(v[idx:], prefix)
			if rel < 0 {
				return
			}
			start := idx + rel
			end := strings.Index(v[start:], "}")
			if end < 0 {
				return
			}
			token := v[start : start+end+1]
			if !contains(*out, token) {
				*out = append(*out, token)
			}
			idx = start + end + 1
		}
	}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
