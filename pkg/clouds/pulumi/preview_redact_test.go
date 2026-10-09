// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

// `sc provision -P` and `sc deploy -P` print the preview summary as it is, so
// the summary itself must not carry the old credentials of a provider.
func TestPreviewAndUpdateSummariesAreRedacted(t *testing.T) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalECPrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	key := string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}))
	body := strings.Split(key, "\n")[1]
	stdout := "Previewing update:\n  - credentials: " + key + "Resources: 1 to update\n"
	p := &pulumi{}
	for name, summary := range map[string]string{
		"preview": p.toPreviewResult("app", auto.PreviewResult{StdOut: stdout}).Summary,
		"update":  p.toUpdateResult("app", auto.UpResult{StdOut: stdout}).Summary,
	} {
		if strings.Contains(summary, body[4:24]) || !strings.Contains(summary, "Resources: 1 to update") {
			t.Errorf("%s summary:\n%s", name, summary)
		}
	}
}
