// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cloudru

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

func keyPair() *AccountConfig {
	return &AccountConfig{AccessKey: "AK", SecretKey: "SK"}
}

// A key pair and nothing else has to be a working config: it is what the Cloud.ru
// console hands an operator, and every other field here is derivable from it.
func TestEffectiveDefaults(t *testing.T) {
	RegisterTestingT(t)

	cfg := keyPair()
	Expect(cfg.EffectiveRegion()).To(Equal(DefaultRegion))
	// The project defaults to the region because that is how Cloud.ru names the
	// project it creates with an account — not because a project and a region are the
	// same kind of thing.
	Expect(cfg.EffectiveProjectName()).To(Equal(DefaultRegion))
	Expect(cfg.EffectiveAuthURL()).To(Equal("https://iam.ru-moscow-1.hc.sbercloud.ru/v3"))
	Expect(cfg.ProjectIdValue()).To(Equal(DefaultRegion))
	Expect(cfg.ProviderType()).To(Equal(ProviderType))
}

func TestEffectiveOverrides(t *testing.T) {
	RegisterTestingT(t)

	cfg := keyPair()
	cfg.Region = "ru-moscow-2"
	Expect(cfg.EffectiveProjectName()).To(Equal("ru-moscow-2"))
	Expect(cfg.EffectiveAuthURL()).To(Equal("https://iam.ru-moscow-2.hc.sbercloud.ru/v3"))

	// An explicit project is a different tenancy in the same region — a shape the
	// region-derived default cannot express, and the reason the field exists.
	cfg.ProjectName = "forge-staging"
	Expect(cfg.EffectiveProjectName()).To(Equal("forge-staging"))
	Expect(cfg.ProjectIdValue()).To(Equal("forge-staging"))

	cfg.AuthURL = "https://iam.example.ru/v3"
	Expect(cfg.EffectiveAuthURL()).To(Equal("https://iam.example.ru/v3"))
}

// WithRegion exists because region is not one input among many on this cloud: it
// also picks the Keystone endpoint and the project resources are created in. Setting
// the provider's region alone authenticates against one region's tenancy and then
// creates in another.
func TestWithRegion(t *testing.T) {
	RegisterTestingT(t)

	cfg := keyPair()
	scoped := cfg.WithRegion("ru-moscow-2")

	Expect(scoped.EffectiveRegion()).To(Equal("ru-moscow-2"))
	Expect(scoped.EffectiveProjectName()).To(Equal("ru-moscow-2"))
	Expect(scoped.EffectiveAuthURL()).To(ContainSubstring("iam.ru-moscow-2."))

	// A copy, not a mutation: the credential is read once per stack and handed to
	// more than one resource, each of which may name its own region.
	Expect(cfg.Region).To(BeEmpty())
	Expect(cfg.EffectiveRegion()).To(Equal(DefaultRegion))

	// An empty region is "not specified", not "reset to the default" — a vm resource
	// always has one, but nothing else here guarantees that.
	cfg.Region = "ru-moscow-3"
	Expect(cfg.WithRegion("").EffectiveRegion()).To(Equal("ru-moscow-3"))

	// An explicit project survives the scoping: it was given deliberately, so it
	// outranks the region-derived fallback that WithRegion would otherwise reach.
	cfg.ProjectName = "forge-staging"
	Expect(cfg.WithRegion("ru-moscow-2").EffectiveProjectName()).To(Equal("forge-staging"))
}

