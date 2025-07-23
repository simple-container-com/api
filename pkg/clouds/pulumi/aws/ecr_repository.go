package aws

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ecr"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func EcrRepository(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != aws.ResourceTypeEcrRepository {
		return nil, errors.Errorf("unsupported ECR repository type %q", input.Descriptor.Type)
	}

	ecrCfg, ok := input.Descriptor.Config.Config.(*aws.EcrRepository)
	if !ok {
		return nil, errors.Errorf("failed to convert bucket config for %q", input.Descriptor.Type)
	}

	ecrRepoName := ecrCfg.Name
	repo, err := createEcrRegistry(ctx, stack, params, *input.StackParams, ecrRepoName, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create ECR repository %q for %q in %q", ecrRepoName, input.StackParams.StackName, input.StackParams.Environment)
	}

	return &api.ResourceOutput{Ref: repo}, nil
}

func createEcrRegistry(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.StackParams, repoName string, ecrRepoConfig *aws.EcrRepository) (EcsFargateRepository, error) {
	res := EcsFargateRepository{}
	ecrRepoName := fmt.Sprintf("%s-%s", stack.Name, repoName)
	params.Log.Info(ctx.Context(), "configure ECR repository %q for stack %q in %q...", ecrRepoName, stack.Name, deployParams.Environment)
	ecrRepo, err := ecr.NewRepository(ctx, ecrRepoName, &ecr.RepositoryArgs{
		ForceDelete: sdk.BoolPtr(true),
		Name:        sdk.String(awsResName(ecrRepoName, "ecr")),
	}, sdk.Provider(params.Provider), sdk.DependsOn(params.ComputeContext.Dependencies()))
	if err != nil {
		return res, errors.Wrapf(err, "failed to provision ECR repository %q for stack %q in %q", ecrRepoName, stack.Name, deployParams.Environment)
	}
	res.Repository = ecrRepo
	ctx.Export(toEcrRepositoryURLExport(ecrRepoName), ecrRepo.RepositoryUrl)
	ctx.Export(toEcrRepositoryIDExport(ecrRepoName), ecrRepo.RegistryId)

	var lifecyclePolicyDocument []byte

	if ecrRepoConfig != nil && ecrRepoConfig.LifecyclePolicy != nil {
		lifecyclePolicyDocument, err = json.Marshal(ecrRepoConfig.LifecyclePolicy)
		if err != nil {
			return res, errors.Wrapf(err, "failed to marshal ECR lifecycle policy for ECR registry %s", ecrRepoName)
		}
	} else if lifecyclePolicyDocument, err = json.Marshal(aws.DefaultEcrLifecyclePolicy); err != nil {
		return res, errors.Wrapf(err, "failed to marshal ECR lifecycle policy for ecr registry %s", ecrRepoName)
	}

	// Apply the lifecycle policy to the ECR repository.
	_, err = ecr.NewLifecyclePolicy(ctx, fmt.Sprintf("%s-lc-policy", ecrRepoName), &ecr.LifecyclePolicyArgs{
		Repository: ecrRepo.Name,
		Policy:     sdk.String(lifecyclePolicyDocument),
	}, sdk.Provider(params.Provider), sdk.DependsOn(params.ComputeContext.Dependencies()))
	if err != nil {
		return res, errors.Wrapf(err, "failed to create ecr lifecycle policy for ECR registry %s", ecrRepoName)
	}

	registryPassword := ecrRepo.RegistryId.ApplyT(func(registryId string) (string, error) {
		// Fetch the auth token for the registry
		creds, err := ecr.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenArgs{
			RegistryId: lo.ToPtr(registryId),
		}, sdk.Provider(params.Provider))
		if err != nil {
			return "", err
		}

		decodedCreds, err := base64.StdEncoding.DecodeString(creds.AuthorizationToken)
		if err != nil {
			return "", errors.Wrapf(err, "failed to decode auth token for ECR registry %q", ecrRepoName)
		}
		return strings.TrimPrefix(string(decodedCreds), "AWS:"), nil
	}).(sdk.StringOutput)

	res.Password = registryPassword
	ctx.Export(toEcrRepositoryPasswordExport(ecrRepoName), sdk.ToSecret(registryPassword))

	return res, nil
}

func toEcrRepositoryIDExport(ecrRepoName string) string {
	return fmt.Sprintf("%s-id", ecrRepoName)
}

func toEcrRepositoryURLExport(ecrRepoName string) string {
	return fmt.Sprintf("%s-url", ecrRepoName)
}

func toEcrRepositoryPasswordExport(ecrRepoName string) string {
	return fmt.Sprintf("%s-password", ecrRepoName)
}
