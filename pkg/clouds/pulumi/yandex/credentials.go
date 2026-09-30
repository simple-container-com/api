// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// secretStringPtr wraps a credential so Pulumi treats it as a secret.
//
// This exists because of a live leak, not for tidiness. Pulumi prints the
// properties of every resource it *creates or changes* in its preview and update
// summaries, SC forwards that summary to stdout, and GitHub Actions archives it
// for 90 days. A provider's credentials are ordinary properties, so the first
// deploy of a YC stack printed the whole authorized-key document — `private_key`
// included — into a world-readable log (forge-atrium run 36687152934,
// 2026-09-30; twice, once in the preview and once in the update).
//
// Why it bit here and not on AWS: pulumi-aws's schema marks `secretKey` secret,
// so the engine masks it however it is passed. Our pulumi-yandex bridge does not
// mark `serviceAccountKeyFile` — reasonably, since upstream it is normally a
// PATH, and SC is the one passing the document itself. Marking it at the call
// site is what closes that gap, and it also encrypts the value in the Pulumi
// checkpoint, where it was previously stored in plaintext.
//
// A plain `sdk.StringPtr` on any credential-bearing provider field is therefore
// a bug. See yandex/provider.go and yandex/registrar.go for the call sites, and
// forge's docs/roadmap/items/yc-provider-key-in-deploy-log.md for the incident.
func secretStringPtr(value string) sdk.StringPtrOutput {
	return sdk.ToSecret(sdk.String(value)).(sdk.StringOutput).ToStringPtrOutput()
}
