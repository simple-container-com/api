// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"github.com/pkg/errors"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// fleetDnsRecords points a stack's domain at every machine of the fleet serving it:
// one A record per host, all sharing the name.
//
// Why here and not in the parent stack. A vm resource describes machines; it does not
// know which stacks will be deployed onto them or what domains those stacks claim. The
// record belongs to whoever named the domain, which is the stack — the same split every
// other template already uses (kubernetes/kube_run.go:259 and
// gcp/gke_autopilot_stack.go:206 both read the cluster's ingress address out of the
// parent and create the A record from the child).
//
// Why N records rather than one with N values. api.DnsRecord is one name to one value
// (pkg/api/dns.go), and the NewDnsRecord RPC carries one value per call, so N machines
// are N calls. That is the right shape for Cloudflare, where N values at one name are N
// resources; it is the WRONG shape for a provider whose model is a recordset holding
// every value for a name+type, and that difference is the registrar's to absorb, not
// this function's.
//
// Not ordered behind the deploys, because it cannot be: Registrar.NewRecord takes a
// record and nothing else — no resource options, so no DependsOn. The record may
// therefore appear a moment before the containers answer on a machine. Harmless in
// practice (a first deploy has no clients, and a re-deploy keeps the previous slot
// serving until its replacement passes the health gate) but worth knowing before
// reading an early 502 as a DNS bug.
//
// What this does NOT give you, stated here because DNS round-robin is routinely sold as
// high availability: a resolver hands clients the addresses in some order and nothing
// checks whether a machine is alive. A dead node means roughly 1/N of requests fail and
// are retried client-side. It spreads load and survives nothing. A cloud load balancer
// with health checks is strictly better where the cloud has one.
func fleetDnsRecords(
	ctx *sdk.Context,
	params pApi.ProvisionParams,
	stackName, domain string,
	addresses []string,
	proxied bool,
) error {
	if domain == "" || len(addresses) == 0 {
		return nil
	}
	if params.Registrar == nil {
		// Not an error. A parent stack with no registrar is a supported arrangement —
		// the operator creates the record by hand, which is what the docs for every
		// cloud without a registrar entry already say — but it has to be said out
		// loud, because "the certificate never appeared" is otherwise a silent
		// failure with the cause two stacks away.
		params.Log.Warn(ctx.Context(),
			"stack %q claims domain %q but the parent stack declares no registrar, so SC creates no DNS record. "+
				"Point %q at %s yourself, %s",
			stackName, domain, domain, addresses, manualRecordAdvice(proxied))
		return nil
	}

	urns := make(map[string]int, len(addresses))
	for i, address := range addresses {
		out, err := params.Registrar.NewRecord(ctx, api.DnsRecord{
			Name:    domain,
			Type:    "A",
			Value:   address,
			Proxied: proxied,
		})
		if err != nil {
			if errors.Is(err, pApi.NotConfiguredRegistrarError) {
				// Same situation as a nil Registrar, reached the other way: the
				// descriptor exists but is not configured.
				params.Log.Warn(ctx.Context(),
					"stack %q claims domain %q but the parent stack's registrar is not configured, so SC creates "+
						"no DNS record. Point %q at %s yourself, %s",
					stackName, domain, domain, addresses, manualRecordAdvice(proxied))
				return nil
			}
			return errors.Wrapf(err, "failed to create the A record for %q at %s (host %d of the fleet)",
				domain, address, i)
		}

		// The collision guard, and the reason this loop collects anything at all.
		//
		// NewDnsRecordRequest carries no resource name or URN hint — just name, type,
		// value, proxied, priority — so the plugin is the only side that can keep two
		// records at one name apart, and each RPC sees its own call with no
		// cross-record context. Three of the four registrar plugins derive the
		// resource name from name+type ALONE (sc-plugin-cloudru, sc-plugin-gcp) or
		// from the name alone (sc-plugin-yandex); only sc-plugin-cloudflare hashes the
		// value in. On the other three, the second call silently REPLACES the first,
		// and the fleet ends up with one A record pointing at one machine: a green
		// deploy, a resolvable name, and two thirds of the fleet receiving no traffic.
		//
		// SC does not need to know which registrars are fixed to catch that. If two
		// calls hand back the same resource, the registrar collapsed them. Refused
		// rather than warned about for the usual reason — the deploy succeeds either
		// way, so a warning is the one thing that would not be noticed.
		ref, ok := out.Ref.(sdk.StringOutput)
		if !ok {
			// A registrar whose Ref is not a string Output. Nothing to compare, so
			// nothing is claimed: the records were still created.
			continue
		}
		res, err := internals.UnsafeAwaitOutput(ctx.Context(), ref)
		if err != nil {
			return errors.Wrapf(err, "failed to resolve the reference of the A record for %q at %s", domain, address)
		}
		id, _ := res.Value.(string)
		if !res.Known || id == "" {
			// Unknown during a preview, which is normal — the resource does not exist
			// yet. The guard runs on apply.
			continue
		}
		if first, dup := urns[id]; dup {
			return errors.Errorf(
				"the registrar gave the A records for hosts %d and %d of this fleet the same resource (%q), so the "+
					"second replaced the first and %q would resolve to one machine instead of %d. The registrar "+
					"plugin derives its resource name without the record's value, which is a bug in that plugin — "+
					"upgrade it, or point %q at the fleet by hand",
				first, i, id, domain, len(addresses), domain,
			)
		}
		urns[id] = i
	}

	params.Log.Info(ctx.Context(),
		"stack %q: %q resolves to %d machine(s) of the fleet — %s. DNS round-robin spreads load; it does not fail "+
			"over, so a dead machine costs roughly 1/%d of requests until it is replaced",
		stackName, domain, len(addresses), addresses, len(addresses))
	return nil
}

// manualRecordAdvice is what to tell an operator who has to create the records by hand
// because the parent stack has no usable registrar.
//
// Which half is right depends on the fleet's front door, and the wrong half is not
// merely unhelpful. Told to proxy a fleet that terminates its own TLS, they get a name
// whose HTTP-01 challenge is answered at Cloudflare's edge instead of on the machine, so
// no certificate ever issues and Caddy retries forever. Told NOT to proxy a fleet
// running `ingress.originTls: false`, they point clients straight at an origin that
// serves plain HTTP on :80 by design — the edge that was supposed to terminate TLS is
// bypassed, and the only thing that was ever protecting that hop is gone.
//
// Both halves used to be one hardcoded "unproxied", correct until #566 made
// `domainProxied: true` reachable.
func manualRecordAdvice(proxied bool) string {
	if proxied {
		return "proxied — the fleet serves plain HTTP and the edge in front of it terminates TLS"
	}
	return "unproxied — the machines answer the ACME challenge themselves"
}
