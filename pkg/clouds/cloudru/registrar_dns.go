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
type DnsRegistrarConfig struct {
	EvolutionAccountConfig `json:",inline" yaml:",inline"`

	// ZoneName is the DNS zone this registrar serves, e.g. `simple-forge.ru`,
	// written the way it appears in a domain. It is what routing matches a
	// service's `domain:` against, and it is checked against the zone actually
	// looked up.
	ZoneName string `json:"zoneName" yaml:"zoneName"`

	// ZoneID skips the lookup-by-name, and is the form to PREFER on this cloud.
	//
	// It is unambiguous and it is right there in the console: open the zone and
	// the id is the path segment of the URL,
	// `/spa/dns/zones/public/<zoneId>/edit`. Setting it makes ParentID
	// unnecessary — which matters, because ParentID is the one value here nobody
	// has been able to point at reliably (see below).
	ZoneID string `json:"zoneId,omitempty" yaml:"zoneId,omitempty"`

	// ParentID scopes a lookup-by-name to the Cloud DNS parent the zone lives
	// under. **Prefer ZoneID and leave this empty** unless you have a reason not
	// to.
	//
	// What this id actually is remains UNRESOLVED, and the honest state of our
	// knowledge is worth recording rather than guessing at again:
	//
	//   - It is not the project id in the sense of "any project id works" — but
	//     that has never been *disproved* either, because every call this package
	//     can make to Cloud DNS answers 403 (the service account lacks the role),
	//     so neither the project id nor the customer id has ever been observed
	//     either working or being rejected on its merits.
	//   - An earlier note here claimed it was a "Cloud DNS service instance id,
	//     visible in the console URL `/spa/cloud-dns/<parentId>`". That URL shape
	//     does not exist: an operator's real zone URL is
	//     `/spa/dns/zones/public/<zoneId>/edit?customerId=…&projectId=…`, which
	//     exposes a zone id, a customer id and a project id, and no fourth
	//     "service instance" id at all. Do not send people hunting for it.
	//
	// The cost of a wrong value is what makes this worth the warning: Cloud DNS
	// answers a list scoped to an unrelated parent with an **empty set**, which is
	// indistinguishable from a parent that has no zones. So `sc cloudru dns-record
	// apply` prints the zones it can see when a lookup comes up empty.
	//
	// Required only when ZoneID is unset, since it is what the lookup is scoped to.
	ParentID string `json:"parentId,omitempty" yaml:"parentId,omitempty"`

	// RecordTtl is the TTL in seconds for every record written here. Cloud DNS has
	// no automatic-TTL sentinel (unlike Cloudflare's ttl=1), so some number is
	// always picked — see evolution.DefaultDnsRecordTtl.
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

// EffectiveRecordTtl resolves the configured TTL, falling back to the cloud's
// documented default.
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
	if r.ParentID == "" && r.ZoneID == "" {
		return errors.Errorf("registrar type %q for zone %q requires `zoneId` (preferred) or `parentId`. "+
			"`zoneId` is the path segment of the zone's own console URL "+
			"(.../dns/zones/public/<zoneId>/edit) and needs nothing else; `parentId` scopes a lookup by name, "+
			"and a wrong value there does not error — it lists an empty set of zones, which looks identical to "+
			"having none",
			RegistrarTypeCloudruDns, r.ZoneName)
	}
	// The credential half is validated by the embedded config, which also refuses
	// an unexpanded `${…}` placeholder — the mistake that costs a mid-deploy
	// failure rather than a config error (see unexpandedPlaceholder).
	return r.EvolutionAccountConfig.Validate()
}
