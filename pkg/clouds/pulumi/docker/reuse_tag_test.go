// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package docker

import (
	"context"
	"strings"
	"testing"

	cerrdefs "github.com/containerd/errdefs"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"

	"github.com/simple-container-com/api/pkg/api"
)

func TestIsCommitPinnedVersion(t *testing.T) {
	RegisterTestingT(t)

	for _, tt := range []struct {
		version string
		want    bool
	}{
		{"2026.09.14-4fc2fda", true},
		{"2026.01.01-0000000", true},
		// git rev-parse --short=7 lengthens the abbreviation when seven
		// characters are ambiguous, so these are commit-pinned too.
		{"2026.09.14-4fc2fdaa", true},
		{"2026.09.14-" + strings.Repeat("a", 40), true},
		{"latest", false},
		{"", false},
		{"2026.09.14-nogit", false},
		{"2026.09.14-nohash", false},
		{"2026.09.14-gitfail", false},
		{"2026.09.14", false},
		{"2026.09.14-4fc2fd", false},  // six hex, shorter than git ever abbreviates
		{"2026.09.14-4FC2FDA", false}, // upper case is not what the pipeline writes
		{"2026.09.14-" + strings.Repeat("a", 41), false},
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
			// This is the shape the daemon actually returns for a registry 404:
			// an error carrying the containerd not-found errdef, with no
			// NotFound() method anywhere in the chain.
			name:     "the errdef the daemon really returns means build",
			imageRef: ref,
			inspect: func(context.Context, string, string) (string, error) {
				return "", errors.Wrap(cerrdefs.ErrNotFound, "manifest for app:2026.09.14-4fc2fda")
			},
			want: "",
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
			name:     "registry name unknown means build",
			imageRef: ref,
			inspect: func(context.Context, string, string) (string, error) {
				return "", errors.New("name unknown: repository name not known to registry")
			},
			want: "",
		},
		{
			// A 404 from something that is not a registry. Classifying this as
			// absence would push over an existing tag with the registry unread,
			// which is the failure the whole check exists to prevent.
			name:     "a stdlib 404 body is not absence",
			imageRef: ref,
			inspect: func(context.Context, string, string) (string, error) {
				return "", errors.New("404 page not found")
			},
			wantErr: "image reuse check failed",
		},
		{
			name:     "a missing credential helper is not absence",
			imageRef: ref,
			inspect: func(context.Context, string, string) (string, error) {
				return "", errors.New(`exec: "docker-credential-ecr-login": executable file not found in $PATH`)
			},
			wantErr: "image reuse check failed",
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
			wantErr: "image reuse check failed",
		},
		{
			name:     "transport failure is not absence",
			imageRef: ref,
			inspect: func(context.Context, string, string) (string, error) {
				return "", errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock")
			},
			wantErr: "image reuse check failed",
		},
		{
			name:     "server error is not absence",
			imageRef: ref,
			inspect: func(context.Context, string, string) (string, error) {
				return "", errors.New("received unexpected HTTP status: 503 Service Unavailable")
			},
			wantErr: "image reuse check failed",
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
		{
			// The colon in "sha256:" sits after the last slash, so the no-tag
			// guard passes and splitting there would yield repo@sha256@sha256:...
			name:     "a reference that is already a digest is rejected",
			imageRef: "europe-north1-docker.pkg.dev/proj/repo/app@" + testDigest,
			inspect:  func(context.Context, string, string) (string, error) { return testDigest, nil },
			wantErr:  "already a digest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := resolveReusableDigest(context.Background(), tt.inspect, tt.imageRef, "user", "pass")
			if tt.wantErr != "" {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(tt.wantErr))
				Expect(got).To(BeEmpty())
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(tt.want))
		})
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

func reuseTestImage(version string, withCreds bool) Image {
	img := Image{Name: "app", Version: version}
	if withCreds {
		img.Registry = docker.RegistryArgs{
			Server:   sdk.String("registry.example.com"),
			Username: sdk.StringPtr("user"),
			Password: sdk.StringPtr("pass"),
		}
	}
	return img
}

