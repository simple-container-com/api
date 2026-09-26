// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// DefaultRegistrarName is the key the legacy single `registrar:` block is normalised
// under. It is deliberately the empty string so that anything derived from a
// registrar's name is unchanged for stacks written before `registrars:` existed.
const DefaultRegistrarName = ""

type RegistrarDescriptor struct {
	Type    string `json:"type" yaml:"type"`
	Config  `json:",inline" yaml:",inline"`
	Inherit `json:",inline" yaml:",inline"`
}

// IsConfigured reports whether the descriptor declares anything at all. A server.yaml
// with no registrar block yields the zero value, which is legal and means "this stack
// manages no DNS".
func (r RegistrarDescriptor) IsConfigured() bool {
	return r.Type != "" || r.IsInherited() || r.Config.Config != nil
}

// AllRegistrars normalises the two ways a server.yaml may declare registrars — the
// legacy single `registrar:` block and the `registrars:` map — into a single map.
// Declaring both at once is an error rather than a silent precedence rule. The result
// is empty when no registrar is configured, which is a valid configuration.
func (s PerStackResourcesDescriptor) AllRegistrars() (map[string]RegistrarDescriptor, error) {
	if len(s.Registrars) == 0 {
		if !s.Registrar.IsConfigured() {
			return map[string]RegistrarDescriptor{}, nil
		}
		return map[string]RegistrarDescriptor{DefaultRegistrarName: s.Registrar}, nil
	}
	if s.Registrar.IsConfigured() {
		return nil, errors.New("cannot declare both `registrar:` and `registrars:` in the same stack: " +
			"move the single `registrar:` block into `registrars:` under a name of your choosing")
	}
	return lo.Assign(s.Registrars), nil
}

// RegistrarZonesAware is implemented by a RegistrarConfig that knows which DNS zones it
// is authoritative for, so a domain can be routed to the right registrar without having
// to construct (and therefore authenticate) any of them first. Optional: a registrar
// config that does not implement it can still be the only registrar in a stack.
type RegistrarZonesAware interface {
	Zones() []string
}

// DomainInZone reports whether a domain is served by a DNS zone — the zone itself, or
// anything under it. Both sides are compared without a trailing dot and without regard
// to case, and a match must land on a label boundary so that `notsimple-forge.com` is
// not read as being inside `simple-forge.com`.
func DomainInZone(domain, zone string) bool {
	if zone == "" || domain == "" {
		return false
	}
	domain, zone = strings.TrimSuffix(domain, "."), strings.TrimSuffix(zone, ".")
	return strings.EqualFold(domain, zone) || strings.HasSuffix(strings.ToLower(domain), "."+strings.ToLower(zone))
}

type DnsRecord struct {
	Name     string           `json:"name" yaml:"name"`
	Type     string           `json:"type" yaml:"type"`
	ValueOut sdk.StringOutput `json:"valueOut" yaml:"valueOut"`
	Value    string           `json:"value" yaml:"value"`
	Proxied  bool             `json:"proxied" yaml:"proxied"`
}
