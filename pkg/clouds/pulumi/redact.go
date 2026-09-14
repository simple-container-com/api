// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"regexp"
	"strings"
)

// redactedValue is what a credential is replaced with. It matches the engine's
// own rendering of a secret, so a redacted summary reads like a preview of a
// stack that never had the problem in the first place.
const redactedValue = "[secret]"

// credentialKeys are the property names that carry a credential, in every
// spelling the engine may print one: snake_case inside a decoded service
// account key, camelCase for a resource property, hyphenated inside a
// kubeconfig. Longest first, so `private_key_id` is not read as `private_key`
// followed by junk.
var credentialKeys = []string{
	`private[_-]?key[_-]?id`,
	`private[_-]?key`,
	`client[_-]?key[_-]?data`,
	`secret[_-]?access[_-]?key`,
	`secret[_-]?key`,
	`client[_-]?secret`,
	`refresh[_-]?token`,
	`access[_-]?token`,
	`auth[_-]?token`,
	`id[_-]?token`,
	`api[_-]?token`,
	`api[_-]?key`,
	`password`,
	`token`,
	`auth`,
}

var (
	// A PEM private key, whether printed across real newlines or inside a
	// string whose newlines the engine escaped. The body is bounded to what
	// base64 and line breaks can produce: an unbounded `.*?` between the
	// markers would let an orphaned BEGIN swallow every line up to an END
	// marker belonging to something else, deleting the diff in between.
	pemPrivateKeyRe = regexp.MustCompile(
		`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----(?:[A-Za-z0-9+/=\s]|\\[rn])*?-----END [A-Z0-9 ]*PRIVATE KEY-----`,
	)

	// A BEGIN marker with no END, which is what truncated or interleaved
	// output produces. Each chunk consumed after the marker has to be a base64
	// run, so the sweep stops at the first line that is prose or another
	// property rather than running to the end of the summary.
	pemOrphanRe = regexp.MustCompile(
		`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----(?:[\s+]|\\[rn])*(?:[A-Za-z0-9+/=]{20,}(?:[\s+]|\\[rn])*)*`,
	)

	// `key: value`, in the shapes the engine actually emits. A string property
	// that parses as JSON or YAML is decoded and re-printed as an object with
	// bare keys, so quotes around the key are optional; a line in a diff
	// carries an operation prefix; and a changed value is printed as
	// `old => new`, where the new one is the credential now in use.
	credentialFieldRe = regexp.MustCompile(
		`(?i)((?:^|\n|\\n|[{,])[ \t]*[-+~*<>]?[ \t]*(?:\\?")?(?:` + strings.Join(credentialKeys, "|") +
			`)(?:\\?")?[ \t]*:[ \t]*)(` + redactValuePattern + `)((?:[ \t]*=>[ \t]*)(?:` + redactValuePattern + `))?`,
	)
)

// redactValuePattern matches a single rendered value. A value sitting inside
// another string carries escaped quotes and has to be matched as its own case:
// treating the escape as "an optional backslash" lets the body run through the
// closing quote and consume the rest of the blob. The bare branch stops at a
// backslash so it cannot swallow an escaped line break and the key that
// follows it.
//
// It deliberately does not match a value that opens a structure -- `(json) {`,
// `{`, `[`, or a YAML block scalar -- because those are containers whose own
// leaves this same rule redacts one by one; matching them would replace the
// opening brace and leave the contents behind.
const redactValuePattern = `\\"(?:[^\\]|\\[^"])*\\"|"(?:\\.|[^"\\])*"|\$\{[^}\s]*\}|[^\s"(\[{|>\\][^\s,}\]\\]*`

// redactCredentials removes credential-shaped values from text that Simple
// Container prints or returns.
//
// This is defence in depth, not the fix: a credential reaches a summary
// because something handed the engine a plaintext input, and the repair for
// that is to mark the input secret where it is set (see pApi.SecretString).
// But a summary is a raw dump of engine output that ends up in CI logs, so
// anything credential-shaped that survives upstream is worth removing on the
// way out. The cost of a false positive is one masked diff line; the cost of a
// miss is a rotation.
//
// Two limits worth knowing. The patterns are written against uncoloured
// output, which is what the automation API produces today because Simple
// Container never asks for colour; adding it would put escape sequences
// between the operation prefix and the key. And a credential rendered as a
// YAML block scalar keeps its body on following lines, which is not reachable
// from a line-oriented rule -- in practice the engine decodes a kubeconfig and
// prints it as a structure, so the leaves land back under the rule above.
func redactCredentials(summary string) string {
	res := pemPrivateKeyRe.ReplaceAllLiteralString(summary, redactedValue)
	res = pemOrphanRe.ReplaceAllLiteralString(res, redactedValue)
	return credentialFieldRe.ReplaceAllStringFunc(res, func(match string) string {
		groups := credentialFieldRe.FindStringSubmatch(match)
		head, value, rotated := groups[1], groups[2], groups[3]
		// Keep whatever quoting the value had, including the escaped kind a
		// value nested in another string carries, so a redacted summary is
		// still the shape it was: JSON that parsed before still parses.
		masked := quoteOf(value) + redactedValue + quoteOf(value)
		if rotated != "" {
			return head + masked + " => " + masked
		}
		return head + masked
	})
}

func quoteOf(value string) string {
	switch {
	case strings.HasPrefix(value, `\"`):
		return `\"`
	case strings.HasPrefix(value, `"`):
		return `"`
	default:
		return ""
	}
}
