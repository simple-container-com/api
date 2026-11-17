// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// registrarDelegate is one configured registrar, not yet constructed. Zones come from
// config, so a domain can be routed without authenticating against any provider.
type registrarDelegate struct {
	name      string
	desc      api.RegistrarDescriptor
	zones     []string
	isDefault bool
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
	// mu guards built: a plugin resource sidecar resolves its domain's registrar
	// from inside an Apply, concurrently with DNS calls from other resources.
	mu    sync.Mutex
	built map[string]pApi.Registrar
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
			name:      name,
			desc:      desc,
			zones:     declaredZones(desc),
			isDefault: desc.Default,
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
	return api.DomainInZone(domain, zone)
}

// forDomain picks the registrar authoritative for a domain: the longest declared zone
// that contains it, then whichever registrar declares the preferred base zone, then the
// one marked `default: true`.
//
// The base-zone fallback matters because a stack may legitimately declare a zone its
// services do not sit under and redirect them there via baseDnsZone. The default
// fallback matters because a declared zone is narrower than what a registrar can serve
// — see api.RegistrarDescriptor.Default — and without it the fleet's own registrar,
// which declares one zone and publishes services in another, would match nothing.
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
		for i := range m.delegates {
			if m.delegates[i].isDefault {
				best = i
				// SAY SO. Choosing the default for a domain no zone claims is
				// correct and is what the marker exists for — but it is also
				// indistinguishable, from the outside, from a zone that SHOULD
				// have matched and did not. On 2026-11-09 every domain in the
				// fleet took this branch (no registrar could report its zones)
				// and the only visible symptom was the default provider failing
				// to find a zone it was never meant to serve.
				if m.log != nil {
					m.log.Info(context.TODO(),
						"registrar routing: no declared zone matches %q, falling back to the default registrar %q (type %q). "+
							"Declared: %s. If that domain was meant to be served by another registrar, its `zoneName:`/`zones:` is not being read.",
						domain, m.delegates[i].name, m.delegates[i].desc.Type, strings.Join(m.declaredSummary(), "; "))
				}
				break
			}
		}
	}
	if best < 0 {
		return nil, errors.Errorf("no registrar of stack %q handles domain %q and none is marked `default: true`, "+
			"declared registrars: %s", m.stackName, domain, strings.Join(m.declaredSummary(), "; "))
	}
	return &m.delegates[best], nil
}

// declaredSummary renders "name (zone, zone)" per delegate — the operator-
// facing inventory both the no-registrar error and the default-fallback notice
// need. A delegate with NO zones renders as `name ()`, which is itself the
// useful signal: it means that registrar's config could not report a zone, so
// it can never win a domain on merit.
func (m *multiRegistrar) declaredSummary() []string {
	var declared []string
	for i := range m.delegates {
		declared = append(declared, m.delegates[i].name+" ("+strings.Join(m.delegates[i].zones, ", ")+")")
	}
	return declared
}

func (m *multiRegistrar) build(ctx *sdk.Context, delegate *registrarDelegate) (pApi.Registrar, error) {
	return m.buildForBaseZone(ctx, delegate, "")
}

