// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

const fakeServiceAccountKey = `{"type":"service_account","project_id":"p","private_key_id":"k","private_key":"-----BEGIN PRIVATE KEY-----\nMA==\n-----END PRIVATE KEY-----\n","client_email":"deployer@p.iam.gserviceaccount.com","token_uri":"https://oauth2.googleapis.com/token"}`

// What google-github-actions/auth writes for Workload Identity Federation.
const fakeExternalAccount = `{"type":"external_account","audience":"//iam.googleapis.com/projects/1/locations/global/workloadIdentityPools/p/providers/gh","subject_token_type":"urn:ietf:params:oauth:token-type:jwt","token_url":"https://sts.googleapis.com/v1/token","credential_source":{"file":"/dev/null"},"service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/deployer@p.iam.gserviceaccount.com:generateAccessToken"}`

func writeADC(t *testing.T, content string) {
	path := filepath.Join(t.TempDir(), "adc.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", path)
}

func providerInputs(t *testing.T, credentials string) map[string]any {
	return providerProps(t, credentials).Mappable()
}

func providerProps(t *testing.T, credentials string) resource.PropertyMap {
	mocks := newKmsMocks()
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := Provider(ctx, api.Stack{}, api.ResourceInput{
			Descriptor: &api.ResourceDescriptor{Name: "gcp", Config: api.Config{Config: &gcloud.Credentials{
				Credentials:          api.Credentials{Credentials: credentials},
				ServiceAccountConfig: gcloud.ServiceAccountConfig{ProjectId: "p"},
			}}},
			StackParams: &api.StackParams{Environment: "test"},
		}, createBasicProvisionParams())
		return err
	}, pulumi.WithMocks("project", "stack", mocks))
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range mocks.created {
		if a.TypeToken == "pulumi:providers:gcp" {
			return a.Inputs
		}
	}
	t.Fatal("no gcp provider was created")
	return nil
}

func TestProvider_AmbientCredentialsAreNotPassed(t *testing.T) {
	RegisterTestingT(t)
	// Passing an empty string would override Application Default Credentials with
	// "no credentials" instead of falling back to them.
	Expect(providerInputs(t, "")).NotTo(HaveKey("credentials"))
}

// The one property that must never be printable. pulumi-gcp's generated
// NewProvider marks only `accessToken`, so `credentials` — the whole
// service-account document, `private_key` included — is left to the caller, and
// an unmarked provider credential is how a Yandex service-account private key
// reached a world-readable CI log on 2026-09-30.
func TestProvider_ConfiguredKeyIsSecretAndStillPassed(t *testing.T) {
	RegisterTestingT(t)
	creds := providerProps(t, fakeServiceAccountKey)["credentials"]
	Expect(creds.IsSecret()).To(BeTrue(),
		"credentials reached the engine unmarked — the service-account document will be "+
			"printed verbatim in the preview and update summaries and archived in CI logs")
	// ...and arrive intact. Masking is the engine's job, so a secret
	// PropertyValue still carries the plaintext document as its element — that
	// is not the leak. The leak is it reaching the engine unmarked, above.
	Expect(creds.SecretValue().Element.StringValue()).To(Equal(fakeServiceAccountKey))
}

// secretCredentials' own contract, asserted directly so a refactor of the helper
// cannot pass the call-site test by accident.
func TestSecretCredentialsMarksSecret(t *testing.T) {
	RegisterTestingT(t)

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		got, err := internals.UnsafeAwaitOutput(ctx.Context(), secretCredentials("a-credential"))
		if err != nil {
			return err
		}
		Expect(got.Secret).To(BeTrue())
		Expect(*got.Value.(*string)).To(Equal("a-credential"))
		return nil
	}, pulumi.WithMocks("project", "stack", newKmsMocks()))
	Expect(err).To(BeNil())
}

func TestClientOptions(t *testing.T) {
	RegisterTestingT(t)
	Expect(clientOptions("")).To(BeEmpty())
	Expect(clientOptions("  \n")).To(BeEmpty())
	Expect(clientOptions(fakeServiceAccountKey)).To(HaveLen(1))
}

func TestTokenSource_AmbientAcceptsWorkloadIdentityFederation(t *testing.T) {
	RegisterTestingT(t)
	writeADC(t, fakeExternalAccount)
	ts, err := tokenSource(context.Background(), "")
	Expect(err).NotTo(HaveOccurred())
	Expect(ts).NotTo(BeNil())
}

func TestTokenSource_AmbientWithoutADCFails(t *testing.T) {
	RegisterTestingT(t)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(t.TempDir(), "missing.json"))
	_, err := tokenSource(context.Background(), "")
	Expect(err).To(MatchError(ContainSubstring("ambient mode")))
}

func TestTokenSource_ConfiguredKeyMustBeAServiceAccount(t *testing.T) {
	RegisterTestingT(t)
	// A configured value keeps the old pin: only a service-account key is accepted
	// from config. An external-account file is only honoured through ADC.
	_, err := tokenSource(context.Background(), fakeExternalAccount)
	Expect(err).To(HaveOccurred())
	ts, err := tokenSource(context.Background(), fakeServiceAccountKey)
	Expect(err).NotTo(HaveOccurred())
	Expect(ts).NotTo(BeNil())
}
