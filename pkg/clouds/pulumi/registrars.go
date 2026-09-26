// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"sort"
	"strings"

	"github.com/pkg/errors"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// registrarDelegate is one configured registrar, not yet constructed. Zones come from
// config, so a domain can be routed without authenticating against any provider.
type registrarDelegate struct {
	name  string
	desc  api.RegistrarDescriptor
	zones []string
}

// multiRegistrar routes each DNS operation to whichever configured registrar is
// authoritative for the domain in question, so a single stack can serve one zone from
// Cloudflare and another from Yandex Cloud DNS.
//
// It is only used when a stack declares two or more registrars. With one (or none) the
// registrar is assigned directly — see initRegistrars — so that every stack predating
// `registrars:` keeps its exact behaviour, including type assertions on the concrete
// registrar such as *notConfigured in provisionProgram.
//
// Delegates are constructed lazily and memoised: declaring a registrar must not force
// every deploy to hold credentials for a zone it never touches.
type multiRegistrar struct {
	stackName string
	pref      *pApi.DnsPreference
	log       logger.Logger
	delegates []registrarDelegate
	built     map[string]pApi.Registrar
}

func newMultiRegistrar(stackName string, registrars map[string]api.RegistrarDescriptor, pref *pApi.DnsPreference, log logger.Logger) *multiRegistrar {
	m := &multiRegistrar{
		stackName: stackName,
		pref:      pref,
		log:       log,
		built:     map[string]pApi.Registrar{},
	}
	for name, desc := range registrars {
		m.delegates = append(m.delegates, registrarDelegate{
			name:  name,
			desc:  desc,
			zones: declaredZones(desc),
		})
	}
	// map iteration order is random, and which registrar wins a tie must not be
	sort.Slice(m.delegates, func(i, j int) bool { return m.delegates[i].name < m.delegates[j].name })
	return m
}

// declaredZones reads the zones a registrar serves off its already-parsed config.
func declaredZones(desc api.RegistrarDescriptor) []string {
	zonesAware, ok := desc.Config.Config.(api.RegistrarZonesAware)
	if !ok {
		return nil
	}
	return zonesAware.Zones()
}

func domainInZone(domain, zone string) bool {
	if zone == "" || domain == "" {
		return false
	}
	domain, zone = strings.TrimSuffix(domain, "."), strings.TrimSuffix(zone, ".")
	return strings.EqualFold(domain, zone) || strings.HasSuffix(strings.ToLower(domain), "."+strings.ToLower(zone))
}

// forDomain picks the registrar authoritative for a domain: the longest declared zone
// that contains it, falling back to whichever registrar declares the preferred base
// zone. The fallback matters because a stack may legitimately declare a zone its
// services do not sit under and redirect them there via baseDnsZone.
func (m *multiRegistrar) forDomain(domain string) (*registrarDelegate, error) {
	best, bestLen := -1, -1
	for i := range m.delegates {
		for _, zone := range m.delegates[i].zones {
			if domainInZone(domain, zone) && len(zone) > bestLen {
				best, bestLen = i, len(zone)
			}
		}
	}
	if best < 0 && m.pref != nil && m.pref.BaseZone != "" {
		for i := range m.delegates {
			for _, zone := range m.delegates[i].zones {
				if domainInZone(m.pref.BaseZone, zone) {
					best = i
					break
				}
			}
		}
	}
	if best < 0 {
		var declared []string
		for i := range m.delegates {
			declared = append(declared, m.delegates[i].name+" ("+strings.Join(m.delegates[i].zones, ", ")+")")
		}
		return nil, errors.Errorf("no registrar of stack %q handles domain %q, declared registrars: %s",
			m.stackName, domain, strings.Join(declared, "; "))
	}
	return &m.delegates[best], nil
}

func (m *multiRegistrar) build(ctx *sdk.Context, delegate *registrarDelegate) (pApi.Registrar, error) {
	if registrar, found := m.built[delegate.name]; found {
		return registrar, nil
	}
	registrarInit, ok := pApi.RegistrarFuncByType[delegate.desc.Type]
	if !ok {
		return nil, errors.Errorf("unsupported registrar type %q for stack %q", delegate.desc.Type, m.stackName)
	}
	registrar, err := registrarInit(ctx, delegate.desc, pApi.ProvisionParams{
		Log:           m.log,
		DnsPreference: m.pref,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to init registrar %q for stack %q", delegate.name, m.stackName)
	}
	m.built[delegate.name] = registrar
	return registrar, nil
}

func (m *multiRegistrar) registrarFor(ctx *sdk.Context, domain string) (pApi.Registrar, error) {
	delegate, err := m.forDomain(domain)
	if err != nil {
		return nil, err
	}
	return m.build(ctx, delegate)
}

// MainDomain answers from config rather than from a delegate, since it takes no context
// to construct one with. The preferred base zone wins, matching what a registrar bound
// to that zone would report.
func (m *multiRegistrar) MainDomain() string {
	if m.pref != nil && m.pref.BaseZone != "" {
		return m.pref.BaseZone
	}
	for i := range m.delegates {
		if len(m.delegates[i].zones) > 0 {
			return m.delegates[i].zones[0]
		}
	}
	return ""
}

// ProvisionRecords provisions the statically declared records of every registrar. This
// runs on the parent stack, which owns all of them, so there is nothing to defer here.
func (m *multiRegistrar) ProvisionRecords(ctx *sdk.Context, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	var out *api.ResourceOutput
	for i := range m.delegates {
		registrar, err := m.build(ctx, &m.delegates[i])
		if err != nil {
			return nil, err
		}
		if out, err = registrar.ProvisionRecords(ctx, params); err != nil {
			return nil, errors.Wrapf(err, "failed to provision records of registrar %q", m.delegates[i].name)
		}
	}
	return out, nil
}

func (m *multiRegistrar) NewRecord(ctx *sdk.Context, dnsRecord api.DnsRecord) (*api.ResourceOutput, error) {
	registrar, err := m.registrarFor(ctx, dnsRecord.Name)
	if err != nil {
		return nil, err
	}
	return registrar.NewRecord(ctx, dnsRecord)
}

func (m *multiRegistrar) NewOverrideHeaderRule(ctx *sdk.Context, stack api.Stack, rule pApi.OverrideHeaderRule) (*api.ResourceOutput, error) {
	registrar, err := m.registrarFor(ctx, rule.FromHost)
	if err != nil {
		return nil, err
	}
	return registrar.NewOverrideHeaderRule(ctx, stack, rule)
}

func (m *multiRegistrar) ProvisionDomainForEndpoint(ctx *sdk.Context, stack api.Stack, endpoint pApi.DomainEndpoint) (*api.ResourceOutput, error) {
	registrar, err := m.registrarFor(ctx, endpoint.Domain)
	if err != nil {
		return nil, err
	}
	return registrar.ProvisionDomainForEndpoint(ctx, stack, endpoint)
}
