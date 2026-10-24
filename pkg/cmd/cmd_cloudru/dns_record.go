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

` + "`parentId`" + ` is the Cloud DNS **service instance** id, not the project id — the one
Cloud.ru API in this package that is not project-scoped. Getting it wrong does not
fail: a list scoped to an unrelated parent comes back empty, which is
indistinguishable from a project that has no zones.

A zone whose ` + "`delegated`" + ` flag is false is reported, not refused: the record is
created successfully and resolves nowhere, and a deploy log is the only place the
operator would ever learn that.

Reads from stdin:
  {"keyId": "...", "keySecret": "...", "projectId": "...",
   "dnsSpec": {"parentId": "...", "zoneName": "example.ru",
               "name": "api.example.ru", "type": "A", "ttl": 3600,
               "values": ["203.0.113.10"]}}

Prints:
  {"name": "api.example.ru.", "zoneId": "...", "zoneName": "example.ru.",
   "recordName": "api.example.ru.", "recordType": "A", "zoneDelegated": true}
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

Reads from stdin:
  {"keyId": "...", "keySecret": "...", "projectId": "...",
   "dnsSpec": {"parentId": "...", "zoneName": "example.ru",
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
