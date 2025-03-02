package yandex

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"path/filepath"

	pDocker "github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	pYandex "github.com/pulumi/pulumi-yandex/sdk/go/yandex"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/docker"
	"github.com/simple-container-com/api/pkg/clouds/yandex"
)

type ServerlessContainerOutput struct {
	sdk.Output
}

func ServerlessContainer(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != yandex.TemplateTypeYandexServerlessContainer {
		return nil, errors.Errorf("unsupported template type %q", input.Descriptor.Type)
	}
	if input.StackParams == nil {
		return nil, errors.Errorf("missing deploy params for %q in stack %q", input.Descriptor.Type, stack.Name)
	}

	deployParams := *input.StackParams

	folderName := fmt.Sprintf("%s-%s", input.StackParams.StackName, input.StackParams.Environment)
	folder, err := pYandex.NewResourcemanagerFolder(ctx, folderName, &pYandex.ResourcemanagerFolderArgs{Name: sdk.String(folderName)}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, errors.Errorf("unable to create folder: %v", err)
	}

	ref := &ServerlessContainerOutput{}
	output := &api.ResourceOutput{Ref: ref}

	crInput, ok := input.Descriptor.Config.Config.(*yandex.ServerlessContainerInput)
	if !ok {
		return output, errors.Errorf("failed to convert yandex-cloud config for %q in stack %q in %q", input.Descriptor.Type, stack.Name, deployParams.Environment)
	}

	// Create a Yandex Cloud Container Repository
	repoName := containerRegistryName(input.StackParams.StackName, input.StackParams.Environment)

	registry, err := pYandex.NewContainerRegistry(ctx, repoName, &pYandex.ContainerRegistryArgs{
		Name:     sdk.String(repoName),
		FolderId: folder.ID(),
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, errors.Errorf("unable to create container registry: %v", err)
	}

	stackConfig := crInput.StackConfig

	authorizedKey, err := NewAuthorizedKey(crInput.CredentialsValue())
	if err != nil {
		return nil, errors.Wrapf(err, "invalid credentials value %q", crInput.CredentialsValue()) // TODO: secret logging should be removed
	}

	serviceAccountId := sdk.String(authorizedKey.ServiceAccountId)

	dockerfile := stackConfig.Image.Dockerfile
	if !filepath.IsAbs(dockerfile) {
		dockerfile = filepath.Join(input.StackParams.StacksDir, input.StackParams.StackName, stackConfig.Image.Dockerfile)
	}

	iamToken, err := authorizedKey.GetIAMToken()
	if err != nil {
		return nil, errors.Errorf("unable to get IAM token: %v", err)
	}

	repoUrlOutput := registry.ID().ApplyT(func(id string) string {
		repoUrl := fmt.Sprintf("cr.yandex/%s", id)
		return repoUrl
	}).(sdk.StringOutput)

	dockerImage := docker.Image{
		Name:                   stack.Name,
		Dockerfile:             dockerfile,
		Args:                   lo.FromPtr(stackConfig.Image.Build).Args,
		Context:                stackConfig.Image.Context,
		Version:                lo.If(deployParams.Version != "", deployParams.Version).Else("latest"),
		RepositoryUrlWithImage: true, // since repository is individual for each image
		ProviderOptions:        nil,
		RepositoryUrl:          repoUrlOutput,
		Registry: pDocker.RegistryArgs{
			Server:   repoUrlOutput,
			Username: sdk.String("iam"),
			Password: sdk.String(iamToken),
		},
	}
	// Build a Docker image
	_, err = docker.BuildAndPushImage(ctx, stack, params, deployParams, dockerImage)
	if err != nil {
		return nil, errors.Errorf("unable to build and push image: %v", err)
	}

	imageNameOutput := registry.ID().ApplyT(func(id string) string {
		repoUrl := fmt.Sprintf("cr.yandex/%s:%s", id, dockerImage.Version)
		return repoUrl
	}).(sdk.StringOutput)

	timeout := lo.If(stackConfig.Timeout != nil, lo.FromPtr(stackConfig.Timeout)).Else(10)
	strTimeout := fmt.Sprintf("%ds", timeout)
	serverlessContainerName := fmt.Sprintf("%s-serverless-container", folderName)
	_, err = pYandex.NewServerlessContainer(ctx, serverlessContainerName, &pYandex.ServerlessContainerArgs{
		ExecutionTimeout: sdk.StringPtrFromPtr(&strTimeout),
		FolderId:         folder.ID(),
		Image: &pYandex.ServerlessContainerImageArgs{
			Url: imageNameOutput,
		},
		Memory:           sdk.Int(lo.If(stackConfig.MaxMemory == nil, 128).Else(lo.FromPtr(stackConfig.MaxMemory))),
		Name:             sdk.String(serverlessContainerName),
		ServiceAccountId: serviceAccountId,
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, errors.Errorf("unable to create serverless container: %v", err)
	}

	return output, nil
}

func containerRegistryName(stackName string, imageName string) string {
	return fmt.Sprintf("%s-%s", stackName, imageName)
}
