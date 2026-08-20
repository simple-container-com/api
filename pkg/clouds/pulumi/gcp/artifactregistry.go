// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"fmt"
	"regexp"
	"strconv"
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

	// cleanupPolicies is authoritative in the provider: a Repository resource
	// that omits it sends an update clearing whatever is there. So either
	// declare it, or tell the engine to leave it alone. Sending nothing is the
	// one option that silently deletes another tool's retention policy.
	// Scoped to this resource only. opts is shared with the IAM policy, both
	// service accounts and their keys, none of which has a cleanupPolicies
	// property, so appending in place attaches a meaningless option to eight
	// resources and persists it in their state.
	repoOpts := opts
	if arCfg.ManagesCleanupPolicies() {
		declared := arCfg.DeclaredCleanupPolicies()
		policies, err := cleanupPolicyArgs(declared)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid cleanup policies for artifact registry %q in %q",
				artifactRegistryName, input.StackParams.Environment)
		}
		// Unset means dry run. The failure modes are not symmetric: dry-run when
		// you wanted enforcement costs storage and is undone by flipping one
		// boolean, while enforcement when you wanted dry run destroys image
		// layers that no provision can restore. Enforcing is therefore an
		// explicit, reviewable act.
		dryRun := true
		if arCfg.CleanupPolicyDryRun != nil {
			dryRun = *arCfg.CleanupPolicyDryRun
		}
		repoArgs.CleanupPolicies = policies
		repoArgs.CleanupPolicyDryRun = sdk.Bool(dryRun)
		if len(declared) == 0 {
			// dryRun gates deletion of VERSIONS by policies, not removal of the
			// policies themselves, so it offers no protection on this path.
			params.Log.Warn(ctx.Context(), "artifact registry %q: cleanupPolicies is declared empty, so ALL retention "+
				"policies on this repository will be removed, including any set outside SC; "+
				"remove the cleanupPolicies block instead if you meant to leave retention alone",
				artifactRegistryName)
		} else if dryRun {
			params.Log.Info(ctx.Context(), "artifact registry %q: SC manages %d cleanup policies in DRY RUN; "+
				"policies set outside SC will be replaced, nothing is deleted until cleanupPolicyDryRun is false",
				artifactRegistryName, len(declared))
		} else {
			params.Log.Warn(ctx.Context(), "artifact registry %q: cleanup policies are ENFORCING (%d policies); "+
				"versions matching a DELETE policy will be permanently removed",
				artifactRegistryName, len(declared))
		}
	} else {
		if arCfg.CleanupPolicyDryRun != nil {
			return nil, errors.Errorf("artifact registry %q in %q: cleanupPolicyDryRun is set but cleanupPolicies is not declared, "+
				"so it would have no effect; declare cleanupPolicies or remove cleanupPolicyDryRun",
				artifactRegistryName, input.StackParams.Environment)
		}
		params.Log.Info(ctx.Context(), "artifact registry %q: cleanup policies not declared, leaving any out-of-band retention untouched",
			artifactRegistryName)
		repoOpts = append(append([]sdk.ResourceOption{}, opts...), sdk.IgnoreChanges(cleanupPolicyFields))
	}

	params.Log.Info(ctx.Context(), "configure artifact registry repository %q", artifactRegistryName)
	repo, err := artifactregistry.NewRepository(ctx, artifactRegistryName, &repoArgs, repoOpts...)
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

// cleanupPolicyFields are the property paths SC declines to manage when no
// retention is configured. Both are needed: leaving cleanupPolicyDryRun out
// would let a provision flip an out-of-band dry-run repository into enforcing.
var cleanupPolicyFields = []string{"cleanupPolicies", "cleanupPolicyDryRun"}

// maxCleanupPolicies is the Artifact Registry limit. Exceeding it fails at
// apply, after other resources in the stack have already been mutated.
const maxCleanupPolicies = 10

// maxCleanupPolicyNameLen mirrors the provider's documented limit on the policy
// id; over it the apply fails.
const maxCleanupPolicyNameLen = 128

// cleanupDurationRe accepts the duration forms the provider accepts. Its own
// acceptance tests use the day form ("30d", "7d"), while the REST API reports
// seconds ("2592000s"); DurationDiffSuppress treats them as equivalent, so
// rejecting either would reject valid configuration.
// The quantity must be an INTEGER: the provider expands the m/h/d forms with
// strconv.Atoi before converting to seconds, so a fractional value such as
// "1.5h" passes any regex that allows it and then fails at registration.
var cleanupDurationRe = regexp.MustCompile(`^([0-9]+)(s|m|h|d)$`)

