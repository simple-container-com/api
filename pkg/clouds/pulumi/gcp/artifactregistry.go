package gcp

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/artifactregistry"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"

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
		params.Log.Info(ctx.Context(), "configure repository IAM policy for public access...")
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
	ctx.Export(fmt.Sprintf("%s-url", artifactRegistryName), sdk.Sprintf("%s/%s/%s", targetDomain, repo.Project, repo.RepositoryId))

	if arCfg.Domain != nil {
		sdk.All(repo.Project, repo.RepositoryId).ApplyT(func(outs []any) any {
			project, repoId := outs[0].(string), outs[1].(string)
			dnsRecord, err := params.Registrar.NewRecord(ctx, api.DnsRecord{
				Name:    *arCfg.Domain,
				Value:   targetDomain,
				Type:    "CNAME",
				Proxied: true,
			})
			if err != nil {
				return errors.Wrapf(err, "failed to create new DNS record for artifact registry")
			}
			_, err = params.Registrar.NewOverrideHeaderRule(ctx, stack, pApi.OverrideHeaderRule{
				FromHost:   *arCfg.Domain,
				ToHost:     targetDomain,
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
