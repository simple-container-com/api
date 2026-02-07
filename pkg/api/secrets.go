package api

import (
	"strings"

	"github.com/pkg/errors"
)

const SecretsSchemaVersion = "1.0"

// SecretsDescriptor describes the secrets schema
type SecretsDescriptor struct {
	SchemaVersion string                    `json:"schemaVersion" yaml:"schemaVersion"`
	Auth          map[string]AuthDescriptor `json:"auth" yaml:"auth"`
	Values        map[string]string         `json:"values" yaml:"values"`
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

// SecretResolver resolves secret references based on environment-specific configuration
type SecretResolver struct {
	config     *EnvironmentSecretsConfig
	allSecrets map[string]string
}

// NewSecretResolver creates a new secret resolver
func NewSecretResolver(config *EnvironmentSecretsConfig, allSecrets map[string]string) *SecretResolver {
	return &SecretResolver{
		config:     config,
		allSecrets: allSecrets,
	}
}

// Resolve returns the filtered and resolved secrets map based on the configuration
func (r *SecretResolver) Resolve() (map[string]string, error) {
	if r.config == nil {
		// No environment-specific config, return all secrets as-is
		return r.allSecrets, nil
	}

	result := make(map[string]string)

	switch r.config.Mode {
	case SecretsConfigModeInclude:
		// Only include specified secrets
		for key, ref := range r.config.Secrets {
			value, err := r.resolveSecretReference(key, ref)
			if err != nil {
				return nil, err
			}
			result[key] = value
		}

	case SecretsConfigModeExclude:
		// Include all secrets except those specified
		if r.config.InheritAll {
			// Start with all secrets
			for k, v := range r.allSecrets {
				result[k] = v
			}
			// Remove excluded secrets
			for key := range r.config.Secrets {
				delete(result, key)
			}
		} else {
			// If inheritAll is false, exclude mode without inheritAll is invalid
			return nil, errors.Errorf("exclude mode requires inheritAll to be true")
		}

	case SecretsConfigModeOverride:
		// Start with all secrets if inheritAll is true
		if r.config.InheritAll {
			for k, v := range r.allSecrets {
				result[k] = v
			}
		}
		// Override/add specified secrets
		for key, ref := range r.config.Secrets {
			value, err := r.resolveSecretReference(key, ref)
			if err != nil {
				return nil, err
			}
			result[key] = value
		}

	default:
		return nil, errors.Errorf("unknown secrets config mode: %q", r.config.Mode)
	}

	return result, nil
}

// resolveSecretReference resolves a single secret reference
func (r *SecretResolver) resolveSecretReference(key string, ref SecretReference) (string, error) {
	refStr := string(ref)

	// Pattern 1: Direct reference (~) - use secret with same name from secrets.yaml
	if ref == DirectSecretReference {
		value, found := r.allSecrets[key]
		if !found {
			return "", errors.Errorf("secret %q not found in secrets.yaml", key)
		}
		return value, nil
	}

	// Pattern 2: Mapped reference (${secret:KEY}) - use secret named KEY from secrets.yaml
	if strings.HasPrefix(refStr, "${secret:") && strings.HasSuffix(refStr, "}") {
		mappedKey := strings.TrimPrefix(refStr, "${secret:")
		mappedKey = strings.TrimSuffix(mappedKey, "}")
		value, found := r.allSecrets[mappedKey]
		if !found {
			return "", errors.Errorf("mapped secret %q not found in secrets.yaml", mappedKey)
		}
		return value, nil
	}

	// Pattern 3: Literal value - use the literal value directly
	return refStr, nil
}

// ValidateSecretConfig validates the environment secrets configuration
func ValidateSecretConfig(config *EnvironmentSecretsConfig) error {
	if config == nil {
		return nil
	}

	// Validate mode
	switch config.Mode {
	case SecretsConfigModeInclude, SecretsConfigModeExclude, SecretsConfigModeOverride:
		// Valid modes
	default:
		return errors.Errorf("invalid secrets config mode %q, must be one of: include, exclude, override", config.Mode)
	}

	// Validate that exclude mode has inheritAll
	if config.Mode == SecretsConfigModeExclude && !config.InheritAll {
		return errors.Errorf("exclude mode requires inheritAll to be true")
	}

	return nil
}
