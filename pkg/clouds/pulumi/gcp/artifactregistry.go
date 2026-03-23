package gcp

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/artifactregistry"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/serviceaccount"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	taggingUtil "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

type ArtifactRegistryOut struct {
	Repository *artifactregistry.Repository
	URL        sdk.StringOutput
}

func ArtifactRegistry(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.ResourceTypeArtifactRegistry {
		return nil, errors.Errorf("unsupported artifact-registry type %q", input.Descriptor.Type)
	}

	arCfg, ok := input.Descriptor.Config.Config.(*gcloud.ArtifactRegistryConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert artifact-registry config for %q", input.Descriptor.Type)
	}

	iamServiceName := fmt.Sprintf("projects/%s/services/iam.googleapis.com", arCfg.ProjectId)
	if err := enableServicesAPI(ctx.Context(), input.Descriptor.Config.Config, iamServiceName); err != nil {
		return nil, errors.Wrapf(err, "failed to enable %s", iamServiceName)
	}
	gcpServiceName := fmt.Sprintf("projects/%s/services/artifactregistry.googleapis.com", arCfg.ProjectId)
	if err := enableServicesAPI(ctx.Context(), input.Descriptor.Config.Config, gcpServiceName); err != nil {
		return nil, errors.Wrapf(err, "failed to enable %s", gcpServiceName)
	}

	opts := []sdk.ResourceOption{sdk.Provider(params.Provider)}

	artifactRegistryName := toArtifactRegistryName(input, input.Descriptor.Name)
	location := arCfg.Location

	if location == "" {
		return nil, errors.Errorf("`location` must be specified for artifact registry %q in %q", artifactRegistryName, input.StackParams.Environment)
	}

	out := &ArtifactRegistryOut{}

	// Build unified labels using the tagging utility
	var stackParams api.StackParams
	if input.StackParams != nil {
		stackParams = *input.StackParams
	}
	labels := taggingUtil.BuildTagsFromStackParams(stackParams).ToGCPLabels()

	// Create a new Artifact Registry repository for Docker images
	repoArgs := artifactregistry.RepositoryArgs{
		RepositoryId: sdk.String(artifactRegistryName),
		Location:     sdk.String(location),
		Project:      sdk.StringPtr(arCfg.ProjectId),
		Labels:       sdk.ToStringMap(labels),
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
			Location:   sdk.String(location),
			Repository: repo.RepositoryId,
			PolicyData: sdk.String(`{
				"bindings": [
					{
						"role": "roles/artifactregistry.reader",
						"members": ["allUsers"]
					} 
				]
			}`),
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to apply IAM policy on artifact registry %q in %q", artifactRegistryName, input.StackParams.Environment)
		}
	}

	urlPrefix := strings.ToLower(location)
	urlSuffix := ""
	if arCfg.Docker != nil {
		urlSuffix = "-docker"
	}
	targetDomain := fmt.Sprintf("%s%s.pkg.dev", urlPrefix, urlSuffix)
	registryURL := sdk.Sprintf("%s/%s/%s", targetDomain, repo.Project, repo.RepositoryId)
	ctx.Export(toRegistryUrlExport(artifactRegistryName), registryURL)
	out.Repository = repo
	out.URL = registryURL

	// Create a GCP service account
	params.Log.Info(ctx.Context(), "configure service account for admin access to %q...", artifactRegistryName)
	_, err = createArtifactRegistryServiceAccount(ctx, arServiceAccountArgs{
		arCfg:        arCfg,
		registryName: artifactRegistryName,
		saType:       "admin",
		saRole:       "roles/artifactregistry.repoAdmin",
		input:        input,
		params:       params,
		opts:         opts,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision admin service account for artifact registry %q", artifactRegistryName)
	}
	params.Log.Info(ctx.Context(), "configure service account for read access to %q...", artifactRegistryName)
	_, err = createArtifactRegistryServiceAccount(ctx, arServiceAccountArgs{
		arCfg:        arCfg,
		registryName: artifactRegistryName,
		saType:       "reader",
		saRole:       "roles/artifactregistry.reader",
		input:        input,
		params:       params,
		opts:         opts,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision reader service account for artifact registry %q", artifactRegistryName)
	}

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
			overrideHeaderRule := pApi.OverrideHeaderRule{
				Name:       strings.ReplaceAll(*arCfg.Domain, ".", "-"),
				FromHost:   *arCfg.Domain,
				ToHost:     sdk.String(targetDomain),
				PathPrefix: fmt.Sprintf("/%s/%s", project, repoId),
			}
			if arCfg.BasicAuth != nil { //
				overrideHeaderRule.BasicAuth = &pApi.BasicAuth{
					Username: arCfg.BasicAuth.Username,
					Password: arCfg.BasicAuth.Password,
					Realm:    fmt.Sprintf("%s / %s / %s", input.StackParams.StackName, input.StackParams.Environment, input.Descriptor.Name),
				}
			}
			_, err = params.Registrar.NewOverrideHeaderRule(ctx, stack, overrideHeaderRule)
			if err != nil {
				return errors.Wrapf(err, "failed to create override host rule from %q to %q", *arCfg.Domain, targetDomain)
			}
			return dnsRecord
		})
	}

	return &api.ResourceOutput{Ref: out}, nil
}

