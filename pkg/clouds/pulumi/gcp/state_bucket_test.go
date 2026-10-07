// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"fmt"
	"net/http"
	"testing"

	gcpStorage "cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStateBucketReadOutcome(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want stateBucketRead
	}{
		{"not found", gcpStorage.ErrBucketNotExist, stateBucketMissing},
		{"not found, wrapped", fmt.Errorf("attrs: %w", gcpStorage.ErrBucketNotExist), stateBucketMissing},
		{"permission denied (JSON API)", &googleapi.Error{Code: http.StatusForbidden, Message: "no storage.buckets.get"}, stateBucketForbidden},
		{"permission denied, wrapped", fmt.Errorf("attrs: %w", &googleapi.Error{Code: http.StatusForbidden}), stateBucketForbidden},
		{"permission denied (gRPC)", status.Error(codes.PermissionDenied, "denied"), stateBucketForbidden},
		{"transient", &googleapi.Error{Code: http.StatusServiceUnavailable}, stateBucketReadFailed},
		{"unauthenticated", &googleapi.Error{Code: http.StatusUnauthorized}, stateBucketReadFailed},
		{"other", fmt.Errorf("dial tcp: timeout"), stateBucketReadFailed},
	}
	for _, c := range cases {
		if got := stateBucketReadOutcome(c.err); got != c.want {
			t.Errorf("%s: stateBucketReadOutcome = %v; want %v", c.name, got, c.want)
		}
	}
}
