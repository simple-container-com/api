package api

import "github.com/pkg/errors"

const SecretsSchemaVersion = "2.0"

// SecretsDescriptor describes the secrets schema
type SecretsDescriptor struct {
	SchemaVersion string                         `json:"schemaVersion" yaml:"schemaVersion"`
	Auth          map[string]AuthDescriptor      `json:"auth" yaml:"auth"`
	Values        map[string]string              `json:"values" yaml:"values"` // Shared values for all environments (backward compatibility)
	Environments  map[string]EnvironmentSecrets  `json:"environments" yaml:"environments"` // Environment-specific secrets (v2.0)
}

// EnvironmentSecrets contains environment-specific secret values
type EnvironmentSecrets struct {
	Values map[string]string `json:"values" yaml:"values"`
}

// GetSecretValue retrieves a secret value considering environment context
// It looks up secrets in the following order:
// 1. Environment-specific value (if environment is provided)
// 2. Shared value (fallback for backward compatibility)
func (s *SecretsDescriptor) GetSecretValue(secretName, environment string) (string, bool) {
	// First try environment-specific value if environment is provided
	if environment != "" {
		if envSecrets, ok := s.Environments[environment]; ok {
			if value, ok := envSecrets.Values[secretName]; ok {
				return value, true
			}
		}
	}

	// Fall back to shared values (backward compatibility)
	if value, ok := s.Values[secretName]; ok {
		return value, true
	}

	return "", false
}

// HasEnvironment checks if an environment configuration exists
func (s *SecretsDescriptor) HasEnvironment(environment string) bool {
	if s.Environments == nil {
		return false
	}
	_, exists := s.Environments[environment]
	return exists
}

// GetEnvironments returns a list of all configured environments
func (s *SecretsDescriptor) GetEnvironments() []string {
	var environments []string
	for envName := range s.Environments {
		environments = append(environments, envName)
	}
	return environments
}

// IsV2Schema returns true if this descriptor uses v2.0 schema features
func (s *SecretsDescriptor) IsV2Schema() bool {
	return len(s.Environments) > 0
}

type AuthDescriptor struct {
	Type    string `json:"type" yaml:"type"`
	Config  `json:",inline" yaml:",inline"`
	Inherit `json:",inline" yaml:",inline"`
}

func (a *AuthDescriptor) AuthConfig() (AuthConfig, error) {
	c, ok := a.Config.Config.(AuthConfig)
	if !ok {
		return nil, errors.Errorf("auth config %q does not implement AuthConfig", a)
	}
	return c, nil
}
