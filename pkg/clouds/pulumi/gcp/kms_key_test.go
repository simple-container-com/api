// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"sync"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	gcpsdk "github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// createBasicProvisionParams leaves Provider nil, which panics inside
// kms.NewKeyRing. Build a real provider under the mocks instead.
func kmsProvisionParams(ctx *pulumi.Context) (pApi.ProvisionParams, error) {
	params := createBasicProvisionParams()
	prov, err := gcpsdk.NewProvider(ctx, "test-gcp", &gcpsdk.ProviderArgs{
		Project: pulumi.String("test-project"),
	})
	if err != nil {
		return params, err
	}
	params.Provider = prov
	return params, nil
}

// kmsMocks records what the provisioner actually asked Pulumi to create, so the
// tests can assert on the value that reaches the CryptoKey rather than on the
// helper that computes it. Without this, reverting the default in
// KmsKeySecretsProvider leaves every config-level test green.
type kmsMocks struct {
	mu      sync.Mutex
	created []pulumi.MockResourceArgs
}

func newKmsMocks() *kmsMocks { return &kmsMocks{} }

func (m *kmsMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.created = append(m.created, args)
	return args.Name + "-id", args.Inputs, nil
}

func (m *kmsMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func (m *kmsMocks) countOf(typeToken string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, a := range m.created {
		if a.TypeToken == typeToken {
			n++
		}
	}
	return n
}

func (m *kmsMocks) rotationPeriod() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.created {
		if a.TypeToken == "gcp:kms/cryptoKey:CryptoKey" {
			if v, ok := a.Inputs["rotationPeriod"]; ok && v.IsString() {
				return v.StringValue()
			}
		}
	}
	return ""
}

func kmsResourceInput(cfg *gcloud.SecretsProviderConfig) api.ResourceInput {
	return api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type:   gcloud.SecretsProviderTypeGcpKms,
			Name:   "test-stack-sc",
			Config: api.Config{Config: cfg},
		},
		StackParams: &api.StackParams{Environment: "test"},
	}
}

func baseKmsConfig() *gcloud.SecretsProviderConfig {
	return &gcloud.SecretsProviderConfig{
		Provision:   true,
		KeyName:     "test-key",
		KeyLocation: "global",
		Credentials: gcloud.Credentials{
			ServiceAccountConfig: gcloud.ServiceAccountConfig{ProjectId: "test-project"},
		},
	}
}

// The default is the whole point of the change: an unset keyRotationPeriod must
// reach the CryptoKey as 90 days, not the previous 100000s (27.8h).
func TestKmsKeySecretsProvider_AppliesDefaultRotationPeriod(t *testing.T) {
	RegisterTestingT(t)

	setGlobalServicesAPIClient(newMockServicesAPIClient())
	defer resetGlobalServicesAPIClient()

	mocks := newKmsMocks()
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		params, err := kmsProvisionParams(ctx)
		if err != nil {
			return err
		}
		_, err = KmsKeySecretsProvider(ctx, api.Stack{}, kmsResourceInput(baseKmsConfig()), params)
		return err
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).To(BeNil())
	Expect(mocks.rotationPeriod()).To(Equal(gcloud.DefaultKeyRotationPeriod),
		"unset keyRotationPeriod must provision the 90-day default")
	Expect(mocks.rotationPeriod()).NotTo(Equal("100000s"),
		"the 27.8h default is what caused the key-version cost blowup")
}

func TestKmsKeySecretsProvider_AppliesExplicitRotationPeriod(t *testing.T) {
	RegisterTestingT(t)

	setGlobalServicesAPIClient(newMockServicesAPIClient())
	defer resetGlobalServicesAPIClient()

	cfg := baseKmsConfig()
	cfg.KeyRotationPeriod = "31536000s"

	mocks := newKmsMocks()
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		params, err := kmsProvisionParams(ctx)
		if err != nil {
			return err
		}
		_, err = KmsKeySecretsProvider(ctx, api.Stack{}, kmsResourceInput(cfg), params)
		return err
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).To(BeNil())
	Expect(mocks.rotationPeriod()).To(Equal("31536000s"))
}

// Ordering matters more than the message: a GCP KeyRing can never be deleted
// (destroying the Pulumi resource only drops it from state), so a rejected
// config must not leave one behind. Asserting zero created resources pins that,
// where asserting only the error would pass even if validation ran last.
func TestKmsKeySecretsProvider_RejectsBadPeriodBeforeCreatingAnything(t *testing.T) {
	RegisterTestingT(t)

	setGlobalServicesAPIClient(newMockServicesAPIClient())
	defer resetGlobalServicesAPIClient()

	cfg := baseKmsConfig()
	cfg.KeyRotationPeriod = "100000s" // the old default: below the 30-day floor

	mocks := newKmsMocks()
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		params, err := kmsProvisionParams(ctx)
		if err != nil {
			return err
		}
		_, err = KmsKeySecretsProvider(ctx, api.Stack{}, kmsResourceInput(cfg), params)
		return err
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).NotTo(BeNil())
	Expect(err.Error()).To(ContainSubstring("30 day"))
	Expect(mocks.countOf("gcp:kms/keyRing:KeyRing")).To(Equal(0),
		"a KeyRing can never be deleted; validation must run before it is created")
	Expect(mocks.countOf("gcp:kms/cryptoKey:CryptoKey")).To(Equal(0))
}
