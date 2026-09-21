// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	gcpStorage "cloud.google.com/go/storage"
	gcpOptions "google.golang.org/api/option"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func InitStateStore(ctx context.Context, stateStoreCfg api.StateStorageConfig, log logger.Logger) error {
	authCfg, ok := stateStoreCfg.(api.AuthConfig)
	if !ok {
		return errors.Errorf("failed to convert gcloud state storage config to api.AuthConfig")
	}
	log.Info(ctx, "Initializing gcp statestore...")
	// SECURITY: Never log actual credential values
	log.Debug(ctx, "🔍 GCP Auth Debug - Credentials length: %d", len(authCfg.CredentialsValue()))

	credValue := authCfg.CredentialsValue()
	if credValue == "" {
		log.Debug(ctx, "❌ GCP credentials are EMPTY!")
	} else if credValue[0] == '$' {
		log.Debug(ctx, "❌ GCP credentials contain unresolved placeholder (starts with '$')")
	} else if credValue[0] == '{' {
		log.Debug(ctx, "✅ GCP credentials appear to be valid JSON")
	} else {
		log.Debug(ctx, "⚠️  GCP credentials format unknown (doesn't start with '{' or '$')")
	}

	// hackily set google creds env variable, so that bucket can access it (see github.com/pulumi/pulumi/pkg/v3/authhelpers/gcpauth.go:28)
	if err := os.Setenv("GOOGLE_CREDENTIALS", credValue); err != nil {
		fmt.Println("Failed to set GOOGLE_CREDENTIALS env variable: ", err.Error())
	}

	if gcloudPath, err := exec.LookPath("gcloud"); err != nil {
		fmt.Println("WARN: Failed to find gcloud command")
	} else if f, err := os.CreateTemp(os.TempDir(), "google-creds.json"); err != nil {
		fmt.Println("WARN: failed to create temp file for google creds: ", err.Error())
	} else if _, err := f.Write([]byte(authCfg.CredentialsValue())); err != nil {
		fmt.Println("WARN: failed to write temp file for google creds: ", err.Error())
	} else if err := exec.Command(gcloudPath, "auth", "activate-service-account", "--key-file", f.Name()).Run(); err != nil {
		fmt.Println("WARN: failed to activate gcloud service account: ", err.Error())
	}

	if !stateStoreCfg.IsProvisionEnabled() {
		return nil
	}

	// provision bucket
	gcpStateCfg, ok := authCfg.(*gcloud.StateStorageConfig)
	if !ok {
		return errors.Errorf("failed to convert auth config to *gcloud.Credentials")
	}
	client, err := gcpStorage.NewClient(ctx, gcpOptions.WithCredentialsJSON([]byte(authCfg.CredentialsValue()))) //nolint:staticcheck // SA1019: no in-memory replacement available
	if err != nil {
		return errors.Wrapf(err, "failed to initialize gcp client")
	}
	defer func(client *gcpStorage.Client) {
		_ = client.Close()
	}(client)
	bucketRef := client.Bucket(gcpStateCfg.GetBucketName())
	retentionDays := gcpStateCfg.EffectiveNoncurrentVersionRetentionDays()

	attrs, err := bucketRef.Attrs(ctx)
	if err != nil {
		// does not exist
		return bucketRef.Create(ctx, gcpStateCfg.ProjectId, &gcpStorage.BucketAttrs{
			Location:  lo.FromPtr(gcpStateCfg.Location),
			Lifecycle: StateHistoryLifecycle(retentionDays),
		})
	}

	if ShouldApplyStateHistoryLifecycle(attrs, retentionDays) {
		log.Info(ctx, "state bucket %q has object versioning enabled and no lifecycle policy of its own; "+
			"bounding superseded state generations and %s to %d days",
			gcpStateCfg.GetBucketName(), PulumiBackupsPrefix, retentionDays)
		if _, err := bucketRef.Update(ctx, gcpStorage.BucketAttrsToUpdate{
			Lifecycle: lo.ToPtr(StateHistoryLifecycle(retentionDays)),
		}); err != nil {
			return errors.Wrapf(err, "failed to bound state history on bucket %q", gcpStateCfg.GetBucketName())
		}
	}
	return nil
}

