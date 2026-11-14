// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"strings"

	"github.com/pkg/errors"
)

// DefaultsSentinel is the prefix SC core looks for on a string value
// in a plugin's ComponentDefaults map to decide whether the string is
// a literal or an interpolation expression. Chosen to be obvious in a
// yaml inspection AND to collide with no path, URL, bucket name,
// env var shape, or other common value the operator might write.
//
// An OPERATOR-written string with this prefix is refused at parse
// time (see `RefuseSentinelInOperatorRaw`). Plugin-written strings
// with this prefix get interpolated at dispatch time (see
// `InterpolateDefaults`).
const DefaultsSentinel = "$$sc:"

// DefaultsContext carries the dispatch-time values the interpolation
// layer substitutes into `${…}` tokens inside plugin-default strings.
// Populated by SC core's `dispatchComponentTemplate` from the live
// deploy context.
type DefaultsContext struct {
	// Name of the component resource being dispatched (e.g.
	// "my-backup"). Substituted for `${name}`.
	Name string

	// Env (dispatch environment, e.g. "staging", "prod", "smoke").
	// Substituted for `${env}`.
	Env string

	// Stack (dispatch stack name, e.g. "forge-conductor--staging").
	// Substituted for `${stack}`.
	Stack string

	// TemplateName is the `deployTo:` value naming the chosen template
	// (e.g. "lambda" / "ecs-fargate"). Substituted for
	// `${template.name}`.
	TemplateName string

	// TemplateConfig is the chosen template's `config:` map from the
	// parent stack's `templates:` block. `${template.region}` reads
	// TemplateConfig["region"]; `${template.account}` reads
	// TemplateConfig["account"]. Values must be string-shaped; a non-
	// string value is treated as missing + the token is left
	// unresolved (with a warning — see InterpolateDefaults' fail-open
	// contract).
	TemplateConfig map[string]any
}

// InterpolateDefaults walks the merged ComponentRaw map produced by
// DeepMergeComponentRaw and substitutes `${…}` tokens inside
// $$sc:-prefixed strings. Other values pass through unchanged.
//
// The substitution is intentionally SHALLOW on the token set — only
// the tokens listed in DefaultsContext. An unrecognised token
// (e.g. `${foo}`) is left as-is with no error; the caller gets back
// a value that still contains the sentinel, which downstream
// consumers can log or surface as a plugin-author bug. Fail-open
// matches how `mergeComponentDefaults` handles bad defaults JSON
// (see plugin_sidecars.go) — a plugin mistake must not break every
// dispatch for that component type.
//
// Mutates nothing — returns a fresh copy. Safe to call with a nil or
// empty map; returns the input shape unchanged.
//
// The `${parent:res.field}` token is NOT resolved here — it needs
// StackReference resolution with async Pulumi outputs. That's a
// follow-up slice of the same roadmap item
// (sc-components-defaults-template-interpolation, parent-output
// tokens section). Strings that still contain `${parent:…}` after
// this pass flow through to the dispatched template verbatim; a
// future slice adds the parent-resolver on top.
func InterpolateDefaults(merged map[string]any, ctx DefaultsContext) map[string]any {
	if len(merged) == 0 {
		return merged
	}
	return interpolateNode(merged, ctx).(map[string]any)
}

// interpolateNode is the recursive walker. Switches on node type:
// maps recurse, slices recurse element-wise, strings run through
// substituteString (which checks the sentinel), everything else
// passes through as-is.
func interpolateNode(node any, ctx DefaultsContext) any {
	switch v := node.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, el := range v {
			out[k] = interpolateNode(el, ctx)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, el := range v {
			out[i] = interpolateNode(el, ctx)
		}
		return out
	case string:
		return substituteString(v, ctx)
	default:
		return v
	}
}

