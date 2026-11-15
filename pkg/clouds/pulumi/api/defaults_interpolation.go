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

	// ParentResolver resolves `${parent:<resource>.<field>}` tokens
	// against the consuming stack's parent-stack exports. Invoked at
	// interpolation time; callers (SC core's dispatcher) supply a
	// closure that opens `sdk.NewStackReference` to the parent stack
	// and reads exports via `GetOutputDetails`, synchronously awaited
	// via `internals.UnsafeAwaitOutput`.
	//
	// Nil-safe: a nil resolver leaves `${parent:…}` tokens UNRESOLVED
	// — the sentinel is retained so (a) the dispatched template reads
	// the literal string + can log the gap + (b) a later resolver
	// pass (if we ever add one) can pick up where this run stopped.
	// Older plugin binaries continue to work.
	//
	// Returns ok=false when the resource or field is missing. The
	// interpolation keeps the token in place (fail-open) and logs via
	// the plugin's regular stream.
	ParentResolver func(resource, field string) (string, bool)

	// ParentListResolver is the list-shaped sibling of ParentResolver.
	// Resolves `${parent-list:<resource>.<field>}` tokens against
	// parent-stack exports that return a `[]string` (typically VPC
	// subnet lists, security-group lists, zone lists).
	//
	// A sentinel string containing EXACTLY `${parent-list:<res>.<field>}`
	// (nothing else after the sentinel prefix besides optional
	// whitespace) is REPLACED with the resolved `[]string` value at
	// its map-leaf — the walker swaps the string node for a `[]any`
	// list of strings. Any OTHER content around the token (a prefix,
	// a suffix, a second token) leaves the sentinel scalar unchanged
	// and keeps the token in place: there is no string-concat
	// semantics for a list.
	//
	// Nil-safe: a nil resolver leaves `${parent-list:…}` unresolved,
	// same as ParentResolver.
	ParentListResolver func(resource, field string) ([]string, bool)
}

// InterpolateDefaults walks the merged ComponentRaw map produced by
// DeepMergeComponentRaw and substitutes `${…}` tokens inside
// $$sc:-prefixed strings. Other values pass through unchanged.
//
// Resolves: `${name}` / `${env}` / `${stack}` / `${template.name}` /
// `${template.<key>}` from the DefaultsContext fields; and
// `${parent:<resource>.<field>}` from `ctx.ParentResolver` when
// supplied. Unknown tokens are left in place (fail-open — matches
// `mergeComponentDefaults` on bad defaults JSON).
//
// Mutates nothing — returns a fresh copy. Safe to call with a nil or
// empty map; returns the input shape unchanged.
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
//
// substituteString may return a non-string value (a `[]any` list,
// when a sole-content `${parent-list:…}` token substitutes a list
// into a map leaf). The walker stores whatever substituteString
// returns into its parent slot — a map leaf or a list element can
// hold any shape.
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
// plain string (or a `[]any` list, when a sole-content list token
// substitutes).
//
// Resolution order:
//  1. Self-derived tokens (`${name}` / `${env}` / `${stack}` /
//     `${template.*}`) via substituteSelfTokens.
//  2. `${parent:<resource>.<field>}` via ctx.ParentResolver, when
//     supplied. Each token resolves to the resolver's returned
//     string; a false ok leaves the token in place.
//  3. `${parent-list:<resource>.<field>}` via ctx.ParentListResolver,
//     when supplied AND the expression's remaining content is EXACTLY
//     one list token with optional whitespace. The return TYPE
//     widens from string to `[]any` for this case — the walker
//     stores the list at the map leaf. Any other content around the
//     token keeps the string unchanged (there is no concat
//     semantics for a list).
//
// Return type is `any` to cover the list case; callers (interpolateNode)
// pass the value through to the parent slot untyped.
//
// Sentinel retention: only when a `${parent:…}` or `${parent-list:…}`
// token remains after all passes — a signal that a LATER resolver
// pass could still substitute (older plugins without ParentResolver
// wired, or a resolver that returned ok=false for a known-missing
// output). Unknown non-parent tokens (plugin typos like `${nam}`)
// drop the sentinel so the dispatched template sees the raw literal
// without SC-core-assist confusion.
func substituteString(s string, ctx DefaultsContext) any {
	if !strings.HasPrefix(s, DefaultsSentinel) {
		return s
	}
	expr := strings.TrimPrefix(s, DefaultsSentinel)
	resolved := substituteSelfTokens(expr, ctx)
	if ctx.ParentResolver != nil {
		resolved = substituteParentTokens(resolved, ctx.ParentResolver)
	}
	// Sole-content list-token path: the entire remaining expression
	// is one `${parent-list:<res>.<field>}` token. Replaces the
	// string leaf with a `[]any` list of strings.
	if ctx.ParentListResolver != nil {
		if list, ok := substituteSoleParentListToken(resolved, ctx.ParentListResolver); ok {
			return list
		}
	}
	// Retain the sentinel only when a `${parent:…}` or unresolved
	// `${parent-list:…}` token survived resolution — a later pass
	// may complete it.
	if strings.Contains(resolved, "${parent:") || strings.Contains(resolved, "${parent-list:") {
		return DefaultsSentinel + resolved
	}
	return resolved
}

