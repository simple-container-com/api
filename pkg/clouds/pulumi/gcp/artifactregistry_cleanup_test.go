// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	gcpsdk "github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// arCapture records what ArtifactRegistry() actually asks Pulumi to register.
//
// Testing the helpers in isolation is what let the original change ship with
// its own fix untested: deleting the IgnoreChanges branch entirely — reverting
// to the behaviour that silently deleted retention policies — left the suite
// green, because nothing observed the ResourceOption that IS the fix.
// MockResourceArgs.RegisterRPC exposes both the inputs and the ignore list, so
// the property can be asserted where it lives.
type arCapture struct {
	inputs        resource.PropertyMap
	ignoreChanges []string
	seen          bool
}

func (c *arCapture) NewResource(a sdk.MockResourceArgs) (string, resource.PropertyMap, error) {
	if a.TypeToken == "gcp:artifactregistry/repository:Repository" {
		c.seen, c.inputs = true, a.Inputs
		if a.RegisterRPC != nil {
			c.ignoreChanges = a.RegisterRPC.IgnoreChanges
		}
	}
	out := a.Inputs.Mappable()
	out["email"] = "sa@test-project.iam.gserviceaccount.com"
	out["privateKey"] = "x"
	return a.Name + "-id", resource.NewPropertyMapFromMap(out), nil
}

func (c *arCapture) Call(sdk.MockCallArgs) (resource.PropertyMap, error) {
	return resource.PropertyMap{}, nil
}

// renderARRepo runs the real provisioning function under mocks and returns what
// reached the Repository resource. err is the provisioning error, if any.
func renderARRepo(cfg *gcloud.ArtifactRegistryConfig) (*arCapture, error) {
	setGlobalServicesAPIClient(newMockServicesAPIClient())
	defer resetGlobalServicesAPIClient()

	cfg.Docker = &gcloud.DockerConfig{ImmutableTags: lo.ToPtr(false)}
	cfg.Location = "europe-west3"
	cfg.ProjectId = "test-project"

	capture := &arCapture{}
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		prov, err := gcpsdk.NewProvider(ctx, "test-provider", &gcpsdk.ProviderArgs{
			Project: sdk.String("test-project"),
		})
		if err != nil {
			return err
		}
		_, err = ArtifactRegistry(ctx, api.Stack{}, api.ResourceInput{
			Descriptor: &api.ResourceDescriptor{
				Name:   "registry",
				Type:   gcloud.ResourceTypeArtifactRegistry,
				Config: api.Config{Config: cfg},
			},
			StackParams: &api.StackParams{Environment: "test", StackName: "s"},
		}, pApi.ProvisionParams{Log: logger.New(), Provider: prov})
		return err
	}, sdk.WithMocks("test", "test", capture))
	return capture, err
}

// The fix itself: with no retention declared, SC must ask the engine to leave
// the field alone and must not send an empty value, which is what deleted the
// policies. Removing the IgnoreChanges branch fails this test.
func TestUndeclaredRetentionIsIgnoredNotEmptied(t *testing.T) {
	RegisterTestingT(t)

	c, err := renderARRepo(&gcloud.ArtifactRegistryConfig{})
	Expect(err).To(BeNil())
	Expect(c.seen).To(BeTrue())

	Expect(c.ignoreChanges).To(ConsistOf("cleanupPolicies", "cleanupPolicyDryRun"),
		"undeclared retention must be ignored, including dryRun, or a provision can flip an "+
			"out-of-band dry-run repository into enforcing")
	Expect(c.inputs).NotTo(HaveKey(resource.PropertyKey("cleanupPolicies")),
		"sending an empty value is what deleted the policies")
	Expect(c.inputs).NotTo(HaveKey(resource.PropertyKey("cleanupPolicyDryRun")))
}

