// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"

	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// Helpers for the OTHER side of this bus: a plugin implementing the RPCs, not core
// dispatching them.
//
// Everything here was hand-written, identically, in sc-plugin-yandex and
// sc-plugin-cloudflare before it was written once here — including the two things a
// plugin author gets wrong by default, both of which fail silently:
//
//   - A pulumi.Context must be opened against the monitor address core puts in the
//     request header, one per RPC. Forget it and nothing is registered; share one
//     across RPCs and a ProviderResource from the wrong context routes nowhere.
//   - Pulumi fires RegisterResource from a goroutine, so a handler that returns
//     before its outputs resolve lets pctx.Close() tear the engine connection down
//     mid-registration. The request vanishes: no resource, no error, and a deploy
//     that hangs. See Addendum H of
//     forge/docs/design/2026-10-24/sc-plugin-architecture/04-no-destroy-invariant.md.

// PulumiContext opens a pulumi.Context against the monitor address core put in the
// request header, and returns a close function the handler must defer.
//
// Call it once per RPC and let it close at the end of that RPC. A *sdk.Context is
// not reusable across calls and neither is anything built inside one: an
// sdk.ProviderResource cannot cross context boundaries, so a plugin that needs a
// provider must build it inside the same handler that registers resources against
// it. Re-registering the provider per call is correct and cheap — the URN stays
// byte-identical (same name, same version), so the engine sees the same resource
// and the no-destroy invariant holds.
//
// Await everything you care about BEFORE the deferred close runs; see AwaitURN.
func PulumiContext(ctx context.Context, h *scpluginv1.RequestHeader) (*sdk.Context, func(), error) {
	if h.GetMonitorAddress() == "" {
		return nil, nil, errors.New("no monitor address in request header: " +
			"this RPC was dispatched outside an active plugin session, or core did not fill the header")
	}
	pctx, err := sdk.NewContext(ctx, sdk.RunInfo{
		MonitorAddr: h.GetMonitorAddress(),
		Project:     h.GetProjectName(),
		Stack:       h.GetStackName(),
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "open pulumi context from monitor address")
	}
	return pctx, func() { _ = pctx.Close() }, nil
}

// AwaitURN blocks until the engine has assigned res its URN, and returns it.
//
// This is the call that makes a resource real before the handler returns, and the
// one that makes a ResourcePropertyRef answerable: core resolves a ref by asking the
// engine for that URN's state, which it can only answer once the resource's step has
// completed. Awaiting the URN is exactly what waits for that.
//
// A URN is known even during a preview — the engine assigns it at RegisterResource,
// before anything is created — so an unknown URN means the resource was never
// registered, and that is reported as an error rather than an empty string.
func AwaitURN(ctx context.Context, res sdk.Resource) (string, error) {
	if res == nil {
		return "", errors.New("await urn of a nil resource")
	}
	out, err := internals.UnsafeAwaitOutput(ctx, res.URN())
	if err != nil {
		return "", errors.Wrap(err, "await resource urn")
	}
	if !out.Known {
		return "", errors.New("resource urn is unknown after registration — the resource was never registered")
	}
	urn, ok := out.Value.(sdk.URN)
	if !ok {
		return "", errors.Errorf("resource urn resolved to %T, not a pulumi.URN", out.Value)
	}
	if urn == "" {
		return "", errors.New("resource urn resolved to the empty string")
	}
	return string(urn), nil
}

// AwaitURNs is AwaitURN over several resources, in order, and is what a handler
// wanting the dependency list for a VmHostFacts calls.
func AwaitURNs(ctx context.Context, resources ...sdk.Resource) ([]string, error) {
	urns := make([]string, 0, len(resources))
	for i, res := range resources {
		urn, err := AwaitURN(ctx, res)
		if err != nil {
			return nil, errors.Wrapf(err, "resource #%d", i)
		}
		urns = append(urns, urn)
	}
	return urns, nil
}

// AwaitAll blocks until every passed output has resolved, and is the generic form of
// the "do not return before your outputs resolve" rule for a handler that has no URN
// it needs the value of — it just needs the registrations to have landed.
//
// An output that resolves UNKNOWN (a preview, or a value derived from something not
// yet created) satisfies this: unknown is a resolution, and refusing it would make
// every plugin fail `sc preview`.
func AwaitAll(ctx context.Context, outputs ...sdk.Output) error {
	for i, out := range outputs {
		if out == nil {
			continue
		}
		if _, err := internals.UnsafeAwaitOutput(ctx, out); err != nil {
			return errors.Wrapf(err, "await output #%d", i)
		}
	}
	return nil
}

