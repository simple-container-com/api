package cloudflare

import (
	"fmt"

	"github.com/samber/lo"

	"github.com/pkg/errors"
	cfImpl "github.com/pulumi/pulumi-cloudflare/sdk/v5/go/cloudflare"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	cfApi "github.com/simple-container-com/api/pkg/clouds/cloudflare"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type provisioner struct {
	provider *cfImpl.Provider
	config   *cfApi.RegistrarConfig
	zone     *cfImpl.LookupZoneResult
}

func NewCloudflare(ctx *sdk.Context, config api.RegistrarDescriptor) (pApi.Registrar, error) {
	cfg, ok := config.Config.Config.(*cfApi.RegistrarConfig)
	if !ok {
		return nil, errors.Errorf("invalid config type %T is not *cloudflare.RegistrarConfig", config.Config.Config)
	}

	provider, err := cfImpl.NewProvider(ctx, cfg.AccountId, &cfImpl.ProviderArgs{
		ApiToken: sdk.StringPtr(cfg.AuthConfig.Credentials.Credentials),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to init pulumi")
	}

	cfZone, err := cfImpl.LookupZone(ctx, &cfImpl.LookupZoneArgs{
		Name: &cfg.ZoneName,
	}, sdk.Provider(provider))
	if err != nil {
		return nil, errors.Wrapf(err, "error retrieving zone ID for domain %q", cfg.ZoneName)
	}

	return &provisioner{
		provider: provider,
		config:   cfg,
		zone:     cfZone,
	}, nil
}

func (r *provisioner) ProvisionRecords(ctx *sdk.Context, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	res := lo.Map(r.config.Records, func(record api.DnsRecord, _ int) *cfImpl.Record {
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
		return res
	})
	return &api.ResourceOutput{
		Ref: res,
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
		Ref: ref,
	}, nil
}
