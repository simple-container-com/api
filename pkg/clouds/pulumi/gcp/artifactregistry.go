package gcp

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/artifactregistry"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/serviceaccount"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func ArtifactRegistry(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.ResourceTypeArtifactRegistry {
		return nil, errors.Errorf("unsupported artifact-registry type %q", input.Descriptor.Type)
	}

	arCfg, ok := input.Descriptor.Config.Config.(*gcloud.ArtifactRegistryConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert artifact-registry config for %q", input.Descriptor.Type)
	}

	opts := []sdk.ResourceOption{sdk.Provider(params.Provider)}

	artifactRegistryName := input.ToResName(lo.FromPtr(input.Descriptor).Name)

	// Create a new Artifact Registry repository for Docker images
	repoArgs := artifactregistry.RepositoryArgs{
		RepositoryId: sdk.String(artifactRegistryName),
		Location:     sdk.String(arCfg.Location),
		Project:      sdk.StringPtr(arCfg.ProjectId),
	}
	if arCfg.Docker != nil {
		repoArgs.Format = sdk.String("DOCKER")
		repoArgs.DockerConfig = &artifactregistry.RepositoryDockerConfigArgs{
			ImmutableTags: sdk.Bool(lo.FromPtr(arCfg.Docker.ImmutableTags)),
		}
	} else {
		return nil, errors.Errorf("registry format is not supported")
	}

	params.Log.Info(ctx.Context(), "configure artifact registry repository %q", artifactRegistryName)
	repo, err := artifactregistry.NewRepository(ctx, artifactRegistryName, &repoArgs, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create artifact registry")
	}

	if lo.FromPtr(arCfg.Public) {
		params.Log.Info(ctx.Context(), "configure repository IAM policy for public access for %q...", artifactRegistryName)
		_, err = artifactregistry.NewRepositoryIamPolicy(ctx, fmt.Sprintf("%s-iam", artifactRegistryName), &artifactregistry.RepositoryIamPolicyArgs{
			Project:    sdk.String(arCfg.ProjectId),
			Location:   sdk.String(arCfg.Location),
			Repository: repo.RepositoryId,
			PolicyData: sdk.String(`{
				"bindings": [
					{
						"role": "roles/artifactregistry.reader",
						"members": [
							"allUsers"
						]
					}
				]
			}`),
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to apply IAM policy on artifact registry")
		}
	}

	urlPrefix := strings.ToLower(arCfg.Location)
	urlSuffix := ""
	if arCfg.Docker != nil {
		urlSuffix = "-docker"
	}
	targetDomain := fmt.Sprintf("%s%s.pkg.dev", urlPrefix, urlSuffix)
	ctx.Export(toRegistryUrlExport(input, artifactRegistryName), sdk.Sprintf("%s/%s/%s", targetDomain, repo.Project, repo.RepositoryId))

	// Create a GCP service account
	params.Log.Info(ctx.Context(), "configure service account for read access for %q...", artifactRegistryName)
	serviceAccountName := fmt.Sprintf("%s-sa", artifactRegistryName)
	sa, err := serviceaccount.NewAccount(ctx, serviceAccountName, &serviceaccount.AccountArgs{
		AccountId:   sdk.String(serviceAccountName),
		DisplayName: sdk.String(fmt.Sprintf("%s-service-account", artifactRegistryName)),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision service account for artifact registry %q", artifactRegistryName)
	}

	// Grant the service account access to the repository
	params.Log.Info(ctx.Context(), "grant service account read access to %q...", artifactRegistryName)
	_, err = artifactregistry.NewRepositoryIamMember(ctx, fmt.Sprintf("%s-sa-iam-binding", artifactRegistryName), &artifactregistry.RepositoryIamMemberArgs{
		Repository: repo.Name,
		Role:       sdk.String("roles/artifactregistry.reader"), // Grant read access
		Member:     sdk.Sprintf("serviceAccount:%s", sa.Email),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision Iam membership for service account for registry %q", artifactRegistryName)
	}
	ctx.Export(toRegistryServiceAccountEmailExport(input, artifactRegistryName), sa.Email)

	if arCfg.Domain != nil {
		sdk.All(repo.Project, repo.RepositoryId).ApplyT(func(outs []any) any {
			project, repoId := outs[0].(string), outs[1].(string)
			dnsRecord, err := params.Registrar.NewRecord(ctx, api.DnsRecord{
				Name:     *arCfg.Domain,
				ValueOut: sdk.String(targetDomain).ToStringOutput(),
				Type:     "CNAME",
				Proxied:  true,
			})
			if err != nil {
				return errors.Wrapf(err, "failed to create new DNS record for artifact registry")
			}
			_, err = params.Registrar.NewOverrideHeaderRule(ctx, stack, pApi.OverrideHeaderRule{
				FromHost:   *arCfg.Domain,
				ToHost:     sdk.String(targetDomain),
				PathPrefix: fmt.Sprintf("/%s/%s", project, repoId),
			})
			if err != nil {
				return errors.Wrapf(err, "failed to create override host rule from %q to %q", *arCfg.Domain, targetDomain)
			}
			return dnsRecord
		})
	}

	return &api.ResourceOutput{Ref: repo}, nil
}

func toRegistryUrlExport(input api.ResourceInput, registryName string) string {
	return input.ToResName(fmt.Sprintf("%s-url", registryName))
}

func toRegistryServiceAccountEmailExport(input api.ResourceInput, registryName string) string {
	return input.ToResName(fmt.Sprintf("%s-sa", registryName))
}
