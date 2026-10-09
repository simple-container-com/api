// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package logger

import (
	"encoding/base64"
	"io"
	"regexp"
	"strings"
)

// Private key material must never reach a log. It gets there without anyone
// asking: a cloud provider configured with a service-account key keeps that key
// as a plain input in Pulumi state, and the preview diff of a stack whose
// provider changes credentials prints the old input in full, private key
// included. GitHub masks only the secrets a workflow references, not values read
// from state, so the key would sit in the Actions log for anyone who can read it.
var (
	// A PEM or PGP private key of any kind (PKCS#8, RSA, EC, OpenSSH, encrypted),
	// with real or escaped (\n, \\n) line breaks. A block cut off before its END
	// line is redacted up to the first text that cannot be part of a key body.
	privateKeyBlock = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY(?: BLOCK)?-----(?:(?s:.*?)-----END [A-Z0-9 ]*PRIVATE KEY(?: BLOCK)?-----|` + keyBody + `)`)
	// The end of a key whose BEGIN line was cut off, as a truncated message or a
	// diff that shows only the last lines of a changed value leaves it.
	privateKeyTail = regexp.MustCompile(`[A-Za-z0-9+/=]{16,}` + keyBody + keySep + `-----END [A-Z0-9 ]*PRIVATE KEY(?: BLOCK)?-----`)
	// The id of a service-account key, as JSON (escaped any number of times) or
	// as a Pulumi diff renders it.
	privateKeyID = regexp.MustCompile(`(private_key_id(?:\\*")?\s*[:=]\s*(?:\\*")?)[0-9a-f]{40}`)
	// A long base64 run; it is redacted when it decodes to a private key or to
	// credentials that hold one (a kubeconfig client-key-data, a base64
	// service-account JSON). Base64 wrapped over several lines is not caught.
	base64Run = regexp.MustCompile(`[A-Za-z0-9+/]{100,}={0,2}`)
)

const (
	// keySep is what separates the lines of a key body: whitespace, escaped line
	// breaks, and the quotes and diff markers Pulumi puts around lines.
	keySep = `(?:\s|\\+[rnt]|["+~|-])*`
	// keyBody is a run of base64 lines of key-like length.
	keyBody  = `(?:` + keySep + `[A-Za-z0-9+/=]{16,})*`
	redacted = "[redacted]"
)

// Redact returns s with private key material replaced.
func Redact(s string) string {
	s = privateKeyBlock.ReplaceAllString(s, "-----"+redacted+" PRIVATE KEY-----")
	s = privateKeyTail.ReplaceAllString(s, redacted+" -----END PRIVATE KEY-----")
	s = privateKeyID.ReplaceAllString(s, "${1}"+redacted)
	return base64Run.ReplaceAllStringFunc(s, func(run string) string {
		if holdsPrivateKey(run) {
			return redacted
		}
		return run
	})
}

// holdsPrivateKey decodes run, also from the next three offsets in case the run
// starts with a letter of an escape such as \n.
func holdsPrivateKey(run string) bool {
	run = strings.TrimRight(run, "=")
	for skip := 0; skip < 4 && skip < len(run); skip++ {
		decoded, err := base64.RawStdEncoding.DecodeString(run[skip : skip+(len(run)-skip)/4*4])
		if err != nil {
			continue
		}
		if strings.Contains(string(decoded), "PRIVATE KEY") {
			return true
		}
	}
	return false
}

type redactingWriter struct{ w io.Writer }

// RedactingWriter wraps w so every write is passed through Redact. Each write is
// redacted on its own, so a key split across two writes is not caught: use it
// for messages written whole (cobra's error output), never for a stream.
func RedactingWriter(w io.Writer) io.Writer { return redactingWriter{w: w} }

func (r redactingWriter) Write(p []byte) (int, error) {
	s := Redact(string(p))
	n, err := io.WriteString(r.w, s)
	if err == nil && n < len(s) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return 0, err
	}
	return len(p), nil
}