func reuseTestStack(enabled bool, security *api.SecurityDescriptor) api.Stack {
	client := api.ClientDescriptor{Security: security}
	if enabled {
		client.ImageBuild = &api.ImageBuildDescriptor{ReuseExistingCommitTag: true}
	}
	return api.Stack{Name: "s", Client: client}
}

// Every gate that can answer "do not reuse" without the registry must also
// avoid contacting it. Each of these branches used to be unreachable from any
// test, so deleting one shipped green.
func TestResolveReuseOutputGates(t *testing.T) {
	signedNoVerify := &api.SecurityDescriptor{
		Enabled: true,
		Signing: &api.SigningDescriptor{Enabled: true, Keyless: true},
	}

	for _, tt := range []struct {
		name     string
		dryRun   bool
		stack    api.Stack
		image    Image
		wantCall bool
	}{
		{
			name:  "a preview never looks the tag up",
			stack: reuseTestStack(true, nil), image: reuseTestImage("2026.09.14-4fc2fda", true),
			dryRun: true,
		},
		{
			name:  "reuse not opted into",
			stack: reuseTestStack(false, nil), image: reuseTestImage("2026.09.14-4fc2fda", true),
		},
		{
			name:  "version does not name a commit",
			stack: reuseTestStack(true, nil), image: reuseTestImage("latest", true),
		},
		{
			name:  "registry has no credentials",
			stack: reuseTestStack(true, nil), image: reuseTestImage("2026.09.14-4fc2fda", false),
		},
		{
			name:  "signing without verification cannot prove an adopted image is ours",
			stack: reuseTestStack(true, signedNoVerify), image: reuseTestImage("2026.09.14-4fc2fda", true),
		},
		{
			name:  "eligible, so the registry is consulted",
			stack: reuseTestStack(true, nil), image: reuseTestImage("2026.09.14-4fc2fda", true),
			wantCall: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			called := 0
			restore := reuseInspector
			reuseInspector = func(context.Context, string, string) (string, error) {
				called++
				return testDigest, nil
			}
			defer func() { reuseInspector = restore }()

			got := resolveUnderMocks[string](t, tt.name, tt.dryRun, func(ctx *sdk.Context) sdk.Output {
				return resolveReuseOutput(ctx, tt.stack, tt.image,
					sdk.String("registry.example.com/repo:"+tt.image.Version).ToStringOutput())
			})

			if !tt.wantCall {
				Expect(called).To(Equal(0), "the registry must not be contacted")
				Expect(got).To(BeEmpty())
				return
			}
			Expect(called).To(Equal(1))
			Expect(got).To(Equal("registry.example.com/repo@" + testDigest))
		})
	}
}

// A registry that cannot be read has to fail the deploy, not fall through to a
// push. The error has to travel on the output itself: that output feeds
// SkipPush, and a resource input that is awaited while carrying an error is what
// aborts the update. Nothing asserted that the error escapes the apply at all.
//
// Note the error does NOT surface from RunErr on its own. An output nobody
// consumes is never awaited, so asserting on RunErr alone would pass whether or
// not the error propagated.
func TestResolveReuseOutputSurfacesRegistryErrors(t *testing.T) {
	RegisterTestingT(t)

	restore := reuseInspector
	reuseInspector = func(context.Context, string, string) (string, error) {
		return "", errors.New("unauthorized: authentication required")
	}
	defer func() { reuseInspector = restore }()

	var awaitErr error
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		out := resolveReuseOutput(ctx, reuseTestStack(true, nil),
			reuseTestImage("2026.09.14-4fc2fda", true),
			sdk.String("registry.example.com/repo:2026.09.14-4fc2fda").ToStringOutput())
		_, awaitErr = internals.UnsafeAwaitOutput(ctx.Context(), skipPushOutput(ctx, out))
		return nil
	}, sdk.WithMocks("project", "stack", &deployImageRefMocks{}))

	Expect(err).NotTo(HaveOccurred())
	Expect(awaitErr).To(HaveOccurred())
	Expect(awaitErr.Error()).To(ContainSubstring("image reuse check failed"))
}
