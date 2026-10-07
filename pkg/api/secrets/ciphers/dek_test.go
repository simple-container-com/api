// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package ciphers

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"strings"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
)

func TestEnvelopeAEADRoundTripAndBinding(t *testing.T) {
	key, err := GenerateDEK()
	if err != nil || len(key) != DEKSize {
		t.Fatalf("GenerateDEK = %d bytes, %v", len(key), err)
	}
	other, _ := GenerateDEK()
	if bytes.Equal(key, other) {
		t.Fatal("two data keys are equal")
	}

	blob, err := SealAEAD(key, []byte("value"), []byte("scope\x00key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidEnvelopeBlob(blob); err != nil {
		t.Errorf("a sealed blob fails the shape check: %v", err)
	}
	again, _ := SealAEAD(key, []byte("value"), []byte("scope\x00key"))
	if bytes.Equal(blob, again) {
		t.Error("two seals of one value are identical: the nonce is not random")
	}
	pt, err := OpenAEAD(key, blob, []byte("scope\x00key"))
	if err != nil || string(pt) != "value" {
		t.Fatalf("OpenAEAD = %q, %v", pt, err)
	}

	if _, err := OpenAEAD(key, blob, []byte("other\x00key")); err == nil {
		t.Error("opened under different associated data")
	}
	if _, err := OpenAEAD(other, blob, []byte("scope\x00key")); err == nil {
		t.Error("opened with another key")
	}
	tampered := append([]byte(nil), blob...)
	tampered[len(tampered)-1] ^= 1
	if _, err := OpenAEAD(key, tampered, []byte("scope\x00key")); err == nil {
		t.Error("opened a tampered blob")
	}
	if _, err := OpenAEAD(key, blob[:chacha20poly1305.NonceSize], nil); err == nil || !strings.Contains(err.Error(), "too short") {
		t.Errorf("short blob: err = %v", err)
	}
	if _, err := SealAEAD(key[:8], []byte("v"), nil); err == nil {
		t.Error("sealed with a short key")
	}
	if _, err := OpenAEAD(key[:8], blob, nil); err == nil {
		t.Error("opened with a short key")
	}
	if err := ValidEnvelopeBlob(blob[:10]); err == nil {
		t.Error("a 10-byte blob passed the shape check")
	}
}

func TestValidateCiphertextShape(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	edPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCiphertextShape(&rsaKey.PublicKey, make([]byte, rsaKey.Size())); err != nil {
		t.Errorf("modulus-sized RSA chunk: %v", err)
	}
	if err := ValidateCiphertextShape(&rsaKey.PublicKey, []byte("plaintext")); err == nil {
		t.Error("plaintext passed as an RSA chunk")
	}

	sealed, err := EncryptLargeString(edPub, "value")
	if err != nil || len(sealed) == 0 {
		t.Fatalf("EncryptLargeString: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(sealed[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCiphertextShape(edPub, raw); err != nil {
		t.Errorf("X25519 sealed box: %v", err)
	}
	if err := ValidateCiphertextShape(edPub, []byte("plaintext")); err == nil {
		t.Error("plaintext passed as an X25519 sealed box")
	}
	if err := ValidateCiphertextShape(edPub, raw[:len(x25519Magic)+1]); err == nil {
		t.Error("a truncated sealed box passed")
	}
	if err := ValidateCiphertextShape("not a key", raw); err == nil {
		t.Error("an unsupported key type passed")
	}
}
