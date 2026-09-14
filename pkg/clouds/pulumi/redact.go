// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"regexp"
	"strings"

	"github.com/pkg/errors"
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
//
// They are matched as a SUFFIX of the key, because the names this codebase and
// its consumers actually use are qualified: `rootPassword`,
// `registryPassword`, `AWS_SECRET_ACCESS_KEY`, `COSIGN_PASSWORD`. Anchoring at
// the start of the key missed every one of them. The suffix stays anchored to
// the colon, so `passwordPolicy` and `tokenSecretName` are still left alone.
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
	`access[_-]?key[_-]?id`,
	`auth[_-]?header`,
	`credentials`,
	`kubeconfig`,
	`password`,
	`token`,
	`auth`,
}

var (
	// Colour escapes. Simple Container never asks the engine for colour, so
	// today this matches nothing; it is here so that adding colour later does
	// not silently defeat every rule below by putting an escape sequence
	// between the diff marker and the key.
	ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

	// A PEM private key, whether printed across real newlines or inside a
	// string whose newlines the engine escaped. The body is bounded to what
	// base64 and line breaks can produce: an unbounded `.*?` between the
	// markers would let an orphaned BEGIN swallow every line up to an END
	// marker belonging to something else, deleting the diff in between.
	pemPrivateKeyRe = regexp.MustCompile(
		`-----BEGIN [A-Z0-9 ]*PRIVATE KEY(?: BLOCK)?-----(?:[A-Za-z0-9+/=\s]|\\[rn])*?-----END [A-Z0-9 ]*PRIVATE KEY(?: BLOCK)?-----`,
	)

	// A BEGIN marker, used to find an orphaned one: output that was truncated
	// or interleaved can show a key's opening marker with no closing one.
	pemBeginRe = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY(?: BLOCK)?-----`)

	// A line holding nothing but key body: indentation, an optional diff
	// marker, then base64. Anything else -- a property name, a colon, a quote
	// -- ends the sweep.
	pemBodyLineRe = regexp.MustCompile(`^[ \t]*[-+~*]?[ \t]*(?:[A-Za-z0-9+/=]|\\[rn])+[ \t]*$`)

	// `key: value`, in the shapes the engine actually emits. A string property
	// that parses as JSON or YAML is decoded and re-printed as an object with
	// bare keys, so quotes around the key are optional; a line in a diff
	// carries an operation prefix; and a changed value is printed as
	// `old => new`, where the new one is the credential now in use.
	credentialFieldRe = regexp.MustCompile(
		`(?i)((?:^|\n|\\n|[{,]|[ \t])[ \t]*[-+~*<>]?[ \t]*(?:\\?")?[A-Za-z0-9_.-]*?(?:` + strings.Join(credentialKeys, "|") +
			`)(?:\\?")?[ \t]*:[ \t]*)(` + redactValuePattern + `)((?:[ \t]*=>[ \t]*)(?:` + redactValuePattern + `))?`,
	)
)

// redactValuePattern matches a single rendered value. The engine's own markers
// (`[secret]`, `[unknown]`) are one of the alternatives: a rotation prints
// `old => new`, and if the old side is a marker the pattern has to match it, or
// the whole rule misses the line and the new value -- the plaintext one, the
// one worth having -- is printed in the clear.
//
// A value sitting inside
// another string carries escaped quotes and has to be matched as its own case:
// treating the escape as "an optional backslash" lets the body run through the
// closing quote and consume the rest of the blob. The bare branch stops at a
// backslash so it cannot swallow an escaped line break and the key that
// follows it.
//
// Every branch is bounded to a single line. The engine always balances quotes
// within a line (a multi-line value is rendered with its newlines escaped), so
// an unterminated quote means truncated output -- and a pattern that ran on to
// the next quote would swallow the diff lines in between.
//
// It deliberately does not match a value that opens a structure -- `(json) {`,
// `{`, `[`, or a YAML block scalar -- because those are containers whose own
// leaves this same rule redacts one by one; matching them would replace the
// opening brace and leave the contents behind.
const redactValuePattern = `\[[a-z]+\]|\\"(?:[^\\\n]|\\[^"\n])*\\"|"(?:\\[^\n]|[^"\\\n])*"|\$\{[^}\s]*\}|[^\s"(\[{|>\\][^\s,}\]\\]*`

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
	res := ansiRe.ReplaceAllLiteralString(summary, "")
	res = pemPrivateKeyRe.ReplaceAllLiteralString(res, redactedValue)
	res = redactOrphanedPEM(res)
	return credentialFieldRe.ReplaceAllStringFunc(res, func(match string) string {
		groups := credentialFieldRe.FindStringSubmatch(match)
		if groups == nil {
			return match
		}
		head, value, rotated := groups[1], groups[2], groups[3]
		if rotated == "" && isProse(value) {
			return match
		}
		// Keep whatever quoting each value had, including the escaped kind a
		// value nested in another string carries, so a redacted summary is
		// still the shape it was: JSON that parsed before still parses.
		out := head + mask(value)
		if rotated != "" {
			out += " => " + mask(rotatedValue(rotated))
		}
		return out
	})
}

// redactOrphanedPEM removes a key whose closing marker never arrived, which is
// what truncated or interleaved output produces.
//
// This is a line scan rather than one more pattern because the pattern has to
// know where to stop, and "stop at the first line that is not key material" is
// a statement about lines. Expressed as a regex it kept walking past the key
// into the diff below and deleting the property names there, which is worse
// than the leak it closes: a masked value is visible, a deleted line is not.
func redactOrphanedPEM(summary string) string {
	if !pemBeginRe.MatchString(summary) {
		return summary
	}
	lines := strings.Split(summary, "\n")
	for i := 0; i < len(lines); i++ {
		marker := pemBeginRe.FindStringIndex(lines[i])
		if marker == nil {
			continue
		}
		// From the marker to the end of its line: a key rendered inside
		// another string has its whole body on this one line.
		lines[i] = lines[i][:marker[0]] + redactedValue
		for i+1 < len(lines) && pemBodyLineRe.MatchString(lines[i+1]) {
			lines = append(lines[:i+1], lines[i+2:]...)
		}
	}
	return strings.Join(lines, "\n")
}

// isProse reports whether a bare value reads as a word rather than as a
// credential. Provider diagnostics run through this function, and
// `auth: could not refresh credentials` must not become
// `auth: [secret] not refresh credentials`: mangling the sentence costs
// debugging time at exactly the wrong moment. The trade is that a bare,
// all-letter, short credential survives; quoted, it does not.
func isProse(value string) bool {
	if value == "" || len(value) >= 16 {
		return false
	}
	for _, r := range value {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

func mask(value string) string {
	return quoteOf(value) + redactedValue + quoteOf(value)
}

// rotatedValue strips the `=>` that joins the two halves of a changed value,
// so the new value keeps its own quoting rather than the old value's.
func rotatedValue(rotated string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rotated), "=>"))
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

// redactError removes credential-shaped text from an error's message.
//
// The automation API embeds the whole engine stdout and stderr in the error it
// returns from a failed preview or update (`autoError.Error()`), and those
// errors travel up to the CLI unchanged. So the failure path printed what the
// success path no longer does, which is the worse half: a provider that cannot
// configure itself is exactly where a credential gets echoed back.
//
// The original error is returned untouched when there was nothing to remove,
// so wrapping costs no type information in the ordinary case; only an error
// that actually carried a credential is replaced by its redacted text.
func redactError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	redacted := redactCredentials(msg)
	if redacted == msg {
		return err
	}
	return errors.New(redacted)
}
