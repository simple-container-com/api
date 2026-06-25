// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"errors"
	"fmt"
	"testing"
)

// Note: we deliberately don't test the gcerrors.Code() == NotFound branch
// here. Constructing a `*gcerr.Error` requires gocloud.dev/internal/gcerr
// which is an internal package; the gcerrors.Code lookup uses errors.As on
// that concrete type. Functional coverage of that branch comes from gocloud
// itself; what's regressed and needs unit coverage is the string-fallback
// path that the customer's deploy actually hit.

func TestStackCheckpointNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil",
			err:  nil,
			want: false,
		},
		{
			name: "GCS 404 wrapped through Pulumi diy backend (the customer regression case)",
			err: fmt.Errorf("failed to load checkpoint: %w",
				errors.New(`blob (key ".pulumi/stacks/demo/wize-rooms-api.json") (code=Unknown): storage: object doesn't exist: googleapi: Error 404: No such object: likeclaw-simple-container-state/.pulumi/stacks/demo/wize-rooms-api.json, notFound`)),
			want: true,
		},
		{
			name: "GCS 404 without the 'failed to load checkpoint' prefix — out of scope, don't swallow",
			err:  errors.New(`storage: object doesn't exist: googleapi: Error 404`),
			want: false,
		},
		{
			name: "S3 v1 NoSuchKey wrapped through Pulumi diy backend",
			err: fmt.Errorf("failed to load checkpoint: %w",
				errors.New(`blob (key ".pulumi/stacks/foo/bar.json") (code=Unknown): NoSuchKey: The specified key does not exist`)),
			want: true,
		},
		{
			name: "S3 v2 SDK 'api error NotFound' wrapped through Pulumi diy backend",
			err: fmt.Errorf("failed to load checkpoint: %w",
				errors.New(`blob (key ".pulumi/stacks/foo/bar.json") (code=Unknown): operation error S3: HeadObject, https response error StatusCode: 404, RequestID: x, HostID: y, api error NotFound: Not Found`)),
			want: true,
		},
		{
			name: "Azure BlobNotFound wrapped through Pulumi diy backend",
			err: fmt.Errorf("failed to load checkpoint: %w",
				errors.New(`blob (key ".pulumi/stacks/foo/bar.json") (code=Unknown): BlobNotFound`)),
			want: true,
		},
		{
			name: "GCS NotFound with capitalized 'Not Found' (case-insensitivity guard)",
			err: fmt.Errorf("failed to load checkpoint: %w",
				errors.New(`blob (key ".pulumi/stacks/foo/bar.json") (code=Unknown): NotFound: object Not Found`)),
			want: true,
		},
		{
			name: "Generic 'StatusCode: 404' wrap (covers future client SDKs we don't enumerate)",
			err: fmt.Errorf("failed to load checkpoint: %w",
				errors.New(`blob (key ".pulumi/stacks/foo/bar.json") (code=Unknown): StatusCode: 404`)),
			want: true,
		},
		{
			name: "unrelated error containing 'failed to load checkpoint' but no NotFound marker",
			err: fmt.Errorf("failed to load checkpoint: %w",
				errors.New("permission denied: 403")),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stackCheckpointNotFound(tc.err)
			if got != tc.want {
				t.Errorf("stackCheckpointNotFound(%q) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