// validateCleanupDuration rejects shapes the provider would reject at apply,
// and zero, which is not a syntax error but a semantic one: "olderThan: 0s" on
// a DELETE policy matches every version in the repository.
func validateCleanupDuration(policy, field, v string) error {
	m := cleanupDurationRe.FindStringSubmatch(v)
	if m == nil {
		return errors.Errorf("cleanup policy %q: %s must be a positive duration such as %q or %q, got %q",
			policy, field, "30d", "2592000s", v)
	}
	// m[1] is [0-9]+ by construction, so only the range error is reachable.
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return errors.Errorf("cleanup policy %q: %s is out of range, got %q", policy, field, v)
	}
	if n <= 0 {
		return errors.Errorf("cleanup policy %q: %s must be greater than zero, got %q", policy, field, v)
	}
	return nil
}

// hasEmptyPrefix reports whether a prefix list contains an empty entry, which
// matches everything.
//
// Reachable without anyone typing "": an ${env:VAR} whose variable is unset
// resolves to the empty string. (An UNRESOLVED placeholder stays literal
// "${...}" instead, so that is not the path.)
func hasEmptyPrefix(vals []string) bool {
	return lo.Contains(vals, "")
}

// cleanupPolicyArgs converts the declared retention into provider inputs.
//
// Validation is deliberate rather than passing strings through. Two classes of
// mistake matter here and neither is caught by the provider in time to help:
// shapes the API rejects fail at APPLY, halfway through a provision; and
// shapes the API ACCEPTS but which match far more than the author intended
// delete images that are still deployed. The guards below are ordered so the
// second class is impossible to express, not merely discouraged.
func cleanupPolicyArgs(policies []gcloud.ArtifactRegistryCleanupPolicy) (artifactregistry.RepositoryCleanupPolicyArray, error) {
	if len(policies) > maxCleanupPolicies {
		return nil, errors.Errorf("artifact registry accepts at most %d cleanup policies, got %d", maxCleanupPolicies, len(policies))
	}
	out := make(artifactregistry.RepositoryCleanupPolicyArray, 0, len(policies))
	seen := make(map[string]bool, len(policies))
	for i, p := range policies {
		if p.Name == "" {
			return nil, errors.Errorf("cleanup policy #%d is missing a name", i+1)
		}
		if len(p.Name) >= maxCleanupPolicyNameLen {
			return nil, errors.Errorf("cleanup policy %q: name must be under %d characters", p.Name, maxCleanupPolicyNameLen)
		}
		if seen[p.Name] {
			return nil, errors.Errorf("duplicate cleanup policy name %q", p.Name)
		}
		seen[p.Name] = true

		action := strings.ToUpper(p.Action)
		if action != "DELETE" && action != "KEEP" {
			return nil, errors.Errorf("cleanup policy %q: action must be DELETE or KEEP, got %q", p.Name, p.Action)
		}
		if p.MostRecentVersions != nil && action != "KEEP" {
			return nil, errors.Errorf("cleanup policy %q: mostRecentVersions is only valid with a KEEP action", p.Name)
		}
		// condition and mostRecentVersions are a union field in the API.
		if p.Condition != nil && p.MostRecentVersions != nil {
			return nil, errors.Errorf("cleanup policy %q: condition and mostRecentVersions are mutually exclusive", p.Name)
		}
		if p.Condition == nil && p.MostRecentVersions == nil {
			return nil, errors.Errorf("cleanup policy %q: needs a condition or mostRecentVersions", p.Name)
		}

		args := &artifactregistry.RepositoryCleanupPolicyArgs{
			Id:     sdk.String(p.Name),
			Action: sdk.String(action),
		}
		if c := p.Condition; c != nil {
			tagState := strings.ToUpper(c.TagState)
			switch tagState {
			case "", "TAGGED", "UNTAGGED", "ANY":
			default:
				return nil, errors.Errorf("cleanup policy %q: tagState must be TAGGED, UNTAGGED or ANY, got %q", p.Name, c.TagState)
			}
			// A condition whose every field is empty is not a narrow policy, it
			// is every version in the repository: tagState defaults to ANY
			// server-side and nothing else constrains it. Config decoding is
			// non-strict, so a mistyped key ("olderThen") produces exactly this.
			if tagState == "" && c.OlderThan == "" && c.NewerThan == "" &&
				len(c.TagPrefixes)+len(c.PackageNamePrefixes)+len(c.VersionNamePrefixes) == 0 {
				return nil, errors.Errorf("cleanup policy %q: condition has no criteria and would match every version; "+
					"note that an unrecognised key is silently ignored, so check for a typo", p.Name)
			}
			for _, d := range []struct{ field, value string }{
				{"olderThan", c.OlderThan},
				{"newerThan", c.NewerThan},
			} {
				if d.value == "" {
					continue
				}
				if err := validateCleanupDuration(p.Name, d.field, d.value); err != nil {
					return nil, err
				}
			}
			// Tag prefixes only mean anything against tagged versions; the API
			// rejects the combination rather than ignoring it.
			if len(c.TagPrefixes) > 0 && tagState != "TAGGED" {
				got := c.TagState
				if got == "" {
					got = `"" (defaults to ANY)`
				} else {
					got = strconv.Quote(got)
				}
				return nil, errors.Errorf("cleanup policy %q: tagPrefixes requires tagState TAGGED, got %s", p.Name, got)
			}
			for _, pl := range []struct {
				field string
				vals  []string
			}{
				{"condition.tagPrefixes", c.TagPrefixes},
				{"condition.packageNamePrefixes", c.PackageNamePrefixes},
				{"condition.versionNamePrefixes", c.VersionNamePrefixes},
			} {
				if hasEmptyPrefix(pl.vals) {
					return nil, errors.Errorf("cleanup policy %q: %s contains an empty prefix, which matches everything", p.Name, pl.field)
				}
			}
			// Only AGE, or restricting to untagged versions, separates "old" from
			// "still running". Prefixes select which PACKAGES a policy covers,
			// not which ages, so `DELETE` + packageNamePrefixes deletes every
			// version of that package including the deployed digest, and for a
			// Docker repository versionNamePrefixes is the digest itself.
			// An earlier revision of this guard accepted prefixes as narrowing
			// and named them in the error, which steered anyone blocked on the
			// safe shape towards the destructive one.
			narrowed := c.OlderThan != "" || tagState == "UNTAGGED"
			if action == "DELETE" && !narrowed {
				return nil, errors.Errorf("cleanup policy %q: a DELETE condition must set olderThan, or target tagState UNTAGGED; "+
					"prefixes select which packages are covered, not which ages, so a prefix alone deletes the running version", p.Name)
			}
			cond := &artifactregistry.RepositoryCleanupPolicyConditionArgs{}
			if len(c.TagPrefixes) > 0 {
				cond.TagPrefixes = sdk.ToStringArray(c.TagPrefixes)
			}
			if len(c.PackageNamePrefixes) > 0 {
				cond.PackageNamePrefixes = sdk.ToStringArray(c.PackageNamePrefixes)
			}
			if len(c.VersionNamePrefixes) > 0 {
				cond.VersionNamePrefixes = sdk.ToStringArray(c.VersionNamePrefixes)
			}
			if tagState != "" {
				cond.TagState = sdk.StringPtr(tagState)
			}
			if c.OlderThan != "" {
				cond.OlderThan = sdk.StringPtr(c.OlderThan)
			}
			if c.NewerThan != "" {
				cond.NewerThan = sdk.StringPtr(c.NewerThan)
			}
			args.Condition = cond
		}
		if m := p.MostRecentVersions; m != nil {
			// keepCount nil or 0 sends most_recent_versions {} with no count,
			// which GCP treats as keeping nothing. Since KEEP is what outranks a
			// companion DELETE, that reads as a safety net while being none.
			if m.KeepCount == nil || *m.KeepCount < 1 {
				return nil, errors.Errorf("cleanup policy %q: mostRecentVersions requires keepCount >= 1", p.Name)
			}
			if hasEmptyPrefix(m.PackageNamePrefixes) {
				return nil, errors.Errorf("cleanup policy %q: mostRecentVersions.packageNamePrefixes contains an empty prefix, which matches everything", p.Name)
			}
			mrv := &artifactregistry.RepositoryCleanupPolicyMostRecentVersionsArgs{
				KeepCount: sdk.IntPtr(*m.KeepCount),
			}
			if len(m.PackageNamePrefixes) > 0 {
				mrv.PackageNamePrefixes = sdk.ToStringArray(m.PackageNamePrefixes)
			}
			args.MostRecentVersions = mrv
		}
		out = append(out, args)
	}
	return out, nil
}
