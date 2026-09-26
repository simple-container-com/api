// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"context"

	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	auth "golang.org/x/oauth2/google"
	gcpOptions "google.golang.org/api/option"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

const cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

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
