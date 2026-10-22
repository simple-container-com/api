// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package imageship moves locally-held docker images onto a remote docker host
// over SSH, without a registry and without the docker CLI on the machine doing the
// shipping.
//
// It is the transport behind `sc image ship` and behind the vm delivery path's
// `imageDelivery: ssh-tar` (P5.1). The shape is the operator's own description of the
// delivery they wanted: *«собрать образ в ci/cd, залить его прямо архивом на
// виртуалку и импортировать в локальный docker images»* — so: `ImageSave` stream →
// compress → `docker load` on the far end, one pipe, no temporary file on either
// side.
//
// Several images per ship, rather than one call per image, because `docker save`
// deduplicates shared layers across the images it is given: a stack's own image and
// the third-party images beside it usually share a base, and an app image and the
// Caddy in front of it both ship a distroless or alpine root. One stream also means
// one `docker load`, one SSH connection and one compressor warm-up.
//
// See forge/docs/design/2026-10-13/sc-russia-market-readiness/compose-on-vm.md.
package imageship

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/moby/moby/api/types/image"
	"github.com/pkg/errors"
)

// The value of Options.Compression.
//
// `docker load` detects the compression from the stream itself (the daemon runs it
// through moby/go-archive's DecompressStream), so none of these need a matching
// binary on the machine — not even zstd.
const (
	// CompressionZstd is the default: ~3x faster than gzip at a better ratio, which
	// on a 400 MiB image over a link to another country is the difference between a
	// deploy and a coffee break.
	CompressionZstd = "zstd"
	// CompressionGzip is for a docker engine old enough to predate zstd support in
	// its load path (before moby 23). It fails with an opaque "unexpected EOF" from
	// `docker load` rather than naming the compression, which is why this escape
	// hatch exists at all.
	CompressionGzip = "gzip"
	CompressionNone = "none"
)

// DockerAPI is the slice of moby's client that shipping needs. It is an interface so
// the logic above it can be tested without a docker daemon — the arch gate and the
// skip-if-present decision are the parts worth pinning, and neither needs real bytes.
type DockerAPI interface {
	ImageInspect(ctx context.Context, ref string) (image.InspectResponse, error)
	ImageSave(ctx context.Context, refs []string) (io.ReadCloser, error)
	// ImagePull fetches ref into the local docker. Only called when Options.Pull is
	// set, and only for a reference the local docker does not already have.
	ImagePull(ctx context.Context, ref string) error
}

// Remote runs commands on the machine being shipped to.
type Remote interface {
	// Run executes cmd and returns its stdout with surrounding whitespace trimmed.
	// stdin may be nil. An exit status other than 0 is an error.
	Run(ctx context.Context, cmd string, stdin io.Reader) (string, error)
	// Describe names the target for log lines and error messages. It must never
	// include a key, a password or anything else that cannot be printed.
	Describe() string
}

type Options struct {
	// Images are the local references to ship (`ghcr.io/org/app:2026.10.16-abc1234`).
	// They are validated against security.ValidateImageRef by the caller; that
	// validation is also what makes them safe to interpolate into a remote shell
	// command, since the pattern admits no shell metacharacter.
	Images      []string
	Compression string
	// Force re-ships even when the machine already holds this exact image id.
	Force bool
	// Pull fetches an image the local docker does not have before saving it.
	//
	// This is what makes shipping usable for images SC does not build — a `mongo:7`
	// in someone's compose file, or the ingress Caddy — where the runner is the host
	// with a route to the registry and the machine is the one without. It pulls only
	// what is ABSENT: an image already in the local docker is shipped as it stands,
	// so a tag that moved upstream is not re-fetched on every deploy. For a pinned
	// third-party tag that is the behaviour you want; for a moving tag the ship is
	// as stale as the runner's cache, and `docker pull` before `sc image ship` is
	// the way to say otherwise.
	Pull bool
	// AllowArchMismatch downgrades the architecture gate to a warning. `docker load`
	// accepts a foreign-arch image happily and the failure surfaces much later, as a
	// container that exits with "exec format error".
	AllowArchMismatch bool
	// Log receives progress lines. Optional.
	Log func(format string, args ...any)
}

// ImageResult is the outcome for one image of a ship.
type ImageResult struct {
	Image string
	// ImageID is the image's config digest — the identity `docker load` preserves,
	// and therefore the one worth comparing across the two hosts.
	ImageID string
	// Skipped reports that the machine already held this image id under this
	// reference and this image contributed nothing to the transfer.
	Skipped bool
	// Pulled reports that the image was absent locally and Options.Pull fetched it.
	Pulled bool
	// Platform is `os/arch`, as the arch gate read it off the image.
	Platform string
}

