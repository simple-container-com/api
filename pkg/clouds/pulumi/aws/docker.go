package aws

import (
	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	pDocker "github.com/simple-container-com/api/pkg/clouds/pulumi/docker"
)

type dockerImage struct {
	name       string
	dockerfile string
	args       map[string]string
	context    string
	version    string
}

type dockerImageOut struct {
	image   *docker.Image
	addOpts []sdk.ResourceOption
}

func buildAndPushDockerImageV2(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.StackParams, image dockerImage) (*dockerImageOut, error) {
	repository, err := createEcrRegistry(ctx, stack, params, deployParams, image.name, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create ecr repository")
	}
	out, err := pDocker.BuildAndPushImage(ctx, stack, params, deployParams, pDocker.Image{
		Name:                   image.name,
		Dockerfile:             image.dockerfile,
		Args:                   image.args,
		Context:                image.context,
		Version:                image.version,
		RepositoryUrlWithImage: true, // since repository is individual for each image
		RepositoryUrl:          repository.Repository.RepositoryUrl,
		Registry: docker.RegistryArgs{
			Server:   repository.Repository.RepositoryUrl,
			Username: sdk.String("AWS"), // Use 'AWS' for ECR registry authentication
			Password: repository.Password,
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build and push docker image %q (from %q) for stack %q env %q",
			image.name, image.context, stack.Name, deployParams.Environment)
	}
	return &dockerImageOut{
		image:   out.Image,
		addOpts: out.AddOpts,
	}, nil
}
