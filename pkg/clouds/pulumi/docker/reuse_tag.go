// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package docker

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"github.com/pkg/errors"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/security/signing"
	"github.com/simple-container-com/api/pkg/security/tools"
)

// registryLookupTimeout bounds the manifest lookup. Without it a registry that
// accepts the connection and never answers blocks SkipPush forever, which
// blocks the image resource, which hangs the whole update with no diagnostic.
const registryLookupTimeout = 30 * time.Second

// verifyAdoptedTimeout bounds making cosign available and running it. It is
// separate from the manifest lookup because the two are not the same order of
// magnitude: verification may have to download the pinned cosign first, and
// keyless verification then talks to Fulcio, Rekor and TUF. Sharing the lookup
// budget would turn a slow download into "this image is not signed by us",
// which reads as a security verdict and is not one.
const verifyAdoptedTimeout = 3 * time.Minute

// commitPinnedVersionRe matches the CalVer version the release pipeline
// produces: date plus the abbreviated commit sha. A tag of this shape names one
// commit, which is what makes reusing the image already under it safe.
//
// The sha is a range, not exactly seven: GITHUB_SHA is sliced to seven, but the
// local fallback is git rev-parse --short=7, and --short is a minimum that git
// lengthens when seven characters are ambiguous.
//
// Shape is all this can check. A version handed in by --deploy-version or
// VERSION that happens to have this shape qualifies too, so the guarantee is
// "this tag names one commit", not "this tag was derived from the commit being
// deployed".
var commitPinnedVersionRe = regexp.MustCompile(`^\d{4}\.\d{2}\.\d{2}-[0-9a-f]{7,40}$`)

// IsCommitPinnedVersion reports whether a version has the shape of a tag that
// names exactly one commit.
func IsCommitPinnedVersion(version string) bool {
	return commitPinnedVersionRe.MatchString(version)
}

// registryInspector resolves a tag to its registry digest. It exists so the
// reuse decision can be tested without a daemon.
type registryInspector func(ctx context.Context, imageRef, encodedAuth string) (string, error)

// reuseInspector is the inspector the production path uses. It is a variable so
// resolveReuseOutput, which owns every eligibility gate, can be tested without
// a daemon; the gates are the part worth testing and they are unreachable
// through resolveReusableDigest.
var reuseInspector registryInspector = inspectViaDaemon

// ensureCosign makes the pinned cosign available, the same way the sign, SBOM
// and provenance commands do. It is a variable so the reuse decision can be
// tested without installing anything.
var ensureCosign = func(ctx context.Context) error {
	return tools.NewToolInstaller().InstallIfMissing(ctx, "cosign")
}

// inspectViaDaemon asks the Docker daemon to resolve the tag against the
// registry. This contacts the registry for the manifest only; layers are not
// pulled. The daemon is already required for the build itself.
func inspectViaDaemon(ctx context.Context, imageRef, encodedAuth string) (string, error) {
	cli, err := client.New(client.FromEnv)
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
	// "repo@sha256:..." also satisfies the test above, because the colon in the
	// algorithm prefix sits after the last slash. Splitting there would yield
	// "repo@sha256@sha256:...", which still matches repoDigestRe and would fail
	// only at pull time.
	if at := strings.LastIndex(imageRef, "@"); at > strings.LastIndex(imageRef, "/") {
		return "", errors.Errorf("image reference %q is already a digest", imageRef)
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
		return "", errors.Wrapf(err,
			"image reuse check failed for %q (set imageBuild.reuseExistingCommitTag to false to bypass)", imageRef)
	}
	if !repoDigestRe.MatchString("@" + digest) {
		return "", errors.Errorf("registry returned an unusable digest %q for %q", digest, imageRef)
	}
	return imageRef[:tagSep] + "@" + digest, nil
}

