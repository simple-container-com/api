package cloudflare

import (
	"fmt"

	"github.com/pkg/errors"
	cfImpl "github.com/pulumi/pulumi-cloudflare/sdk/v5/go/cloudflare"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	cfApi "github.com/simple-container-com/api/pkg/clouds/cloudflare"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type provisioner struct {
	provider  *cfImpl.Provider
	config    *cfApi.RegistrarConfig
	zone      *cfImpl.LookupZoneResult
	accountId string
	log       logger.Logger
}

func Registrar(ctx *sdk.Context, config api.RegistrarDescriptor, params pApi.ProvisionParams) (pApi.Registrar, error) {
	cfg, ok := config.Config.Config.(*cfApi.RegistrarConfig)
	if !ok {
		return nil, errors.Errorf("invalid config type %T is not *cloudflare.RegistrarConfig", config.Config.Config)
	}
	baseZoneName := cfg.ZoneName
	if params.DnsPreference != nil && params.DnsPreference.BaseZone != "" {
		baseZoneName = params.DnsPreference.BaseZone
		params.Log.Info(ctx.Context(), "stack overrides preferred base DNS zone from %q to %q", cfg.ZoneName, baseZoneName)
	}

	provider, err := cfImpl.NewProvider(ctx, cfg.AccountId, &cfImpl.ProviderArgs{
		ApiToken: sdk.StringPtr(cfg.AuthConfig.Credentials.Credentials),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to init pulumi")
	}

	cfZone, err := cfImpl.LookupZone(ctx, &cfImpl.LookupZoneArgs{
		Name: &baseZoneName,
	}, sdk.Provider(provider))
	if err != nil {
		return nil, errors.Wrapf(err, "error retrieving zone ID for domain %q", baseZoneName)
	}

	return &provisioner{
		provider:  provider,
		config:    cfg,
		zone:      cfZone,
		accountId: cfg.AccountId,
		log:       params.Log,
	}, nil
}

func (r *provisioner) ProvisionRecords(ctx *sdk.Context, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	res := lo.Map(r.config.Records, func(record api.DnsRecord, _ int) sdk.Output {
		res, err := cfImpl.NewRecord(ctx, fmt.Sprintf("%s-record", record.Name), &cfImpl.RecordArgs{
			ZoneId:  sdk.String(r.zone.ZoneId),
			Name:    sdk.String(record.Name),
			Type:    sdk.String(record.Type),
			Value:   sdk.StringPtr(record.Value),
			Proxied: sdk.Bool(record.Proxied),
		}, sdk.Provider(r.provider))
		if err != nil {
			params.Log.Error(ctx.Context(), "failed to create record %q: %v", record.Name, err)
		}
		return res.ID()
	})
	return &api.ResourceOutput{
		Ref: sdk.ToArrayOutput(res),
	}, nil
}

func (r *provisioner) MainDomain() string {
	return r.zone.Name
}

func (r *provisioner) NewRecord(ctx *sdk.Context, dnsRecord api.DnsRecord) (*api.ResourceOutput, error) {
	ref, err := cfImpl.NewRecord(ctx, fmt.Sprintf("%s-record", dnsRecord.Name), &cfImpl.RecordArgs{
		ZoneId:  sdk.String(r.zone.ZoneId),
		Name:    sdk.String(dnsRecord.Name),
		Type:    sdk.String(dnsRecord.Type),
		Value:   sdk.StringPtr(dnsRecord.Value),
		Proxied: sdk.Bool(dnsRecord.Proxied),
	}, sdk.Provider(r.provider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create record %q", dnsRecord.Name)
	}

	return &api.ResourceOutput{
		Ref: ref.ID(),
	}, nil
}

func (r *provisioner) NewOverrideHeaderRule(ctx *sdk.Context, stack api.Stack, rule pApi.OverrideHeaderRule) (*api.ResourceOutput, error) {
	ruleName := fmt.Sprintf("%s-host-override", stack.Name)
	r.log.Info(ctx.Context(), "configure cloudflare worker script overriding header from %q to %q...", rule.FromHost, rule.ToHost)
	scriptName := fmt.Sprintf("%s-script", ruleName)

	pagesCode := ""
	if rule.OverridePages != nil {
		if rule.OverridePages.IndexPage != "" {
			pagesCode += fmt.Sprintf(`
	if (url.pathname == '/') { url.pathname += '/%s'; } else if (url.pathname.endsWith('/')) { url.pathname = url.pathname + '%s'; }
`, rule.OverridePages.IndexPage, rule.OverridePages.IndexPage)
		}
		if rule.OverridePages.NotFoundPage != "" {
			pagesCode += fmt.Sprintf(`
	let res = await fetch(url.toString(), request);
	if (res.status == 404) {
		url.pathname = '/%s';
	} else {
		return res;
	}
`, rule.OverridePages.NotFoundPage)
		}
	}
	workerScript, err := cfImpl.NewWorkerScript(ctx, scriptName, &cfImpl.WorkerScriptArgs{
		Name:      sdk.String(scriptName),
		AccountId: sdk.String(r.accountId),
		Content: sdk.String(fmt.Sprintf(`
addEventListener('fetch', event => {
  event.respondWith(handleRequest(event.request));
});

async function handleRequest(request) {
	const overrideHost = "%s";
	const url = new URL(request.url);
	url.hostname = overrideHost;

	%s
	return await fetch(url.toString(), request);
};
`, rule.ToHost, pagesCode)),
	}, sdk.Provider(r.provider))
	if err != nil {
		return nil, err
	}
	ctx.Export(fmt.Sprintf("%s-script", ruleName), workerScript.ToWorkerScriptOutput())

	routeName := fmt.Sprintf("%s-route", ruleName)
	workerRoute, err := cfImpl.NewWorkerRoute(ctx, routeName, &cfImpl.WorkerRouteArgs{
		ZoneId:     sdk.String(r.zone.ZoneId),
		Pattern:    sdk.String(fmt.Sprintf("%s/*", rule.FromHost)),
		ScriptName: workerScript.Name,
	}, sdk.Provider(r.provider))
	if err != nil {
		return nil, err
	}
	ctx.Export(fmt.Sprintf("%s-route", ruleName), workerRoute.ToWorkerRouteOutput())

	return &api.ResourceOutput{
		Ref: workerRoute.ID(),
	}, nil
}
