// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package docker

import (
	"context"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

func TestIsCommitPinnedVersion(t *testing.T) {
	RegisterTestingT(t)

	for _, tt := range []struct {
		version string
		want    bool
	}{
		{"2026.09.14-4fc2fda", true},
		{"2026.01.01-0000000", true},
		{"latest", false},
		{"", false},
		{"2026.09.14-nogit", false},
		{"2026.09.14-nohash", false},
		{"2026.09.14-gitfail", false},
		{"2026.09.14", false},
		{"2026.09.14-4fc2fdaa", false}, // eight hex, not the short sha
		{"2026.09.14-4fc2fd", false},   // six hex
		{"2026.09.14-4FC2FDA", false},  // upper case is not what the pipeline writes
		{"v1.2.3", false},
		{"feature/branch", false},
		{"2026.09.14-4fc2fda-dirty", false},
		{" 2026.09.14-4fc2fda", false},
	} {
		Expect(IsCommitPinnedVersion(tt.version)).To(Equal(tt.want), "version %q", tt.version)
	}
}

const testDigest = "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

type notFoundErr struct{ msg string }

func (e notFoundErr) Error() string { return e.msg }
func (e notFoundErr) NotFound()     {}

func TestResolveReusableDigest(t *testing.T) {
	RegisterTestingT(t)

	const ref = "europe-north1-docker.pkg.dev/proj/repo/app:2026.09.14-4fc2fda"

	tests := []struct {
		name     string
		imageRef string
		inspect  registryInspector
		want     string
		wantErr  string
	}{
		{
			name:     "tag present",
			imageRef: ref,
			inspect:  func(context.Context, string, string) (string, error) { return testDigest, nil },
			want:     "europe-north1-docker.pkg.dev/proj/repo/app@" + testDigest,
		},
		{
			name:     "typed not found means build",
			imageRef: ref,
			inspect: func(context.Context, string, string) (string, error) {
				return "", notFoundErr{msg: "no such image"}
			},
			want: "",
		},
		{
			name:     "registry manifest unknown means build",
			imageRef: ref,
			inspect: func(context.Context, string, string) (string, error) {
				return "", errors.New("Error response from daemon: manifest unknown")
			},
			want: "",
		},
		{
			// The whole point of the check: a registry we cannot read must not
			// look like a registry with nothing in it, or a broken credential
			// silently pushes over a tag that already exists.
			name:     "authentication failure is not absence",
			imageRef: ref,
			inspect: func(context.Context, string, string) (string, error) {
				return "", errors.New("unauthorized: authentication required")
			},
			wantErr: "failed to resolve",
		},
		{
			name:     "transport failure is not absence",
			imageRef: ref,
			inspect: func(context.Context, string, string) (string, error) {
				return "", errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock")
			},
			wantErr: "failed to resolve",
		},
		{
			name:     "server error is not absence",
			imageRef: ref,
			inspect: func(context.Context, string, string) (string, error) {
				return "", errors.New("received unexpected HTTP status: 503 Service Unavailable")
			},
			wantErr: "failed to resolve",
		},
		{
			name:     "truncated digest is rejected",
			imageRef: ref,
			inspect:  func(context.Context, string, string) (string, error) { return "sha256:abcdef", nil },
			wantErr:  "unusable digest",
		},
		{
			name:     "empty digest is rejected",
			imageRef: ref,
			inspect:  func(context.Context, string, string) (string, error) { return "", nil },
			wantErr:  "unusable digest",
		},
		{
			name:     "reference without a tag is rejected",
			imageRef: "europe-north1-docker.pkg.dev/proj/repo/app",
			inspect:  func(context.Context, string, string) (string, error) { return testDigest, nil },
			wantErr:  "carries no tag",
		},
		{
			// A host port is a colon that is not a tag separator.
			name:     "registry port is not mistaken for a tag",
			imageRef: "localhost:5000/repo/app",
			inspect:  func(context.Context, string, string) (string, error) { return testDigest, nil },
			wantErr:  "carries no tag",
		},
		{
			name:     "registry port with a tag resolves",
			imageRef: "localhost:5000/repo/app:2026.09.14-4fc2fda",
			inspect:  func(context.Context, string, string) (string, error) { return testDigest, nil },
			want:     "localhost:5000/repo/app@" + testDigest,
		},
	}

	for _, tt := range tests {
		got, err := resolveReusableDigest(context.Background(), tt.inspect, tt.imageRef, "user", "pass")
		if tt.wantErr != "" {
			Expect(err).To(HaveOccurred(), tt.name)
			Expect(err.Error()).To(ContainSubstring(tt.wantErr), tt.name)
			Expect(got).To(BeEmpty(), tt.name)
			continue
		}
		Expect(err).NotTo(HaveOccurred(), tt.name)
		Expect(got).To(Equal(tt.want), tt.name)
	}
}

// The auth blob the daemon is handed must actually carry the credentials, or
// every private registry lookup fails as unauthorized and -- were absence and
// failure ever conflated -- would read as "tag missing, rebuild".
func TestResolveReusableDigestPassesCredentials(t *testing.T) {
	RegisterTestingT(t)

	var seen string
	_, err := resolveReusableDigest(context.Background(),
		func(_ context.Context, _ string, encodedAuth string) (string, error) {
			seen = encodedAuth
			return testDigest, nil
		},
		"registry.example.com/repo:2026.09.14-4fc2fda", "oauth2accesstoken", "s3cret")
	Expect(err).NotTo(HaveOccurred())

	want, err := EncodeDockerAuthHeader("oauth2accesstoken", "s3cret")
	Expect(err).NotTo(HaveOccurred())
	Expect(seen).To(Equal(want))
	Expect(seen).NotTo(BeEmpty())
	Expect(strings.TrimSpace(seen)).To(Equal(seen))
}
