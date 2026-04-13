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

// PolicyViolationError is returned when scan results exceed the configured severity threshold.
// It is distinct from tool errors so callers can apply soft-fail logic (warn but continue).
type PolicyViolationError struct {
	Message string
}

func (e *PolicyViolationError) Error() string { return e.Message }

// Enforce enforces the vulnerability policy on scan results.
// Returns *PolicyViolationError if the failOn threshold is exceeded,
// or nil for configuration problems / no violations.
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

// checkFailOn checks if scan results violate the failOn threshold.
// Returns *PolicyViolationError so callers can distinguish policy violations from tool errors.
func (p *PolicyEnforcer) checkFailOn(summary VulnerabilitySummary) error {
	var msg string
	switch p.config.FailOn {
	case SeverityCritical:
		if summary.HasCritical() {
			msg = fmt.Sprintf("policy violation: found %d critical vulnerabilities (failOn: critical)", summary.Critical)
		}
	case SeverityHigh:
		if summary.HasCritical() || summary.HasHigh() {
			msg = fmt.Sprintf("policy violation: found %d critical and %d high vulnerabilities (failOn: high)", summary.Critical, summary.High)
		}
	case SeverityMedium:
		if summary.HasCritical() || summary.HasHigh() || summary.HasMedium() {
			msg = fmt.Sprintf("policy violation: found %d critical, %d high, %d medium vulnerabilities (failOn: medium)", summary.Critical, summary.High, summary.Medium)
		}
	case SeverityLow:
		if summary.HasCritical() || summary.HasHigh() || summary.HasMedium() || summary.HasLow() {
			msg = fmt.Sprintf("policy violation: found %d critical, %d high, %d medium, %d low vulnerabilities (failOn: low)", summary.Critical, summary.High, summary.Medium, summary.Low)
		}
	default:
		// Unknown severity string — validate() should have caught this earlier, but be
		// conservative: block if any vulnerability was found rather than silently passing.
		if summary.Total > 0 {
			msg = fmt.Sprintf("policy violation: unrecognized failOn severity %q; found %d total vulnerabilities", p.config.FailOn, summary.Total)
		}
	}
	if msg != "" {
		return &PolicyViolationError{Message: msg}
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
