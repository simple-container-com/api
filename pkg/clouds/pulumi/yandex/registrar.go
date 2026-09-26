// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/yandex"
	sdkYandex "github.com/simple-container-com/pulumi-yandex/sdk/go/yandex"
)

type registrar struct {
	provider *sdkYandex.Provider
	config   *yandex.RegistrarConfig
	zone     *sdkYandex.LookupDnsZoneResult
	// zoneID is the id every recordset is written against. It is kept separate
	// from zone because the lookup does not always put it in the same field.
	zoneID string
	log    logger.Logger
}

// Registrar resolves a Yandex Cloud DNS zone into something records can be written
// to. The zone is looked up and never created — see yandex.RegistrarConfig.
//
// Unlike the Cloudflare registrar, this one does NOT honour
// DnsPreference.BaseZone as a zone override. There it is a way to point a whole
// stack at a different zone of the same account; here a stack with more than one
// registrar uses the preference to choose *between* registrars (see
// pulumi.multiRegistrar), and letting it also rewrite the chosen registrar's zone
// would mean the registrar picked for a zone then serves a different one.
func Registrar(ctx *sdk.Context, config api.RegistrarDescriptor, params pApi.ProvisionParams) (pApi.Registrar, error) {
	cfg, ok := config.Config.Config.(*yandex.RegistrarConfig)
	if !ok {
		return nil, errors.Errorf("invalid config type %T is not *yandex.RegistrarConfig", config.Config.Config)
	}
	// `${auth:yc}` resolves to the opaque credentials blob only, so the ids live
	// inside it and have to be unpacked before anything below can use them.
	if err := api.ConvertAuth(cfg, &cfg.AccountConfig); err != nil {
		return nil, errors.Wrapf(err, "failed to convert auth config to yandex.AccountConfig")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	providerName := fmt.Sprintf("%s-dns", cfg.EffectiveZoneResourceName())
	providerArgs := &sdkYandex.ProviderArgs{
		CloudId:  sdk.StringPtr(cfg.CloudID),
		FolderId: sdk.StringPtr(cfg.FolderID),
		RegionId: sdk.StringPtr(cfg.EffectiveRegion()),
		Zone:     sdk.StringPtr(cfg.EffectiveZone()),
	}
	if cfg.ServiceAccountKey != "" {
		providerArgs.ServiceAccountKeyFile = sdk.StringPtr(cfg.ServiceAccountKey)
	}
	provider, err := sdkYandex.NewProvider(ctx, providerName, providerArgs)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to init yandex provider for DNS zone %q", cfg.ZoneName)
	}

	zone, err := lookupZone(ctx, cfg, provider)
	if err != nil {
		return nil, err
	}
	zoneID, err := zoneIDOf(zone, cfg)
	if err != nil {
		return nil, err
	}
	params.Log.Info(ctx.Context(), "resolved yandex DNS zone %q (%s) for %q", zone.Name, zoneID, cfg.ZoneName)

	return &registrar{
		provider: provider,
		config:   cfg,
		zone:     zone,
		zoneID:   zoneID,
		log:      params.Log,
	}, nil
}

// zoneIDOf reads the zone's id out of a lookup result. `DnsZoneId` echoes the
// argument, so it is empty on a lookup by name — the id then lives in `Id`, the
// provider-assigned one. Looking only at `DnsZoneId` yields an empty ZoneId that YC
// rejects with a generic message several minutes into a deploy (live-caught
// 2026-09-26).
func zoneIDOf(zone *sdkYandex.LookupDnsZoneResult, cfg *yandex.RegistrarConfig) (string, error) {
	for _, candidate := range []string{zone.DnsZoneId, zone.Id} {
		if candidate != "" {
			return candidate, nil
		}
	}
	return "", errors.Errorf("yandex DNS zone %q for %q resolved without an id", zone.Name, cfg.ZoneName)
}

// lookupZone finds the zone by id when one is configured and by resource name
// otherwise, then checks that what came back actually serves the configured DNS
// zone. That check is the point: YC's lookup-by-name takes the *resource* name, so
// a folder holding two zones is one typo away from writing every record into the
// wrong one, and a recordset in the wrong zone is silently inert rather than an
// error.
func lookupZone(ctx *sdk.Context, cfg *yandex.RegistrarConfig, provider *sdkYandex.Provider) (*sdkYandex.LookupDnsZoneResult, error) {
	args := &sdkYandex.LookupDnsZoneArgs{FolderId: lo.ToPtr(cfg.FolderID)}
	if cfg.ZoneID != "" {
		args.DnsZoneId = lo.ToPtr(cfg.ZoneID)
	} else {
		args.Name = lo.ToPtr(cfg.EffectiveZoneResourceName())
	}

	zone, err := sdkYandex.LookupDnsZone(ctx, args, sdk.Provider(provider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to look up yandex DNS zone for %q (searched folder %q by %s). "+
			"Note the name searched is the YC RESOURCE name, not the DNS zone — `yc dns zone list --folder-id %s` "+
			"prints both; set `zoneId` or `zoneResourceName` if they differ",
			cfg.ZoneName, cfg.FolderID, lo.If(cfg.ZoneID != "", "id "+cfg.ZoneID).Else("name "+cfg.EffectiveZoneResourceName()), cfg.FolderID)
	}
	if served := strings.TrimSuffix(zone.Zone, "."); !strings.EqualFold(served, strings.TrimSuffix(cfg.ZoneName, ".")) {
		return nil, errors.Errorf("yandex DNS zone %q (%s) serves %q, not the configured zoneName %q",
			zone.Name, lo.If(zone.DnsZoneId != "", zone.DnsZoneId).Else(zone.Id), served, cfg.ZoneName)
	}
	return zone, nil
}

