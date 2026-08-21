// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"testing"

	gcpStorage "cloud.google.com/go/storage"
	. "github.com/onsi/gomega"
)

func managedNoncurrentRule(days int64) gcpStorage.LifecycleRule {
	return gcpStorage.LifecycleRule{
		Action: gcpStorage.LifecycleAction{Type: gcpStorage.DeleteAction},
		Condition: gcpStorage.LifecycleCondition{
			Liveness:                gcpStorage.Archived,
			DaysSinceNoncurrentTime: days,
		},
	}
}

func managedBackupsRule(days int64) gcpStorage.LifecycleRule {
	return gcpStorage.LifecycleRule{
		Action: gcpStorage.LifecycleAction{Type: gcpStorage.DeleteAction},
		Condition: gcpStorage.LifecycleCondition{
			AgeInDays:     days,
			MatchesPrefix: []string{PulumiBackupsPrefix},
		},
	}
}

// The single most dangerous thing in this file. The backups rule matches LIVE
// objects by age, so it is only safe while it is scoped to the backups prefix.
// Unscoped, it deletes the current state file of every stack in the bucket.
func TestBackupsRuleIsAlwaysScopedToThePrefix(t *testing.T) {
	RegisterTestingT(t)

	lc := StateHistoryLifecycle(30)
	Expect(lc.Rules).To(HaveLen(2))

	for i, rule := range lc.Rules {
		if rule.Condition.AgeInDays == 0 {
			continue
		}
		Expect(rule.Condition.MatchesPrefix).To(Equal([]string{PulumiBackupsPrefix}),
			"rule %d matches live objects by age and MUST be scoped to the backups prefix; "+
				"unscoped it deletes the live state file", i)
	}
}

// Restating the invariant independently of rule ordering or count, so a future
// rule cannot slip in unscoped.
func TestNoAgeBasedRuleIsUnscoped(t *testing.T) {
	RegisterTestingT(t)

	for _, days := range []int{1, 7, 30, 365} {
		for _, rule := range StateHistoryLifecycle(days).Rules {
			if rule.Condition.AgeInDays > 0 {
				Expect(rule.Condition.MatchesPrefix).ToNot(BeEmpty(),
					"an AgeInDays rule with no prefix expires live state")
			}
		}
	}
}

func TestStateHistoryLifecycleKeepsTheNoncurrentRule(t *testing.T) {
	RegisterTestingT(t)

	lc := StateHistoryLifecycle(30)
	Expect(lc.Rules).To(ContainElement(managedNoncurrentRule(30)),
		"the backups rule is additional to the noncurrent rule, not a replacement")
	Expect(lc.Rules).To(ContainElement(managedBackupsRule(30)))
}

func TestStateHistoryLifecycleDisabled(t *testing.T) {
	RegisterTestingT(t)

	Expect(StateHistoryLifecycle(0).Rules).To(BeEmpty(), "zero is a deliberate opt-out")
	Expect(StateHistoryLifecycle(-1).Rules).To(BeEmpty())
}

func TestShouldApplyStateHistoryLifecycle(t *testing.T) {
	RegisterTestingT(t)

	versioned := func(rules ...gcpStorage.LifecycleRule) *gcpStorage.BucketAttrs {
		return &gcpStorage.BucketAttrs{
			VersioningEnabled: true,
			Lifecycle:         gcpStorage.Lifecycle{Rules: rules},
		}
	}
	operatorRule := gcpStorage.LifecycleRule{
		Action:    gcpStorage.LifecycleAction{Type: gcpStorage.DeleteAction},
		Condition: gcpStorage.LifecycleCondition{AgeInDays: 365},
	}
	operatorBackupsRule := gcpStorage.LifecycleRule{
		Action: gcpStorage.LifecycleAction{Type: gcpStorage.DeleteAction},
		Condition: gcpStorage.LifecycleCondition{
			AgeInDays:     90,
			MatchesPrefix: []string{".pulumi/backups/"},
		},
	}
	broaderOperatorPrefix := gcpStorage.LifecycleRule{
		Action: gcpStorage.LifecycleAction{Type: gcpStorage.DeleteAction},
		Condition: gcpStorage.LifecycleCondition{
			AgeInDays:     90,
			MatchesPrefix: []string{".pulumi/"},
		},
	}

	for _, tc := range []struct {
		name  string
		attrs *gcpStorage.BucketAttrs
		days  int
		want  bool
		why   string
	}{
		{
			name:  "empty policy is the original case",
			attrs: versioned(),
			days:  30,
			want:  true,
		},
		{
			name:  "a policy this package wrote is upgradable",
			attrs: versioned(managedNoncurrentRule(30)),
			days:  30,
			want:  true,
			why: "without this the guard excludes every bucket SC already touched, " +
				"so the buckets that have the problem could never be fixed",
		},
		{
			name:  "recognised even when the day count has since changed in config",
			attrs: versioned(managedNoncurrentRule(7)),
			days:  30,
			want:  true,
			why:   "the count comes from operator config and may legitimately differ",
		},
		{
			name:  "already fully managed, so nothing to add",
			attrs: versioned(managedNoncurrentRule(30), managedBackupsRule(30)),
			days:  30,
			want:  false,
			why:   "the backups prefix is already bounded; rewriting it is a pointless write",
		},
		{
			name:  "an operator's own rule is still never touched",
			attrs: versioned(operatorRule),
			days:  30,
			want:  false,
			why:   "this is the property the original guard exists for",
		},
		{
			name:  "a managed rule alongside an operator rule is left alone",
			attrs: versioned(managedNoncurrentRule(30), operatorRule),
			days:  30,
			want:  false,
			why:   "any unrecognised rule means the policy is not ours to rewrite",
		},
		{
			name:  "an operator bounding backups their own way wins",
			attrs: versioned(operatorBackupsRule),
			days:  30,
			want:  false,
			why:   "their retention choice for the prefix must not be overridden",
		},
		{
			name:  "an operator rule covering a parent of the prefix also wins",
			attrs: versioned(broaderOperatorPrefix),
			days:  30,
			want:  false,
			why:   ".pulumi/ already covers .pulumi/backups/",
		},
		{
			name:  "versioning off means the noncurrent half would be inert",
			attrs: &gcpStorage.BucketAttrs{VersioningEnabled: false},
			days:  30,
			want:  false,
		},
		{
			name:  "retention explicitly disabled",
			attrs: versioned(),
			days:  0,
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
			Expect(ShouldApplyStateHistoryLifecycle(tc.attrs, tc.days)).To(Equal(tc.want), tc.why)
		})
	}
}