// The ignore list must not leak onto the IAM policy, service accounts or keys,
// none of which has a cleanupPolicies property.
func TestIgnoreChangesIsScopedToTheRepository(t *testing.T) {
	RegisterTestingT(t)

	var others []string
	leakCheck := &arLeakCapture{other: &others}
	setGlobalServicesAPIClient(newMockServicesAPIClient())
	defer resetGlobalServicesAPIClient()
	cfg := &gcloud.ArtifactRegistryConfig{
		Docker: &gcloud.DockerConfig{ImmutableTags: lo.ToPtr(false)}, Location: "europe-west3",
	}
	cfg.ProjectId = "test-project"
	Expect(sdk.RunErr(func(ctx *sdk.Context) error {
		prov, err := gcpsdk.NewProvider(ctx, "p", &gcpsdk.ProviderArgs{Project: sdk.String("test-project")})
		if err != nil {
			return err
		}
		_, err = ArtifactRegistry(ctx, api.Stack{}, api.ResourceInput{
			Descriptor:  &api.ResourceDescriptor{Name: "registry", Type: gcloud.ResourceTypeArtifactRegistry, Config: api.Config{Config: cfg}},
			StackParams: &api.StackParams{Environment: "test", StackName: "s"},
		}, pApi.ProvisionParams{Log: logger.New(), Provider: prov})
		return err
	}, sdk.WithMocks("test", "test", leakCheck))).To(BeNil())

	Expect(others).To(BeEmpty(), "cleanup ignore list leaked onto non-repository resources: %v", others)
}

type arLeakCapture struct{ other *[]string }

func (c *arLeakCapture) NewResource(a sdk.MockResourceArgs) (string, resource.PropertyMap, error) {
	if a.TypeToken != "gcp:artifactregistry/repository:Repository" && a.RegisterRPC != nil {
		for _, ic := range a.RegisterRPC.IgnoreChanges {
			if ic == "cleanupPolicies" || ic == "cleanupPolicyDryRun" {
				*c.other = append(*c.other, a.TypeToken+":"+ic)
			}
		}
	}
	out := a.Inputs.Mappable()
	out["email"] = "sa@test-project.iam.gserviceaccount.com"
	out["privateKey"] = "x"
	return a.Name + "-id", resource.NewPropertyMapFromMap(out), nil
}
func (c *arLeakCapture) Call(sdk.MockCallArgs) (resource.PropertyMap, error) {
	return resource.PropertyMap{}, nil
}

// Declared retention must reach the resource intact. Every field asserted here
// survives a mutant that drops or hardcodes it.
func TestDeclaredRetentionReachesRepoArgs(t *testing.T) {
	RegisterTestingT(t)

	c, err := renderARRepo(&gcloud.ArtifactRegistryConfig{
		CleanupPolicies: &[]gcloud.ArtifactRegistryCleanupPolicy{
			{
				Name: "delete-untagged-older-30d", Action: "delete",
				Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{TagState: "untagged", OlderThan: "2592000s"},
			},
			{
				Name: "keep-most-recent-20", Action: "KEEP",
				MostRecentVersions: &gcloud.ArtifactRegistryCleanupMostRecentVersions{KeepCount: lo.ToPtr(20)},
			},
		},
		CleanupPolicyDryRun: lo.ToPtr(true),
	})
	Expect(err).To(BeNil())
	Expect(c.ignoreChanges).To(BeEmpty(), "declared retention is managed, so nothing may be ignored")
	Expect(c.inputs["cleanupPolicyDryRun"].BoolValue()).To(BeTrue())

	arr := c.inputs["cleanupPolicies"].ArrayValue()
	Expect(arr).To(HaveLen(2))

	p0 := arr[0].ObjectValue()
	Expect(p0["id"].StringValue()).To(Equal("delete-untagged-older-30d"))
	Expect(p0["action"].StringValue()).To(Equal("DELETE"), "action must be upper-cased for the API")
	Expect(p0["condition"].ObjectValue()["tagState"].StringValue()).To(Equal("UNTAGGED"))
	Expect(p0["condition"].ObjectValue()["olderThan"].StringValue()).To(Equal("2592000s"))

	p1 := arr[1].ObjectValue()
	Expect(p1["action"].StringValue()).To(Equal("KEEP"))
	Expect(p1["mostRecentVersions"].ObjectValue()["keepCount"].NumberValue()).To(BeEquivalentTo(20))
}