// isNotFound distinguishes "this tag does not exist" from every other failure.
//
// The daemon returns a genuine 404 as an error carrying the containerd
// not-found errdef, NOT as a type with a NotFound() method: in the client this
// module pins, only objectNotFoundError has that method and DistributionInspect
// returns it solely for an empty reference, which is rejected above. The errdef
// is therefore the real predicate and the marker interface is kept only for
// clients that still use it.
//
// The remaining string matches are registry manifest wording. A bare "not
// found" is deliberately absent: Go's own 404 body is "404 page not found" and
// a missing credential helper reads "executable file not found in $PATH",
// neither of which is an absent tag, and both of which would push over the tag
// with the registry unread.
func isNotFound(err error) bool {
	if cerrdefs.IsNotFound(err) {
		return true
	}
	var notFound interface{ NotFound() }
	if errors.As(err, &notFound) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{"manifest unknown", "no such image", "name unknown"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// reuseSkipReason reports why reuse does not apply, or "" when it does. The
// reasons are returned rather than logged here so the caller can log once and
// the decision stays testable.
func reuseSkipReason(stack api.Stack, image Image) string {
	if !stack.Client.ReuseExistingCommitTagEnabled() {
		return ""
	}
	if !IsCommitPinnedVersion(image.Version) {
		return fmt.Sprintf("version %q does not name a commit", image.Version)
	}
	if image.Registry.Username == nil || image.Registry.Password == nil {
		return "the registry has no credentials configured"
	}
	// Reuse adopts whatever is already under the tag. Without verification the
	// pipeline would sign and attest an artifact it did not build, turning push
	// access to the registry into a valid signature from this pipeline. The
	// verify step is what makes an adopted image prove it was ours.
	if securitySigningEnabled(stack.Client.Security) {
		sign := stack.Client.Security.Signing
		verify := sign.Verify
		if verify == nil || !verify.Enabled {
			return "signing is enabled but signing.verify is not, so an adopted image could not be proven to be ours"
		}
		if sign.Keyless {
			if verify.OIDCIssuer == "" || verify.IdentityRegexp == "" {
				return "signing.verify is missing oidcIssuer or identityRegexp"
			}
		} else if sign.PublicKey == "" {
			return "signing.verify is enabled but signing.publicKey is not set"
		}
	}
	return ""
}

// verifyAdoptedImage checks that the image already sitting under the tag
// carries a signature this pipeline would accept.
//
// Without it, reuse turns push access to the registry into a signature from
// this pipeline: whatever is under the tag gets adopted, signed and attested as
// though it had been built here. Rebuilding used to overwrite a planted image
// on the next deploy; reuse is what removes that self-healing, so the check
// belongs at the moment of adoption.
//
// The tool has to be made available first. Every other consumer of cosign in
// this codebase installs it on the way in, and the reuse check runs before all
// of them: measured on a real deploy, it ran before the signing step had
// installed anything, so cosign was absent from PATH and a correctly signed
// image was refused. Refusing is the safe direction, but it made reuse
// unreachable for every stack that signs, which is every stack that may reuse.
func verifyAdoptedImage(ctx context.Context, security *api.SecurityDescriptor, digestRef string) error {
	if err := ensureCosign(ctx); err != nil {
		return errors.Wrap(err, "cosign is required to check the image already under the tag")
	}
	verify := security.Signing.Verify
	_, err := signing.VerifyImage(ctx, &signing.Config{
		Enabled:        true,
		Keyless:        security.Signing.Keyless,
		PublicKey:      security.Signing.PublicKey,
		OIDCIssuer:     verify.OIDCIssuer,
		IdentityRegexp: verify.IdentityRegexp,
	}, digestRef)
	return err
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
	if reason := reuseSkipReason(stack, image); reason != "" {
		if ctx != nil {
			_ = ctx.Log.Warn(fmt.Sprintf(
				"imageBuild.reuseExistingCommitTag is enabled for %q but %s; building and pushing instead",
				image.Name, reason), nil)
		}
		return empty
	}

	lookupCtx := context.Background()
	if ctx != nil {
		lookupCtx = ctx.Context()
	}

	// Unsecret keeps the registry password from tainting the result. The digest
	// is not a secret, and without this the deployed image reference and
	// skipPush would render as [secret] the moment anyone marks the password.
	return sdk.Unsecret(sdk.All(imageFullUrl, image.Registry.Username, image.Registry.Password).
		ApplyT(func(values []interface{}) (string, error) {
			reqCtx, cancel := context.WithTimeout(lookupCtx, registryLookupTimeout)
			defer cancel()

			ref := resolveStringArg(values[0])
			digestRef, err := resolveReusableDigest(
				reqCtx, reuseInspector, ref,
				resolveStringArg(values[1]), resolveStringArg(values[2]),
			)
			if err != nil {
				return "", err
			}
			if digestRef == "" {
				return "", nil
			}
			if securitySigningEnabled(stack.Client.Security) {
				verifyCtx, cancelVerify := context.WithTimeout(lookupCtx, verifyAdoptedTimeout)
				defer cancelVerify()
				if err := verifyAdoptedImage(verifyCtx, stack.Client.Security, digestRef); err != nil {
					// Not fatal on purpose: refusing to reuse falls back to
					// building and pushing, which overwrites whatever is under
					// the tag. That is the behaviour before this feature and it
					// is the one that heals a planted image.
					if ctx != nil {
						_ = ctx.Log.Warn(fmt.Sprintf(
							"refusing to reuse %s: could not prove it carries a signature this stack accepts (%v); building and pushing over it",
							digestRef, err), nil)
					}
					return "", nil
				}
			}
			if ctx != nil {
				_ = ctx.Log.Info(fmt.Sprintf(
					"reusing %s: the tag already resolves to %s, so it is not pushed over (the image is still built locally)",
					ref, digestRef), nil)
			}
			return digestRef, nil
		})).(sdk.StringOutput)
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
