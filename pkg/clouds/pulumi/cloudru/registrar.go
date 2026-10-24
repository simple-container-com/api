// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cloudru

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/cloudru"
	"github.com/simple-container-com/api/pkg/clouds/cloudru/evolution"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// registrar writes records into a Cloud.ru Evolution Cloud DNS zone.
//
// Every record is a pulumi-command `local.Command` running `sc cloudru dns-record
// apply|destroy`, for the same reason the Container App is one: Cloud DNS appears in
// no Terraform or Pulumi provider — not in `cloud-ru/evo-terraform` either — and
// Pulumi's Go SDK has no dynamic providers, so driving the CLI is the only way to put
// a record inside Pulumi's resource model. Calling the REST API in-process from here
// would be shorter and wrong twice over: it would run during `pulumi preview`, where
// nothing may change, and it would sit outside the resource model, so a destroy would
// leave the record behind.
//
// The zone is NOT resolved here, unlike the Yandex registrar, which looks its zone up
// at plan time. There is nothing to look it up with: a lookup is a REST call, and this
// tier may not make one. The zone is therefore resolved inside each command, against
// the same credentials — which also means a misconfigured `projectId` or `zoneId`
// surfaces on the first record rather than on the plan. The config's Validate catches
// the shape of the mistake; only the cloud can catch the value.
//
// One Cloud DNS property leaks into this tier and is worth knowing before reading
// NewRecord: a record is not addressable on that API, so applying one is a
// read-modify-write of the zone's entire record array (evolution.ApplyDnsRecord). Two
// records provisioned concurrently would each write the array they read, and the
// loser's record would vanish. ProvisionRecords therefore builds them in sequence,
// and each command re-reads the zone when it runs.
type registrar struct {
	provider sdk.ProviderResource
	config   *cloudru.DnsRegistrarConfig
	creds    evolution.Credentials
	log      logger.Logger

	// lastRecord is the previously-created record command, and every new one
	// depends on it. This is a correctness requirement, not tidiness: see the
	// read-modify-write note above. Pulumi runs sibling resources concurrently by
	// default, so without this chain two records in one zone race and the deploy
	// reports green having written only one of them — the classic shape of "the
	// record was there a minute ago".
	lastRecord sdk.Resource
}

