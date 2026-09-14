// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/pkg/v3/backend/display"
	"github.com/pulumi/pulumi/pkg/v3/resource/deploy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/diag/colors"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
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

	// Everything that makes the preview useful is still there. A blob the
	// engine could not decode is masked whole, so the fields inside it go with
	// it; the decoded rendering, which is what the engine actually produces
	// for this property, keeps its non-credential fields (see the engine test
	// below).
	Expect(got).To(ContainSubstring(`project    : "acme-staging"`))
	Expect(got).To(ContainSubstring("+ 4 to create"))
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
	Expect(strings.Count(got, redactedValue)).To(BeNumerically(">=", 3))
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

// The rules have to leave a legible preview legible. The fixture is built from
// near misses rather than unrelated text, because a rule that fires on
// `passwordPolicy` or on a public certificate would blank out the lines an
// operator reads to decide whether to approve a deploy.
func TestRedactCredentials_LeavesOrdinaryOutputAlone(t *testing.T) {
	RegisterTestingT(t)

	summary := `Previewing update (staging):

    + gcp:sql/databaseInstance:DatabaseInstance: (create)
        name           : "acme-db"
        databaseVersion: "POSTGRES_15"
        passwordPolicy : "ENFORCE_COMPLEXITY"
        tokenSecretName: "acme-db-token"
        settings       : {
            tier: "db-custom-2-7680"
        }
        caCert         : "-----BEGIN CERTIFICATE-----
MIIDdzCCAl+gAwIBAgIEexample
-----END CERTIFICATE-----"

Resources:
    + 1 to create
    ~ 2 to update
`
	Expect(redactCredentials(summary)).To(Equal(summary))
	Expect(redactCredentials("")).To(Equal(""))
}

// The accepted cost of the rules, pinned so a change in blast radius is a test
// change rather than a surprise: a property named like a credential is masked
// whether or not its value is one.
func TestRedactCredentials_MasksNonSecretsNamedLikeSecrets(t *testing.T) {
	RegisterTestingT(t)

	got := redactCredentials("        token: ${GITHUB_TOKEN}\n        password: \"\"\n")

	Expect(got).To(ContainSubstring("token: " + redactedValue))
	Expect(got).ToNot(ContainSubstring("GITHUB_TOKEN"), "a placeholder is masked whole, not left half-eaten")
	Expect(got).To(ContainSubstring(`password: "`+redactedValue+`"`), "quoting is preserved")
}

// Redaction runs on text that may already have been redacted (a summary is
// logged and returned), so a second pass must not chew on its own output.
func TestRedactCredentials_IsIdempotent(t *testing.T) {
	RegisterTestingT(t)

	for name, fixture := range map[string]string{
		"escaped service account key": previewWithEscapedKey,
		"multiline PEM":               previewWithMultilineKey,
		"kubeconfig":                  "    client-key-data: LS0tLS1CRUdJTg==\n    token: eyJhbGciOiJSUzI1NiJ9.payload\n",
		"rotation":                    `      ~ password  : "OLD" => "NEW"`,
	} {
		once := redactCredentials(fixture)
		Expect(redactCredentials(once)).To(Equal(once), "redacting %s twice must equal redacting it once", name)
	}
}

