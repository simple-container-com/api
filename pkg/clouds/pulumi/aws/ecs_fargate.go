package aws

import (
	"encoding/base64"
	"fmt"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ecr"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-docker/sdk/v3/go/docker"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/util"
	"strings"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type EcsFargateOutput struct {
	Images           []*docker.Image
	Repository       *ecr.Repository
	RepoPolicy       *ecr.RepositoryPolicy
	RegistryPassword sdk.StringOutput
}

func ProvisionEcsFargate(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != aws.TemplateTypeEcsFargate {
		return nil, errors.Errorf("unsupported template type %q", input.Descriptor.Type)
	}
	if input.DeployParams == nil {
		return nil, errors.Errorf("missing deploy params for %q in stack %q", input.Descriptor.Type, stack.Name)
	}
	deployParams := *input.DeployParams

	ref := &EcsFargateOutput{}
	output := &api.ResourceOutput{Ref: ref}

	crInput, ok := input.Descriptor.Config.Config.(*aws.EcsFargateInput)
	if !ok {
		return output, errors.Errorf("failed to convert ecs_fargate config for %q in stack %q in %q", input.Descriptor.Type, stack.Name, deployParams.Environment)
	}
	params.Log.Debug(ctx.Context(), "provisioning ECS Fargate for stack %q in %q: %q...", stack.Name, deployParams.Environment, crInput)

	err := createEcrRegistry(ctx, stack, params, deployParams, crInput, ref)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision ECR registry for stack %q in %q", stack.Name, deployParams.Environment)
	}

	err = buildAndPushImages(ctx, stack, params, deployParams, crInput, ref)
	if err != nil {
		return output, errors.Wrapf(err, "failed to build and push images for stack %q", stack.Name)
	}

	return output, nil
}

func buildAndPushImages(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.DeployParams, crInput *aws.EcsFargateInput, ref *EcsFargateOutput) error {
	images, err := util.MapErr(crInput.Containers, func(container aws.EcsFargateContainer, _ int) (*docker.Image, error) {
		imageName := fmt.Sprintf("%s/%s", stack.Name, container.Name)
		version := "latest" // TODO: support versioning
		imageFullUrl := ref.Repository.RepositoryUrl.ApplyT(func(repoUri string) string {
			return fmt.Sprintf("%s/%s:%s", repoUri, imageName, version)
		}).(sdk.StringOutput)
		params.Log.Info(ctx.Context(), "building docker image %q (from %q) for service %q in stack %q env %q",
			imageName, container.Image.Context, container.Name, stack.Name, deployParams.Environment)
		image, err := docker.NewImage(ctx, imageName, &docker.ImageArgs{
			Build: &docker.DockerBuildArgs{
				Context:    sdk.String(container.Image.Context),
				Dockerfile: sdk.String(container.Image.Dockerfile),
			},
			ImageName: imageFullUrl,
			Registry: docker.ImageRegistryArgs{
				Server:   ref.Repository.RepositoryUrl,
				Username: sdk.String("AWS"), // Use 'AWS' for ECR registry authentication
				Password: ref.RegistryPassword,
			},
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to build and push image for container %q in stack %q env %q", container.Name, stack.Name, deployParams.Environment)
		}
		return image, nil
	})
	if err != nil {
		return err
	}
	ref.Images = images
	for i, image := range images {
		ctx.Export(fmt.Sprintf("%s--%s--%d--image", stack.Name, deployParams.Environment, i), image.ImageName)
	}
	return nil
}

func createEcrRegistry(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.DeployParams, crInput *aws.EcsFargateInput, ref *EcsFargateOutput) error {
	ecrRepoName := strings.ReplaceAll(fmt.Sprintf("%s-ecr", stack.Name), "--", "_") // to comply with regexp (?:[a-z0-9]+(?:[._-][a-z0-9]+)*/)*[a-z0-9]+(?:[._-][a-z0-9]+)*'
	params.Log.Info(ctx.Context(), "provisioning ECR repository %q for stack %q in %q...", ecrRepoName, stack.Name, deployParams.Environment)
	ecrRepo, err := ecr.NewRepository(ctx, ecrRepoName, &ecr.RepositoryArgs{
		ForceDelete: sdk.BoolPtr(true),
		Name:        sdk.String(ecrRepoName),
	}, sdk.Provider(params.Provider))
	if err != nil {
		return errors.Wrapf(err, "failed to provision ECR repository %q for stack %q in %q", ecrRepoName, stack.Name, deployParams.Environment)
	}
	ref.Repository = ecrRepo
	ctx.Export(fmt.Sprintf("%s-registry-url", ecrRepoName), ecrRepo.RepositoryUrl)

	ecrPolicyDoc, err := iam.GetPolicyDocument(ctx, &iam.GetPolicyDocumentArgs{
		Statements: []iam.GetPolicyDocumentStatement{
			{
				Sid:    sdk.StringRef(fmt.Sprintf("%s-policy", ecrRepoName)),
				Effect: sdk.StringRef("Allow"),
				Principals: []iam.GetPolicyDocumentStatementPrincipal{
					{
						Type: "AWS",
						Identifiers: []string{
							crInput.Account,
						},
					},
				},
				Actions: []string{
					"ecr:*",
				},
			},
		},
	}, sdk.Provider(params.Provider))
	if err != nil {
		return err
	}
	ecrPolicy, err := ecr.NewRepositoryPolicy(ctx, fmt.Sprintf("%s-policy", ecrRepoName), &ecr.RepositoryPolicyArgs{
		Repository: ecrRepo.Name,
		Policy:     sdk.String(ecrPolicyDoc.Json),
	}, sdk.Provider(params.Provider))
	if err != nil {
		return err
	}
	ref.RepoPolicy = ecrPolicy

	registryPassword := ecrRepo.RegistryId.ApplyT(func(registryId string) (string, error) {
		// Fetch the auth token for the registry
		creds, err := ecr.GetCredentials(ctx, &ecr.GetCredentialsArgs{
			RegistryId: registryId,
		}, sdk.Provider(params.Provider))
		if err != nil {
			return "", err
		}

		decodedCreds, err := base64.StdEncoding.DecodeString(creds.AuthorizationToken)
		if err != nil {
			return "", err
		}
		return strings.TrimPrefix(string(decodedCreds), "AWS:"), nil
	}).(sdk.StringOutput)

	ref.RegistryPassword = registryPassword
	ctx.Export(fmt.Sprintf("%s-registry-password", ecrRepoName), sdk.ToSecret(registryPassword))

	return nil
}
