package scan

import (
	"testing"
)

func TestPolicyEnforcer_Enforce_Critical(t *testing.T) {
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
			result := &ScanResult{
				Summary: tt.summary,
			}
			err := enforcer.Enforce(result)
			if (err != nil) != tt.shouldErr {
				t.Errorf("Enforce() error = %v, shouldErr = %v", err, tt.shouldErr)
			}
		})
	}
}

func TestPolicyEnforcer_Enforce_High(t *testing.T) {
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
			result := &ScanResult{
				Summary: tt.summary,
			}
			err := enforcer.Enforce(result)
			if (err != nil) != tt.shouldErr {
				t.Errorf("Enforce() error = %v, shouldErr = %v", err, tt.shouldErr)
			}
		})
	}
}

func TestPolicyEnforcer_Enforce_Medium(t *testing.T) {
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
			result := &ScanResult{
				Summary: tt.summary,
			}
			err := enforcer.Enforce(result)
			if (err != nil) != tt.shouldErr {
				t.Errorf("Enforce() error = %v, shouldErr = %v", err, tt.shouldErr)
			}
		})
	}
}

func TestPolicyEnforcer_Enforce_Low(t *testing.T) {
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
			result := &ScanResult{
				Summary: tt.summary,
			}
			err := enforcer.Enforce(result)
			if (err != nil) != tt.shouldErr {
				t.Errorf("Enforce() error = %v, shouldErr = %v", err, tt.shouldErr)
			}
		})
	}
}

func TestPolicyEnforcer_ShouldBlock(t *testing.T) {
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
			result := &ScanResult{
				Summary: tt.summary,
			}
			blocked := enforcer.ShouldBlock(result)
			if blocked != tt.shouldBlock {
				t.Errorf("ShouldBlock() = %v, want %v", blocked, tt.shouldBlock)
			}
		})
	}
}

func TestPolicyEnforcer_Enforce_NilResult(t *testing.T) {
	config := &Config{
		FailOn: SeverityCritical,
	}
	enforcer := NewPolicyEnforcer(config)

	err := enforcer.Enforce(nil)
	if err != nil {
		t.Errorf("Enforce(nil) should not error, got: %v", err)
	}
}

func TestPolicyEnforcer_Enforce_NoFailOn(t *testing.T) {
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

	err := enforcer.Enforce(result)
	if err != nil {
		t.Errorf("Enforce() with no failOn should not error, got: %v", err)
	}
}