// The summary is not only logged, it is also returned to callers (the CLI
// prints it, CI publishes it), so the redaction has to sit on the conversion
// rather than on the log line.
func TestPreviewAndUpdateResultsRedactSummary(t *testing.T) {
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

// A preview of a change renders each property with a diff marker, and an
// updated value as "old => new". Both shapes have to survive the regex, and
// the second one has two values to remove, not one.
func TestRedactCredentials_DiffMarkersAndRotation(t *testing.T) {
	RegisterTestingT(t)

	summary := `    ~ pulumi:providers:kubernetes: (update)
      ~ kubeconfig: apiVersion: v1
users:
- name: admin
  user:
    token: OLDTOKENAAA => NEWTOKENBBB
      + password: hunter2
      ~ client-key-data: "OLDKEYDATA" => "NEWKEYDATA"
`
	got := redactCredentials(summary)

	for _, secret := range []string{"OLDTOKENAAA", "NEWTOKENBBB", "hunter2", "OLDKEYDATA", "NEWKEYDATA"} {
		Expect(got).ToNot(ContainSubstring(secret), "%s must not survive redaction", secret)
	}
	Expect(got).To(ContainSubstring("(update)"))
	Expect(got).To(ContainSubstring("- name: admin"))
}

// Pulumi renders resource properties in camelCase, so a credential that never
// passes through a service-account JSON shows up under a different spelling.
func TestRedactCredentials_CamelCaseProperties(t *testing.T) {
	RegisterTestingT(t)

	summary := `    + docker:index/image:Image: (create)
        registry  : {
            password: "ya29.a0AfB_exampleAccessToken"
            server  : "europe-docker.pkg.dev"
        }
    + pulumi:providers:aws: (create)
        secretKey: "wJalrXUtnFEMIexampleKEY"
        apiToken : "cf-Abc123SuperSecretToken"
        region   : "us-east-1"
`
	got := redactCredentials(summary)

	for _, secret := range []string{"ya29.a0AfB_exampleAccessToken", "wJalrXUtnFEMIexampleKEY", "cf-Abc123SuperSecretToken"} {
		Expect(got).ToNot(ContainSubstring(secret), "%s must not survive redaction", secret)
	}
	Expect(got).To(ContainSubstring(`server  : "europe-docker.pkg.dev"`))
	Expect(got).To(ContainSubstring(`region   : "us-east-1"`))
}

// A kubeconfig authenticated by an OIDC auth-provider keeps its credentials
// under hyphenated keys, and a diff may render the value escaped.
//
// The docker auth below is deliberately not a decodable `user:password`: the
// rules key on the field name, so the fixture loses nothing by it, and a
// fixture that decodes is a finding for the repo's own secret scanner.
func TestRedactCredentials_AuthProviderKubeconfig(t *testing.T) {
	RegisterTestingT(t)

	summary := `        kubeconfig: "users:\n- name: oidc\n  user:\n    auth-provider:\n      config:\n        id-token: eyJhbGciOiJSUzI1NiJ9.idtoken\n        refresh-token: \"1//0eRefreshTokenValue\"\n        client-secret: oidc-client-secret-value\n"
        dockerconfig: "{\"auths\":{\"registry.example\":{\"auth\":\"placeholder-not-a-real-credential\"}}}"
`
	got := redactCredentials(summary)

	for _, secret := range []string{
		"eyJhbGciOiJSUzI1NiJ9.idtoken", "1//0eRefreshTokenValue",
		"oidc-client-secret-value", "placeholder-not-a-real-credential",
	} {
		Expect(got).ToNot(ContainSubstring(secret), "%s must not survive redaction", secret)
	}
}

// renderCreate and renderUpdate produce the text the engine itself would print
// for a resource, rather than a hand-written guess at it. The distinction
// matters: a string property that parses as JSON or YAML is decoded and
// re-printed as a structure with BARE keys, which is not the shape a
// hand-written fixture reaches for, and the redaction has to match what is
// actually emitted. colors.Never mirrors what the automation API returns,
// since Simple Container never asks the engine for colour.
// truncateOutput is the production setting, not a convenience: the CLI's
// --show-full-output defaults to false and the automation API never passes it,
// so every preview Simple Container produces cuts long values to three lines
// of 150 characters. Rendering the fixtures without it would test a shape the
// engine does not emit -- and truncation is exactly what strips a PEM key of
// its closing marker.
const truncateOutput = true

func renderCreate(props resource.PropertyMap) string {
	var b bytes.Buffer
	display.PrintObject(&b, props, true, 1, deploy.OpCreate, true, truncateOutput, false, false)
	return colors.Never.Colorize(b.String())
}

func renderUpdate(old, updated resource.PropertyMap) string {
	var b bytes.Buffer
	diff := old.Diff(updated)
	Expect(diff).ToNot(BeNil(), "the two property maps must actually differ")
	display.PrintObjectDiff(&b, *diff, nil, true, 1, false, truncateOutput, false, false, nil)
	return colors.Never.Colorize(b.String())
}

func serviceAccountKey() string {
	return `{"type":"service_account","project_id":"acme-staging",` +
		`"private_key_id":"9f1c0de4cafe",` +
		`"private_key":"-----BEGIN PRIVATE KEY-----\n` + testKeyBody + `\n-----END PRIVATE KEY-----\n",` +
		`"client_email":"deploy@acme-staging.iam.gserviceaccount.com"}`
}

// The reported leak, reproduced through the engine's own printer: creating the
// GCP provider renders its credentials input.
func TestRedactCredentials_EngineCreateRendering(t *testing.T) {
	RegisterTestingT(t)

	rendered := renderCreate(resource.PropertyMap{
		"credentials": resource.NewStringProperty(serviceAccountKey()),
		"project":     resource.NewStringProperty("acme-staging"),
	})
	Expect(rendered).To(ContainSubstring(testKeyBody), "fixture check: the engine really does print the key")
	Expect(rendered).To(ContainSubstring("(json)"), "fixture check: the engine decodes the credential blob")

	got := redactCredentials(rendered)

	Expect(got).ToNot(ContainSubstring(testKeyBody))
	Expect(got).ToNot(ContainSubstring("9f1c0de4cafe"))
	Expect(got).To(ContainSubstring(`project_id    : "acme-staging"`), "non-credential fields stay readable")
	Expect(got).To(ContainSubstring(`type          : "service_account"`))
}

// An update prints `old => new`. The new value is the credential now in use,
// so redacting only the left-hand side would leak the one that matters.
func TestRedactCredentials_EngineUpdateRendering(t *testing.T) {
	RegisterTestingT(t)

	kubeconfig := `apiVersion: v1
clusters:
- cluster:
    server: https://203.0.113.10
  name: acme
users:
- name: acme
  user:
    client-key-data: OLDKEYDATA_AAAA
    token: OLDTOKEN_AAAA
`
	updated := strings.NewReplacer("OLDKEYDATA_AAAA", "NEWKEYDATA_BBBB", "OLDTOKEN_AAAA", "NEWTOKEN_BBBB").Replace(kubeconfig)

	rendered := renderUpdate(
		resource.PropertyMap{
			"kubeconfig": resource.NewStringProperty(kubeconfig),
			"password":   resource.NewStringProperty("OLDPASSWORD_AAAA"),
		},
		resource.PropertyMap{
			"kubeconfig": resource.NewStringProperty(updated),
			"password":   resource.NewStringProperty("NEWPASSWORD_BBBB"),
		},
	)
	Expect(rendered).To(ContainSubstring("NEWTOKEN_BBBB"), "fixture check: the engine really does print the new value")

	got := redactCredentials(rendered)

	for _, secret := range []string{
		"OLDKEYDATA_AAAA", "NEWKEYDATA_BBBB", "OLDTOKEN_AAAA", "NEWTOKEN_BBBB",
		"OLDPASSWORD_AAAA", "NEWPASSWORD_BBBB",
	} {
		Expect(got).ToNot(ContainSubstring(secret), "%s must not survive redaction", secret)
	}
	Expect(got).To(ContainSubstring("203.0.113.10"), "the cluster endpoint is not a credential")
}

// Output can be truncated or interleaved, leaving a BEGIN marker with no END.
// The key still has to go, and the sweep must not run past it: an unbounded
// match would delete every line up to an END marker belonging to something
// else, and a preview that silently loses most of its diff is worse than one
// that shows a masked line.
func TestRedactCredentials_UnterminatedPEMDoesNotSwallowTheDiff(t *testing.T) {
	RegisterTestingT(t)

	summary := `    + credentials: "-----BEGIN PRIVATE KEY-----
` + testKeyBody + `
  ... (output truncated)
    + name       : "important-resource"
    + other      : "-----BEGIN PRIVATE KEY-----
` + testKeyBody + `
-----END PRIVATE KEY-----"
`
	got := redactCredentials(summary)

	Expect(got).ToNot(ContainSubstring(testKeyBody), "neither key body may survive")
	Expect(got).ToNot(ContainSubstring("BEGIN PRIVATE KEY"))
	Expect(got).To(ContainSubstring(`name       : "important-resource"`),
		"the diff between the two keys must survive the sweep")
}

// A rotation can have an engine marker on one side: the old value was secret
// in the checkpoint, the new one is not (a call site that regressed, or a
// revert). The plaintext side is the one that matters, and a rule that cannot
// match `[secret]` misses the whole line rather than half of it.
func TestRedactCredentials_RotationFromEngineMarker(t *testing.T) {
	RegisterTestingT(t)

	got := redactCredentials(`      ~ password  : [secret] => "NEWPLAINTEXTAAA"
      ~ token     : [unknown] => NEWBEARERBBB
      ~ apiToken  : "OLDPLAINCCC" => [secret]
`)

	for _, secret := range []string{"NEWPLAINTEXTAAA", "NEWBEARERBBB", "OLDPLAINCCC"} {
		Expect(got).ToNot(ContainSubstring(secret), "%s must not survive redaction", secret)
	}
}

// The automation API puts the whole engine stdout and stderr into the error it
// returns from a failed operation, and those errors are printed by the CLI. A
// provider that cannot configure itself is exactly where a credential is
// echoed back, so the failure path needs the same treatment as the summary.
func TestRedactErrorRemovesCredentialsFromFailedOperations(t *testing.T) {
	RegisterTestingT(t)

	leaky := errors.New("failed to configure provider\ncode: 255\nstdout: \nstderr: " +
		`error: unable to parse credentials: {"private_key":"-----BEGIN PRIVATE KEY-----\n` + testKeyBody +
		`\n-----END PRIVATE KEY-----\n","private_key_id":"9f1c0de4cafe"}`)

	got := redactError(leaky)
	Expect(got).To(HaveOccurred())
	Expect(got.Error()).ToNot(ContainSubstring(testKeyBody))
	Expect(got.Error()).ToNot(ContainSubstring("9f1c0de4cafe"))
	Expect(got.Error()).To(ContainSubstring("unable to parse credentials"), "the diagnosis has to survive")
	Expect(got.Error()).To(ContainSubstring("code: 255"))

	// An error with nothing to remove is returned as-is, so wrapping costs no
	// type information on the path every other failure takes.
	ordinary := errors.New("error: resource acme-db already exists")
	Expect(redactError(ordinary)).To(BeIdenticalTo(ordinary))
	Expect(redactError(nil)).To(BeNil())
}

// The credential names this codebase and its consumers actually produce are
// qualified: a property is `rootPassword`, an environment variable is
// `AWS_SECRET_ACCESS_KEY`. A rule anchored at the start of the key matched none
// of them.
func TestRedactCredentials_QualifiedCredentialKeys(t *testing.T) {
	RegisterTestingT(t)

	got := redactCredentials(`        AWS_SECRET_ACCESS_KEY: "wJalrXUtnFEMIexampleAAA"
        AWS_ACCESS_KEY_ID    : "AKIAIOSFODNN7EXAMPLE"
        rootPassword         : "hunter2-rootBBB"
        registryPassword     : "ya29.registrytokenCCC"
        COSIGN_PASSWORD      : "cosign-passDDD"
        authHeader           : "eyJ1c2VybmFtZSI6ImFFEE"
`)

	for _, secret := range []string{
		"wJalrXUtnFEMIexampleAAA", "AKIAIOSFODNN7EXAMPLE", "hunter2-rootBBB",
		"ya29.registrytokenCCC", "cosign-passDDD", "eyJ1c2VybmFtZSI6ImFFEE",
	} {
		Expect(got).ToNot(ContainSubstring(secret), "%s must not survive redaction", secret)
	}
}

// PGP keys carry a BLOCK suffix in their armour, which is still a private key.
func TestRedactCredentials_PGPArmouredKey(t *testing.T) {
	RegisterTestingT(t)

	got := redactCredentials(`        signingKey: -----BEGIN PGP PRIVATE KEY BLOCK-----
` + testKeyBody + `
-----END PGP PRIVATE KEY BLOCK-----
        keyId     : "ABCD1234"
`)

	Expect(got).ToNot(ContainSubstring(testKeyBody))
	Expect(got).ToNot(ContainSubstring("BEGIN PGP PRIVATE KEY BLOCK"))
	Expect(got).To(ContainSubstring(`keyId     : "ABCD1234"`))
}

// A truncated key loses its closing marker, and the sweep that removes what is
// left has to stop at the first line that is not key material. Deleting the
// diff below it would be worse than the leak: a masked value is visible, a
// missing line is not.
func TestRedactCredentials_OrphanedKeyDoesNotEatTheNextProperty(t *testing.T) {
	RegisterTestingT(t)

	got := redactCredentials(`      + privateKey           : "-----BEGIN PRIVATE KEY-----
` + testKeyBody + `
      + clusterCaCertificate: "LS0tLS1CRUdJTkNFUlQtLS0t"
      + endpoint            : "203.0.113.10"
`)

	Expect(got).ToNot(ContainSubstring(testKeyBody))
	Expect(got).ToNot(ContainSubstring("BEGIN PRIVATE KEY"))
	Expect(got).To(ContainSubstring(`clusterCaCertificate: "LS0tLS1CRUdJTkNFUlQtLS0t"`),
		"the property after the key must survive, name and value")
	Expect(got).To(ContainSubstring(`endpoint            : "203.0.113.10"`))
}

// Provider diagnostics run through the redactor now, and they are prose. A
// sentence that happens to start with a credential word must stay readable:
// mangling it costs debugging time at exactly the wrong moment.
func TestRedactCredentials_LeavesDiagnosticsLegible(t *testing.T) {
	RegisterTestingT(t)

	diagnostic := `auth: could not refresh credentials, falling back to ADC
token: exchange failed for account deploy@acme
`
	Expect(redactCredentials(diagnostic)).To(Equal(diagnostic))

	// The same key with something credential-shaped after it is still masked.
	Expect(redactCredentials(`token: eyJhbGciOiJSUzI1NiJ9.payload`)).To(ContainSubstring(redactedValue))
	Expect(redactCredentials(`token: eyJhbGciOiJSUzI1NiJ9.payload`)).ToNot(ContainSubstring("payload"))
}

// Colour is not switched on today, and the rules are line-oriented enough that
// it would defeat them silently if it ever were.
func TestRedactCredentials_SurvivesColourEscapes(t *testing.T) {
	RegisterTestingT(t)

	got := redactCredentials("\x1b[32m+\x1b[0m password: \x1b[1mhunter2-coloured\x1b[0m")

	Expect(got).ToNot(ContainSubstring("hunter2-coloured"))
	Expect(got).To(ContainSubstring(redactedValue))
}
