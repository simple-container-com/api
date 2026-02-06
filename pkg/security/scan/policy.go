package scan

import (
	"fmt"
)

// PolicyEnforcer enforces vulnerability policies
type PolicyEnforcer struct {
	config *Config
}

// NewPolicyEnforcer creates a new PolicyEnforcer
func NewPolicyEnforcer(config *Config) *PolicyEnforcer {
	return &PolicyEnforcer{
		config: config,
	}
}

// Enforce enforces the vulnerability policy on scan results
// Returns error if policy is violated (deployment should be blocked)
func (p *PolicyEnforcer) Enforce(result *ScanResult) error {
	if result == nil {
		return nil
	}

	summary := result.Summary

	// Check failOn threshold
	if p.config.FailOn != "" {
		if err := p.checkFailOn(summary); err != nil {
			return err
		}
	}

	// Check warnOn threshold (log warning but don't block)
	if p.config.WarnOn != "" {
		p.checkWarnOn(summary)
	}

	return nil
}

// checkFailOn checks if scan results violate the failOn threshold
func (p *PolicyEnforcer) checkFailOn(summary VulnerabilitySummary) error {
	switch p.config.FailOn {
	case SeverityCritical:
		if summary.HasCritical() {
			return fmt.Errorf("policy violation: found %d critical vulnerabilities (failOn: critical)", summary.Critical)
		}
	case SeverityHigh:
		if summary.HasCritical() || summary.HasHigh() {
			return fmt.Errorf("policy violation: found %d critical and %d high vulnerabilities (failOn: high)", summary.Critical, summary.High)
		}
	case SeverityMedium:
		if summary.HasCritical() || summary.HasHigh() || summary.HasMedium() {
			return fmt.Errorf("policy violation: found %d critical, %d high, %d medium vulnerabilities (failOn: medium)", summary.Critical, summary.High, summary.Medium)
		}
	case SeverityLow:
		if summary.HasCritical() || summary.HasHigh() || summary.HasMedium() || summary.HasLow() {
			return fmt.Errorf("policy violation: found %d critical, %d high, %d medium, %d low vulnerabilities (failOn: low)", summary.Critical, summary.High, summary.Medium, summary.Low)
		}
	}

	return nil
}

// checkWarnOn checks if scan results exceed the warnOn threshold and logs warnings
func (p *PolicyEnforcer) checkWarnOn(summary VulnerabilitySummary) {
	switch p.config.WarnOn {
	case SeverityCritical:
		if summary.HasCritical() {
			fmt.Printf("WARNING: found %d critical vulnerabilities (warnOn: critical)\n", summary.Critical)
		}
	case SeverityHigh:
		if summary.HasCritical() || summary.HasHigh() {
			fmt.Printf("WARNING: found %d critical and %d high vulnerabilities (warnOn: high)\n", summary.Critical, summary.High)
		}
	case SeverityMedium:
		if summary.HasCritical() || summary.HasHigh() || summary.HasMedium() {
			fmt.Printf("WARNING: found %d critical, %d high, %d medium vulnerabilities (warnOn: medium)\n", summary.Critical, summary.High, summary.Medium)
		}
	case SeverityLow:
		if summary.HasCritical() || summary.HasHigh() || summary.HasMedium() || summary.HasLow() {
			fmt.Printf("WARNING: found %d critical, %d high, %d medium, %d low vulnerabilities (warnOn: low)\n", summary.Critical, summary.High, summary.Medium, summary.Low)
		}
	}
}

// ShouldBlock returns true if the result violates the policy
func (p *PolicyEnforcer) ShouldBlock(result *ScanResult) bool {
	return p.Enforce(result) != nil
}