func (r *registrar) MainDomain() string {
	return strings.TrimSuffix(r.zone.Zone, ".")
}

func (r *registrar) ProvisionRecords(ctx *sdk.Context, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	var last *api.ResourceOutput
	for _, record := range r.config.Records {
		out, err := r.NewRecord(ctx, record)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to provision record %q", record.Name)
		}
		last = out
	}
	return last, nil
}

func (r *registrar) NewRecord(ctx *sdk.Context, dnsRecord api.DnsRecord) (*api.ResourceOutput, error) {
	if err := guardZoneInfrastructureRecord(dnsRecord); err != nil {
		return nil, err
	}
	r.log.Info(ctx.Context(), "configure yandex DNS recordset %q with type %q in zone %q",
		dnsRecord.Name, dnsRecord.Type, r.config.ZoneName)

	value := dnsRecord.ValueOut
	if dnsRecord.Value != "" {
		value = sdk.String(dnsRecord.Value).ToStringOutput()
	}
	// Proxied has no meaning here. Cloudflare's proxy is what supplies TLS for a
	// record pointed at a bare cloud endpoint; on YC that job belongs to the edge
	// ProvisionDomainForEndpoint creates, so a record is only ever a record.

	recordset, err := sdkYandex.NewDnsRecordset(ctx, fmt.Sprintf("%s-recordset", dnsRecord.Name), &sdkYandex.DnsRecordsetArgs{
		ZoneId: sdk.String(r.zoneID),
		Name:   sdk.String(fqdn(dnsRecord.Name)),
		Type:   sdk.String(dnsRecord.Type),
		// One recordset holds every value for a name+type, so a multi-valued
		// record is one resource here where Cloudflare needs N.
		Datas: sdk.StringArray{value},
		// Mandatory: there is no "automatic" sentinel to pass through.
		Ttl: sdk.Int(r.config.EffectiveRecordTtl()),
	}, sdk.Provider(r.provider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create recordset %q", dnsRecord.Name)
	}
	return &api.ResourceOutput{Ref: recordset.ID()}, nil
}

// ProvisionDomainForEndpoint publishes an endpoint under a custom domain through an
// API Gateway, which is what supplies the two things Cloudflare gives for free on a
// proxied record: a TLS certificate at the edge, and a Host header the endpoint
// recognises (the gateway proxies to the target URL, so the Host it sends is the
// target's own).
//
// Order matters and is the opposite of Cloudflare's. The gateway has to exist before
// the CNAME can point anywhere, because its service domain is generated at creation.
// That is safe: the only DNS record YC requires *before* anything else is the ACME
// challenge that issued the certificate, and a certificate is adopted here rather than
// issued (see yandex.RegistrarConfig.CertificateID). The domain-binding record may be
// created before or after the domain is attached.
//
// The gateway is created in the registrar's own folder, with the registrar's
// credentials — not the service's. In a single-folder setup those are the same; they
// would have to be split if a zone were ever shared across folders.
func (r *registrar) ProvisionDomainForEndpoint(ctx *sdk.Context, stack api.Stack, endpoint pApi.DomainEndpoint) (*api.ResourceOutput, error) {
	if r.config.CertificateID == "" {
		return nil, errors.Errorf("cannot publish %q: the %s registrar for zone %q needs `certificateId` — "+
			"the id of a Certificate Manager certificate covering that domain, which the API Gateway terminates TLS with. "+
			"A wildcard certificate for the zone covers every service; adopt it rather than issuing a second one, "+
			"since a managed certificate validates at a single `_acme-challenge.%s` CNAME",
			endpoint.Domain, yandex.RegistrarTypeYandexDns, r.config.ZoneName, r.config.ZoneName)
	}
	if !api.DomainInZone(endpoint.Domain, r.config.ZoneName) {
		return nil, errors.Errorf("cannot publish %q: the %s registrar serves zone %q",
			endpoint.Domain, yandex.RegistrarTypeYandexDns, r.config.ZoneName)
	}
	gatewayName, err := apiGatewayName(endpoint.Name)
	if err != nil {
		return nil, err
	}

	r.log.Info(ctx.Context(), "configure yandex API gateway %q for domain %q of stack %q...", gatewayName, endpoint.Domain, stack.Name)
	gateway, err := sdkYandex.NewApiGateway(ctx, gatewayName, &sdkYandex.ApiGatewayArgs{
		Name:     sdk.String(gatewayName),
		FolderId: sdk.StringPtr(r.config.FolderID),
		Spec:     proxySpec(gatewayName, endpoint.TargetHost),
		CustomDomains: sdkYandex.ApiGatewayCustomDomainArray{
			sdkYandex.ApiGatewayCustomDomainArgs{
				Fqdn:          sdk.String(strings.TrimSuffix(endpoint.Domain, ".")),
				CertificateId: sdk.String(r.config.CertificateID),
			},
		},
	}, sdk.Provider(r.provider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create API gateway %q for domain %q", gatewayName, endpoint.Domain)
	}

	// The gateway's service domain is a bare hostname; YC DNS wants CNAME data absolute.
	target := gateway.Domain.ApplyT(fqdn).(sdk.StringOutput)
	return r.NewRecord(ctx, api.DnsRecord{
		Name:     endpoint.Domain,
		Type:     "CNAME",
		ValueOut: target,
	})
}

func proxySpec(title string, targetHost sdk.StringInput) sdk.StringOutput {
	return targetHost.ToStringOutput().ApplyT(func(host string) string {
		return proxySpecFor(title, host)
	}).(sdk.StringOutput)
}

// proxySpecFor is an OpenAPI document that forwards every method and every path to the
// endpoint. `/` and `/{path+}` are both needed: the greedy parameter does not match the
// empty path. The integration is `http` rather than `serverless_containers` so that the
// gateway works for any endpoint the registrar is handed, and so that the Host header it
// forwards is the target's own — which is what replaces Cloudflare's rewriting worker.
func proxySpecFor(title, targetHost string) string {
	return fmt.Sprintf(`openapi: 3.0.0
info:
  title: %s
  version: 1.0.0
paths:
  /:
    x-yc-apigateway-any-method:
      x-yc-apigateway-integration:
        type: http
        url: https://%s/
  /{path+}:
    x-yc-apigateway-any-method:
      parameters:
        - name: path
          in: path
          required: true
          schema:
            type: string
      x-yc-apigateway-integration:
        type: http
        url: https://%s/{path}
`, title, targetHost, targetHost)
}

// apiGatewayName derives the gateway's name from the endpoint's. The suffix is what
// keeps it from colliding with the endpoint resource itself, and the truncation is what
// keeps it inside YC's 63-character limit for a long service name.
func apiGatewayName(endpointName string) (string, error) {
	const suffix = "-apigw"
	name := strings.TrimSuffix(strings.ToLower(endpointName), "-")
	if len(name)+len(suffix) > 63 {
		name = strings.TrimRight(name[:63-len(suffix)], "-")
	}
	name += suffix
	if err := validateYcResourceName(name); err != nil {
		return "", errors.Wrapf(err, "cannot derive an API gateway name from %q", endpointName)
	}
	return name, nil
}

// NewOverrideHeaderRule has no Yandex Cloud analogue. Cloudflare implements it with
// a Worker on the proxied record; YC's edge is a resource that routes by Host on
// its own, so the way to publish a service under a custom name here is to declare
// `domain:` on the client stack and let ProvisionDomainForEndpoint build it.
func (r *registrar) NewOverrideHeaderRule(ctx *sdk.Context, stack api.Stack, rule pApi.OverrideHeaderRule) (*api.ResourceOutput, error) {
	return nil, errors.Errorf("overriding the Host header from %q is not supported by the %s registrar: "+
		"declare `domain:` on the service instead, which publishes it through an API Gateway that routes by Host",
		rule.FromHost, yandex.RegistrarTypeYandexDns)
}

// fqdn returns the name Yandex Cloud DNS expects: absolute, with a trailing dot.
func fqdn(name string) string {
	if strings.HasSuffix(name, ".") {
		return name
	}
	return name + "."
}

// zoneInfrastructureRecordTypes are records the zone owns and a deploy must not
// write. NS and SOA are YC's delegation records; overwriting either takes the whole
// zone off the air.
var zoneInfrastructureRecordTypes = map[string]bool{"NS": true, "SOA": true}

// acmeChallengePrefix is where a managed certificate proves control of the zone.
// The CNAME there holds exactly one target and is maintained by Certificate
// Manager, so a deploy writing it breaks renewal — up to 90 days later.
const acmeChallengePrefix = "_acme-challenge."

func guardZoneInfrastructureRecord(record api.DnsRecord) error {
	if zoneInfrastructureRecordTypes[strings.ToUpper(record.Type)] {
		return errors.Errorf("refusing to write a %s record for %q: NS and SOA belong to the zone itself",
			strings.ToUpper(record.Type), record.Name)
	}
	if strings.HasPrefix(strings.ToLower(record.Name), acmeChallengePrefix) {
		return errors.Errorf("refusing to write %q: Certificate Manager owns the ACME challenge record, "+
			"and overwriting it breaks renewal of the certificate already issued for this zone", record.Name)
	}
	return nil
}