type Result struct {
	// Images is one entry per requested image, in the order given (after
	// deduplication).
	Images []ImageResult
	// RemotePlatform is the machine's `os/arch`, as the arch gate compared against.
	RemotePlatform string
	// Bytes and Duration cover the single stream that carried every non-skipped
	// image, so they are not attributable per image: `docker save` shares layers
	// across them, which is the point of shipping them together.
	BytesRaw        int64
	BytesCompressed int64
	Duration        time.Duration
}

// Shipped is the references that were actually transferred.
func (r *Result) Shipped() []string {
	var out []string
	for _, i := range r.Images {
		if !i.Skipped {
			out = append(out, i.Image)
		}
	}
	return out
}

// SkippedAll reports that the machine already held every requested image and nothing
// was transferred.
func (r *Result) SkippedAll() bool {
	return len(r.Shipped()) == 0
}

func (o *Options) withDefaults() (*Options, error) {
	out := *o
	if out.Compression == "" {
		out.Compression = CompressionZstd
	}
	switch out.Compression {
	case CompressionZstd, CompressionGzip, CompressionNone:
	default:
		return nil, errors.Errorf("unknown compression %q; one of: %s, %s, %s",
			out.Compression, CompressionZstd, CompressionGzip, CompressionNone)
	}
	if out.Log == nil {
		out.Log = func(string, ...any) {}
	}
	// Deduplicated rather than rejected: two services of one compose project running
	// the same third-party image is ordinary, and the callers that assemble this list
	// walk services. Order is preserved so the log reads as the caller's list.
	out.Images = dedupe(out.Images)
	if len(out.Images) == 0 {
		return nil, errors.Errorf("no image to ship")
	}
	return &out, nil
}

// Ship copies opts.Images to the remote docker host. It is idempotent per image: a
// machine that already holds an image id under the same reference is left alone, and
// a ship where that is true of every image transfers nothing.
func Ship(ctx context.Context, docker DockerAPI, remote Remote, opts Options) (*Result, error) {
	o, err := opts.withDefaults()
	if err != nil {
		return nil, err
	}

	remotePlatform, err := remoteDockerPlatform(ctx, remote)
	if err != nil {
		return nil, err
	}
	res := &Result{RemotePlatform: remotePlatform}

	// Two passes before anything moves: resolve and gate every image first, so a
	// mismatched arch or a reference that is nowhere to be found fails the ship
	// before it has half-loaded the others onto the machine.
	for _, ref := range o.Images {
		local, pulled, err := localImage(ctx, docker, ref, o)
		if err != nil {
			return nil, err
		}
		platform := platformOf(local.Os, local.Architecture, local.Variant)
		if err := checkPlatform(ref, platform, remotePlatform, o); err != nil {
			return nil, err
		}
		res.Images = append(res.Images, ImageResult{
			Image: ref, ImageID: local.ID, Pulled: pulled, Platform: platform,
		})
	}

	for i := range res.Images {
		img := &res.Images[i]
		present, err := remoteImageID(ctx, remote, img.Image)
		if err != nil {
			return nil, err
		}
		switch {
		case present == img.ImageID && !o.Force:
			o.Log("%s already holds %s (%s) — nothing to ship", remote.Describe(), img.Image, shortID(img.ImageID))
			img.Skipped = true
		case present != "" && present != img.ImageID:
			o.Log("%s holds a different %s (%s); replacing it with %s",
				remote.Describe(), img.Image, shortID(present), shortID(img.ImageID))
		}
	}

	toShip := res.Shipped()
	if len(toShip) == 0 {
		return res, nil
	}

	started := time.Now()
	raw, compressed, err := transfer(ctx, docker, remote, o, toShip)
	res.Duration = time.Since(started)
	res.BytesRaw, res.BytesCompressed = raw, compressed
	if err != nil {
		return nil, err
	}

	// `docker load` exiting 0 is not proof the references now resolve to our images:
	// a tar of the wrong image, or a load that landed the layers under a different
	// tag, both exit 0. Verifying by id is the only check that can fail here.
	for _, img := range res.Images {
		if img.Skipped {
			continue
		}
		loaded, err := remoteImageID(ctx, remote, img.Image)
		if err != nil {
			return nil, err
		}
		if loaded != img.ImageID {
			return nil, errors.Errorf(
				"`docker load` succeeded on %s but %s resolves to %s there, expected %s",
				remote.Describe(), img.Image, orElse(shortID(loaded), "no image"), shortID(img.ImageID),
			)
		}
	}
	return res, nil
}

