// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

// The body of a real service-account key: base64, no markers of its own, and
// the part that is worth a rotation if it is printed.
const testKeyBody = "MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDZ1example"

// previewWithEscapedKey is the shape the reported leak took: a `pulumi preview`
// diff of a newly created provider, with the credentials JSON rendered as one
// escaped string.
const previewWithEscapedKey = `Previewing update (staging):

    pulumi:pulumi:Stack: (same)
    + pulumi:providers:gcp: (create)
        credentials: "{\"type\":\"service_account\",\"project_id\":\"acme-staging\",\"private_key_id\":\"9f1c0de4cafe\",\"private_key\":\"-----BEGIN PRIVATE KEY-----\n` + testKeyBody + `\n-----END PRIVATE KEY-----\n\",\"client_email\":\"deploy@acme-staging.iam.gserviceaccount.com\"}"
        project    : "acme-staging"

Resources:
    + 4 to create
`

// previewWithMultilineKey is the same credential printed across real newlines,
// which is what a diff of a file-shaped input produces.
const previewWithMultilineKey = `    + gcp:serviceaccount/key:Key: (create)
        privateKey: -----BEGIN RSA PRIVATE KEY-----
` + testKeyBody + `
-----END RSA PRIVATE KEY-----
        project   : "acme-staging"
`

func TestRedactCredentials_EscapedServiceAccountKey(t *testing.T) {
	RegisterTestingT(t)

	got := redactCredentials(previewWithEscapedKey)

	Expect(got).ToNot(ContainSubstring(testKeyBody), "private key body must not survive redaction")
	Expect(got).ToNot(ContainSubstring("BEGIN PRIVATE KEY"), "PEM markers must not survive redaction")
	Expect(got).ToNot(ContainSubstring("9f1c0de4cafe"), "private_key_id identifies the key and must be redacted too")
	Expect(got).To(ContainSubstring(redactedValue))

	// Everything that makes the preview useful is still there.
	Expect(got).To(ContainSubstring(`project    : "acme-staging"`))
	Expect(got).To(ContainSubstring("+ 4 to create"))
	Expect(got).To(ContainSubstring("deploy@acme-staging.iam.gserviceaccount.com"),
		"the client email is not a credential and is worth keeping for identifying the account")
}

func TestRedactCredentials_MultilinePEM(t *testing.T) {
	RegisterTestingT(t)

	got := redactCredentials(previewWithMultilineKey)

	Expect(got).ToNot(ContainSubstring(testKeyBody))
	Expect(got).ToNot(ContainSubstring("BEGIN RSA PRIVATE KEY"))
	Expect(got).ToNot(ContainSubstring("END RSA PRIVATE KEY"))
	Expect(got).To(ContainSubstring(redactedValue))
	Expect(got).To(ContainSubstring(`project   : "acme-staging"`))
}

func TestRedactCredentials_Kubeconfig(t *testing.T) {
	RegisterTestingT(t)

	kubeconfig := `    + pulumi:providers:kubernetes: (create)
        kubeconfig: apiVersion: v1
users:
- name: admin
  user:
    client-key-data: LS0tLS1CRUdJTlBSSVZBVEU=
    token: eyJhbGciOiJSUzI1NiIsImtpZCI6ImFiYyJ9.payload.signature
    password: hunter2
`
	got := redactCredentials(kubeconfig)

	Expect(got).ToNot(ContainSubstring("LS0tLS1CRUdJTlBSSVZBVEU="))
	Expect(got).ToNot(ContainSubstring("eyJhbGciOiJSUzI1NiIsImtpZCI6ImFiYyJ9.payload.signature"))
	Expect(got).ToNot(ContainSubstring("hunter2"))
	Expect(strings.Count(got, redactedValue)).To(Equal(3))
	Expect(got).To(ContainSubstring("- name: admin"))
}

func TestRedactCredentials_EscapedKubeconfig(t *testing.T) {
	RegisterTestingT(t)

	// A kubeconfig that reaches the diff as a single escaped string.
	got := redactCredentials(`        kubeconfig: "apiVersion: v1\nusers:\n- name: admin\n  user:\n    token: eyJhbGciOiJSUzI1NiJ9.payload\n"`)

	Expect(got).ToNot(ContainSubstring("eyJhbGciOiJSUzI1NiJ9.payload"))
	Expect(got).To(ContainSubstring(redactedValue))
	Expect(got).To(ContainSubstring("- name: admin"))
}

func TestRedactCredentials_LeavesOrdinaryOutputAlone(t *testing.T) {
	RegisterTestingT(t)

	summary := `Previewing update (staging):

    + gcp:sql/databaseInstance:DatabaseInstance: (create)
        name           : "acme-db"
        databaseVersion: "POSTGRES_15"
        settings       : {
            tier: "db-custom-2-7680"
        }

Resources:
    + 1 to create
    ~ 2 to update
`
	Expect(redactCredentials(summary)).To(Equal(summary))
	Expect(redactCredentials("")).To(Equal(""))
}

func TestRedactCredentials_IsIdempotent(t *testing.T) {
	RegisterTestingT(t)

	once := redactCredentials(previewWithEscapedKey)
	Expect(redactCredentials(once)).To(Equal(once))
}

// The summary is not only logged, it is also returned to callers (the CLI
// prints it, CI publishes it), so the redaction has to sit on the conversion
// rather than on the log line.
func TestToPreviewResultRedactsSummary(t *testing.T) {
	RegisterTestingT(t)

	p := &pulumi{}

	preview := p.toPreviewResult("acme/staging", auto.PreviewResult{StdOut: previewWithEscapedKey})
	Expect(preview.Summary).ToNot(ContainSubstring(testKeyBody))
	Expect(preview.Summary).To(ContainSubstring(redactedValue))
	Expect(preview.StackName).To(Equal("acme/staging"))

	update := p.toUpdateResult("acme/staging", auto.UpResult{StdOut: previewWithEscapedKey})
	Expect(update.Summary).ToNot(ContainSubstring(testKeyBody))
	Expect(update.Summary).To(ContainSubstring(redactedValue))
	Expect(update.StackName).To(Equal("acme/staging"))
}
