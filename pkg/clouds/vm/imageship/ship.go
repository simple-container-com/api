// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package imageship moves a locally-built docker image onto a remote docker host
// over SSH, without a registry and without the docker CLI on the machine doing the
// shipping.
//
// It is the transport behind `sc image ship` and behind the vm delivery path's
// `imageDelivery: ssh` (P5.1). The shape is the operator's own description of the
// delivery they wanted: *«собрать образ в ci/cd, залить его прямо архивом на
// виртуалку и импортировать в локальный docker images»* — so: `ImageSave` stream →
// compress → `docker load` on the far end, one pipe, no temporary file on either
// side.
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
	// Image is the local reference to ship (`ghcr.io/org/app:2026.10.16-abc1234`).
	// It is validated against security.ValidateImageRef by the caller; that
	// validation is also what makes it safe to interpolate into a remote shell
	// command, since the pattern admits no shell metacharacter.
	Image       string
	Compression string
	// Force re-ships even when the machine already holds this exact image id.
	Force bool
	// AllowArchMismatch downgrades the architecture gate to a warning. `docker load`
	// accepts a foreign-arch image happily and the failure surfaces much later, as a
	// container that exits with "exec format error".
	AllowArchMismatch bool
	// Log receives progress lines. Optional.
	Log func(format string, args ...any)
}

type Result struct {
	// ImageID is the image's config digest — the identity `docker load` preserves,
	// and therefore the one worth comparing across the two hosts.
	ImageID string
	// Skipped reports that the machine already held this image id under this
	// reference and nothing was transferred.
	Skipped         bool
	BytesRaw        int64
	BytesCompressed int64
	Duration        time.Duration
	// LocalPlatform and RemotePlatform are `os/arch`, as the arch gate compared them.
	LocalPlatform  string
	RemotePlatform string
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
	return &out, nil
}

// Ship copies opts.Image to the remote docker host. It is idempotent: a machine that
// already holds the image id under the same reference is left alone.
func Ship(ctx context.Context, docker DockerAPI, remote Remote, opts Options) (*Result, error) {
	o, err := opts.withDefaults()
	if err != nil {
		return nil, err
	}

	local, err := docker.ImageInspect(ctx, o.Image)
	if err != nil {
		return nil, errors.Wrapf(err, "image %q is not in the local docker; build or pull it first", o.Image)
	}
	localPlatform := platformOf(local.Os, local.Architecture, local.Variant)

	remotePlatform, err := remoteDockerPlatform(ctx, remote)
	if err != nil {
		return nil, err
	}

	res := &Result{
		ImageID:        local.ID,
		LocalPlatform:  localPlatform,
		RemotePlatform: remotePlatform,
	}

	if err := checkPlatform(localPlatform, remotePlatform, o); err != nil {
		return nil, err
	}

	present, err := remoteImageID(ctx, remote, o.Image)
	if err != nil {
		return nil, err
	}
	if present == local.ID && !o.Force {
		o.Log("%s already holds %s (%s) — nothing to ship", remote.Describe(), o.Image, shortID(local.ID))
		res.Skipped = true
		return res, nil
	}
	if present != "" && present != local.ID {
		o.Log("%s holds a different %s (%s); replacing it with %s",
			remote.Describe(), o.Image, shortID(present), shortID(local.ID))
	}

	started := time.Now()
	raw, compressed, err := transfer(ctx, docker, remote, o)
	res.Duration = time.Since(started)
	res.BytesRaw, res.BytesCompressed = raw, compressed
	if err != nil {
		return nil, err
	}

	// `docker load` exiting 0 is not proof the reference now resolves to our image:
	// a tar of the wrong image, or a load that landed the layers under a different
	// tag, both exit 0. Verifying by id is the only check that can fail here.
	loaded, err := remoteImageID(ctx, remote, o.Image)
	if err != nil {
		return nil, err
	}
	if loaded != local.ID {
		return nil, errors.Errorf(
			"`docker load` succeeded on %s but %s resolves to %s there, expected %s",
			remote.Describe(), o.Image, orElse(shortID(loaded), "no image"), shortID(local.ID),
		)
	}
	return res, nil
}

// transfer streams the image to the machine: ImageSave → compressor → `docker load`
// over one SSH stdin, with no file written on either side.
func transfer(ctx context.Context, docker DockerAPI, remote Remote, o *Options) (raw, compressed int64, err error) {
	saved, err := docker.ImageSave(ctx, []string{o.Image})
	if err != nil {
		return 0, 0, errors.Wrapf(err, "failed to export %q from the local docker", o.Image)
	}
	defer func() { _ = saved.Close() }()

	counted := &countingReader{r: saved}
	stream, countCompressed := compressStream(counted, o.Compression)
	defer func() { _ = stream.Close() }()

	o.Log("shipping %s to %s (%s)…", o.Image, remote.Describe(), o.Compression)
	out, err := remote.Run(ctx, "docker load -q", stream)
	raw, compressed = counted.count(), countCompressed()
	if err != nil {
		return raw, compressed, errors.Wrapf(err, "`docker load` failed on %s", remote.Describe())
	}
	if out != "" {
		o.Log("%s: %s", remote.Describe(), out)
	}
	o.Log("shipped %s (%s on the wire) in %s",
		humanize.IBytes(uint64(raw)), humanize.IBytes(uint64(compressed)), remote.Describe())
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

func checkPlatform(local, remote string, o *Options) error {
	if local == remote {
		return nil
	}
	msg := fmt.Sprintf(
		"image %q is %s but the machine runs %s; `docker load` accepts it and the container then dies with "+
			"\"exec format error\" at run time", o.Image, local, remote,
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
