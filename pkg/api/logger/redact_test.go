// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package logger

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// testKeys returns real private keys in the PEM forms a credential can carry.
func testKeys(t *testing.T) map[string]string {
	t.Helper()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	if err != nil {
		t.Fatal(err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecDER, err := x509.MarshalECPrivateKey(ecKey)
	if err != nil {
		t.Fatal(err)
	}
	_, edKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	openssh, err := ssh.MarshalPrivateKey(edKey, "")
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{
		"pkcs8 (service-account JSON)": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})),
		"rsa":                          string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsaKey)})),
		"ec":                           string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: ecDER})),
		"openssh":                      string(pem.EncodeToMemory(openssh)),
	}
}

// fragments are pieces of the key body; none may survive redaction.
func fragments(key string) []string {
	var out []string
	for _, line := range strings.Split(key, "\n") {
		if len(line) >= 24 && !strings.HasPrefix(line, "-----") {
			out = append(out, line[4:24])
		}
	}
	return out
}

func serviceAccountJSON(key string) string {
	b, _ := json.Marshal(map[string]string{
		"type":           "service_account",
		"client_email":   "deployer@project.iam.gserviceaccount.com",
		"private_key_id": "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c",
		"private_key":    key,
	})
	return string(b)
}

func TestRedactRemovesPrivateKeysInEveryEncoding(t *testing.T) {
	for kind, key := range testKeys(t) {
		saJSON := serviceAccountJSON(key)
		escapedOnce, _ := json.Marshal(saJSON)
		diffLine := fmt.Sprintf("      - private_key                : %q\n      - private_key_id             : \"0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c\"\n", key)
		summary, _ := json.Marshal(map[string]string{"stackName": "organization/app/app--staging", "summary": "Previewing update:\n" + diffLine})
		for encoding, text := range map[string]string{
			"raw PEM":                        "credentials:\n" + key + "done",
			"service-account JSON":           saJSON,
			"JSON inside a JSON string":      string(escapedOnce),
			"Pulumi diff line":               diffLine,
			"preview summary logged as JSON": string(summary),
			"cut off before END":             "x " + key[:len(key)/2],
		} {
			got := Redact(text)
			for _, frag := range fragments(key) {
				if strings.Contains(got, frag) {
					t.Errorf("%s / %s: key material %q survived:\n%s", kind, encoding, frag, got)
					break
				}
			}
			if strings.Contains(got, "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c") {
				t.Errorf("%s / %s: private_key_id survived", kind, encoding)
			}
			if !strings.Contains(got, redacted) {
				t.Errorf("%s / %s: no redaction marker:\n%s", kind, encoding, got)
			}
		}
	}
}

// Public keys, fingerprints and ordinary output are left alone.
func TestRedactLeavesOtherTextAlone(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pubDER, _ := x509.MarshalPKIXPublicKey(&rsaKey.PublicKey)
	pub := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	for _, text := range []string{
		pub,
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGfakefakefakefakefakefakefakefakefake user@host",
		"client_email: deployer@project.iam.gserviceaccount.com",
		"Terraform is using this identity: deployer@project.iam.gserviceaccount.com",
		"Resources: 3 to update, 12 unchanged",
		"commit 0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c",
	} {
		if got := Redact(text); got != text {
			t.Errorf("changed:\n%s\n->\n%s", text, got)
		}
	}
}

// The logger is the one path every SC log line takes.
func TestLoggerRedactsWhatItPrints(t *testing.T) {
	key := testKeys(t)["pkcs8 (service-account JSON)"]
	out := captureStdout(t, func() {
		New().Info(context.Background(), "Preview summary: \n%s", serviceAccountJSON(key))
	})
	for _, frag := range fragments(key) {
		if strings.Contains(out, frag) {
			t.Fatalf("the logger printed key material:\n%s", out)
		}
	}
	if !strings.Contains(out, "Preview summary") || !strings.Contains(out, "deployer@project.iam.gserviceaccount.com") {
		t.Errorf("the rest of the line was lost:\n%s", out)
	}
}

