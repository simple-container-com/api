// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package runtime

import (
	"context"

	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"

	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// HarvestExports reads every `pctx.Export(k, v)` call made during an RPC
// handler and converts each value into an InputValue the wire can carry.
// Awaits every sdk.Output so a late-resolving ApplyT chain (bucket Arn,
// access key, etc.) lands as a concrete string on apply — remains unknown
// during preview, which is expected.
//
// Preserves the Secret flag through the serialisation boundary. Lose it and
// the value lands plaintext in the parent stack root's output map,
// `pulumi stack output` serves it as a bare string, and the apply/preview
// logs print it clear. Live-caught 2026-10-31 on the first aws harvest
// preview — see Addendum V of
// forge/docs/design/2026-10-24/sc-plugin-architecture/04-no-destroy-invariant.md.
//
// SC core re-exports whatever this returns onto its own pctx at
// api/pkg/clouds/pulumi/plugin_sidecars.go (api v2026.10.60+).
func HarvestExports(ctx context.Context, pctx *sdk.Context) (map[string]*scpluginv1.InputValue, error) {
	raw := pctx.GetCurrentExportMap()
	out := make(map[string]*scpluginv1.InputValue, len(raw))
	for k, v := range raw {
		plain, secret, err := awaitInputToPlain(ctx, v)
		if err != nil {
			return out, errors.Wrapf(err, "await export %q", k)
		}
		if iv := anyToInputValue(plain); iv != nil {
			iv.Secret = secret
			out[k] = iv
		}
	}
	return out, nil
}

// awaitInputToPlain returns (concrete value, secret flag, error) for an
// sdk.Input. The Secret flag is critical for secret-bearing outputs
// (database passwords, access keys, ECR auth tokens): SC core wraps the
// re-export with `sdk.ToSecret` when this bool is true.
func awaitInputToPlain(ctx context.Context, v sdk.Input) (any, bool, error) {
	if v == nil {
		return nil, false, nil
	}
	switch tv := v.(type) {
	case sdk.String:
		return string(tv), false, nil
	case sdk.Bool:
		return bool(tv), false, nil
	case sdk.Int:
		return int(tv), false, nil
	case sdk.Float64:
		return float64(tv), false, nil
	}
	if out, ok := v.(sdk.Output); ok {
		resolved, aerr := internals.UnsafeAwaitOutput(ctx, out)
		if aerr != nil {
			return nil, false, aerr
		}
		return unwrapPlain(resolved.Value), resolved.Secret, nil
	}
	return nil, false, nil
}

func unwrapPlain(v any) any {
	switch tv := v.(type) {
	case sdk.URN:
		return string(tv)
	case sdk.ID:
		return string(tv)
	}
	return v
}

func anyToInputValue(v any) *scpluginv1.InputValue {
	if v == nil {
		return nil
	}
	switch sv := v.(type) {
	case string:
		if sv == "" {
			return nil
		}
		return &scpluginv1.InputValue{Value: &scpluginv1.InputValue_StringValue{StringValue: sv}}
	case bool:
		return &scpluginv1.InputValue{Value: &scpluginv1.InputValue_BoolValue{BoolValue: sv}}
	case int:
		return &scpluginv1.InputValue{Value: &scpluginv1.InputValue_IntValue{IntValue: int64(sv)}}
	case int64:
		return &scpluginv1.InputValue{Value: &scpluginv1.InputValue_IntValue{IntValue: sv}}
	case float64:
		return &scpluginv1.InputValue{Value: &scpluginv1.InputValue_DoubleValue{DoubleValue: sv}}
	case []byte:
		return &scpluginv1.InputValue{Value: &scpluginv1.InputValue_BytesValue{BytesValue: sv}}
	}
	return nil
}