// substituteSoleParentListToken recognizes the single-token-only
// list form and returns the resolved `[]any` list.
//
// The input expression is the post-self-token, post-scalar-parent
// value WITH the sentinel already stripped. Matches when, after
// trimming surrounding whitespace, the expression is exactly
// `${parent-list:<resource>.<field>}` — a single dot-separated pair
// inside the braces. Any extra content around the token (a prefix,
// a suffix, or a second token) returns (nil, false): list tokens
// have no concat semantics with surrounding text.
//
// Returns ok=false when the resolver cannot satisfy the lookup
// (unknown resource/field) — the caller retains the sentinel so a
// later pass may complete it. Also ok=false when the expression
// doesn't match the sole-token shape, which is the common case for
// scalar-only defaults.
//
// The returned `[]any` is a fresh slice; the resolver's `[]string`
// is copied element-by-element to widen the element type.
func substituteSoleParentListToken(expr string, resolver func(resource, field string) ([]string, bool)) ([]any, bool) {
	const prefix = "${parent-list:"
	const suffix = "}"
	trimmed := strings.TrimSpace(expr)
	if !strings.HasPrefix(trimmed, prefix) || !strings.HasSuffix(trimmed, suffix) {
		return nil, false
	}
	body := trimmed[len(prefix) : len(trimmed)-len(suffix)]
	// Guard against nested `${...}` by rejecting any `${` in the body.
	if strings.Contains(body, "${") {
		return nil, false
	}
	dot := strings.Index(body, ".")
	if dot <= 0 || dot == len(body)-1 {
		return nil, false
	}
	resource, field := body[:dot], body[dot+1:]
	list, ok := resolver(resource, field)
	if !ok {
		return nil, false
	}
	out := make([]any, len(list))
	for i, v := range list {
		out[i] = v
	}
	return out, true
}

// substituteParentTokens replaces every `${parent:<resource>.<field>}`
// token in the expression with the resolver's string value for that
// pair. Unresolved tokens (resolver returns ok=false) stay in place.
//
// Parser is deliberately small — the token shape is fixed. We scan
// for `${parent:`, find the matching `}`, split on the first `.`, and
// call the resolver. Nested `${...}` inside a parent spec is not
// supported (the grammar doesn't define it); nested braces end the
// scan at the first `}` we see, which keeps the parser one-pass.
func substituteParentTokens(expr string, resolver func(resource, field string) (string, bool)) string {
	const prefix = "${parent:"
	var out strings.Builder
	out.Grow(len(expr))
	i := 0
	for i < len(expr) {
		j := strings.Index(expr[i:], prefix)
		if j < 0 {
			out.WriteString(expr[i:])
			break
		}
		// Copy the segment before the token.
		out.WriteString(expr[i : i+j])
		start := i + j + len(prefix)
		end := strings.Index(expr[start:], "}")
		if end < 0 {
			// No closing brace — the token is malformed. Leave it
			// in place (fail-open) and stop scanning.
			out.WriteString(expr[i+j:])
			break
		}
		body := expr[start : start+end]
		dot := strings.Index(body, ".")
		if dot < 0 {
			// No field — keep the token as-is.
			out.WriteString(expr[i+j : start+end+1])
			i = start + end + 1
			continue
		}
		resource, field := body[:dot], body[dot+1:]
		value, ok := resolver(resource, field)
		if !ok {
			// Resolver couldn't find it — keep the token.
			out.WriteString(expr[i+j : start+end+1])
			i = start + end + 1
			continue
		}
		out.WriteString(value)
		i = start + end + 1
	}
	return out.String()
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
