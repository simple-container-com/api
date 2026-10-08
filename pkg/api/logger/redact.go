// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package logger

import (
	"io"
	"regexp"
)

// Private key material must never reach a log. It gets there without anyone
// asking: a cloud provider configured with a service-account key keeps that key
// as a plain input in Pulumi state, and the preview diff of a stack whose
// provider changes credentials prints the old input in full, private key
// included. GitHub masks only the secrets a workflow references, not values read
// from state, so the key would sit in the Actions log for anyone who can read it.
var (
	// A PEM private key of any kind (PKCS#8, RSA, EC, OpenSSH, encrypted), with
	// real or escaped (\n, \\n) line breaks. A block cut off before its END line
	// is redacted to the end of the text.
	privateKeyBlock = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----(?s:.*?)(?:-----END [A-Z0-9 ]*PRIVATE KEY-----|\z)`)
	// The id of a service-account key, as JSON or as a Pulumi diff renders it.
	privateKeyID = regexp.MustCompile(`(private_key_id\\?"?\s*[:=]\s*\\?"?)[0-9a-f]{40}`)
)

const redacted = "[redacted]"

// Redact returns s with private key material replaced.
func Redact(s string) string {
	s = privateKeyBlock.ReplaceAllString(s, "-----"+redacted+" PRIVATE KEY-----")
	return privateKeyID.ReplaceAllString(s, "${1}"+redacted)
}

type redactingWriter struct{ w io.Writer }

// RedactingWriter wraps w so every write is passed through Redact. A key split
// across two writes is not caught; use it for messages written whole.
func RedactingWriter(w io.Writer) io.Writer { return redactingWriter{w: w} }

func (r redactingWriter) Write(p []byte) (int, error) {
	if _, err := io.WriteString(r.w, Redact(string(p))); err != nil {
		return 0, err
	}
	return len(p), nil
}