// buildForBaseZone constructs (and caches) the registrar for a delegate,
// keyed on (delegate.name, baseZone). When baseZone is empty the behavior
// matches the legacy build — DnsPreference comes from `m.pref`. When
// baseZone is non-empty, the registrar is initialized with a synthetic
// `DnsPreference{BaseZone: baseZone}`, which per-registrar impls use to
// override their init-time zone binding (sc-plugin-cloudflare's
// `Registrar()`: `baseZoneName := cfg.ZoneName; if params.DnsPreference != nil
// && params.DnsPreference.BaseZone != "" { baseZoneName =
// params.DnsPreference.BaseZone }`).
//
// Separate cache slots per baseZone — a parent stack where one component
// targets `simple-container.com` and another targets `simple-forge.com`
// ends up with two distinct bound registrars under the same delegate,
// each with its own zone lookup. The legacy call site
// (baseZone="") stays single-instance per delegate and keeps today's
// behavior unchanged.
//
// Live-caught 2026-11-17 on backup-smoke-tier4 parent provision
// 44143999005: the fleet's lone Cloudflare registrar is init-bound to
// `simple-container.com`, so a Component with `ingress.hostname:
// backup.simple-forge.com` hit Cloudflare's `Route pattern must include
// zone name` 400. See the DomainEndpoint.BaseZone docstring.
func (m *multiRegistrar) buildForBaseZone(ctx *sdk.Context, delegate *registrarDelegate, baseZone string) (pApi.Registrar, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cacheKey := delegate.name
	if baseZone != "" {
		cacheKey = delegate.name + "|" + baseZone
	}
	if registrar, found := m.built[cacheKey]; found {
		return registrar, nil
	}
	registrarInit, ok := pApi.RegistrarFuncByType[delegate.desc.Type]
	if !ok {
		return nil, errors.Errorf("unsupported registrar type %q for stack %q", delegate.desc.Type, m.stackName)
	}
	pref := m.pref
	if baseZone != "" {
		pref = &pApi.DnsPreference{BaseZone: baseZone}
	}
	registrar, err := registrarInit(ctx, delegate.desc, pApi.ProvisionParams{
		Log:           m.log,
		DnsPreference: pref,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to init registrar %q for stack %q", delegate.name, m.stackName)
	}
	m.built[cacheKey] = registrar
	return registrar, nil
}

// RegistrarTypeFor names the type of the registrar authoritative for domain, from
// config alone — nothing is constructed.
func (m *multiRegistrar) RegistrarTypeFor(domain string) (string, error) {
	delegate, err := m.forDomain(domain)
	if err != nil {
		return "", err
	}
	return delegate.desc.Type, nil
}

// RegistrarFor constructs (once) and returns the registrar authoritative for domain.
//
// Exported for the plugin resource sidecar. A client stack's `domain:` is handed to
// a plugin template inside its Provision inputs, and the plugin publishes it with a
// registrar of its own — which it only holds if core has sent it ProvisionRegistrar.
// Core builds delegates lazily, i.e. only for domains it writes itself, so without
// resolving the stack's domain here first the plugin never learns that zone's
// registrar (see makeResourceSidecar).
func (m *multiRegistrar) RegistrarFor(ctx *sdk.Context, domain string) (pApi.Registrar, error) {
	delegate, err := m.forDomain(domain)
	if err != nil {
		return nil, err
	}
	return m.build(ctx, delegate)
}

// MainDomain answers from config rather than from a delegate, since it takes no context
// to construct one with. The preferred base zone wins, matching what a registrar bound
// to that zone would report; then the default registrar's zone, which is the stack's
// own front door; only then the first declared zone.
func (m *multiRegistrar) MainDomain() string {
	if m.pref != nil && m.pref.BaseZone != "" {
		return m.pref.BaseZone
	}
	for i := range m.delegates {
		if m.delegates[i].isDefault && len(m.delegates[i].zones) > 0 {
			return m.delegates[i].zones[0]
		}
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
	registrar, err := m.RegistrarFor(ctx, dnsRecord.Name)
	if err != nil {
		return nil, err
	}
	return registrar.NewRecord(ctx, dnsRecord)
}

func (m *multiRegistrar) NewOverrideHeaderRule(ctx *sdk.Context, stack api.Stack, rule pApi.OverrideHeaderRule) (*api.ResourceOutput, error) {
	registrar, err := m.RegistrarFor(ctx, rule.FromHost)
	if err != nil {
		return nil, err
	}
	return registrar.NewOverrideHeaderRule(ctx, stack, rule)
}

func (m *multiRegistrar) ProvisionDomainForEndpoint(ctx *sdk.Context, stack api.Stack, endpoint pApi.DomainEndpoint) (*api.ResourceOutput, error) {
	// Routing: endpoint.Domain selects the delegate (longest declared
	// zone suffix wins; else fall through to `default: true` registrar).
	delegate, err := m.forDomain(endpoint.Domain)
	if err != nil {
		return nil, err
	}
	// Zone binding: if the caller passed BaseZone (Component ingress
	// path; SC core's `wireComponentDomain` derives it from the hostname),
	// build the registrar bound to THAT zone rather than the delegate's
	// init-time default. `buildForBaseZone` caches per-(name, baseZone)
	// so repeated calls for the same zone reuse the same instance.
	// Legacy callers (client-stack `domain:` wiring) pass empty BaseZone
	// and keep today's behavior — `m.pref` controls the zone.
	registrar, err := m.buildForBaseZone(ctx, delegate, endpoint.BaseZone)
	if err != nil {
		return nil, err
	}
	return registrar.ProvisionDomainForEndpoint(ctx, stack, endpoint)
}