func TestRedactingWriter(t *testing.T) {
	key := testKeys(t)["rsa"]
	var buf bytes.Buffer
	n, err := RedactingWriter(&buf).Write([]byte("Error: " + key))
	if err != nil || n != len("Error: "+key) {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if strings.Contains(buf.String(), fragments(key)[0]) || !strings.HasPrefix(buf.String(), "Error: ") {
		t.Errorf("got:\n%s", buf.String())
	}
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// The shapes reviewers found leaking: base64 keys and credentials, PGP blocks,
// key tails without their BEGIN line, multiply escaped ids and CRLF keys.
func TestRedactRemovesOtherShapes(t *testing.T) {
	key := testKeys(t)["rsa"]
	lines := strings.Split(strings.TrimSpace(key), "\n")
	tail := strings.Join(lines[len(lines)-4:], "\n")
	pgp := "-----BEGIN PGP PRIVATE KEY BLOCK-----\n\n" + strings.Join(lines[1:len(lines)-1], "\n") + "\n-----END PGP PRIVATE KEY BLOCK-----\n"
	saJSON := serviceAccountJSON(key)
	twice, _ := json.Marshal(saJSON)
	thrice, _ := json.Marshal(string(twice))
	for name, text := range map[string]string{
		"kubeconfig client-key-data":   "users:\n- name: admin\n  user:\n    client-key-data: " + b64(key) + "\n",
		"quoted client-key-data diff":  fmt.Sprintf("      - client-key-data: %q\n", b64(key)),
		"base64 after an escaped \\n":  `kubeconfig: "users:\n    client-key-data:\n` + b64(key) + `"`,
		"base64 service-account JSON":  "credentials: " + b64(saJSON),
		"PGP block":                    pgp,
		"tail without BEGIN":           "    ...\n" + tail + "\n",
		"escaped tail without BEGIN":   strings.ReplaceAll("...\n"+tail, "\n", `\n`),
		"twice-escaped tail":           strings.ReplaceAll("...\n"+tail, "\n", `\\n`),
		"tail in a line diff":          "  ~ value: (\n      ...\n    - " + strings.ReplaceAll(tail, "\n", "\n    - ") + "\n    )\n",
		"line cut mid-way":             "  + private_key: \"" + strings.Join(lines[:6], "\n") + "\n" + lines[6][:40] + "...\"\n",
		"JSON escaped three times":     string(thrice),
		"CRLF line endings":            strings.ReplaceAll(key, "\n", "\r\n"),
		"Pulumi error with the stdout": "failed to run update: exit status 255\ncode: 255\nstdout: Previewing update:\n  ~ credentials: " + fmt.Sprintf("%q", saJSON) + "\nstderr: \n",
	} {
		got := Redact(text)
		// Base64 never contains the PEM body verbatim, so look for its own slices.
		frags := append(fragments(key), b64(key)[100:130], b64(saJSON)[300:330])
		for _, frag := range frags {
			if strings.Contains(got, frag) {
				t.Errorf("%s: key material %q survived:\n%s", name, frag, got)
				break
			}
		}
		if strings.Contains(got, "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c") {
			t.Errorf("%s: private_key_id survived:\n%s", name, got)
		}
	}
}

// Redaction removes the key and nothing after it.
func TestRedactKeepsTheTextAroundKeys(t *testing.T) {
	keys := testKeys(t)
	a, b := keys["rsa"], keys["ec"]
	for name, tc := range map[string]struct{ text, keep string }{
		"between two keys":         {a + "\nResources: 3 to update\n" + b, "Resources: 3 to update"},
		"after a cut-off key":      {"  + private_key: \"" + a[:len(a)/2] + "...\n  ~ image: \"app:v2\"\nResources: 3 to update", `~ image: "app:v2"` + "\nResources: 3 to update"},
		"after a BEGIN in a hint":  {"expected -----BEGIN RSA PRIVATE KEY----- header\nResources: 3 to update\nurl=https://x", "header\nResources: 3 to update\nurl=https://x"},
		"after an escaped cut key": {strings.ReplaceAll(a[:len(a)/2], "\n", `\n`) + `...", "image": "app:v2"`, `", "image": "app:v2"`},
	} {
		got := Redact(tc.text)
		if !strings.Contains(got, tc.keep) {
			t.Errorf("%s: lost %q:\n%s", name, tc.keep, got)
		}
		if !strings.Contains(got, redacted) {
			t.Errorf("%s: nothing redacted:\n%s", name, got)
		}
	}
}

// Base64 that is not a private key (a CA certificate, an image digest) stays.
func TestRedactLeavesOtherBase64Alone(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pubDER, _ := x509.MarshalPKIXPublicKey(&rsaKey.PublicKey)
	ca := b64(string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: pubDER})))
	for _, text := range []string{
		"certificate-authority-data: " + ca,
		"token: " + b64(strings.Repeat("not a key ", 30)),
	} {
		if got := Redact(text); got != text {
			t.Errorf("changed:\n%s\n->\n%s", text, got)
		}
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) / 2, nil }

func TestRedactingWriterReportsShortWrites(t *testing.T) {
	if _, err := RedactingWriter(shortWriter{}).Write([]byte("hello")); err == nil {
		t.Error("a short write was reported as complete")
	}
}