// Registrar builds the Cloud.ru Cloud DNS registrar for a server.yaml `registrar:`
// block of type `cloudru-dns`.
func Registrar(ctx *sdk.Context, config api.RegistrarDescriptor, params pApi.ProvisionParams) (pApi.Registrar, error) {
	cfg, ok := config.Config.Config.(*cloudru.DnsRegistrarConfig)
	if !ok {
		return nil, errors.Errorf("invalid config type %T is not *cloudru.DnsRegistrarConfig", config.Config.Config)
	}
	// `${auth:cloudru-evolution}` resolves to the opaque credentials blob only, so
	// the key pair and the project id live inside it and have to be unpacked before
	// anything below can use them.
	if err := api.ConvertAuth(cfg, &cfg.EvolutionAccountConfig); err != nil {
		return nil, errors.Wrapf(err, "failed to convert auth config to cloudru.EvolutionAccountConfig")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	params.Log.Info(ctx.Context(), "configure cloud.ru DNS registrar for zone %q (%s)...",
		cfg.ZoneName, lo.If(cfg.ZoneID != "", "by zoneId "+cfg.ZoneID).Else("by name in project "+cfg.ProjectID))

	return &registrar{
		provider: params.Provider,
		config:   cfg,
		creds: evolution.Credentials{
			KeyID:     cfg.KeyID,
			Secret:    cfg.KeySecret,
			ProjectID: cfg.ProjectID,
		},
		log: params.Log,
	}, nil
}

func (r *registrar) MainDomain() string {
	return strings.TrimSuffix(r.config.ZoneName, ".")
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
	recordType := strings.ToUpper(dnsRecord.Type)
	r.log.Info(ctx.Context(), "configure cloud.ru DNS %s record %q in zone %q",
		recordType, dnsRecord.Name, r.config.ZoneName)

	// Proxied has no meaning here. Cloudflare's proxy is what supplies TLS for a
	// record pointed at a bare cloud endpoint; on Cloud.ru that job belongs to
	// whatever terminates TLS at the target (Caddy on a vm, the Container App's own
	// platform certificate), so a record is only ever a record.

	value := dnsRecord.ValueOut
	if dnsRecord.Value != "" {
		value = sdk.String(dnsRecord.Value).ToStringOutput()
	}

	// Built twice from the same value — once for the request the command reads, once
	// for the hash that triggers it. The request itself cannot be the trigger: a
	// trigger is stored and rendered unencrypted in a Pulumi diff, and this one holds
	// the IAM key secret.
	buildSpec := func(target string) evolution.DnsRecordSpec {
		return evolution.DnsRecordSpec{
			// The lookup scope is the ordinary Evolution project id, carried on the
			// auth entry. Cloud DNS has no scope of its own.
			ProjectID: r.creds.ProjectID,
			ZoneName:  r.config.ZoneName,
			ZoneID:    r.config.ZoneID,
			Name:      dnsRecord.Name,
			Type:      recordType,
			TTL:       r.config.EffectiveRecordTtl(),
			// One record set holds every value for a name+type, so a multi-valued
			// record is one resource here. SC's DnsRecord carries a single value, so
			// this is a one-element list until something needs more.
			Values: []string{target},
		}
	}

	requestJSON := value.ApplyT(func(target string) (string, error) {
		spec := buildSpec(target)
		encoded, err := json.Marshal(evolution.Request{
			Credentials: r.creds,
			Name:        dnsRecord.Name,
			DnsSpec:     &spec,
		})
		if err != nil {
			return "", errors.Wrapf(err, "failed to encode cloud.ru DNS request for record %q", dnsRecord.Name)
		}
		return string(encoded), nil
	}).(sdk.StringOutput)

	specHash := value.ApplyT(func(target string) (string, error) {
		spec := buildSpec(target)
		encoded, err := json.Marshal(spec)
		if err != nil {
			return "", errors.Wrapf(err, "failed to hash cloud.ru DNS spec for record %q", dnsRecord.Name)
		}
		sum := sha256.Sum256(encoded)
		return hex.EncodeToString(sum[:]), nil
	}).(sdk.StringOutput)

	applyScript := scScript("cloudru", "dns-record", "apply")
	destroyScript := scScript("cloudru", "dns-record", "destroy")

	opts := []sdk.ResourceOption{sdk.Provider(r.provider)}
	if r.lastRecord != nil {
		opts = append(opts, sdk.DependsOn([]sdk.Resource{r.lastRecord}))
	}

	cmd, err := local.NewCommand(ctx, dnsRecordResourceName(dnsRecord.Name, recordType), &local.CommandArgs{
		Create: sdk.String(applyScript),
		// Update is the same script, not absent: without it pulumi-command treats any
		// input change as a replacement and runs Delete before Create, which would
		// take the name off the air for the length of a delete+create instead of
		// patching it in place. `apply` is a create-or-patch.
		Update: sdk.String(applyScript),
		Delete: sdk.StringPtr(destroyScript),
		// The script text is a constant, so without a trigger over the *content* a
		// changed target would not re-run the command at all and the deploy would
		// report green while DNS kept answering the old address. The hash covers the
		// target, the TTL, the type and the zone reference.
		Triggers: sdk.Array{specHash},
		// Credentials and the spec on stdin, marked secret so the state file holds it
		// encrypted; LoggingNone so neither the request nor the command's output
		// reaches the Actions log; no AddPreviousOutputInEnv, because the previous
		// run's stdout is a JSON document this script has no use for.
		Stdin:                  sdk.ToSecret(requestJSON).(sdk.StringOutput).ToStringPtrOutput(),
		Logging:                local.LoggingNone.ToLoggingPtrOutput(),
		AddPreviousOutputInEnv: sdk.Bool(false),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to configure cloud.ru DNS %s record %q", recordType, dnsRecord.Name)
	}
	r.lastRecord = cmd
	return &api.ResourceOutput{Ref: cmd.ID()}, nil
}

// ProvisionDomainForEndpoint has no Cloud.ru implementation yet, and says so rather
// than writing a record that cannot work.
//
// What is missing is not the record — NewRecord above would write it — but the edge.
// Publishing a cloud endpoint under a custom domain needs something that terminates
// TLS for that name and sends the endpoint a Host header it recognises. On Yandex
// that is an API Gateway; on Cloudflare it is the proxy. Cloud.ru Evolution's
// equivalent is the Container App's own `configuration.ingress.customDomains`, which
// the Container App driver already sends — but it has never been exercised live, and
// it needs a `certificateId` from a service (Cloud.ru's certificate manager) this
// package does not speak to yet. Writing a CNAME at the app's platform hostname
// without that produces a name that answers with the WRONG certificate, i.e. a
// browser error, which is worse than an explicit refusal here.
//
// The `vm` path does not come through here at all: Caddy on the machine terminates
// TLS itself and the record is a plain A record, which is what ProvisionRecords and
// NewRecord already do.
func (r *registrar) ProvisionDomainForEndpoint(ctx *sdk.Context, stack api.Stack, endpoint pApi.DomainEndpoint) (*api.ResourceOutput, error) {
	if !api.DomainInZone(endpoint.Domain, r.config.ZoneName) {
		return nil, errors.Errorf("cannot publish %q: the %s registrar serves zone %q",
			endpoint.Domain, cloudru.RegistrarTypeCloudruDns, r.config.ZoneName)
	}
	return nil, errors.Errorf("publishing %q through the %s registrar is not implemented yet: a custom domain on "+
		"a Cloud.ru Container App is bound by the app's own `cloudExtras.customDomains` and needs a certificate id "+
		"from Cloud.ru's certificate manager, which simple-container does not manage yet. A CNAME alone would answer "+
		"with the platform's certificate for the wrong name. For a `vm` stack, declare the A record in the "+
		"registrar's `dnsRecords` and let Caddy on the machine issue the certificate",
		endpoint.Domain, cloudru.RegistrarTypeCloudruDns)
}

// NewOverrideHeaderRule has no Cloud.ru analogue. Cloudflare implements it with a
// Worker on the proxied record; there is no programmable edge in front of a Cloud.ru
// endpoint, so the Host a service sees is the one the client sent.
func (r *registrar) NewOverrideHeaderRule(ctx *sdk.Context, stack api.Stack, rule pApi.OverrideHeaderRule) (*api.ResourceOutput, error) {
	return nil, errors.Errorf("overriding the Host header from %q is not supported by the %s registrar: "+
		"Cloud.ru has no programmable edge in front of an endpoint. Serve the name on the target itself "+
		"(Caddy on a `vm` stack routes by Host)", rule.FromHost, cloudru.RegistrarTypeCloudruDns)
}

// dnsRecordResourceName is the Pulumi resource name of one record's command.
//
// The type is part of it because a name can legitimately carry an A and a TXT record
// at once (a domain and its SPF, say) and two resources may not share a name — a
// collision would present as the second record silently replacing the first.
func dnsRecordResourceName(name, recordType string) string {
	return fmt.Sprintf("%s-%s-dns-record", strings.TrimSuffix(name, "."), strings.ToLower(recordType))
}

// zoneInfrastructureRecordTypes are records the zone owns and a deploy must not
// write. NS and SOA are the delegation records Cloud.ru creates with the zone;
// overwriting either takes the whole zone off the air.
var zoneInfrastructureRecordTypes = map[string]bool{"NS": true, "SOA": true}

// acmeChallengePrefix is where an ACME client proves control of a name. Whoever
// issued the certificate for this zone owns that record — Caddy on a `vm` stack
// renews through it — so a deploy writing it breaks renewal, up to 90 days later.
const acmeChallengePrefix = "_acme-challenge."

func guardZoneInfrastructureRecord(record api.DnsRecord) error {
	if zoneInfrastructureRecordTypes[strings.ToUpper(record.Type)] {
		return errors.Errorf("refusing to write a %s record for %q: NS and SOA belong to the zone itself",
			strings.ToUpper(record.Type), record.Name)
	}
	if strings.HasPrefix(strings.ToLower(record.Name), acmeChallengePrefix) {
		return errors.Errorf("refusing to write %q: the ACME client that issued this zone's certificate owns the "+
			"challenge record, and overwriting it breaks renewal of the certificate already issued", record.Name)
	}
	return nil
}