type arServiceAccountArgs struct {
	arCfg        *gcloud.ArtifactRegistryConfig
	registryName string
	saType       string
	saRole       string
	input        api.ResourceInput
	params       pApi.ProvisionParams
	opts         []sdk.ResourceOption
}

func createArtifactRegistryServiceAccount(ctx *sdk.Context, args arServiceAccountArgs) (*serviceaccount.Account, error) {
	input, arCfg, params, registryName, opts := args.input, args.arCfg, args.params, args.registryName, args.opts
	// Create a GCP service account
	params.Log.Info(ctx.Context(), "configure service account for %s access to %q...", args.saType, registryName)

	// need to generate SA name that matches GCP rules
	// GCP service account IDs must match: ^[a-z](?:[-a-z0-9]{4,28}[a-z0-9])$
	// Replace underscores with hyphens to comply with GCP naming requirements
	sanitizedRegistryName := strings.ReplaceAll(strings.ReplaceAll(registryName, "_", "-"), "-", "")
	saName := fmt.Sprintf("%s-%s-sa", args.saType, sanitizedRegistryName)
	saName = strings.ReplaceAll(util.TrimStringMiddle(saName, 28, "-"), "--", "-")

	sa, err := serviceaccount.NewAccount(ctx, saName, &serviceaccount.AccountArgs{
		Description: sdk.String(fmt.Sprintf("Service account to %s images at in %s", args.saType, registryName)),
		AccountId:   sdk.String(saName),
		DisplayName: sdk.String(fmt.Sprintf("%s-%s-service-account", registryName, args.saType)),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision service account for artifact registry %q", registryName)
	}

	// Grant the service account access to the repository
	params.Log.Info(ctx.Context(), "grant service account %s access to %q...", args.saRole, registryName)
	_, err = artifactregistry.NewRepositoryIamMember(ctx, fmt.Sprintf("%s-%s-sa-iam-binding", registryName, args.saType), &artifactregistry.RepositoryIamMemberArgs{
		Repository: sdk.String(registryName),
		Project:    sdk.String(arCfg.ProjectId),
		Location:   sdk.String(arCfg.Location),
		Role:       sdk.String(args.saRole),
		Member:     sdk.Sprintf("serviceAccount:%s", sa.Email),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision IAM membership for %s service account for registry %q", args.saType, registryName)
	}
	opts = append(opts, sdk.Parent(sa))
	serviceAccountKey, err := serviceaccount.NewKey(ctx, fmt.Sprintf("%s-key", saName), &serviceaccount.KeyArgs{
		ServiceAccountId: sa.AccountId,
	}, opts...)
	if err != nil {
		return nil, err
	}

	ctx.Export(toRegistryServiceAccountKeyExport(input, args.saType, registryName), serviceAccountKey.PrivateKey)
	ctx.Export(toRegistryServiceAccountEmailExport(input, args.saType, registryName), sa.Email)

	return sa, err
}

func toArtifactRegistryName(input api.ResourceInput, name string) string {
	return input.ToResName(name)
}

func toRegistryUrlExport(registryName string) string {
	return fmt.Sprintf("%s-url", registryName)
}

func toRegistryServiceAccountKeyExport(input api.ResourceInput, saType string, registryName string) string {
	return input.ToResName(fmt.Sprintf("%s-%s-sa-key", saType, registryName))
}

func toRegistryServiceAccountEmailExport(input api.ResourceInput, saType string, registryName string) string {
	return input.ToResName(fmt.Sprintf("%s-%s-sa", saType, registryName))
}