// PlainToInputs is the inverse of InputsToPlain: it serialises a Go value
// (typed struct or map[string]any) into the plugin bus's InputValue map
// via a JSON round-trip. Nested objects and lists travel as InputMap /
// InputList. SC core uses this to marshal both plain-resource configs
// (serializeConfigToInputs in plugin_sidecars.go — a thin wrapper) and
// template-conversion stack configs (pkg/stackread's ConvertStackConfig
// dialler).
//
// The output is deterministic for a given input: JSON numbers return as
// doubles; a plugin decoding into an int field receives the integral
// value through yaml.v3's number coercion.
func PlainToInputs(cfg any) map[string]*scpluginv1.InputValue {
	out := map[string]*scpluginv1.InputValue{}
	if cfg == nil {
		return out
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return out
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return out
	}
	for k, v := range raw {
		if iv := PlainToInputValue(v); iv != nil {
			out[k] = iv
		}
	}
	return out
}

// PlainToInputValue maps one decoded value onto the bus's InputValue.
// Returns nil for a nil / null input — the caller then omits the key,
// which a plugin reads back as the zero value (same semantics as the
// null meant). Used recursively for nested maps and lists.
func PlainToInputValue(v any) *scpluginv1.InputValue {
	switch sv := v.(type) {
	case string:
		return &scpluginv1.InputValue{Value: &scpluginv1.InputValue_StringValue{StringValue: sv}}
	case bool:
		return &scpluginv1.InputValue{Value: &scpluginv1.InputValue_BoolValue{BoolValue: sv}}
	case float64:
		return &scpluginv1.InputValue{Value: &scpluginv1.InputValue_DoubleValue{DoubleValue: sv}}
	case int:
		return &scpluginv1.InputValue{Value: &scpluginv1.InputValue_DoubleValue{DoubleValue: float64(sv)}}
	case int64:
		return &scpluginv1.InputValue{Value: &scpluginv1.InputValue_DoubleValue{DoubleValue: float64(sv)}}
	case map[string]any:
		fields := make(map[string]*scpluginv1.InputValue, len(sv))
		for k, item := range sv {
			if iv := PlainToInputValue(item); iv != nil {
				fields[k] = iv
			}
		}
		return &scpluginv1.InputValue{Value: &scpluginv1.InputValue_MapValue{MapValue: &scpluginv1.InputMap{Fields: fields}}}
	case []any:
		items := make([]*scpluginv1.InputValue, 0, len(sv))
		for _, item := range sv {
			// A list keeps its positions: a null element becomes an
			// empty InputValue (decoded back as nil) rather than
			// vanishing and shifting every later element.
			iv := PlainToInputValue(item)
			if iv == nil {
				iv = &scpluginv1.InputValue{}
			}
			items = append(items, iv)
		}
		return &scpluginv1.InputValue{Value: &scpluginv1.InputValue_ListValue{ListValue: &scpluginv1.InputList{Items: items}}}
	default:
		return nil
	}
}

// InputsToPlain turns a request's InputValue map back into plain Go values —
// strings, bools, float64, int64, []byte, map[string]any and []any — ready for a
// YAML or JSON round-trip into the plugin's own config struct. It is the inverse
// of what core does when it serialises a resource's config onto the bus, nested
// objects and lists included.
//
// Output and secret references are passed through as their reference strings; a
// plugin that accepts them resolves them itself (ResolveStringOutput /
// ResolveSecret).
func InputsToPlain(inputs map[string]*scpluginv1.InputValue) map[string]any {
	out := make(map[string]any, len(inputs))
	for k, v := range inputs {
		if pv, ok := InputValueToPlain(v); ok {
			out[k] = pv
		}
	}
	return out
}

// InputValueToPlain is InputsToPlain for one value. The bool is false for an
// InputValue with no value set, which the caller should treat as absent (inside
// a list it decodes as nil, keeping the element's position).
func InputValueToPlain(v *scpluginv1.InputValue) (any, bool) {
	switch x := v.GetValue().(type) {
	case *scpluginv1.InputValue_StringValue:
		return x.StringValue, true
	case *scpluginv1.InputValue_BoolValue:
		return x.BoolValue, true
	case *scpluginv1.InputValue_DoubleValue:
		return x.DoubleValue, true
	case *scpluginv1.InputValue_IntValue:
		return x.IntValue, true
	case *scpluginv1.InputValue_BytesValue:
		return x.BytesValue, true
	case *scpluginv1.InputValue_OutputRef:
		return x.OutputRef, true
	case *scpluginv1.InputValue_SecretRef:
		return x.SecretRef, true
	case *scpluginv1.InputValue_MapValue:
		return InputsToPlain(x.MapValue.GetFields()), true
	case *scpluginv1.InputValue_ListValue:
		items := make([]any, 0, len(x.ListValue.GetItems()))
		for _, item := range x.ListValue.GetItems() {
			pv, _ := InputValueToPlain(item)
			items = append(items, pv)
		}
		return items, true
	default:
		return nil, false
	}
}