// Unset dryRun must mean dry run. Enforcement destroys image layers no
// provision can restore; dry-run costs storage and is undone by one boolean.
func TestUnsetDryRunDefaultsToDryRun(t *testing.T) {
	RegisterTestingT(t)

	c, err := renderARRepo(&gcloud.ArtifactRegistryConfig{
		CleanupPolicies: &[]gcloud.ArtifactRegistryCleanupPolicy{{
			Name: "p", Action: "DELETE",
			Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{TagState: "UNTAGGED", OlderThan: "30d"},
		}},
	})
	Expect(err).To(BeNil())
	Expect(c.inputs["cleanupPolicyDryRun"].BoolValue()).To(BeTrue(),
		"omitting cleanupPolicyDryRun must not enforce deletion on the first provision")
}

func TestEnforcingRequiresAnExplicitFalse(t *testing.T) {
	RegisterTestingT(t)

	c, err := renderARRepo(&gcloud.ArtifactRegistryConfig{
		CleanupPolicies: &[]gcloud.ArtifactRegistryCleanupPolicy{{
			Name: "p", Action: "DELETE",
			Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{TagState: "UNTAGGED", OlderThan: "30d"},
		}},
		CleanupPolicyDryRun: lo.ToPtr(false),
	})
	Expect(err).To(BeNil())
	Expect(c.inputs["cleanupPolicyDryRun"].BoolValue()).To(BeFalse())
}

// An explicitly empty list is "managed, and I want none" — the only way to
// remove retention through config.
func TestEmptyListIsManagedAndEmpty(t *testing.T) {
	RegisterTestingT(t)

	c, err := renderARRepo(&gcloud.ArtifactRegistryConfig{
		CleanupPolicies: &[]gcloud.ArtifactRegistryCleanupPolicy{},
	})
	Expect(err).To(BeNil())
	Expect(c.ignoreChanges).To(BeEmpty(),
		"an explicit empty list means SC owns the field; ignoring it would make retention impossible to remove")
	Expect(c.inputs).To(HaveKey(resource.PropertyKey("cleanupPolicies")))
	Expect(c.inputs["cleanupPolicies"].ArrayValue()).To(BeEmpty())
}

func TestDryRunWithoutPoliciesIsRejected(t *testing.T) {
	RegisterTestingT(t)

	_, err := renderARRepo(&gcloud.ArtifactRegistryConfig{CleanupPolicyDryRun: lo.ToPtr(true)})
	Expect(err).NotTo(BeNil(), "config that silently does nothing must fail loudly")
	Expect(err.Error()).To(ContainSubstring("cleanupPolicyDryRun is set but cleanupPolicies is not declared"))
}

func TestArtifactRegistryRejectsInvalidCleanupPolicy(t *testing.T) {
	RegisterTestingT(t)

	_, err := renderARRepo(&gcloud.ArtifactRegistryConfig{
		CleanupPolicies: &[]gcloud.ArtifactRegistryCleanupPolicy{{Name: "p", Action: "PURGE"}},
	})
	Expect(err).NotTo(BeNil())
	Expect(err.Error()).To(ContainSubstring("invalid cleanup policies for artifact registry"),
		"the error must name the registry and environment like every other failure in this function")
}

func TestManagesCleanupPolicies(t *testing.T) {
	RegisterTestingT(t)

	Expect((&gcloud.ArtifactRegistryConfig{}).ManagesCleanupPolicies()).To(BeFalse(),
		"absent means not managed, so out-of-band retention is preserved rather than deleted")
	Expect((&gcloud.ArtifactRegistryConfig{
		CleanupPolicies: &[]gcloud.ArtifactRegistryCleanupPolicy{},
	}).ManagesCleanupPolicies()).To(BeTrue(),
		"an explicit empty list is managed-and-empty, which is how retention is removed")
	Expect((&gcloud.ArtifactRegistryConfig{
		CleanupPolicies: &[]gcloud.ArtifactRegistryCleanupPolicy{{Name: "x"}},
	}).ManagesCleanupPolicies()).To(BeTrue())
}

