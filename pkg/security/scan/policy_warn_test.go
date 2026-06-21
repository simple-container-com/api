// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scan

import (
	"testing"

	. "github.com/onsi/gomega"
)

// TestPolicyEnforcer_WarnOn exercises checkWarnOn across each threshold level.
// checkWarnOn prints to stdout and never blocks, so we assert that Enforce
// completes without error regardless of the warning state, covering each branch.
func TestPolicyEnforcer_WarnOn(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		warnOn  Severity
		summary VulnerabilitySummary
	}{
		{
			name:    "warnOn critical with critical present",
			warnOn:  SeverityCritical,
			summary: VulnerabilitySummary{Critical: 2, Total: 2},
		},
		{
			name:    "warnOn critical with none present",
			warnOn:  SeverityCritical,
			summary: VulnerabilitySummary{High: 5, Total: 5},
		},
		{
			name:    "warnOn high with high present",
			warnOn:  SeverityHigh,
			summary: VulnerabilitySummary{High: 3, Total: 3},
		},
		{
			name:    "warnOn high with critical present",
			warnOn:  SeverityHigh,
			summary: VulnerabilitySummary{Critical: 1, Total: 1},
		},
		{
			name:    "warnOn medium with medium present",
			warnOn:  SeverityMedium,
			summary: VulnerabilitySummary{Medium: 4, Total: 4},
		},
		{
			name:    "warnOn low with low present",
			warnOn:  SeverityLow,
			summary: VulnerabilitySummary{Low: 7, Total: 7},
		},
		{
			name:    "warnOn low with nothing present",
			warnOn:  SeverityLow,
			summary: VulnerabilitySummary{},
		},
		{
			name:    "warnOn unknown severity is a no-op",
			warnOn:  "made-up",
			summary: VulnerabilitySummary{Critical: 9, Total: 9},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			cfg := &Config{WarnOn: tt.warnOn}
			enforcer := NewPolicyEnforcer(cfg)
			result := &ScanResult{Summary: tt.summary}
			// warnOn never blocks; Enforce must succeed.
			Expect(enforcer.Enforce(result)).To(Succeed())
		})
	}
}

// TestPolicyEnforcer_FailOnAndWarnOnTogether confirms that with both thresholds
// set, a failing failOn still returns a PolicyViolationError (warnOn is advisory).
func TestPolicyEnforcer_FailOnAndWarnOnTogether(t *testing.T) {
	RegisterTestingT(t)

	cfg := &Config{FailOn: SeverityHigh, WarnOn: SeverityLow}
	enforcer := NewPolicyEnforcer(cfg)
	result := &ScanResult{Summary: VulnerabilitySummary{High: 1, Low: 3, Total: 4}}

	err := enforcer.Enforce(result)
	Expect(err).To(HaveOccurred())
	var pve *PolicyViolationError
	Expect(isPolicyViolation(err, &pve)).To(BeTrue())
	Expect(pve.Message).To(ContainSubstring("failOn: high"))
}