// PulumiBackupsPrefix is where the Pulumi gs backend writes a per-update copy of
// each stack's state. Every rule that can delete a LIVE object must be scoped to
// this prefix; a prefix-less age rule would delete the current state file.
const PulumiBackupsPrefix = ".pulumi/backups/"

// NoncurrentVersionLifecycle deletes state generations once they have been
// superseded for retentionDays. It targets Archived objects only, so the live
// state file is never a candidate. A non-positive retention yields no rules.
func NoncurrentVersionLifecycle(retentionDays int) gcpStorage.Lifecycle {
	if retentionDays <= 0 {
		return gcpStorage.Lifecycle{}
	}
	return gcpStorage.Lifecycle{
		Rules: []gcpStorage.LifecycleRule{
			{
				Action: gcpStorage.LifecycleAction{Type: gcpStorage.DeleteAction},
				Condition: gcpStorage.LifecycleCondition{
					Liveness:                gcpStorage.Archived,
					DaysSinceNoncurrentTime: int64(retentionDays),
				},
			},
		},
	}
}

// StateHistoryLifecycle bounds BOTH halves of a state bucket's history.
//
// The noncurrent rule alone does not do that, and the gap is not academic.
// Pulumi writes each per-update backup under .pulumi/backups as its own LIVE
// object, so it never becomes noncurrent and a DaysSinceNoncurrentTime rule
// never reaches it. Measured on one real bucket 2026-08-20: 11,947 such objects
// going back to the day the bucket was created, none of them ever eligible for
// deletion.
//
// That matters beyond storage, which is trivial for state files. Each backup
// carries the stack's data key wrapped under whichever KMS key version was
// primary when it was written, so an unbounded backups prefix keeps nearly every
// key version alive and billable. On the bucket above that was ~7,000 versions,
// around $420/mo, none of it reclaimable while the prefix grows without limit.
//
// The second rule is therefore age-based and LIVE-matching, which is only safe
// because it is scoped to the backups prefix. It is emitted only when that
// prefix is non-empty, so a refactor that loses the scope produces no rule
// rather than a rule that deletes live state.
func StateHistoryLifecycle(retentionDays int) gcpStorage.Lifecycle {
	lc := NoncurrentVersionLifecycle(retentionDays)
	if retentionDays <= 0 || PulumiBackupsPrefix == "" {
		return lc
	}
	lc.Rules = append(lc.Rules, gcpStorage.LifecycleRule{
		Action: gcpStorage.LifecycleAction{Type: gcpStorage.DeleteAction},
		Condition: gcpStorage.LifecycleCondition{
			AgeInDays:     int64(retentionDays),
			MatchesPrefix: []string{PulumiBackupsPrefix},
		},
	})
	return lc
}

// isManagedStateHistoryRule reports whether a rule is one this package writes.
//
// Used to tell "a policy Simple Container put here" apart from "a policy the
// operator wrote", so the former can be upgraded when this package learns a new
// rule while the latter is still never touched. The match is on exact shape, not
// on the day count, because the day count comes from the operator's config and
// may legitimately have changed since it was written.
func isManagedStateHistoryRule(rule gcpStorage.LifecycleRule) bool {
	if rule.Action.Type != gcpStorage.DeleteAction {
		return false
	}
	c := rule.Condition
	noExtras := c.CreatedBefore.IsZero() && c.CustomTimeBefore.IsZero() &&
		c.DaysSinceCustomTime == 0 && c.NumNewerVersions == 0 &&
		len(c.MatchesStorageClasses) == 0 && len(c.MatchesSuffix) == 0
	if !noExtras {
		return false
	}
	// The noncurrent rule.
	if c.Liveness == gcpStorage.Archived && c.DaysSinceNoncurrentTime > 0 &&
		c.AgeInDays == 0 && len(c.MatchesPrefix) == 0 {
		return true
	}
	// The backups rule.
	if c.AgeInDays > 0 && c.DaysSinceNoncurrentTime == 0 &&
		len(c.MatchesPrefix) == 1 && c.MatchesPrefix[0] == PulumiBackupsPrefix {
		return true
	}
	return false
}

