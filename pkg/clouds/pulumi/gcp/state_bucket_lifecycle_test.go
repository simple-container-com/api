// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"testing"

	gcpStorage "cloud.google.com/go/storage"
	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

// InitStateStore now touches buckets it did not create. These tests pin the
// guards on that, because the failure mode is editing an operator's lifecycle
// policy on a bucket holding all of their Pulumi state.
func TestShouldApplyNoncurrentVersionLifecycle(t *testing.T) {
	RegisterTestingT(t)

	versioned := func(rules ...gcpStorage.LifecycleRule) *gcpStorage.BucketAttrs {
		return &gcpStorage.BucketAttrs{
			VersioningEnabled: true,
			Lifecycle:         gcpStorage.Lifecycle{Rules: rules},
		}
	}
	someRule := gcpStorage.LifecycleRule{
		Action:    gcpStorage.LifecycleAction{Type: gcpStorage.DeleteAction},
		Condition: gcpStorage.LifecycleCondition{AgeInDays: 365},
	}

	for _, tc := range []struct {
		name  string
		attrs *gcpStorage.BucketAttrs
		days  int
		want  bool
		why   string
	}{
		{
			name:  "versioned bucket with no rules is the case this exists for",
			attrs: versioned(),
			days:  30,
			want:  true,
			why:   "unbounded history keeps every KMS key version load-bearing",
		},
		{
			name:  "an existing rule is left alone even if it does not bound noncurrent versions",
			attrs: versioned(someRule),
			days:  30,
			want:  false,
			why:   "the operator owns their lifecycle policy; guessing at intent is worse than doing nothing",
		},
		{
			name:  "versioning off means noncurrent generations never accumulate",
			attrs: &gcpStorage.BucketAttrs{VersioningEnabled: false},
			days:  30,
			want:  false,
			why:   "the rule would be inert, so do not touch the bucket at all",
		},
		{
			name:  "retention explicitly disabled",
			attrs: versioned(),
			days:  0,
			want:  false,
			why:   "zero is a deliberate opt-out",
		},
		{
			name:  "negative retention is not treated as enabled",
			attrs: versioned(),
			days:  -1,
			want:  false,
		},
		{
			name:  "nil attrs never provokes a write",
			attrs: nil,
			days:  30,
			want:  false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			Expect(ShouldApplyNoncurrentVersionLifecycle(tc.attrs, tc.days)).To(Equal(tc.want), tc.why)
		})
	}
}

// The rule must target archived objects only. A rule that matched live objects
// would delete the current state file, which is the worst outcome available
// here, so assert the shape rather than trusting the constant.
func TestNoncurrentVersionLifecycleTargetsArchivedOnly(t *testing.T) {
	RegisterTestingT(t)

	lc := NoncurrentVersionLifecycle(30)
	Expect(lc.Rules).To(HaveLen(1))

	rule := lc.Rules[0]
	Expect(rule.Action.Type).To(Equal(gcpStorage.DeleteAction))
	Expect(rule.Condition.Liveness).To(Equal(gcpStorage.Archived),
		"a Live or LiveAndArchived rule would delete the current state file")
	Expect(rule.Condition.DaysSinceNoncurrentTime).To(Equal(int64(30)))
	Expect(rule.Condition.AgeInDays).To(BeZero(),
		"AgeInDays counts from creation, not from being superseded, and would expire live state")
}

func TestNoncurrentVersionLifecycleDisabled(t *testing.T) {
	RegisterTestingT(t)

	Expect(NoncurrentVersionLifecycle(0).Rules).To(BeEmpty())
	Expect(NoncurrentVersionLifecycle(-5).Rules).To(BeEmpty())
}

func TestEffectiveNoncurrentVersionRetentionDays(t *testing.T) {
	RegisterTestingT(t)

	cfg := &gcloud.StateStorageConfig{}
	Expect(cfg.EffectiveNoncurrentVersionRetentionDays()).To(Equal(gcloud.DefaultNoncurrentVersionRetentionDays),
		"unset must not mean unbounded, which is the state that caused this")

	zero := 0
	cfg.NoncurrentVersionRetentionDays = &zero
	Expect(cfg.EffectiveNoncurrentVersionRetentionDays()).To(BeZero(),
		"an explicit zero is an opt-out and must be distinguishable from unset")

	seven := 7
	cfg.NoncurrentVersionRetentionDays = &seven
	Expect(cfg.EffectiveNoncurrentVersionRetentionDays()).To(Equal(7))
}
