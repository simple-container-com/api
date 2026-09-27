// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"fmt"
	"net/http"
	"testing"

	gcpStorage "cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
)

func TestStateBucketMissing(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"not found", gcpStorage.ErrBucketNotExist, true},
		{"not found, wrapped", fmt.Errorf("attrs: %w", gcpStorage.ErrBucketNotExist), true},
		{"permission denied", &googleapi.Error{Code: http.StatusForbidden, Message: "no storage.buckets.get"}, false},
		{"transient", &googleapi.Error{Code: http.StatusServiceUnavailable}, false},
	}
	for _, c := range cases {
		if got := stateBucketMissing(c.err); got != c.want {
			t.Errorf("%s: stateBucketMissing = %v; want %v", c.name, got, c.want)
		}
	}
}
