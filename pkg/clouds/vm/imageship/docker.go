// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package imageship

import (
	"context"
	"io"

	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"github.com/pkg/errors"
)

// LocalDocker adapts moby's client to DockerAPI. It talks to whatever DOCKER_HOST
// says, so a CI runner with the daemon on a socket and a developer with a remote
// context both work, and no docker CLI has to be installed next to `sc`.
type LocalDocker struct {
	cli *client.Client
}

var _ DockerAPI = (*LocalDocker)(nil)

func NewLocalDocker() (*LocalDocker, error) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return nil, errors.Wrap(err, "cannot talk to the local docker daemon (is DOCKER_HOST right?)")
	}
	return &LocalDocker{cli: cli}, nil
}

func (d *LocalDocker) ImageInspect(ctx context.Context, ref string) (image.InspectResponse, error) {
	res, err := d.cli.ImageInspect(ctx, ref)
	if err != nil {
		return image.InspectResponse{}, err
	}
	return res.InspectResponse, nil
}

func (d *LocalDocker) ImageSave(ctx context.Context, refs []string) (io.ReadCloser, error) {
	return d.cli.ImageSave(ctx, refs)
}

// ImagePull fetches ref into the local docker and waits for it to finish.
//
// No RegistryAuth is passed, and that is a real limit rather than an oversight: the
// engine API takes credentials per call and does NOT read ~/.docker/config.json (that
// is the CLI's doing), so there is no honest way to reuse a login from here. A pull of
// a private third-party image therefore fails with the registry's own 401. The way
// through is to pull it with the docker CLI before `sc image ship` — Options.Pull only
// fetches what is ABSENT, so an image the runner already holds is shipped as it stands.
//
// Wait, not a drained io.Reader: the engine reports a failed pull inside the JSON
// message stream with a 200 on the HTTP response, so a pull whose stream is merely
// discarded looks successful and the failure resurfaces as "image is still not
// inspectable" one call later.
func (d *LocalDocker) ImagePull(ctx context.Context, ref string) error {
	resp, err := d.cli.ImagePull(ctx, ref, client.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Close() }()
	return resp.Wait(ctx)
}

func (d *LocalDocker) Close() error {
	return d.cli.Close()
}
