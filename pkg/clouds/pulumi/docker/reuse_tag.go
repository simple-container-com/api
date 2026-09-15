// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package docker

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/moby/moby/client"
	"github.com/pkg/errors"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
)

// commitPinnedVersionRe matches the CalVer version the release pipeline
// produces: date plus the short commit sha. A tag of this shape names exactly
// one commit, which is what makes reusing the image already under it safe.
//
// Everything else is excluded on purpose. An empty version becomes "latest",
// a git failure produces "-nogit"/"-nohash"/"-gitfail", and both VERSION and
// --deploy-version accept arbitrary input; none of those identify a commit, so
// reuse under them would serve a stale artifact for changed source.
var commitPinnedVersionRe = regexp.MustCompile(`^\d{4}\.\d{2}\.\d{2}-[0-9a-f]{7}$`)

// IsCommitPinnedVersion reports whether a version names exactly one commit.
func IsCommitPinnedVersion(version string) bool {
	return commitPinnedVersionRe.MatchString(version)
}

// registryInspector resolves a tag to its registry digest. It exists so the
// reuse decision can be tested without a daemon.
type registryInspector func(ctx context.Context, imageRef, encodedAuth string) (string, error)

// inspectViaDaemon asks the Docker daemon to resolve the tag against the
// registry. This contacts the registry for the manifest only; layers are not
// pulled. The daemon is already required for the build itself.
func inspectViaDaemon(ctx context.Context, imageRef, encodedAuth string) (string, error) {
	cli, err := client.New(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", errors.Wrap(err, "failed to connect to docker daemon")
	}
	defer func() { _ = cli.Close() }()

	res, err := cli.DistributionInspect(ctx, imageRef, client.DistributionInspectOptions{
		EncodedRegistryAuth: encodedAuth,
	})
	if err != nil {
		return "", err
	}
	return string(res.Descriptor.Digest), nil
}

// resolveReusableDigest returns the digest reference to reuse, or an empty
// string when the image must be built and pushed.
//
// Only a missing tag means "build". Every other failure -- authentication,
// an unreachable daemon, a transport error, a registry 5xx -- is returned as an
// error. Treating those as absence would turn a broken credential into a silent
// rebuild, which is the failure mode that makes the whole check worthless: it
// would push over the tag precisely when the registry could not be read.
func resolveReusableDigest(ctx context.Context, inspect registryInspector, imageRef, username, password string) (string, error) {
	// A registry host may carry a port, so a bare colon is not a tag. The tag
	// separator is the last colon after the last path separator.
	tagSep := strings.LastIndex(imageRef, ":")
	if tagSep < 0 || tagSep < strings.LastIndex(imageRef, "/") {
		return "", errors.Errorf("image reference %q carries no tag", imageRef)
	}

	encodedAuth, err := EncodeDockerAuthHeader(username, password)
	if err != nil {
		return "", errors.Wrap(err, "failed to encode registry auth")
	}

	digest, err := inspect(ctx, imageRef, encodedAuth)
	if err != nil {
		if isNotFound(err) {
			return "", nil
		}
		return "", errors.Wrapf(err, "failed to resolve %q in the registry", imageRef)
	}
	if !repoDigestRe.MatchString("@" + digest) {
		return "", errors.Errorf("registry returned an unusable digest %q for %q", digest, imageRef)
	}
	return imageRef[:tagSep] + "@" + digest, nil
}

// isNotFound distinguishes "this tag does not exist" from every other failure.
// The daemon's own not-found errors carry the NotFound() marker; a manifest the
// registry rejects arrives as a plain message from the other side of the HTTP
// boundary, so those shapes are matched too.
func isNotFound(err error) bool {
	var notFound interface{ NotFound() }
	if errors.As(err, &notFound) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{"manifest unknown", "not found", "no such image"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// resolveReuseOutput produces the digest of an image already in the registry
// under this exact tag, or an empty string when the image has to be built and
// pushed.
//
// The eligibility checks that can be answered without the registry are made
// here, so a stack that has not opted in, or whose version is not commit
// pinned, never contacts the registry at all. A preview never does either:
// nothing is pushed during a dry run, so there is no decision to make.
func resolveReuseOutput(ctx *sdk.Context, stack api.Stack, image Image, imageFullUrl sdk.StringOutput) sdk.StringOutput {
	empty := sdk.String("").ToStringOutput()
	if ctx != nil && ctx.DryRun() {
		return empty
	}
	if !stack.Client.ReuseExistingCommitTagEnabled() {
		return empty
	}
	if !IsCommitPinnedVersion(image.Version) {
		return empty
	}
	if image.Registry.Username == nil || image.Registry.Password == nil {
		return empty
	}

	return sdk.All(imageFullUrl, image.Registry.Username, image.Registry.Password).
		ApplyT(func(values []interface{}) (string, error) {
			ref := resolveStringArg(values[0])
			digestRef, err := resolveReusableDigest(
				context.Background(), inspectViaDaemon, ref,
				resolveStringArg(values[1]), resolveStringArg(values[2]),
			)
			if err != nil {
				return "", err
			}
			if digestRef != "" && ctx != nil {
				_ = ctx.Log.Info(fmt.Sprintf("reusing %s: the tag already resolves to %s, so it is neither rebuilt nor pushed over", ref, digestRef), nil)
			}
			return digestRef, nil
		}).(sdk.StringOutput)
}

// skipPushOutput keeps the push skipped during a preview, and skips it on a
// reuse so an identical tag is never pushed a second time. A second push under
// the same tag is what a registry with immutable tags rejects, and what makes a
// rerun of one commit produce an artifact nobody scanned.
func skipPushOutput(ctx *sdk.Context, reusableDigest sdk.StringOutput) sdk.BoolPtrOutput {
	dryRun := ctx != nil && ctx.DryRun()
	return reusableDigest.ApplyT(func(digest string) *bool {
		skip := dryRun || digest != ""
		return &skip
	}).(sdk.BoolPtrOutput)
}
