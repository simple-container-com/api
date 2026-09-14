// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// SecretString marks a credential that Simple Container hands to a Pulumi
// provider as a secret.
//
// Provider configuration is a resource like any other: whatever is passed in
// lands in the checkpoint as a plain input and is rendered by the engine in
// preview and update diffs. Most upstream provider SDKs wrap their own
// credential fields (pulumi-aws does it for accessKey/secretKey, pulumi-gcp for
// accessToken, pulumi-cloudflare for apiToken), but the fields Simple Container
// actually uses — gcp `credentials` and kubernetes `kubeconfig` — are not among
// them, so the wrapping has to happen at the call site.
//
// A secret input is stored encrypted by the stack's secrets provider and
// displayed as `[secret]`, which is the difference between a dry-run log and a
// credential disclosure.
func SecretString(value string) sdk.StringOutput {
	return sdk.ToSecret(sdk.String(value)).(sdk.StringOutput)
}

// SecretStringOutput is SecretString for a value that is already an output,
// typically one read back from a parent stack. Marking an output that is
// already secret is a no-op, so it is safe on values whose provenance is not
// obvious at the call site.
func SecretStringOutput(value sdk.StringOutput) sdk.StringOutput {
	return sdk.ToSecret(value).(sdk.StringOutput)
}