// hasBackupsPruningRule reports whether anything already bounds the backups
// prefix, however it was written. An operator who bounds it their own way must
// not be overridden.
func hasBackupsPruningRule(attrs *gcpStorage.BucketAttrs) bool {
	if attrs == nil {
		return false
	}
	for _, rule := range attrs.Lifecycle.Rules {
		for _, prefix := range rule.Condition.MatchesPrefix {
			if strings.HasPrefix(PulumiBackupsPrefix, prefix) || strings.HasPrefix(prefix, PulumiBackupsPrefix) {
				return true
			}
		}
	}
	return false
}

// ShouldApplyStateHistoryLifecycle decides whether to write the full two-rule
// policy onto a bucket that already exists.
//
// It extends ShouldApplyNoncurrentVersionLifecycle with one case: a bucket whose
// rules were all written by this package. Without that, the guard below excludes
// every bucket Simple Container has already touched, so the backups rule would
// only ever reach newly created buckets and the buckets that actually have the
// problem would never be fixed.
//
// The property the original guard exists for is kept intact: a bucket carrying
// any rule this package did not write is left completely alone, and so is a
// bucket where the backups prefix is already bounded by some other means.
func ShouldApplyStateHistoryLifecycle(attrs *gcpStorage.BucketAttrs, retentionDays int) bool {
	if attrs == nil || retentionDays <= 0 {
		return false
	}
	if !attrs.VersioningEnabled {
		return false
	}
	if len(attrs.Lifecycle.Rules) == 0 {
		return true
	}
	if hasBackupsPruningRule(attrs) {
		return false
	}
	for _, rule := range attrs.Lifecycle.Rules {
		if !isManagedStateHistoryRule(rule) {
			return false
		}
	}
	return true
}

// ShouldApplyNoncurrentVersionLifecycle decides whether to modify a state
// bucket that already exists.
//
// Deprecated: use ShouldApplyStateHistoryLifecycle. This one is retained for
// compatibility but is too narrow to be useful on its own: it excludes every
// bucket that already carries a rule, including the buckets whose only rule was
// written by this package, which are exactly the ones still needing the backups
// prefix bounded.
//
// Adding a rule to somebody's existing bucket is
// the kind of thing that should be narrow and predictable, so all three
// conditions must hold:
//
//   - retention is enabled;
//   - object versioning is on, so noncurrent generations actually accumulate
//     and the rule is not merely inert;
//   - the bucket has no lifecycle rules at all, so an operator's own policy is
//     never edited, replaced or merged with.
//
// A bucket that already carries any rule is left alone even if that rule does
// not bound noncurrent versions. Guessing at intent there would be worse than
// leaving it to the operator.
func ShouldApplyNoncurrentVersionLifecycle(attrs *gcpStorage.BucketAttrs, retentionDays int) bool {
	if attrs == nil || retentionDays <= 0 {
		return false
	}
	if !attrs.VersioningEnabled {
		return false
	}
	return len(attrs.Lifecycle.Rules) == 0
}

func Provider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	pcfg, ok := input.Descriptor.Config.Config.(api.AuthConfig)
	if !ok {
		return nil, errors.Errorf("failed to cast config to api.AuthConfig")
	}

	creds := pcfg.CredentialsValue()
	projectId := pcfg.ProjectIdValue()

	provider, err := gcp.NewProvider(ctx, input.ToResName(input.Descriptor.Name), &gcp.ProviderArgs{
		Credentials: sdk.String(creds),
		Project:     sdk.String(projectId),
	})
	return &api.ResourceOutput{
		Ref: provider,
	}, err
}
