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
// DnsRegistrarConfig.Validate also validates it. Its `projectId` is also what a
// zone lookup is scoped to — Cloud DNS has no scope of its own, which is why this
// struct no longer carries a `parentId`.
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

// TestDnsRegistrarEffectiveRecordTtl: an unset TTL defaults rather than sending 0.
//
// Cloud DNS has no "automatic" sentinel (Cloudflare's `ttl: 1`), and an omitted ttl
// is stored as a literal 0 rather than replaced by a platform default — so a zero
// here would put a zero TTL in the operator's zone. A measurement earlier on
// 2026-10-24 read a record back with `ttl: 0` and concluded the field was ignored
// entirely; that record had been created without a ttl, and re-measuring showed 900
// stored as 900.
func TestDnsRegistrarEffectiveRecordTtl(t *testing.T) {
	RegisterTestingT(t)

	Expect((&DnsRegistrarConfig{}).EffectiveRecordTtl()).To(Equal(evolution.DefaultDnsRecordTtl))
	Expect((&DnsRegistrarConfig{RecordTtl: -1}).EffectiveRecordTtl()).To(Equal(evolution.DefaultDnsRecordTtl))
	Expect((&DnsRegistrarConfig{RecordTtl: 60}).EffectiveRecordTtl()).To(Equal(60))
}

func TestDnsRegistrarValidate(t *testing.T) {
	RegisterTestingT(t)

	t.Run("refuses a missing zoneName", func(t *testing.T) {
		cfg := &DnsRegistrarConfig{EvolutionAccountConfig: validEvolutionAuth()}
		Expect(cfg.Validate()).To(MatchError(ContainSubstring("`zoneName` is required")))
	})

	// A lookup-by-name is scoped to the auth entry's `projectId`, and a lookup scoped
	// to the wrong project does not fail — it finds no zones, indistinguishable from a
	// project with none. So with neither a `zoneId` nor a `projectId` there is nothing
	// to look the zone up in, and the message has to name both ways out.
	t.Run("refuses a zone with neither zoneId nor a projectId to search", func(t *testing.T) {
		auth := validEvolutionAuth()
		auth.ProjectID = ""
		cfg := &DnsRegistrarConfig{EvolutionAccountConfig: auth, ZoneName: "simple-forge.ru"}
		err := cfg.Validate()
		Expect(err).To(MatchError(ContainSubstring("`zoneId`")))
		Expect(err).To(MatchError(ContainSubstring("`projectId`")))
		Expect(err).To(MatchError(ContainSubstring("/dns/zones/public/<zoneId>/edit")))
	})

	t.Run("accepts a projectId alone, since the zone is looked up by name", func(t *testing.T) {
		cfg := &DnsRegistrarConfig{EvolutionAccountConfig: validEvolutionAuth(), ZoneName: "simple-forge.ru"}
		Expect(cfg.Validate()).To(Succeed())
	})

	t.Run("accepts an explicit zoneId", func(t *testing.T) {
		cfg := &DnsRegistrarConfig{
			EvolutionAccountConfig: validEvolutionAuth(),
			ZoneName:               "simple-forge.ru",
			ZoneID:                 "88fc588d-4ca5-4504-a2f6-5cbbdf54f331",
		}
		Expect(cfg.Validate()).To(Succeed())
	})

	// The credential half is not re-implemented here, it is delegated — so the
	// unexpanded-placeholder guard that was live-caught mid-deploy has to still fire
	// through a registrar config.
	t.Run("still refuses an unexpanded placeholder in the credentials", func(t *testing.T) {
		auth := validEvolutionAuth()
		auth.KeyID = "${secret:CLOUDRU_KEY_ID}"
		cfg := &DnsRegistrarConfig{EvolutionAccountConfig: auth, ZoneName: "simple-forge.ru"}
		Expect(cfg.Validate()).To(MatchError(ContainSubstring("still holds a `${…}` placeholder")))
	})

	t.Run("refuses missing credentials", func(t *testing.T) {
		cfg := &DnsRegistrarConfig{ZoneName: "simple-forge.ru", ZoneID: "zone-1"}
		Expect(cfg.Validate()).To(MatchError(ContainSubstring("`keyId` is required")))
	})
}

// TestDnsRegistrarNameserversAreTheEvolutionPair keeps the operator-facing answer to
// "what do I set at my registrar" reachable from the registrar itself, and pins it to
// the Evolution pair rather than the sbercloud.ru pair Cloud.ru's own Cloud DNS docs
// name (those pages describe the Advanced estate without saying so).
func TestDnsRegistrarNameserversAreTheEvolutionPair(t *testing.T) {
	RegisterTestingT(t)

	Expect((&DnsRegistrarConfig{}).Nameservers()).To(Equal(evolution.DnsNameservers))
	Expect((&DnsRegistrarConfig{}).Nameservers()).To(ConsistOf("evo-cns01.cloud.ru", "evo-cns02.cloud.ru"))
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