// substituteString returns the input unchanged unless it begins with
// the DefaultsSentinel. For a sentinel string, substitutes known
// `${…}` tokens from the context and returns the resolved value;
// strips the sentinel on the way out so downstream readers see a
// plain string.
//
// `${parent:…}` tokens are deliberately left in place (sentinel
// retained) — they're resolved in a later pass with access to
// StackReference outputs. This lets us compose: the self-derived
// tokens resolve here; the parent tokens resolve in the parent pass.
func substituteString(s string, ctx DefaultsContext) string {
	if !strings.HasPrefix(s, DefaultsSentinel) {
		return s
	}
	expr := strings.TrimPrefix(s, DefaultsSentinel)
	resolvedSelf := substituteSelfTokens(expr, ctx)
	// If the result still has unresolved `${parent:…}` tokens, keep
	// the sentinel so the parent-resolution pass picks it up. If
	// nothing is left unresolved, drop the sentinel.
	if strings.Contains(resolvedSelf, "${parent:") {
		return DefaultsSentinel + resolvedSelf
	}
	return resolvedSelf
}

// substituteSelfTokens replaces every self-derived token in the
// expression with its context value. Unknown tokens (including
// `${parent:…}`, by design) are left in place.
//
// Chosen literal-replace strategy over regex: the token set is small
// and fixed, and literal replacement is O(n*tokens) which is fine for
// strings of this size. Regex would also need to match the same
// token set, with the added hazard of partial-match surprises on
// nested braces.
func substituteSelfTokens(expr string, ctx DefaultsContext) string {
	replacements := []struct {
		tok, val string
	}{
		{"${name}", ctx.Name},
		{"${env}", ctx.Env},
		{"${stack}", ctx.Stack},
		{"${template.name}", ctx.TemplateName},
	}
	for _, r := range replacements {
		if r.val == "" {
			continue
		}
		expr = strings.ReplaceAll(expr, r.tok, r.val)
	}
	// Template-config scalar tokens: `${template.<key>}` reads
	// ctx.TemplateConfig[<key>] when it's a string. A non-string
	// value is left unresolved (treated as missing).
	expr = substituteTemplateConfigTokens(expr, ctx.TemplateConfig)
	return expr
}

// substituteTemplateConfigTokens handles `${template.<key>}` for every
// scalar string key in TemplateConfig. The `.name` form is handled by
// the literal-token path above (it comes from TemplateName, not
// TemplateConfig). Non-string config values are intentionally skipped
// — a `${template.port}` default that resolved to an int would
// surprise a plugin author writing a string template.
func substituteTemplateConfigTokens(expr string, cfg map[string]any) string {
	if cfg == nil {
		return expr
	}
	for k, v := range cfg {
		s, ok := v.(string)
		if !ok {
			continue
		}
		expr = strings.ReplaceAll(expr, "${template."+k+"}", s)
	}
	return expr
}

// RefuseSentinelInOperatorRaw walks an operator's raw map (the
// `.Raw` field on a Component resource's config) and returns a non-
// nil error if any string value starts with the DefaultsSentinel.
// Guards the operator against accidentally activating the
// interpolation path with a copy-paste from a plugin default or a
// typo; also prevents a hostile operator config from faking a
// plugin-authored default value (the sentinel's effective role).
//
// Called from parse-time validation + from
// `dispatchComponentTemplate` as defense-in-depth. The error names
// the offending key path for diagnosability.
func RefuseSentinelInOperatorRaw(raw map[string]any) error {
	return refuseNode(raw, nil)
}

// refuseNode is the recursive walker for RefuseSentinelInOperatorRaw.
// `path` is the json-pointer-style key path accumulated down the
// tree; used only in the error message.
func refuseNode(node any, path []string) error {
	switch v := node.(type) {
	case map[string]any:
		for k, el := range v {
			if err := refuseNode(el, append(path, k)); err != nil {
				return err
			}
		}
	case []any:
		for i, el := range v {
			if err := refuseNode(el, append(path, "["+itoa(i)+"]")); err != nil {
				return err
			}
		}
	case string:
		if strings.HasPrefix(v, DefaultsSentinel) {
			return errors.Errorf(
				"operator-written value at %q uses reserved prefix %q — this prefix is reserved for plugin-authored defaults",
				strings.Join(path, "."), DefaultsSentinel,
			)
		}
	}
	return nil
}

// itoa is a tiny no-alloc int-to-string for the error path — faster
// than strconv.Itoa for small indexes and avoids the import weight.
// The caller passes array indexes 0..N; N is bounded by the operator's
// list size, so the tight loop fits in a stack buffer.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
