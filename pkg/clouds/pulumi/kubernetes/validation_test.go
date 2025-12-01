package kubernetes

import (
	"strings"
	"testing"

	"github.com/simple-container-com/api/pkg/api"
)

func TestValidateParentEnvConfiguration(t *testing.T) {
	tests := []struct {
		name       string
		stackEnv   string
		parentEnv  string
		descriptor *api.StackClientDescriptor
		wantErr    bool
	}{
		{
			name:       "standard stack - no parentEnv",
			stackEnv:   "staging",
			parentEnv:  "",
			descriptor: &api.StackClientDescriptor{Type: "single-image"},
			wantErr:    false,
		},
		{
			name:       "custom stack - valid parentEnv",
			stackEnv:   "staging-preview",
			parentEnv:  "staging",
			descriptor: &api.StackClientDescriptor{Type: "single-image", ParentEnv: "staging"},
			wantErr:    false,
		},
		{
			name:       "self-reference - treated as standard",
			stackEnv:   "staging",
			parentEnv:  "staging",
			descriptor: &api.StackClientDescriptor{Type: "single-image", ParentEnv: "staging"},
			wantErr:    false,
		},
		{
			name:       "production hotfix",
			stackEnv:   "prod-hotfix",
			parentEnv:  "production",
			descriptor: &api.StackClientDescriptor{Type: "single-image", ParentEnv: "production"},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateParentEnvConfiguration(tt.stackEnv, tt.parentEnv, tt.descriptor)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateParentEnvConfiguration() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateDomainUniqueness(t *testing.T) {
	tests := []struct {
		name            string
		domain          string
		namespace       string
		existingDomains map[string]string
		wantErr         bool
		errorContains   string
	}{
		{
			name:            "no domain specified",
			domain:          "",
			namespace:       "staging",
			existingDomains: map[string]string{},
			wantErr:         false,
		},
		{
			name:            "unique domain",
			domain:          "staging.myapp.com",
			namespace:       "staging",
			existingDomains: map[string]string{},
			wantErr:         false,
		},
		{
			name:      "domain conflict in same namespace",
			domain:    "staging.myapp.com",
			namespace: "staging",
			existingDomains: map[string]string{
				"staging.myapp.com": "myapp-staging",
			},
			wantErr:       true,
			errorContains: "conflicts with existing stack",
		},
		{
			name:      "different domains - no conflict",
			domain:    "preview.staging.myapp.com",
			namespace: "staging",
			existingDomains: map[string]string{
				"staging.myapp.com": "myapp-staging",
			},
			wantErr: false,
		},
		{
			name:      "multiple custom stacks with unique domains",
			domain:    "pr-789.staging.myapp.com",
			namespace: "staging",
			existingDomains: map[string]string{
				"staging.myapp.com":        "myapp-staging",
				"pr-123.staging.myapp.com": "myapp-staging-pr-123",
				"pr-456.staging.myapp.com": "myapp-staging-pr-456",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDomainUniqueness(tt.domain, tt.namespace, tt.existingDomains)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDomainUniqueness() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errorContains != "" {
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("ValidateDomainUniqueness() error = %v, should contain %v", err, tt.errorContains)
				}
			}
		})
	}
}

func TestValidationIntegration(t *testing.T) {
	t.Run("preview environment workflow", func(t *testing.T) {
		// Scenario: Adding preview environments to existing staging
		namespace := "staging"
		existingDomains := map[string]string{
			"staging.myapp.com": "myapp-staging",
		}

		// Add first preview
		preview1Domain := "pr-123.staging.myapp.com"
		err := ValidateDomainUniqueness(preview1Domain, namespace, existingDomains)
		if err != nil {
			t.Errorf("Preview 1 should be valid: %v", err)
		}
		existingDomains[preview1Domain] = "myapp-staging-pr-123"

		// Add second preview
		preview2Domain := "pr-456.staging.myapp.com"
		err = ValidateDomainUniqueness(preview2Domain, namespace, existingDomains)
		if err != nil {
			t.Errorf("Preview 2 should be valid: %v", err)
		}
		existingDomains[preview2Domain] = "myapp-staging-pr-456"

		// Try to add conflicting domain
		err = ValidateDomainUniqueness("pr-123.staging.myapp.com", namespace, existingDomains)
		if err == nil {
			t.Error("Duplicate domain should be rejected")
		}
	})

	t.Run("multi-service preview environments", func(t *testing.T) {
		// Scenario: Multiple services each with preview environments
		namespace := "staging"
		existingDomains := map[string]string{
			"api.staging.myapp.com": "api-staging",
			"web.staging.myapp.com": "web-staging",
		}

		// Each service can have its own preview with unique domain
		previews := []struct {
			domain    string
			stackName string
		}{
			{"api.pr-123.staging.myapp.com", "api-staging-pr-123"},
			{"web.pr-123.staging.myapp.com", "web-staging-pr-123"},
		}

		for _, preview := range previews {
			err := ValidateDomainUniqueness(preview.domain, namespace, existingDomains)
			if err != nil {
				t.Errorf("Preview for %s should be valid: %v", preview.stackName, err)
			}
			existingDomains[preview.domain] = preview.stackName
		}
	})
}

func TestParentEnvEdgeCases(t *testing.T) {
	t.Run("empty parentEnv is standard stack", func(t *testing.T) {
		err := ValidateParentEnvConfiguration("staging", "", nil)
		if err != nil {
			t.Errorf("Empty parentEnv should be valid: %v", err)
		}
	})

	t.Run("self-reference is treated as standard stack", func(t *testing.T) {
		descriptor := &api.StackClientDescriptor{
			Type:      "single-image",
			ParentEnv: "staging",
		}
		err := ValidateParentEnvConfiguration("staging", "staging", descriptor)
		if err != nil {
			t.Errorf("Self-reference should be valid: %v", err)
		}
	})

	t.Run("custom stack with different parent", func(t *testing.T) {
		descriptor := &api.StackClientDescriptor{
			Type:      "single-image",
			ParentEnv: "staging",
		}
		err := ValidateParentEnvConfiguration("staging-preview", "staging", descriptor)
		if err != nil {
			t.Errorf("Custom stack should be valid: %v", err)
		}
	})
}

func TestDomainValidationEdgeCases(t *testing.T) {
	t.Run("nil existing domains map", func(t *testing.T) {
		err := ValidateDomainUniqueness("test.com", "staging", nil)
		if err != nil {
			t.Errorf("Should handle nil domains map: %v", err)
		}
	})

	t.Run("empty domain", func(t *testing.T) {
		existingDomains := map[string]string{"test.com": "stack1"}
		err := ValidateDomainUniqueness("", "staging", existingDomains)
		if err != nil {
			t.Errorf("Empty domain should be valid (no domain routing): %v", err)
		}
	})

	t.Run("whitespace domain", func(t *testing.T) {
		existingDomains := map[string]string{"test.com": "stack1"}
		err := ValidateDomainUniqueness("   ", "staging", existingDomains)
		// Whitespace domain treated as no domain
		if err != nil {
			t.Errorf("Whitespace domain should be treated as empty: %v", err)
		}
	})
}
