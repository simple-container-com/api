// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"regexp"
)

// redactedValue is what a credential is replaced with. It matches the engine's
// own rendering of a secret, so a redacted summary reads like a preview of a
// stack that never had the problem in the first place.
const redactedValue = "[secret]"

var (
	// A PEM private key, whether printed across real newlines or inside a JSON
	// string where the newlines are escaped. Both forms are the same byte range
	// between the two markers, so one pattern covers them.
	pemPrivateKeyRe = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)

	// Credential-carrying fields of a service-account key or a cloud auth blob,
	// in JSON that may itself be escaped inside another JSON string.
	jsonCredentialFieldRe = regexp.MustCompile(
		`(?i)(\\?"(?:private_key|private_key_id|client_secret|refresh_token|access_token|secret_access_key|api_token)\\?"\s*:\s*)\\?"(?:[^"\\]|\\.)*?\\?"`,
	)

	// Credential-carrying fields of a kubeconfig, again either on its own line
	// or with the line breaks escaped.
	kubeconfigCredentialFieldRe = regexp.MustCompile(
		`(?i)((?:^|\n|\\n)[ \t-]*(?:client-key-data|token|password)[ \t]*:[ \t]*)(?:"[^"\n]*"|[^\s"\\]+)`,
	)
)

// redactCredentials removes credential-shaped values from text that Simple
// Container prints or returns.
//
// This is defence in depth, not the fix: credentials reach a summary because
// something handed the engine a plaintext input, and the repair for that is to
// mark the input secret where it is set. But a summary is a raw dump of engine
// output that ends up in CI logs, so anything credential-shaped that survives
// upstream is worth removing on the way out — the cost of a false positive is a
// masked diff line, the cost of a miss is a rotation.
func redactCredentials(summary string) string {
	if summary == "" {
		return summary
	}
	res := pemPrivateKeyRe.ReplaceAllLiteralString(summary, redactedValue)
	res = jsonCredentialFieldRe.ReplaceAllString(res, `${1}"`+redactedValue+`"`)
	res = kubeconfigCredentialFieldRe.ReplaceAllString(res, `${1}`+redactedValue)
	return res
}
