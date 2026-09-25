// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// SecretString marks a credential that Simple Container hands to Pulumi as a
// secret.
//
// Provider configuration and resource inputs are recorded in the checkpoint and
// rendered by the engine in preview and update diffs, so a credential passed in
// as a plain value is printed wherever that output goes. Generated SDKs wrap
// the credential fields they own -- pulumi-aws does it for accessKey, secretKey
// and token, pulumi-gcp for accessToken, pulumi-cloudflare for apiToken,
// pulumi-mongodbatlas for privateKey, pulumi-kubernetes for the data of a
// core/v1 Secret -- but several fields Simple Container feeds credentials to
// are not among them: gcp `credentials`, kubernetes `kubeconfig`, docker
// `registry.password` and anything assembled into a command:local:Command.
// Those have to be wrapped at the call site.
//
// Marking the input secret gets it encrypted by the stack's secrets provider
// and rendered as `[secret]`. It is metadata rather than a change of value: the
// provider plugin still receives the credential, and DiffConfig compares the
// unwrapped values, so an existing stack sees no provider replacement.
//
// Wrapping a value that is already secret is a no-op, so it is safe on a value
// whose provenance is not obvious at the call site -- such as one read back
// from a parent stack, where GetValueFromStack has already unwrapped it into a
// plain Go string.
func SecretString(value sdk.StringInput) sdk.StringOutput {
	return sdk.ToSecret(value).(sdk.StringOutput)
}