func TestCleanupPolicyArgsRejectsBadInput(t *testing.T) {
	RegisterTestingT(t)

	ok := func(p gcloud.ArtifactRegistryCleanupPolicy) []gcloud.ArtifactRegistryCleanupPolicy {
		return []gcloud.ArtifactRegistryCleanupPolicy{p}
	}
	for _, tc := range []struct {
		name    string
		in      []gcloud.ArtifactRegistryCleanupPolicy
		wantErr string
		why     string
	}{
		{
			name:    "empty condition matches every version",
			in:      ok(gcloud.ArtifactRegistryCleanupPolicy{Name: "p", Action: "DELETE", Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{}}),
			wantErr: "condition has no criteria",
			why:     "config decoding is non-strict, so a mistyped key produces exactly this and would delete the whole repository",
		},
		{
			name: "tagState alone does not narrow a DELETE",
			in: ok(gcloud.ArtifactRegistryCleanupPolicy{Name: "p", Action: "DELETE",
				Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{TagState: "UNTAGGED"}}),
			wantErr: "must narrow by olderThan or a prefix list",
			why:     "every untagged version, regardless of age, includes ones just pushed",
		},
		{
			name: "newerThan alone targets running images",
			in: ok(gcloud.ArtifactRegistryCleanupPolicy{Name: "p", Action: "DELETE",
				Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{NewerThan: "7d"}}),
			wantErr: "must narrow by olderThan or a prefix list",
			why:     "deleting the most recently pushed images is the shortest path to CannotPull",
		},
		{
			name: "condition and mostRecentVersions together",
			in: ok(gcloud.ArtifactRegistryCleanupPolicy{Name: "p", Action: "KEEP",
				Condition:          &gcloud.ArtifactRegistryCleanupPolicyCondition{OlderThan: "30d"},
				MostRecentVersions: &gcloud.ArtifactRegistryCleanupMostRecentVersions{KeepCount: lo.ToPtr(2)}}),
			wantErr: "mutually exclusive",
			why:     "they are a union field; the API rejects it mid-provision",
		},
		{
			name: "tagPrefixes without TAGGED",
			in: ok(gcloud.ArtifactRegistryCleanupPolicy{Name: "p", Action: "DELETE",
				Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{TagState: "UNTAGGED", TagPrefixes: []string{"v"}}}),
			wantErr: "requires tagState TAGGED",
		},
		{
			name: "empty prefix matches everything",
			in: ok(gcloud.ArtifactRegistryCleanupPolicy{Name: "p", Action: "DELETE",
				Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{TagState: "TAGGED", TagPrefixes: []string{""}}}),
			wantErr: "empty prefix",
			why:     "an unresolved template placeholder collapses to an empty string",
		},
		{
			name: "keepCount unset protects nothing",
			in: ok(gcloud.ArtifactRegistryCleanupPolicy{Name: "p", Action: "KEEP",
				MostRecentVersions: &gcloud.ArtifactRegistryCleanupMostRecentVersions{}}),
			wantErr: "keepCount >= 1",
			why:     "GCP treats a missing count as zero, so the KEEP reads as protective while protecting nothing",
		},
		{
			name: "keepCount zero",
			in: ok(gcloud.ArtifactRegistryCleanupPolicy{Name: "p", Action: "KEEP",
				MostRecentVersions: &gcloud.ArtifactRegistryCleanupMostRecentVersions{KeepCount: lo.ToPtr(0)}}),
			wantErr: "keepCount >= 1",
		},
		{
			name: "olderThan zero matches every version",
			in: ok(gcloud.ArtifactRegistryCleanupPolicy{Name: "p", Action: "DELETE",
				Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{OlderThan: "0s"}}),
			wantErr: "greater than zero",
		},
		{
			name: "garbage duration",
			in: ok(gcloud.ArtifactRegistryCleanupPolicy{Name: "p", Action: "DELETE",
				Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{OlderThan: "30days"}}),
			wantErr: "positive duration",
		},
		{
			name: "negative duration",
			in: ok(gcloud.ArtifactRegistryCleanupPolicy{Name: "p", Action: "DELETE",
				Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{OlderThan: "-5s"}}),
			wantErr: "positive duration",
		},
		{
			name: "newerThan is validated too",
			in: ok(gcloud.ArtifactRegistryCleanupPolicy{Name: "p", Action: "DELETE",
				Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{NewerThan: "abcs", TagPrefixes: []string{"v"}, TagState: "TAGGED"}}),
			wantErr: "positive duration",
			why:     "the original only validated olderThan",
		},
		{
			name:    "unknown action",
			in:      ok(gcloud.ArtifactRegistryCleanupPolicy{Name: "p", Action: "PURGE"}),
			wantErr: "action must be DELETE or KEEP",
		},
		{
			name: "mostRecentVersions under DELETE",
			in: ok(gcloud.ArtifactRegistryCleanupPolicy{Name: "p", Action: "DELETE",
				MostRecentVersions: &gcloud.ArtifactRegistryCleanupMostRecentVersions{KeepCount: lo.ToPtr(5)}}),
			wantErr: "only valid with a KEEP action",
		},
		{
			name:    "neither condition nor mostRecentVersions",
			in:      ok(gcloud.ArtifactRegistryCleanupPolicy{Name: "p", Action: "DELETE"}),
			wantErr: "needs a condition or mostRecentVersions",
		},
		{
			name:    "missing name",
			in:      ok(gcloud.ArtifactRegistryCleanupPolicy{Action: "KEEP", MostRecentVersions: &gcloud.ArtifactRegistryCleanupMostRecentVersions{KeepCount: lo.ToPtr(1)}}),
			wantErr: "missing a name",
		},
		{
			name: "duplicate names",
			in: []gcloud.ArtifactRegistryCleanupPolicy{
				{Name: "dup", Action: "DELETE", Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{OlderThan: "30d"}},
				{Name: "dup", Action: "KEEP", MostRecentVersions: &gcloud.ArtifactRegistryCleanupMostRecentVersions{KeepCount: lo.ToPtr(1)}},
			},
			wantErr: "duplicate cleanup policy name",
			why:     "Artifact Registry keys policies by name, so a duplicate silently drops one",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			_, err := cleanupPolicyArgs(tc.in)
			Expect(err).NotTo(BeNil(), tc.why)
			Expect(err.Error()).To(ContainSubstring(tc.wantErr))
		})
	}
}

