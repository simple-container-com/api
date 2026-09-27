// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"strings"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

// RegistrarTypeYandexDns is the `type:` a server.yaml registrar block declares.
//
// The name matters beyond taste: cmd/schema-gen's guessProviderFromResourceType
// tests the AWS token set (aws, s3, ecr, rds, ecs, lambda, fargate) BEFORE it tests
// yc/yandex, so a name carrying one of those would file the generated schema under
// docs/schemas/aws.
const RegistrarTypeYandexDns = "yc-dns"

// DefaultRecordTtl is the TTL applied to every recordset when the config does not
// set one. Yandex Cloud DNS has no automatic-TTL sentinel — unlike Cloudflare,
// where ttl=1 means "let the edge decide" — so some number always has to be picked.
const DefaultRecordTtl = 300

// RegistrarConfig is a Yandex Cloud DNS zone SC writes records into.
//
// The zone is always looked up, never created. A public zone is authoritative for a
// real domain: creating one from a deploy would either collide with the delegation
// that already exists or silently produce a second zone nobody's nameservers point
// at.
type RegistrarConfig struct {
	AccountConfig `json:",inline" yaml:",inline"`

	// ZoneName is the DNS zone this registrar serves, e.g. `simple-forge.ru`,
	// written the way it appears in a domain. It is what routing matches a
	// service's `domain:` against, and it is checked against the zone actually
	// looked up.
	ZoneName string `json:"zoneName" yaml:"zoneName"`

	// ZoneID is the YC resource id (`dns...`). Set it to skip the lookup-by-name
	// entirely — the surest form, and the one to reach for when the error below
	// says the zone could not be found.
	ZoneID string `json:"zoneId,omitempty" yaml:"zoneId,omitempty"`

	// ZoneResourceName is the *YC resource* name of the zone, which is NOT the DNS
	// zone: the zone serving `simple-forge.ru.` in the fleet's folder is a resource
	// named `simple-forge-ru`. YC's lookup-by-name takes this one. When unset it is
	// derived from ZoneName by replacing dots with hyphens, which is the convention
	// the console's own "create zone" form suggests but does not enforce.
	ZoneResourceName string `json:"zoneResourceName,omitempty" yaml:"zoneResourceName,omitempty"`

	// RecordTtl is the TTL in seconds for every recordset written here.
	RecordTtl int `json:"recordTtl,omitempty" yaml:"recordTtl,omitempty"`

	// CertificateID adopts an existing Certificate Manager certificate for the
	// domains published in this zone instead of provisioning one.
	//
	// Adoption is the default on purpose. A managed certificate validates over
	// DNS_CNAME at `_acme-challenge.<zone>`, a CNAME holds exactly one target, and
	// so a second managed certificate for the same domain contends with the first
	// one's renewal — the failure surfaces up to 90 days later, as an expiry.
	CertificateID string `json:"certificateId,omitempty" yaml:"certificateId,omitempty"`

	Records []api.DnsRecord `json:"dnsRecords,omitempty" yaml:"dnsRecords,omitempty"`
}

func ReadRegistrarConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &RegistrarConfig{})
}

func (r *RegistrarConfig) DnsRecords() []api.DnsRecord {
	return r.Records
}

// Zones implements api.RegistrarZonesAware, so a stack declaring several registrars
// can route a domain to this one without constructing it first.
func (r *RegistrarConfig) Zones() []string {
	if r.ZoneName == "" {
		return nil
	}
	return []string{r.ZoneName}
}

// EffectiveRecordTtl resolves the configured TTL, falling back to DefaultRecordTtl.
func (r *RegistrarConfig) EffectiveRecordTtl() int {
	if r.RecordTtl <= 0 {
		return DefaultRecordTtl
	}
	return r.RecordTtl
}

// EffectiveZoneResourceName resolves the YC resource name to look the zone up by.
func (r *RegistrarConfig) EffectiveZoneResourceName() string {
	if r.ZoneResourceName != "" {
		return r.ZoneResourceName
	}
	return strings.ReplaceAll(strings.TrimSuffix(r.ZoneName, "."), ".", "-")
}

func (r *RegistrarConfig) Validate() error {
	if r.ZoneName == "" {
		return errors.Errorf("zoneName is required: it is the DNS zone this registrar serves, e.g. %q", "simple-forge.ru")
	}
	if r.FolderID == "" && r.ZoneID == "" {
		return errors.Errorf("yandex DNS registrar for zone %q requires folderId (or an explicit zoneId): "+
			"a zone is looked up within a folder", r.ZoneName)
	}
	return nil
}
