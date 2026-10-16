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

func (d *LocalDocker) Close() error {
	return d.cli.Close()
}
