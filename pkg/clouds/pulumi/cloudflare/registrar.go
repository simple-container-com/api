package cloudflare

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	cfImpl "github.com/pulumi/pulumi-cloudflare/sdk/v5/go/cloudflare"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	cfApi "github.com/simple-container-com/api/pkg/clouds/cloudflare"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type Provisioner interface {
	NewWorkerScript(ctx *sdk.Context, workerName string, hostName string, script string) (*api.ResourceOutput, error)
}

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
		var suffix string
		if lo.CountBy(r.config.Records, func(item api.DnsRecord) bool {
			return record.Name == item.Name
		}) > 1 {
			sum := md5.Sum([]byte(record.Value))
			suffix = hex.EncodeToString(sum[:])
		}
		recordName := fmt.Sprintf("%s%s-record", record.Name, suffix)
		res, err := cfImpl.NewRecord(ctx, recordName, &cfImpl.RecordArgs{
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
	r.log.Info(ctx.Context(), "configure DNS record %q with type %q and value %q", dnsRecord.Name, dnsRecord.Type, dnsRecord.Value)

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

func (r *provisioner) NewWorkerScript(ctx *sdk.Context, workerName string, hostName string, script string) (*api.ResourceOutput, error) {
	ruleName := fmt.Sprintf("%s-worker-script", workerName)
	r.log.Info(ctx.Context(), "configure cloudflare worker script %q...", workerName)
	workerScript, err := cfImpl.NewWorkerScript(ctx, fmt.Sprintf("%s-worker-script", workerName), &cfImpl.WorkerScriptArgs{
		Name:      sdk.String(workerName),
		AccountId: sdk.String(r.accountId),
		Content:   sdk.String(script),
	}, sdk.Provider(r.provider))
	if err != nil {
		return nil, err
	}
	routeName := fmt.Sprintf("%s-route", ruleName)
	workerRoute, err := cfImpl.NewWorkerRoute(ctx, routeName, &cfImpl.WorkerRouteArgs{
		ZoneId:     sdk.String(r.zone.ZoneId),
		Pattern:    sdk.String(fmt.Sprintf("%s/*", hostName)),
		ScriptName: workerScript.Name,
	}, sdk.Provider(r.provider))
	if err != nil {
		return nil, err
	}
	ctx.Export(fmt.Sprintf("%s-route", ruleName), workerRoute.ToWorkerRouteOutput())

	ctx.Export(fmt.Sprintf("%s-script", ruleName), workerScript.ToWorkerScriptOutput())
	return &api.ResourceOutput{
		Ref: workerRoute.ID(),
	}, nil
}

func (r *provisioner) NewOverrideHeaderRule(ctx *sdk.Context, stack api.Stack, rule pApi.OverrideHeaderRule) (*api.ResourceOutput, error) {
	ruleName := fmt.Sprintf("%s%s-host-override", stack.Name, rule.Name)
	r.log.Info(ctx.Context(), "configure cloudflare worker script overriding header from %q to %q...", rule.FromHost, rule.ToHost)
	scriptName := fmt.Sprintf("%s-scrpt", ruleName)

	pagesCode := ""
	if rule.PathPrefix != "" {
		pagesCode += fmt.Sprintf(`url.pathname = '%s' + url.pathname;`, rule.PathPrefix)
	}
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
	workerScriptCode := fmt.Sprintf(`
addEventListener('fetch', event => {
  event.respondWith(handleRequest(event.request));
});

async function handleRequest(request) {
	const overrideHost = "%s";
	const url = new URL(request.url);
	url.hostname = overrideHost;

	%s

	let origResponse = await fetch(url.toString(), request);
	let response = new Response(origResponse.body, {
		status: origResponse.status,
		statusText: origResponse.statusText,
		headers: origResponse.headers
	});

	response.headers.append("www-authenticate", response.headers.get("x-amzn-remapped-www-authenticate"));
	return response
};
`, rule.ToHost, pagesCode)
	workerScript, err := cfImpl.NewWorkerScript(ctx, scriptName, &cfImpl.WorkerScriptArgs{
		Name:      sdk.String(scriptName),
		AccountId: sdk.String(r.accountId),
		Content:   sdk.String(workerScriptCode),
	}, sdk.Provider(r.provider))
	if err != nil {
		r.log.Error(ctx.Context(), "failed to create worker script: "+err.Error())
		return nil, errors.Wrapf(err, "failed to create worker script")
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
