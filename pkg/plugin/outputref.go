// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	pulumiplugin "github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

// getResourceTok is the engine's built-in "hydrate this URN" invoke. Not a
// provider function: the resource monitor answers it from the deployment's own
// map of registered resources (pulumi/pkg/v3 resource/deploy/builtins.go).
const getResourceTok = "pulumi:pulumi:getResource"

// OutputRef names one property of a resource that lives in SC core's Pulumi graph
// but was registered by a PLUGIN, against the shared monitor, from a
// pulumi.Context that no longer exists.
//
// This is the only way a plugin can hand core a value it does not know yet. A
// plugin cannot return an sdk.Output over gRPC — but its resources are in core's
// graph (that is what MonitorProxy buys), so core can project the property itself.
// See forge/docs/design/2026-10-24/sc-plugin-architecture/07-plugin-first-for-new-clouds.md
// §3.5 for the full derivation, including the two alternatives that do not work.
type OutputRef struct {
	// URN the plugin has ALREADY awaited. Not optional, and not merely good
	// manners: core can only read a resource the engine has finished the step for,
	// and the engine stores it during step completion — which is also what
	// RegisterResource waits for before replying. So "the plugin received this URN"
	// is exactly the condition that makes this call safe, and a plugin that returns
	// a URN it never awaited has reintroduced the waitForRef bug.
	URN string
	// Property is the PROVIDER's output property name (e.g. "privateKeyOpenssh"),
	// not an SC-level name. Which is why core cannot unmarshal it into a
	// `pulumi:"…"`-tagged struct and let the SDK do this: the name is not known
	// until runtime, and reflect.StructOf cannot build a type that satisfies
	// sdk.Resource (it does not promote embedded methods).
	Property string
}

func (r OutputRef) String() string { return r.URN + "#" + r.Property }

// ResolveStringOutput turns a plugin's OutputRef into a real sdk.StringOutput that
// core can hand to remote.Connection, ready.go or any other resource it builds.
//
// `deps` is attached to the output when the value is still unknown, so that a
// preview keeps the dependency edge it would otherwise lose.
//
// Three properties of the result, all of which matter:
//
//   - An unknown property stays UNKNOWN. In preview a machine's address does not
//     exist yet; resolving it to "" would have core preview a remote.Command
//     against the empty string and report a diff that is not the one that will
//     happen.
//   - A secret property stays SECRET. The engine marshals every invoke return with
//     KeepSecrets hardcoded on (source_eval.go), so the flag does survive the round
//     trip — but callers holding key material should still wrap with sdk.ToSecret
//     unconditionally rather than trust that, because it is one marshalling option
//     in a module we do not control and the failure mode is a private key printed
//     in a preview and archived in a CI log.
//   - A missing property is an ERROR naming what the resource does have. A typo in
//     a plugin's property name would otherwise present as an empty string that
//     silently configures SSH with no key.
func ResolveStringOutput(ctx *sdk.Context, ref OutputRef, deps []sdk.Resource) (sdk.StringOutput, error) {
	monitor, err := monitorFromContext(ctx)
	if err != nil {
		return sdk.StringOutput{}, errors.Wrap(err,
			"cannot reach the Pulumi monitor to resolve a plugin output ref")
	}
	return resolveStringOutput(ctx.Context(), monitor, ref, deps)
}

// resolveStringOutput is ResolveStringOutput with the monitor handed in, so that a
// test can drive it from a fake monitor speaking the engine's exact reply shape
// (pulumi's own WithMocks monitor does not — see getResourceState).
func resolveStringOutput(
	ctx context.Context, monitor monitorAccessor, ref OutputRef, deps []sdk.Resource,
) (sdk.StringOutput, error) {
	state, err := getResourceState(ctx, monitor, ref.URN)
	if err != nil {
		return sdk.StringOutput{}, err
	}
	raw, ok := state[resource.PropertyKey(ref.Property)]
	if !ok {
		return sdk.StringOutput{}, errors.Errorf(
			"resource %q has no output property %q; it has: %s",
			ref.URN, ref.Property, strings.Join(propertyNames(state), ", "),
		)
	}

	value, known, secret := flattenPropertyValue(raw)
	switch {
	case !known:
		// ApplyT on an unknown output does not run the closure — it propagates the
		// unknown into a StringOutput, which is the only reason this is a legal way
		// to build one.
		unknown := sdk.UnsafeUnknownOutput(deps).ApplyT(func(any) string { return "" })
		if secret {
			// A secret that is not yet known is still a secret. The engine collapses
			// it to Secret(Computed) on the wire, so this branch is reachable in any
			// preview of a generated key.
			return sdk.ToSecret(unknown).(sdk.StringOutput), nil
		}
		return unknown.(sdk.StringOutput), nil
	case value.IsNull():
		// A null is a real answer ("" is what `privateKeyPassword` is on every key
		// SC generates), and distinct from the missing-property case above.
		return secretOrPlain("", secret), nil
	case value.IsString():
		return secretOrPlain(value.StringValue(), secret), nil
	default:
		return sdk.StringOutput{}, errors.Errorf(
			"output property %q of resource %q is %T, not a string — a HostFacts field cannot carry it",
			ref.Property, ref.URN, value.V,
		)
	}
}

func secretOrPlain(v string, secret bool) sdk.StringOutput {
	if secret {
		return sdk.ToSecret(sdk.String(v)).(sdk.StringOutput)
	}
	return sdk.String(v).ToStringOutput()
}

