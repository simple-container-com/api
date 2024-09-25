package docker

import (
	"bufio"

	"golang.org/x/sync/errgroup"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api/logger"
)

type PullArgs struct {
	RemoteImage sdk.StringInput `pulumi:"remoteImage"`
	AuthHeader  sdk.StringInput `pulumi:"authHeader"`
	Platform    sdk.StringInput `pulumi:"platform"`
	Log         logger.Logger
}

type Pull struct {
	sdk.ResourceState

	Digest sdk.StringOutput `pulumi:"digest"`
}

func NewDockerPull(ctx *sdk.Context, name string, args *PullArgs, opts ...sdk.ResourceOption) (*Pull, error) {
	pull := &Pull{}

	err := ctx.RegisterComponentResource("simple-container.com:docker:DockerPull", name, pull, opts...)
	if err != nil {
		return nil, err
	}

	dockerAPI, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}
	dockerAPI.NegotiateAPIVersion(ctx.Context())

	msgReader := chanMsgReader{msgChan: make(chan readerNextMessage)}
	digest := sdk.All(args.RemoteImage, args.AuthHeader, args.Platform).ApplyT(func(all []any) (string, error) {
		remoteImage, authHeader, platform := all[0].(string), all[1].(string), all[2].(string)

		reader, err := dockerAPI.ImagePull(ctx.Context(), remoteImage, types.ImagePullOptions{
			RegistryAuth: authHeader,
			Platform:     platform,
		})
		if err != nil {
			return "", errors.Wrapf(err, "failed to invoke docker pull for %q", remoteImage)
		}

		errG := errgroup.Group{}

		errG.Go(func() error {
			return streamMessagesToChannel(bufio.NewReader(reader), msgReader.msgChan)
		})
		var digest string
		errG.Go(func() error {
			return msgReader.Listen(false, func(message *ResponseMessage, err error) error {
				if err != nil {
					args.Log.Error(ctx.Context(), "error while listening for docker pull: %v", err)
					_ = ctx.Log.Error("error while listening for docker pull: "+err.Error(), &sdk.LogArgs{Resource: pull})
				} else {
					_ = ctx.Log.Info(message.summary, &sdk.LogArgs{Resource: pull})
					if message.Aux.Digest != "" {
						digest = message.Aux.Digest
					}
				}
				return nil
			})
		})
		err = errG.Wait()
		return digest, err
	})
	pull.Digest = digest.(sdk.StringOutput)
	err = ctx.RegisterResourceOutputs(pull, sdk.Map{
		"digest": digest,
	})
	if err != nil {
		return nil, err
	}

	return pull, nil
}
