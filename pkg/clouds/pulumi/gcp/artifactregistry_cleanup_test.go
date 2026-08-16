// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

// The provider treats cleanupPolicies as authoritative. A Repository resource
// that omits it sends an update CLEARING whatever is configured, which is how a
// separately-managed retention policy gets deleted on the next provision.
// Observed on payspace-475408 2026-08-12: a policy set at 13:35:10 was cleared
// by pulumi-gcp/v8.41.1 at 13:37:31, in a request whose cleanupPolicies was
// empty. Refresh had loaded the live policy into outputs, the program declared
// no input, and the diff resolved as "delete it".
//
// So the contract is: declaring nothing must mean "not managed", never
// "delete". That is what ManagesCleanupPolicies gates.
func TestManagesCleanupPolicies(t *testing.T) {
	RegisterTestingT(t)

	Expect((&gcloud.ArtifactRegistryConfig{}).ManagesCleanupPolicies()).To(BeFalse(),
		"no declared policies must mean 'SC does not manage retention', so the field is ignored rather than emptied")

	Expect((&gcloud.ArtifactRegistryConfig{
		CleanupPolicies: []gcloud.ArtifactRegistryCleanupPolicy{{Name: "x", Action: "DELETE"}},
	}).ManagesCleanupPolicies()).To(BeTrue())

	// An explicitly empty slice is still "not managed". Treating it as "delete
	// everything declared" would make an empty YAML list destructive.
	Expect((&gcloud.ArtifactRegistryConfig{
		CleanupPolicies: []gcloud.ArtifactRegistryCleanupPolicy{},
	}).ManagesCleanupPolicies()).To(BeFalse())
}

// Both fields must be ignored together. Ignoring only cleanupPolicies would let
// a provision flip a repository that another tool put in dry-run into
// enforcing, which turns a reporting run into real deletions.
func TestCleanupPolicyFieldsCoverDryRun(t *testing.T) {
	RegisterTestingT(t)

	Expect(cleanupPolicyFields).To(ConsistOf("cleanupPolicies", "cleanupPolicyDryRun"))
}

func TestCleanupPolicyArgsAcceptsRealPolicies(t *testing.T) {
	RegisterTestingT(t)

	got, err := cleanupPolicyArgs([]gcloud.ArtifactRegistryCleanupPolicy{
		{
			Name:      "delete-untagged-older-30d",
			Action:    "DELETE",
			Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{TagState: "UNTAGGED", OlderThan: "2592000s"},
		},
		{
			Name:               "keep-most-recent-20",
			Action:             "KEEP",
			MostRecentVersions: &gcloud.ArtifactRegistryCleanupMostRecentVersions{KeepCount: lo.ToPtr(20)},
		},
	})
	Expect(err).To(BeNil())
	Expect(got).To(HaveLen(2))
}

func TestCleanupPolicyArgsRejectsBadInput(t *testing.T) {
	RegisterTestingT(t)

	for _, tc := range []struct {
		name    string
		policy  gcloud.ArtifactRegistryCleanupPolicy
		wantErr string
		why     string
	}{
		{
			name:    "unknown action",
			policy:  gcloud.ArtifactRegistryCleanupPolicy{Name: "p", Action: "PURGE"},
			wantErr: "action must be DELETE or KEEP",
			why:     "the provider rejects this at apply, by which point the config is already merged",
		},
		{
			name: "mostRecentVersions with DELETE",
			policy: gcloud.ArtifactRegistryCleanupPolicy{
				Name: "p", Action: "DELETE",
				MostRecentVersions: &gcloud.ArtifactRegistryCleanupMostRecentVersions{KeepCount: lo.ToPtr(5)},
			},
			wantErr: "only valid with a KEEP action",
			why:     "a keep-count under a DELETE action reads as protective while deleting",
		},
		{
			name:    "no condition and no mostRecentVersions",
			policy:  gcloud.ArtifactRegistryCleanupPolicy{Name: "p", Action: "DELETE"},
			wantErr: "needs a condition or mostRecentVersions",
			why:     "an unconditional DELETE policy matches every version in the repository",
		},
		{
			name: "duration without a seconds suffix",
			policy: gcloud.ArtifactRegistryCleanupPolicy{
				Name: "p", Action: "DELETE",
				Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{OlderThan: "30d"},
			},
			wantErr: "'s' suffix",
			why:     "gcloud docs use 30d style durations but the API wants seconds; silently wrong retention otherwise",
		},
		{
			name: "unknown tagState",
			policy: gcloud.ArtifactRegistryCleanupPolicy{
				Name: "p", Action: "DELETE",
				Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{TagState: "RELEASED", OlderThan: "1s"},
			},
			wantErr: "tagState must be",
			why:     "a typo'd tagState defaults to ANY server-side, widening the delete set",
		},
		{
			name:    "missing name",
			policy:  gcloud.ArtifactRegistryCleanupPolicy{Action: "KEEP", MostRecentVersions: &gcloud.ArtifactRegistryCleanupMostRecentVersions{KeepCount: lo.ToPtr(1)}},
			wantErr: "missing a name",
		},
		{
			name: "negative keepCount",
			policy: gcloud.ArtifactRegistryCleanupPolicy{
				Name: "p", Action: "KEEP",
				MostRecentVersions: &gcloud.ArtifactRegistryCleanupMostRecentVersions{KeepCount: lo.ToPtr(-1)},
			},
			wantErr: "cannot be negative",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			_, err := cleanupPolicyArgs([]gcloud.ArtifactRegistryCleanupPolicy{tc.policy})
			Expect(err).NotTo(BeNil(), tc.why)
			Expect(err.Error()).To(ContainSubstring(tc.wantErr))
		})
	}
}

func TestCleanupPolicyArgsRejectsDuplicateNames(t *testing.T) {
	RegisterTestingT(t)

	// Artifact Registry keys policies by name, so a duplicate silently drops one
	// of them and the repository ends up with retention nobody reviewed.
	_, err := cleanupPolicyArgs([]gcloud.ArtifactRegistryCleanupPolicy{
		{Name: "dup", Action: "DELETE", Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{OlderThan: "1s"}},
		{Name: "dup", Action: "KEEP", MostRecentVersions: &gcloud.ArtifactRegistryCleanupMostRecentVersions{KeepCount: lo.ToPtr(1)}},
	})
	Expect(err).NotTo(BeNil())
	Expect(err.Error()).To(ContainSubstring("duplicate cleanup policy name"))
}

// Case-insensitivity matters because the committed JSON in at least one fleet
// uses "Delete"/"Keep" while the API enum is upper-case.
func TestCleanupPolicyArgsNormalisesCase(t *testing.T) {
	RegisterTestingT(t)

	got, err := cleanupPolicyArgs([]gcloud.ArtifactRegistryCleanupPolicy{{
		Name: "p", Action: "Delete",
		Condition: &gcloud.ArtifactRegistryCleanupPolicyCondition{TagState: "untagged", OlderThan: "1s"},
	}})
	Expect(err).To(BeNil())
	Expect(got).To(HaveLen(1))
}
