// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/yandex"
)

const tokenYandexProvider = "pulumi:providers:yandex"

// inputsWithProp returns the inputs of the first resource of the given type that
// actually carries prop. Every test here runs under bucketProvisionParams, which
// instantiates a *keyless* provider of its own first, so a plain inputsOf() hands
// back that one and every secretness assertion passes vacuously.
func (m *bucketMocks) inputsWithProp(typeToken, prop string) resource.PropertyMap {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.created {
		if a.TypeToken == typeToken && a.Inputs.HasValue(resource.PropertyKey(prop)) {
			return a.Inputs
		}
	}
	return nil
}

// The one property that must never be printable. A provider's credentials are
// ordinary Pulumi properties, so an unmarked one lands in the update summary and
// from there in the CI log — which is how a real service-account private key
// reached a world-readable GitHub Actions log on 2026-09-30.
func TestProvider_ServiceAccountKeyIsSecret(t *testing.T) {
	RegisterTestingT(t)

	mocks := newBucketMocks()
	Expect(provisionProvider(providerAuthConfig(), mocks)).To(BeNil())

	inputs := mocks.inputsWithProp(tokenYandexProvider, "serviceAccountKeyFile")
	Expect(inputs).NotTo(BeNil(), "no provider was created with a service-account key")
	key := inputs["serviceAccountKeyFile"]
	Expect(key.IsSecret()).To(BeTrue(),
		"serviceAccountKeyFile reached the engine unmarked — it will be printed verbatim in the "+
			"preview and update summaries and archived in CI logs")
	// ...and arrive intact. Masking is the engine's job, so a secret PropertyValue
	// still carries the plaintext document as its element — that is not the leak.
	// The leak is the document reaching the engine unmarked, which is the assertion
	// above.
	Expect(key.SecretValue().Element.StringValue()).To(Equal(testServiceAccountKey))
}

// The static pair is a second, independent credential (Object Storage + Message
// Queue speak SigV4). Only the halves that are secrets are marked — the access
// key ids are identifiers and masking them makes a diff unreadable for nothing.
func TestProvider_StaticSecretKeysAreSecret(t *testing.T) {
	RegisterTestingT(t)

	cfg := providerAuthConfig()
	cfg.AccessKey = "YCAJE-test-access-key"
	cfg.SecretAccessKey = "YCsecret-test-secret-key"

	mocks := newBucketMocks()
	Expect(provisionProvider(cfg, mocks)).To(BeNil())

	inputs := mocks.inputsWithProp(tokenYandexProvider, "storageSecretKey")
	Expect(inputs).NotTo(BeNil(), "no provider was created with a static key pair")
	Expect(inputs["storageSecretKey"].IsSecret()).To(BeTrue())
	Expect(inputs["ymqSecretKey"].IsSecret()).To(BeTrue())
	Expect(inputs["storageAccessKey"].IsSecret()).To(BeFalse())
	Expect(inputs["ymqAccessKey"].IsSecret()).To(BeFalse())
}

// The registrar builds its own provider from its own auth config, so it leaks
// independently of Provider() and needs its own assertion.
func TestRegistrarProvider_ServiceAccountKeyIsSecret(t *testing.T) {
	RegisterTestingT(t)

	mocks := newBucketMocks()
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		params, err := bucketProvisionParams(ctx)
		if err != nil {
			return err
		}
		// The zone lookup fails under mocks (there is no zone to find), which is fine:
		// the provider is created before it, so its inputs are already recorded.
		_, _ = Registrar(ctx, api.RegistrarDescriptor{
			Type: yandex.RegistrarTypeYandexDns,
			Config: api.Config{Config: &yandex.RegistrarConfig{
				AccountConfig: providerAuthConfig(),
				ZoneName:      "atriumdev.ru",
				ZoneID:        "dnstest",
			}},
		}, params)
		return nil
	}, pulumi.WithMocks("project", "stack", mocks))
	Expect(err).To(BeNil())

	inputs := mocks.inputsWithProp(tokenYandexProvider, "serviceAccountKeyFile")
	Expect(inputs).NotTo(BeNil(), "the registrar did not create a provider with a service-account key")
	Expect(inputs["serviceAccountKeyFile"].IsSecret()).To(BeTrue())
}

// secretStringPtr's own contract, asserted directly so a refactor of the helper
// cannot pass the call-site tests by accident.
func TestSecretStringPtrMarksSecret(t *testing.T) {
	RegisterTestingT(t)

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		got, err := internals.UnsafeAwaitOutput(ctx.Context(), secretStringPtr("a-credential"))
		if err != nil {
			return err
		}
		Expect(got.Secret).To(BeTrue())
		Expect(*got.Value.(*string)).To(Equal("a-credential"))
		return nil
	}, pulumi.WithMocks("project", "stack", newBucketMocks()))
	Expect(err).To(BeNil())
}

func providerAuthConfig() yandex.AccountConfig {
	// No Credentials blob on purpose: with it empty, CredentialsValue() serialises
	// the whole config, which is what api.ConvertAuth reads back. A `"{}"` blob (as
	// the container tests use, where nothing re-converts) would wipe every field.
	return yandex.AccountConfig{
		CloudID:           "test-cloud",
		FolderID:          "test-folder",
		ServiceAccountKey: testServiceAccountKey,
	}
}

func provisionProvider(cfg yandex.AccountConfig, mocks *bucketMocks) error {
	return pulumi.RunErr(func(ctx *pulumi.Context) error {
		params, err := bucketProvisionParams(ctx)
		if err != nil {
			return err
		}
		_, err = Provider(ctx, api.Stack{Name: "test-stack"}, api.ResourceInput{
			Descriptor: &api.ResourceDescriptor{
				Type:   yandex.ProviderType,
				Name:   "yc",
				Config: api.Config{Config: &cfg},
			},
			StackParams: &api.StackParams{Environment: "test"},
		}, params)
		return err
	}, pulumi.WithMocks("project", "stack", mocks))
}