// Refused at provision time, naming the field, rather than at the provider's first
// API call — Keystone answers a half-credential with a 401 that says nothing about
// which half is missing.
func TestValidate(t *testing.T) {
	RegisterTestingT(t)

	Expect(keyPair().Validate()).To(Succeed())

	noAccess := keyPair()
	noAccess.AccessKey = ""
	Expect(noAccess.Validate().Error()).To(ContainSubstring("`accessKey` is required"))

	noSecret := keyPair()
	noSecret.SecretKey = ""
	Expect(noSecret.Validate().Error()).To(ContainSubstring("`secretKey` is required"))
	// Deliberately NOT advising `${secret:<name>}` here, which this message used to
	// do. `auth:` exists only in `secrets.yaml` (api.SecretsDescriptor), which IS the
	// encrypted store — a placeholder written there is never expanded and reaches the
	// cloud verbatim. The advice was therefore an instruction to break the config; see
	// TestValidateRefusesAnUnexpandedPlaceholder for what that cost live.
	Expect(noSecret.Validate().Error()).ToNot(ContainSubstring("${secret:"))
}

// A `${…}` left in an auth field must be refused HERE, where the field has a name.
//
// It is an easy mistake — every other SC file takes `${secret:<name>}`, and
// `secrets.yaml` is the one that does not — and it fails spectacularly far from the
// cause: the value travels through provisioning into the cloud, and Cloud.ru IAM
// answers `HTTP 400 … (key_id) is invalid argument: key_id does not match required
// format`, which reads like a revoked or mistyped key. Live-caught 2026-10-24,
// mid-deploy, with resources already created.
func TestValidateRefusesAnUnexpandedPlaceholder(t *testing.T) {
	RegisterTestingT(t)

	for field, cfg := range map[string]*AccountConfig{
		"accessKey":   {AccessKey: "${secret:CLOUDRU_KEY_ID}", SecretKey: "SK"},
		"secretKey":   {AccessKey: "AK", SecretKey: "${secret:CLOUDRU_KEY_SECRET}"},
		"projectName": {AccessKey: "AK", SecretKey: "SK", ProjectName: "${secret:PROJECT}"},
	} {
		err := cfg.Validate()
		Expect(err).ToNot(BeNil(), "`"+field+"` holding a placeholder must not validate")
		Expect(err.Error()).To(ContainSubstring("`" + field + "`"))
		// Says where the placeholder DOES belong, so the fix is one read away.
		Expect(err.Error()).To(ContainSubstring("server.yaml"))
	}

	// Evolution shares the check, because the trap belongs to `secrets.yaml` rather
	// than to either of the two clouds Cloud.ru sells under one brand.
	evo := &EvolutionAccountConfig{
		KeyID:     "f7b1c0a2-0000-4000-8000-000000000001",
		KeySecret: "SK",
		ProjectID: "${secret:CLOUDRU_PROJECT_ID}",
	}
	Expect(evo.Validate()).ToNot(Succeed())
	Expect(evo.Validate().Error()).To(ContainSubstring("`projectId`"))

	evo.ProjectID = "11111111-aaaa-4bbb-8ccc-222222222222"
	Expect(evo.Validate()).To(Succeed())
}

// CredentialsValue is what `${auth:...}` resolves to, and api.ConvertAuth round-trips
// it back into an AccountConfig in the provisioning tier. Every field therefore has
// to survive JSON — a field that does not is silently dropped between the auth entry
// and the provider, which presents as an authentication failure against a config that
// reads correctly.
func TestCredentialsValueRoundTrips(t *testing.T) {
	RegisterTestingT(t)

	cfg := &AccountConfig{
		AccessKey:           "AK",
		SecretKey:           "SK",
		SecurityToken:       "ST",
		Region:              "ru-moscow-2",
		ProjectName:         "forge-staging",
		DomainName:          "forge",
		DomainID:            "d-1",
		EnterpriseProjectID: "ep-1",
		AuthURL:             "https://iam.example.ru/v3",
	}

	var back AccountConfig
	Expect(json.Unmarshal([]byte(cfg.CredentialsValue()), &back)).To(Succeed())
	back.Credentials = api.Credentials{}
	Expect(&back).To(Equal(cfg))

	// An opaque blob supplied by the operator wins verbatim, exactly as in
	// yandex.AccountConfig.
	cfg.Credentials = api.Credentials{Credentials: `{"accessKey":"other"}`}
	Expect(cfg.CredentialsValue()).To(Equal(`{"accessKey":"other"}`))
}
