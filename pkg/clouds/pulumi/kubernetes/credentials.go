// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// SecretKubeconfig wraps a kubeconfig document so Pulumi treats it as a secret.
//
// A kubeconfig is a credential: an operator-supplied one carries
// `client-key-data` or a bearer `token` for the cluster. Pulumi prints the
// properties of every resource it creates or changes in its preview and update
// summaries, SC forwards that summary to stdout, and GitHub Actions archives it
// — so an unmarked kubeconfig on `pulumi:providers:kubernetes` is published by
// the first deploy of the stack that uses it. That is exactly how a Yandex
// service-account private key reached a world-readable log on 2026-09-30; see
// yandex/credentials.go and forge's
// docs/roadmap/items/yc-provider-key-in-deploy-log.md.
//
// pulumi-kubernetes is worse than most providers here: its generated
// NewProvider declares **no** AdditionalSecretOutputs at all, so nothing marks
// `kubeconfig` unless the caller does (contrast pulumi-aws, which marks
// `secretKey`/`token` itself). A plain `sdk.String` on this field is therefore
// a bug, even where the document currently happens to be credential-free —
// a derived GKE kubeconfig authenticates through the `gke-gcloud-auth-plugin`
// exec block today, and a later config change that embeds a static token must
// not silently re-open the leak.
func SecretKubeconfig(kubeconfig string) sdk.StringOutput {
	return sdk.ToSecret(sdk.String(kubeconfig)).(sdk.StringOutput)
}

// SecretKubeconfigOutput is SecretKubeconfig for a kubeconfig that is already an
// Output — e.g. one assembled from cluster attributes. Marking the input is what
// matters: it also encrypts the value in the Pulumi checkpoint, where provider
// properties were otherwise stored in plaintext.
func SecretKubeconfigOutput(kubeconfig sdk.StringOutput) sdk.StringOutput {
	return sdk.ToSecret(kubeconfig).(sdk.StringOutput)
}
