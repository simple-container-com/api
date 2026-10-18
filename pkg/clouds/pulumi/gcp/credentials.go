// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"context"

	"golang.org/x/oauth2"
	auth "golang.org/x/oauth2/google"
	gcpOptions "google.golang.org/api/option"

	"github.com/pkg/errors"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

const cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

// secretCredentials wraps the service-account document so Pulumi treats it as a
// secret.
//
// Pulumi prints the properties of every resource it creates or changes in its
// preview and update summaries, SC forwards that summary to stdout, and GitHub
// Actions archives it — so a provider credential passed as a plain property
// value is published by the first deploy of the stack. That is not theoretical:
// the same shape leaked a Yandex service-account private key into a
// world-readable log on 2026-09-30 (forge-atrium run 36687152934). See
// yandex/credentials.go and forge's
// docs/roadmap/items/yc-provider-key-in-deploy-log.md.
//
// pulumi-gcp's generated NewProvider marks only `accessToken` secret; the
// `credentials` field — which is where SC puts the whole service-account JSON,
// `private_key` included — is left to the caller. Marking it here also encrypts
// the value in the Pulumi checkpoint, where it was otherwise stored in
// plaintext.
func secretCredentials(value string) sdk.StringPtrOutput {
	return sdk.ToSecret(sdk.String(value)).(sdk.StringOutput).ToStringPtrOutput()
}

// clientOptions authenticates a Google API client with the configured key, or with
// Application Default Credentials when none is configured.
func clientOptions(credentials string) []gcpOptions.ClientOption {
	if gcloud.UsesAmbientCredentials(credentials) {
		return nil
	}
	return []gcpOptions.ClientOption{gcpOptions.WithCredentialsJSON([]byte(credentials))} //nolint:staticcheck // SA1019: no in-memory replacement available
}

// tokenSource returns an OAuth2 token source for the configured key, or for
// Application Default Credentials when none is configured. A configured key must
// still be a service-account key: pinning the type keeps an unexpected credential
// shape from being accepted from config.
func tokenSource(ctx context.Context, credentials string) (oauth2.TokenSource, error) {
	if gcloud.UsesAmbientCredentials(credentials) {
		creds, err := auth.FindDefaultCredentials(ctx, cloudPlatformScope)
		if err != nil {
			return nil, errors.Wrap(err, "credentials are empty (ambient mode) and no Application Default Credentials were found")
		}
		return creds.TokenSource, nil
	}
	creds, err := auth.CredentialsFromJSONWithTypeAndParams(ctx, []byte(credentials), auth.ServiceAccount, auth.CredentialsParams{
		Scopes: []string{cloudPlatformScope},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse GCP service account credentials")
	}
	return creds.TokenSource, nil
}