// localImage resolves one reference in the local docker, pulling it first when it is
// absent and Options.Pull allows it.
func localImage(ctx context.Context, docker DockerAPI, ref string, o *Options) (image.InspectResponse, bool, error) {
	local, err := docker.ImageInspect(ctx, ref)
	if err == nil {
		return local, false, nil
	}
	if !o.Pull {
		return image.InspectResponse{}, false, errors.Wrapf(err,
			"image %q is not in the local docker; build or pull it first, or pass --pull to fetch it", ref)
	}
	o.Log("pulling %s (not in the local docker)…", ref)
	if perr := docker.ImagePull(ctx, ref); perr != nil {
		// Both errors, because they are different diagnoses: the inspect failing is
		// "not here", the pull failing is "and this is why it cannot be fetched" —
		// a rate limit, an auth failure, or no route to the registry from whatever
		// is running the deploy.
		return image.InspectResponse{}, false, errors.Wrapf(perr,
			"image %q is not in the local docker and could not be pulled here (inspect said: %v)", ref, err)
	}
	local, err = docker.ImageInspect(ctx, ref)
	if err != nil {
		return image.InspectResponse{}, false, errors.Wrapf(err,
			"image %q was pulled but is still not inspectable in the local docker", ref)
	}
	return local, true, nil
}

// transfer streams the images to the machine: ImageSave → compressor → `docker load`
// over one SSH stdin, with no file written on either side.
func transfer(
	ctx context.Context,
	docker DockerAPI,
	remote Remote,
	o *Options,
	refs []string,
) (raw, compressed int64, err error) {
	saved, err := docker.ImageSave(ctx, refs)
	if err != nil {
		return 0, 0, errors.Wrapf(err, "failed to export %s from the local docker", strings.Join(refs, ", "))
	}
	defer func() { _ = saved.Close() }()

	counted := &countingReader{r: saved}
	stream, countCompressed := compressStream(counted, o.Compression)
	defer func() { _ = stream.Close() }()

	o.Log("shipping %s to %s (%s)…", strings.Join(refs, ", "), remote.Describe(), o.Compression)
	out, err := remote.Run(ctx, "docker load -q", stream)
	raw, compressed = counted.count(), countCompressed()
	if err != nil {
		return raw, compressed, errors.Wrapf(err, "`docker load` failed on %s", remote.Describe())
	}
	if out != "" {
		o.Log("%s: %s", remote.Describe(), out)
	}
	o.Log("shipped %d image(s), %s (%s on the wire) in %s",
		len(refs), humanize.IBytes(uint64(raw)), humanize.IBytes(uint64(compressed)), remote.Describe())
	return raw, compressed, nil
}

// remoteDockerPlatform asks the remote daemon what it is, rather than asking the
// kernel via `uname -m`: the daemon's own answer is already in docker's arch
// vocabulary (`amd64`, not `x86_64`), so there is no mapping table to get wrong.
func remoteDockerPlatform(ctx context.Context, remote Remote) (string, error) {
	out, err := remote.Run(ctx, `docker version --format '{{.Server.Os}}/{{.Server.Arch}}'`, nil)
	if err != nil {
		return "", errors.Wrapf(err,
			"cannot talk to docker on %s; if the machine was just created, cloud-init may still be "+
				"installing it (SSH answers well before docker exists)", remote.Describe())
	}
	if out == "" || !strings.Contains(out, "/") {
		return "", errors.Errorf("docker on %s reported an unreadable platform %q", remote.Describe(), out)
	}
	return out, nil
}

// remoteImageID returns the image id the reference resolves to on the machine, or ""
// when the machine does not have it. A docker that answered `docker version` and then
// fails this call is treated as "absent": the expected failure is exactly
// "No such image", and shipping again is safe where guessing is not.
func remoteImageID(ctx context.Context, remote Remote, ref string) (string, error) {
	out, err := remote.Run(ctx, fmt.Sprintf("docker image inspect --format '{{.Id}}' '%s'", ref), nil)
	if err != nil {
		return "", nil
	}
	return out, nil
}

func checkPlatform(ref, local, remote string, o *Options) error {
	if local == remote {
		return nil
	}
	msg := fmt.Sprintf(
		"image %q is %s but the machine runs %s; `docker load` accepts it and the container then dies with "+
			"\"exec format error\" at run time", ref, local, remote,
	)
	if o.AllowArchMismatch {
		o.Log("WARNING: %s (--allow-arch-mismatch)", msg)
		return nil
	}
	return errors.Errorf("%s. Build for %s, or pass --allow-arch-mismatch if you know better", msg, remote)
}

func platformOf(os, arch, variant string) string {
	if variant != "" {
		return fmt.Sprintf("%s/%s/%s", os, arch, variant)
	}
	return fmt.Sprintf("%s/%s", os, arch)
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func orElse(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
