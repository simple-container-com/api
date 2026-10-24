// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cloudru

import (
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/cloudru/evolution"
)

// RegistrarTypeCloudruDns is the `type:` a server.yaml registrar block declares to
// write records into a Cloud.ru **Evolution** Cloud DNS zone.
//
// `cloudru-dns` and not `evolution-dns`: the `cloudru-` prefix is what
// cmd/schema-gen's guessProviderFromResourceType matches to file the generated
// schema under docs/schemas/cloudru, and the Advanced estate has no DNS service of
// its own to collide with (its ex-SberCloud DNS is reached through the bridged
// provider, which this is not).
const RegistrarTypeCloudruDns = "cloudru-dns"

// DnsRegistrarConfig is a Cloud.ru Cloud DNS zone SC writes records into.
//
// The zone is looked up and never created, for the same reason as the Yandex
// registrar: a public zone is authoritative for a real domain, so creating one from
// a deploy either collides with the delegation that already exists or produces a
// second zone nobody's nameservers point at.
//
// Delegate the domain to the nameservers in evolution.DnsNameservers before
// expecting a record written here to resolve. There is no verification step and no
// TXT challenge — a zone is authoritative as soon as it exists (the API has no
// `valid` field at all; an earlier version of this package gated on one, which the
// live API would have left false forever).
type DnsRegistrarConfig struct {
	// EvolutionAccountConfig supplies the credential AND `projectId`, which is what
	// a lookup-by-name is scoped to. There is no separate DNS-specific scope: an
	// earlier version of this struct carried a `parentId` for a "Cloud DNS parent"
	// that the real API does not have.
	EvolutionAccountConfig `json:",inline" yaml:",inline"`

	// ZoneName is the DNS zone this registrar serves, e.g. `simple-forge.ru`,
	// written the way it appears in a domain. It is what routing matches a
	// service's `domain:` against, and it is checked against the zone actually
	// looked up.
	//
	// Matched against the zone's `domain`, not its `name` — in the console those
	// differ: `atriumdev.online` is stored with `name: atriumdev-online` and
	// `domain: atriumdev.online.`. Write the domain.
	ZoneName string `json:"zoneName" yaml:"zoneName"`

	// ZoneID skips the lookup-by-name. Optional, and worth setting when a project
	// holds several zones: it is the path segment of the zone's own console URL,
	// `/spa/dns/zones/public/<zoneId>/edit`.
	ZoneID string `json:"zoneId,omitempty" yaml:"zoneId,omitempty"`

	// RecordTtl is the TTL in seconds for records written here.
	//
	// Cloud DNS honours it (a record written with 900 reads back as 900), but has no
	// "automatic" sentinel the way Cloudflare's `ttl: 1` does — omitting the field
	// stores a literal 0. So some number always has to be chosen, and leaving this
	// unset picks evolution.DefaultDnsRecordTtl rather than writing a zero TTL into
	// the operator's zone.
	RecordTtl int `json:"recordTtl,omitempty" yaml:"recordTtl,omitempty"`

	Records []api.DnsRecord `json:"dnsRecords,omitempty" yaml:"dnsRecords,omitempty"`
}

func ReadDnsRegistrarConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &DnsRegistrarConfig{})
}

func (r *DnsRegistrarConfig) DnsRecords() []api.DnsRecord {
	return r.Records
}

// Zones implements api.RegistrarZonesAware, so a stack declaring several registrars
// can route a domain to this one without constructing it first.
func (r *DnsRegistrarConfig) Zones() []string {
	if r.ZoneName == "" {
		return nil
	}
	return []string{r.ZoneName}
}

// EffectiveRecordTtl resolves the configured TTL, defaulting rather than sending a
// zero — an omitted ttl is stored as a literal 0 by Cloud DNS, not replaced by a
// platform default. See RecordTtl.
func (r *DnsRegistrarConfig) EffectiveRecordTtl() int {
	if r.RecordTtl <= 0 {
		return evolution.DefaultDnsRecordTtl
	}
	return r.RecordTtl
}

func (r *DnsRegistrarConfig) Validate() error {
	if r.ZoneName == "" {
		return errors.Errorf("`zoneName` is required for registrar type %q: it is the DNS zone this registrar "+
			"serves, e.g. %q", RegistrarTypeCloudruDns, "simple-forge.ru")
	}
	if r.ZoneID == "" && r.EvolutionAccountConfig.ProjectID == "" {
		return errors.Errorf("registrar type %q for zone %q needs either `zoneId` or a `projectId` on its auth "+
			"entry to look the zone up in. `zoneId` is the path segment of the zone's own console URL "+
			"(.../dns/zones/public/<zoneId>/edit); `projectId` is the project that owns the zone, and a lookup "+
			"scoped to the wrong one does not error — it finds no zones, which looks identical to having none",
			RegistrarTypeCloudruDns, r.ZoneName)
	}
	// The credential half is validated by the embedded config, which also refuses
	// an unexpanded `${…}` placeholder — the mistake that costs a mid-deploy
	// failure rather than a config error (see unexpandedPlaceholder).
	return r.EvolutionAccountConfig.Validate()
}

// Nameservers is the NS set `atriumdev.online`'s registrar must point at for this
// registrar to be able to serve it. Surfaced here so the error paths and the CLI can
// name them without reaching into the evolution package.
func (r *DnsRegistrarConfig) Nameservers() []string {
	return evolution.DnsNameservers
}