// The shape match is what separates "ours" from "theirs". If it were loose
// enough to accept an operator's rule, the guard above would start rewriting
// policies it must never touch.
func TestIsManagedStateHistoryRuleRejectsLookalikes(t *testing.T) {
	RegisterTestingT(t)

	for _, tc := range []struct {
		name string
		rule gcpStorage.LifecycleRule
		want bool
	}{
		{"our noncurrent rule", managedNoncurrentRule(30), true},
		{"our backups rule", managedBackupsRule(30), true},
		{
			name: "not a delete action",
			rule: gcpStorage.LifecycleRule{
				Action:    gcpStorage.LifecycleAction{Type: gcpStorage.SetStorageClassAction, StorageClass: "NEARLINE"},
				Condition: gcpStorage.LifecycleCondition{Liveness: gcpStorage.Archived, DaysSinceNoncurrentTime: 30},
			},
		},
		{
			name: "age rule with a different prefix",
			rule: gcpStorage.LifecycleRule{
				Action: gcpStorage.LifecycleAction{Type: gcpStorage.DeleteAction},
				Condition: gcpStorage.LifecycleCondition{
					AgeInDays:     30,
					MatchesPrefix: []string{"other/"},
				},
			},
		},
		{
			name: "unscoped age rule is emphatically not ours",
			rule: gcpStorage.LifecycleRule{
				Action:    gcpStorage.LifecycleAction{Type: gcpStorage.DeleteAction},
				Condition: gcpStorage.LifecycleCondition{AgeInDays: 30},
			},
		},
		{
			name: "carries an extra condition we never set",
			rule: gcpStorage.LifecycleRule{
				Action: gcpStorage.LifecycleAction{Type: gcpStorage.DeleteAction},
				Condition: gcpStorage.LifecycleCondition{
					Liveness:                gcpStorage.Archived,
					DaysSinceNoncurrentTime: 30,
					NumNewerVersions:        3,
				},
			},
		},
		{
			name: "matches a storage class we never filter on",
			rule: gcpStorage.LifecycleRule{
				Action: gcpStorage.LifecycleAction{Type: gcpStorage.DeleteAction},
				Condition: gcpStorage.LifecycleCondition{
					Liveness:                gcpStorage.Archived,
					DaysSinceNoncurrentTime: 30,
					MatchesStorageClasses:   []string{"STANDARD"},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			Expect(isManagedStateHistoryRule(tc.rule)).To(Equal(tc.want))
		})
	}
}

// A freshly created bucket must get both rules, not just the noncurrent one.
// Getting this wrong is how the original fix ended up only half-applied.
func TestNewBucketGetsBothRules(t *testing.T) {
	RegisterTestingT(t)

	lc := StateHistoryLifecycle(30)
	var archived, backups int
	for _, r := range lc.Rules {
		if r.Condition.Liveness == gcpStorage.Archived && r.Condition.DaysSinceNoncurrentTime > 0 {
			archived++
		}
		if r.Condition.AgeInDays > 0 && len(r.Condition.MatchesPrefix) == 1 {
			backups++
		}
	}
	Expect(archived).To(Equal(1), "superseded generations must be bounded")
	Expect(backups).To(Equal(1), "the backups prefix must be bounded too, or KMS versions stay pinned")
}