// RehydrateResource returns a live sdk.Resource handle for a resource a plugin
// registered, so core can put it in DependsOn.
//
// The type token and name come out of the URN itself rather than from the plugin,
// so core never has to know what resource types a plugin's cloud has. The handle
// is deliberately a CustomResourceState and nothing more: core wants the ordering
// edge, not the properties (those travel as OutputRefs, which this call cannot
// serve — see the Property comment above).
func RehydrateResource(ctx *sdk.Context, urnStr string) (sdk.Resource, error) {
	parsed, err := resource.ParseURN(urnStr)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot rehydrate %q as a Pulumi resource", urnStr)
	}
	var st sdk.CustomResourceState
	// URN_ makes RegisterResource read the existing resource out of the engine
	// instead of registering a new one (sdk/go/pulumi/context.go), so this adds
	// nothing to the stack's state.
	if err := ctx.RegisterResource(
		string(parsed.Type()), parsed.Name(), nil, &st, sdk.URN_(urnStr),
	); err != nil {
		return nil, errors.Wrapf(err, "failed to rehydrate plugin resource %q", urnStr)
	}
	return &st, nil
}

// getResourceState issues the built-in getResource invoke against the live monitor
// and returns the resource's output property map.
//
// It talks to the monitor directly rather than through the SDK because the SDK's
// door to this invoke (RegisterResource + URN_) unmarshals into a tagged struct,
// and the property name is only known at runtime. The monitor handle comes from the
// same reflect+unsafe accessor MonitorProxy uses; if a Pulumi upgrade moves the
// field, both fail together and the roundtrip test catches it.
func getResourceState(
	ctx context.Context, monitor monitorAccessor, urnStr string,
) (resource.PropertyMap, error) {
	if urnStr == "" {
		return nil, errors.New("cannot resolve an output ref with an empty URN")
	}
	args, err := pulumiplugin.MarshalProperties(
		resource.NewPropertyMapFromMap(map[string]any{"urn": urnStr}),
		pulumiplugin.MarshalOptions{KeepUnknowns: true, KeepSecrets: true, KeepResources: true},
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal getResource args for %q", urnStr)
	}
	// AcceptSecrets is deliberately not set: ResourceInvokeRequest has no such
	// field, and the engine keeps secrets on invoke returns regardless.
	resp, err := monitor.Invoke(ctx, &pulumirpc.ResourceInvokeRequest{
		Tok:             getResourceTok,
		Args:            args,
		AcceptResources: true,
	})
	if err != nil {
		// The engine's own message for a URN it does not know is "unknown resource
		// <urn>", which on its own reads like a corrupt state file. Say which side
		// produced the URN.
		return nil, errors.Wrapf(err,
			"failed to read plugin-registered resource %q from the Pulumi engine "+
				"(a plugin must await a resource's URN before returning it)", urnStr)
	}
	if len(resp.GetFailures()) > 0 {
		reasons := make([]string, 0, len(resp.GetFailures()))
		for _, f := range resp.GetFailures() {
			reasons = append(reasons, fmt.Sprintf("%s: %s", f.GetProperty(), f.GetReason()))
		}
		return nil, errors.Errorf("getResource(%s) failed: %s", urnStr, strings.Join(reasons, "; "))
	}
	ret, err := pulumiplugin.UnmarshalProperties(
		resp.GetReturn(),
		pulumiplugin.MarshalOptions{KeepUnknowns: true, KeepSecrets: true, KeepResources: true},
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal getResource(%s) return", urnStr)
	}
	return stateFromGetResourceReturn(ret, urnStr)
}

// stateFromGetResourceReturn pulls the output property map out of a getResource
// reply.
//
// The engine answers with an envelope — {urn, id, provider, state} — and `state`
// is the resource's outputs (pkg/v3 resource/deploy/builtins.go getResource).
// Pulumi's OWN WithMocks monitor answers with the registered property map FLAT,
// no envelope (sdk/go/pulumi/mocks.go). So does `ret` contain a `state` object or
// is it the state? Both are accepted, in that order. The divergence is not ours to
// fix and the SDK's own getResource path silently returns empty state under mocks
// because of it; tolerating it here is what lets this mechanism be exercised by an
// ordinary unit test rather than only by a deploy.
func stateFromGetResourceReturn(ret resource.PropertyMap, urnStr string) (resource.PropertyMap, error) {
	if stateV, ok := ret["state"]; ok {
		flat, _, _ := flattenPropertyValue(stateV)
		if !flat.IsObject() {
			return nil, errors.Errorf("getResource(%s) returned a %T `state`, not an object", urnStr, flat.V)
		}
		return flat.ObjectValue(), nil
	}
	return ret, nil
}

// flattenPropertyValue unwraps the Secret / Output / Computed envelopes a property
// can arrive in and reports what it found. Secretness is sticky through nesting: a
// Secret wrapping an Output wrapping a string is still a secret string.
//
// In practice the Output case does not arrive over this wire — the engine marshals
// invoke returns without KeepOutputValues, which collapses an output value into
// Secret and/or Computed (sdk resource/plugin/rpc.go MarshalPropertyValue). It is
// handled anyway: the collapse is a marshalling option, not a guarantee, and the
// cost of covering it is four lines.
func flattenPropertyValue(v resource.PropertyValue) (value resource.PropertyValue, known, secret bool) {
	secret = false
	for {
		switch {
		case v.IsSecret():
			secret = true
			v = v.SecretValue().Element
		case v.IsOutput():
			out := v.OutputValue()
			secret = secret || out.Secret
			if !out.Known {
				return v, false, secret
			}
			v = out.Element
		case v.IsComputed():
			return v, false, secret
		default:
			return v, true, secret
		}
	}
}

func propertyNames(state resource.PropertyMap) []string {
	names := make([]string, 0, len(state))
	for k := range state {
		names = append(names, string(k))
	}
	return names
}
