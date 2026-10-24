// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cloudru

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/cloudru/evolution"
)

// validEvolutionAuth is the credential half every case below needs, since
// DnsRegistrarConfig.Validate also validates it.
func validEvolutionAuth() EvolutionAccountConfig {
	return EvolutionAccountConfig{
		KeyID:     "11111111-2222-3333-4444-555555555555",
		KeySecret: "s3cret",
		ProjectID: "a8236278-5c01-4de3-bad5-b4737287ae1a",
	}
}

// TestDnsRegistrarConfigIsZonesAware is what makes a stack able to route a domain to
// this registrar without constructing it first. The interface is optional, so dropping
// the method would not fail to compile — it would silently stop the registrar from
// ever being chosen.
func TestDnsRegistrarConfigIsZonesAware(t *testing.T) {
	RegisterTestingT(t)

	var cfg any = &DnsRegistrarConfig{ZoneName: "simple-forge.ru"}
	aware, ok := cfg.(api.RegistrarZonesAware)
	Expect(ok).To(BeTrue(), "*DnsRegistrarConfig must implement api.RegistrarZonesAware")
	Expect(aware.Zones()).To(Equal([]string{"simple-forge.ru"}))

	Expect((&DnsRegistrarConfig{}).Zones()).To(BeNil(), "an unconfigured registrar claims no zone")
}

// TestDnsRegistrarEffectiveRecordTtl pins that some TTL is always produced: Cloud DNS
// has no "automatic" sentinel (unlike Cloudflare's ttl=1), so a zero would travel
// verbatim.
func TestDnsRegistrarEffectiveRecordTtl(t *testing.T) {
	RegisterTestingT(t)

	Expect((&DnsRegistrarConfig{}).EffectiveRecordTtl()).To(Equal(evolution.DefaultDnsRecordTtl))
	Expect((&DnsRegistrarConfig{RecordTtl: -1}).EffectiveRecordTtl()).To(Equal(evolution.DefaultDnsRecordTtl))
	Expect((&DnsRegistrarConfig{RecordTtl: 60}).EffectiveRecordTtl()).To(Equal(60))
}

func TestDnsRegistrarValidate(t *testing.T) {
	RegisterTestingT(t)

	t.Run("refuses a missing zoneName", func(t *testing.T) {
		cfg := &DnsRegistrarConfig{EvolutionAccountConfig: validEvolutionAuth(), ParentID: "svc-1"}
		Expect(cfg.Validate()).To(MatchError(ContainSubstring("`zoneName` is required")))
	})

	// The expensive mistake on this cloud: `parentId` is the Cloud DNS SERVICE
	// INSTANCE id, and a wrong one does not fail — it lists an empty set of zones.
	// Validate can only catch it being absent, which is why the message says which
	// id it wants.
	t.Run("refuses a zone with neither parentId nor zoneId", func(t *testing.T) {
		cfg := &DnsRegistrarConfig{EvolutionAccountConfig: validEvolutionAuth(), ZoneName: "simple-forge.ru"}
		err := cfg.Validate()
		Expect(err).To(MatchError(ContainSubstring("requires `parentId`")))
		Expect(err).To(MatchError(ContainSubstring("not the project id")))
	})

	t.Run("accepts an explicit zoneId without parentId", func(t *testing.T) {
		cfg := &DnsRegistrarConfig{
			EvolutionAccountConfig: validEvolutionAuth(),
			ZoneName:               "simple-forge.ru",
			ZoneID:                 "zone-1",
		}
		Expect(cfg.Validate()).To(Succeed())
	})

	// The credential half is not re-implemented here, it is delegated — so the
	// unexpanded-placeholder guard that was live-caught mid-deploy has to still fire
	// through a registrar config.
	t.Run("still refuses an unexpanded placeholder in the credentials", func(t *testing.T) {
		auth := validEvolutionAuth()
		auth.KeyID = "${secret:CLOUDRU_KEY_ID}"
		cfg := &DnsRegistrarConfig{EvolutionAccountConfig: auth, ZoneName: "simple-forge.ru", ParentID: "svc-1"}
		Expect(cfg.Validate()).To(MatchError(ContainSubstring("still holds a `${…}` placeholder")))
	})

	t.Run("refuses missing credentials", func(t *testing.T) {
		cfg := &DnsRegistrarConfig{ZoneName: "simple-forge.ru", ParentID: "svc-1"}
		Expect(cfg.Validate()).To(MatchError(ContainSubstring("`keyId` is required")))
	})
}

// TestDnsRegistrarIsRegistered guards the wiring: a registrar type absent from the
// config registry is reported as an unknown `type:` at provision time, which reads
// like a typo in the operator's server.yaml rather than like a missing registration.
func TestDnsRegistrarIsRegistered(t *testing.T) {
	RegisterTestingT(t)

	read, found := api.GetRegisteredProviderConfigs()[RegistrarTypeCloudruDns]
	Expect(found).To(BeTrue(), "%q must be registered in pkg/clouds/cloudru/init.go", RegistrarTypeCloudruDns)
	Expect(read).ToNot(BeNil())
}
