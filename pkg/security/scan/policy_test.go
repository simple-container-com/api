package scan

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestPolicyEnforcer_Enforce_Critical(t *testing.T) {
	RegisterTestingT(t)

	config := &Config{
		FailOn: SeverityCritical,
	}
	enforcer := NewPolicyEnforcer(config)

	tests := []struct {
		name      string
		summary   VulnerabilitySummary
		shouldErr bool
	}{
		{
			name: "critical vulnerability blocks",
			summary: VulnerabilitySummary{
				Critical: 1,
				High:     0,
				Medium:   0,
				Low:      0,
			},
			shouldErr: true,
		},
		{
			name: "high vulnerability allowed",
			summary: VulnerabilitySummary{
				Critical: 0,
				High:     5,
				Medium:   10,
				Low:      20,
			},
			shouldErr: false,
		},
		{
			name: "no vulnerabilities allowed",
			summary: VulnerabilitySummary{
				Critical: 0,
				High:     0,
				Medium:   0,
				Low:      0,
			},
			shouldErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			result := &ScanResult{Summary: tt.summary}
			err := enforcer.Enforce(result)
			if tt.shouldErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestPolicyEnforcer_Enforce_High(t *testing.T) {
	RegisterTestingT(t)

	config := &Config{
		FailOn: SeverityHigh,
	}
	enforcer := NewPolicyEnforcer(config)

	tests := []struct {
		name      string
		summary   VulnerabilitySummary
		shouldErr bool
	}{
		{
			name: "critical vulnerability blocks",
			summary: VulnerabilitySummary{
				Critical: 1,
				High:     0,
				Medium:   0,
				Low:      0,
			},
			shouldErr: true,
		},
		{
			name: "high vulnerability blocks",
			summary: VulnerabilitySummary{
				Critical: 0,
				High:     1,
				Medium:   0,
				Low:      0,
			},
			shouldErr: true,
		},
		{
			name: "medium vulnerability allowed",
			summary: VulnerabilitySummary{
				Critical: 0,
				High:     0,
				Medium:   10,
				Low:      20,
			},
			shouldErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			result := &ScanResult{Summary: tt.summary}
			err := enforcer.Enforce(result)
			if tt.shouldErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestPolicyEnforcer_Enforce_Medium(t *testing.T) {
	RegisterTestingT(t)

	config := &Config{
		FailOn: SeverityMedium,
	}
	enforcer := NewPolicyEnforcer(config)

	tests := []struct {
		name      string
		summary   VulnerabilitySummary
		shouldErr bool
	}{
		{
			name: "medium vulnerability blocks",
			summary: VulnerabilitySummary{
				Critical: 0,
				High:     0,
				Medium:   1,
				Low:      0,
			},
			shouldErr: true,
		},
		{
			name: "low vulnerability allowed",
			summary: VulnerabilitySummary{
				Critical: 0,
				High:     0,
				Medium:   0,
				Low:      10,
			},
			shouldErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			result := &ScanResult{Summary: tt.summary}
			err := enforcer.Enforce(result)
			if tt.shouldErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestPolicyEnforcer_Enforce_Low(t *testing.T) {
	RegisterTestingT(t)

	config := &Config{
		FailOn: SeverityLow,
	}
	enforcer := NewPolicyEnforcer(config)

	tests := []struct {
		name      string
		summary   VulnerabilitySummary
		shouldErr bool
	}{
		{
			name: "low vulnerability blocks",
			summary: VulnerabilitySummary{
				Critical: 0,
				High:     0,
				Medium:   0,
				Low:      1,
			},
			shouldErr: true,
		},
		{
			name: "no vulnerabilities allowed",
			summary: VulnerabilitySummary{
				Critical: 0,
				High:     0,
				Medium:   0,
				Low:      0,
			},
			shouldErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			result := &ScanResult{Summary: tt.summary}
			err := enforcer.Enforce(result)
			if tt.shouldErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestPolicyEnforcer_ShouldBlock(t *testing.T) {
	RegisterTestingT(t)

	config := &Config{
		FailOn: SeverityCritical,
	}
	enforcer := NewPolicyEnforcer(config)

	tests := []struct {
		name        string
		summary     VulnerabilitySummary
		shouldBlock bool
	}{
		{
			name: "critical blocks",
			summary: VulnerabilitySummary{
				Critical: 1,
			},
			shouldBlock: true,
		},
		{
			name: "high allowed",
			summary: VulnerabilitySummary{
				High: 5,
			},
			shouldBlock: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			result := &ScanResult{Summary: tt.summary}
			Expect(enforcer.ShouldBlock(result)).To(Equal(tt.shouldBlock))
		})
	}
}

func TestPolicyEnforcer_UnknownSeverity(t *testing.T) {
	RegisterTestingT(t)

	cfg := &Config{FailOn: "bogus"}
	enforcer := NewPolicyEnforcer(cfg)

	t.Run("blocks when any vulnerability found", func(t *testing.T) {
		RegisterTestingT(t)
		result := &ScanResult{
			Summary: VulnerabilitySummary{Total: 1, Low: 1},
		}
		err := enforcer.Enforce(result)
		Expect(err).To(HaveOccurred())
		var pve *PolicyViolationError
		Expect(isPolicyViolation(err, &pve)).To(BeTrue())
	})

	t.Run("passes when no vulnerabilities", func(t *testing.T) {
		RegisterTestingT(t)
		result := &ScanResult{
			Summary: VulnerabilitySummary{},
		}
		Expect(enforcer.Enforce(result)).ToNot(HaveOccurred())
	})
}

func TestPolicyViolationError_IsDistinctType(t *testing.T) {
	RegisterTestingT(t)

	cfg := &Config{FailOn: SeverityCritical}
	enforcer := NewPolicyEnforcer(cfg)

	result := &ScanResult{
		Summary: VulnerabilitySummary{Critical: 1},
	}
	err := enforcer.Enforce(result)
	Expect(err).To(HaveOccurred())

	var pve *PolicyViolationError
	Expect(isPolicyViolation(err, &pve)).To(BeTrue())
	Expect(pve.Message).ToNot(BeEmpty())
	Expect(pve.Error()).To(Equal(pve.Message))
}

// isPolicyViolation is a helper that mirrors errors.As without importing errors in this package.
func isPolicyViolation(err error, target **PolicyViolationError) bool {
	pve, ok := err.(*PolicyViolationError)
	if ok && target != nil {
		*target = pve
	}
	return ok
}

func TestPolicyEnforcer_Enforce_NilResult(t *testing.T) {
	RegisterTestingT(t)

	config := &Config{
		FailOn: SeverityCritical,
	}
	enforcer := NewPolicyEnforcer(config)

	Expect(enforcer.Enforce(nil)).ToNot(HaveOccurred())
}

func TestPolicyEnforcer_Enforce_NoFailOn(t *testing.T) {
	RegisterTestingT(t)

	config := &Config{
		FailOn: "",
	}
	enforcer := NewPolicyEnforcer(config)

	result := &ScanResult{
		Summary: VulnerabilitySummary{
			Critical: 10,
			High:     20,
		},
	}

	Expect(enforcer.Enforce(result)).ToNot(HaveOccurred())
}