func TestCleanupPolicyArgsRejectsTooManyPolicies(t *testing.T) {
	RegisterTestingT(t)

	many := make([]gcloud.ArtifactRegistryCleanupPolicy, 0, maxCleanupPolicies+1)
	for i := 0; i <= maxCleanupPolicies; i++ {
		many = append(many, gcloud.ArtifactRegistryCleanupPolicy{
			Name: string(rune('a' + i)), Action: "DELETE",
			Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{OlderThan: "30d"},
		})
	}
	_, err := cleanupPolicyArgs(many)
	Expect(err).NotTo(BeNil())
	Expect(err.Error()).To(ContainSubstring("at most 10 cleanup policies"))
}

// The provider's own acceptance tests use the day form, while the REST API
// reports seconds. Rejecting either would reject valid configuration.
func TestCleanupDurationAcceptsBothProviderForms(t *testing.T) {
	RegisterTestingT(t)

	for _, d := range []string{"30d", "7d", "2592000s", "720h", "45m", "1.5h"} {
		Expect(validateCleanupDuration("p", "olderThan", d)).To(BeNil(), "%q must be accepted", d)
	}
	for _, d := range []string{"30days", "abcs", "s", "-5s", "0s", "0d", "30 s", ""} {
		Expect(validateCleanupDuration("p", "olderThan", d)).NotTo(BeNil(), "%q must be rejected", d)
	}
}
