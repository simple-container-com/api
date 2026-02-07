package api

import (
	"regexp"
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

// SecretResolver handles environment-specific secret resolution
type SecretResolver struct {
	baseSecrets *SecretsDescriptor
	config      *EnvironmentSecretsConfigDescriptor
}

// NewSecretResolver creates a new SecretResolver for the given base secrets and environment config
func NewSecretResolver(baseSecrets *SecretsDescriptor, config *EnvironmentSecretsConfigDescriptor) (*SecretResolver, error) {
	if baseSecrets == nil {
		return nil, errors.Errorf("base secrets cannot be nil")
	}
	return &SecretResolver{
		baseSecrets: baseSecrets,
		config:      config,
	}, nil
}

// Resolve returns the filtered/overridden secrets map based on the environment configuration
func (r *SecretResolver) Resolve() (map[string]string, error) {
	if r.config == nil {
		// No environment-specific config, return all base secrets
		return r.baseSecrets.Values, nil
	}

	switch r.config.Mode {
	case "include":
		return r.resolveInclude()
	case "exclude":
		return r.resolveExclude()
	case "override":
		return r.resolveOverride()
	default:
		return nil, errors.Errorf("unknown secretsConfig mode: %q (must be 'include', 'exclude', or 'override')", r.config.Mode)
	}
}

// resolveInclude implements include mode - only specified secrets are available
func (r *SecretResolver) resolveInclude() (map[string]string, error) {
	result := make(map[string]string)

	for refName, value := range r.config.Secrets {
		resolved, err := r.resolveSingleSecret(refName, value)
		if err != nil {
			return nil, err
		}
		result[refName] = resolved
	}

	return result, nil
}

// resolveExclude implements exclude mode - all secrets except excluded ones are available
func (r *SecretResolver) resolveExclude() (map[string]string, error) {
	if !r.config.InheritAll {
		return nil, errors.Errorf("exclude mode requires inheritAll: true to be set")
	}

	result := make(map[string]string)

	// Start with all base secrets
	for k, v := range r.baseSecrets.Values {
		result[k] = v
	}

	// Remove excluded secrets
	for refName := range r.config.Secrets {
		delete(result, refName)
	}

	return result, nil
}

// resolveOverride implements override mode - all secrets available with overrides applied
func (r *SecretResolver) resolveOverride() (map[string]string, error) {
	result := make(map[string]string)

	// Start with all base secrets
	for k, v := range r.baseSecrets.Values {
		result[k] = v
	}

	// Apply overrides from config
	for refName, value := range r.config.Secrets {
		resolved, err := r.resolveSingleSecret(refName, value)
		if err != nil {
			return nil, err
		}
		result[refName] = resolved
	}

	return result, nil
}

// resolveSingleSecret resolves a single secret reference
func (r *SecretResolver) resolveSingleSecret(refName, value string) (string, error) {
	// Check if it's a literal reference (starts with ${secret:})
	if strings.HasPrefix(value, "${secret:") && strings.HasSuffix(value, "}") {
		// Extract the actual secret key from ${secret:KEY}
		secretRef := strings.TrimPrefix(value, "${secret:")
		secretRef = strings.TrimSuffix(secretRef, "}")

		// Look up the secret from base secrets
		if actualValue, found := r.baseSecrets.Values[secretRef]; found {
			return actualValue, nil
		}
		return "", errors.Errorf("secret reference %q not found in base secrets (referenced by %q)", secretRef, refName)
	}

	// Check if value is an indirect reference to another secret key
	if actualValue, found := r.baseSecrets.Values[value]; found {
		return actualValue, nil
	}

	// Value itself is the literal secret value (or use the value as key if not found)
	if actualValue, found := r.baseSecrets.Values[refName]; found {
		return actualValue, nil
	}

	return "", errors.Errorf("secret %q not found in base secrets", refName)
}

// ValidateSecretReference validates a secret reference string
func ValidateSecretReference(ref string) error {
	// Pattern matches: ${secret:KEY} where KEY is alphanumeric with underscore and dash
	pattern := `^\$\{secret:[a-zA-Z0-9_-]+\}$`
	matched, err := regexp.MatchString(pattern, ref)
	if err != nil {
		return errors.Wrapf(err, "failed to validate secret reference %q", ref)
	}
	if !matched {
		return errors.Errorf("invalid secret reference format %q (expected format: ${secret:KEY})", ref)
	}
	return nil
}

// IsSecretReference checks if a string is a secret reference
func IsSecretReference(value string) bool {
	return strings.HasPrefix(value, "${secret:") && strings.HasSuffix(value, "}")
}
