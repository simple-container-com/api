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
