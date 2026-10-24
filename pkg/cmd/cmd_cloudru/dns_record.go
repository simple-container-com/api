// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_cloudru

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/clouds/cloudru/evolution"
)

// NewDnsRecordCmd creates the `cloudru dns-record` command group.
func NewDnsRecordCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dns-record",
		Short: "Cloud.ru Evolution Cloud DNS record operations",
	}
	cmd.AddCommand(newDnsRecordApplyCmd())
	cmd.AddCommand(newDnsRecordDestroyCmd())
	return cmd
}

func newDnsRecordApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply",
		Short: "Create or update one Cloud DNS record set",
		Long: `Converge one record set in a Cloud.ru Cloud DNS zone to the spec on stdin.

The zone is looked up and never created. A public zone is authoritative for a real
domain, so creating one from a deploy either collides with the delegation that
already exists or produces a second zone nobody's nameservers point at.

The zone is found by name within ` + "`projectId`" + `, or named outright with ` + "`zoneId`" + ` — the
path segment of the zone's own console URL (.../dns/zones/public/<zoneId>/edit).
Either works; ` + "`zoneId`" + ` is worth setting when a project holds several zones, since a
lookup scoped to the wrong project does not fail, it finds nothing.

Note what ` + "`name`" + ` means here. Cloud DNS stores a record name as a label RELATIVE to
the zone and rejects a fully-qualified one, so this command converts: write
` + "`api.example.ru`" + ` or ` + "`api`" + `, both reach the API as ` + "`api`" + `, and the zone apex (an empty
name, ` + "`@`" + `, or the zone's own domain) reaches it as the empty string. A wildcard is a
flag on the record, not a ` + "`*`" + ` in the name.

` + "`ttl`" + ` is honoured, but there is no "automatic" value: omitting it stores a literal
0 rather than a platform default, so a TTL is always sent (300 unless configured).

Every write replaces the zone's ENTIRE record set — Cloud DNS has no per-record
route — so this reads the zone before writing it back, and then re-reads it to check
that the write survived. That last step is not belt-and-braces: a write issued while
a previous one is still settling silently discards records, up to emptying the zone,
and reports success. If this command reports that the zone did not converge, look at
the zone in the console before re-running.

Nothing here verifies delegation, because the API exposes nothing to verify it with:
a zone has no ` + "`delegated`" + ` or ` + "`valid`" + ` field, and is authoritative as soon as it
exists. Point the domain's registrar at evo-cns01.cloud.ru and evo-cns02.cloud.ru,
and confirm with ` + "`dig NS <zone>`" + ` against a resolver you trust.

Reads from stdin:
  {"keyId": "...", "keySecret": "...", "projectId": "...",
   "dnsSpec": {"zoneName": "example.ru", "zoneId": "...",
               "name": "api.example.ru", "type": "A",
               "values": ["203.0.113.10"]}}

Prints:
  {"name": "api", "zoneId": "...", "zoneName": "example.ru",
   "recordName": "api", "recordType": "a"}
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), func(ctx context.Context, c *evolution.Client, req evolution.Request) (*evolution.Result, error) {
				res, err := c.ApplyDnsRecord(ctx, req, progress)
				if err != nil {
					return nil, err
				}
				progress("cloud.ru DNS %s record %q in zone %q is up to date", res.RecordType, res.RecordName, res.ZoneName)
				return res, nil
			})
		},
	}
}

func newDnsRecordDestroyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "destroy",
		Short: "Delete one Cloud DNS record set",
		Long: `Delete the named record set from a Cloud.ru Cloud DNS zone.

A record that is already gone is success, and so is a zone that is already gone —
deleting a zone deletes its records, so there is nothing left to do and nothing
wrong. The zone reference is still required: the zone is resolved by lookup, so a
destroy needs it as much as an apply does.

Only the named record set is removed. Cloud DNS has no per-record endpoint, so this
reads the zone, drops one entry and writes the rest back; every other record in the
zone is preserved by that read, not by the API.

Reads from stdin:
  {"keyId": "...", "keySecret": "...", "projectId": "...",
   "dnsSpec": {"zoneName": "example.ru", "zoneId": "...",
               "name": "api.example.ru", "type": "A"}}
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), func(ctx context.Context, c *evolution.Client, req evolution.Request) (*evolution.Result, error) {
				res, err := c.DestroyDnsRecord(ctx, req, progress)
				if err != nil {
					return nil, err
				}
				progress("cloud.ru DNS record %q is gone", res.RecordName)
				return res, nil
			})
		},
	}
}
