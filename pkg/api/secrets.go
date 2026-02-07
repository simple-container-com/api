package api

import (
	"strings"

	"github.com/pkg/errors"
)

const SecretsSchemaVersion = "1.0"
const secretReferencePrefix = "${secret:"

// SecretResolver handles filtering and resolution of environment-specific secrets
type SecretResolver struct {
	baseSecrets map[string]string
	config      *EnvironmentSecretsConfigDescriptor
}

// NewSecretResolver creates a new SecretResolver
func NewSecretResolver(baseSecrets map[string]string, config *EnvironmentSecretsConfigDescriptor) *SecretResolver {
	return &SecretResolver{
		baseSecrets: baseSecrets,
		config:      config,
	}
}

// Resolve returns the filtered secrets based on the configured mode
// Supported modes:
// - "include": only secrets specified in the config are available
// - "exclude": all secrets except those specified in the config are available (requires inheritAll)
// - "override": all secrets are available with specified overrides applied
func (sr *SecretResolver) Resolve() (map[string]string, error) {
	if sr.config == nil {
		return sr.baseSecrets, nil
	}

	switch sr.config.Mode {
	case "include":
		return sr.resolveInclude()
	case "exclude":
		return sr.resolveExclude()
	case "override":
		return sr.resolveOverride()
	default:
		return nil, errors.Errorf("invalid secrets mode %q, must be one of: include, exclude, override", sr.config.Mode)
	}
}

// resolveInclude returns only the secrets specified in the config
func (sr *SecretResolver) resolveInclude() (map[string]string, error) {
	result := make(map[string]string)
	for localName, sourceRef := range sr.config.Secrets {
		value, err := sr.resolveValue(sourceRef)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to resolve secret %q", localName)
		}
		result[localName] = value
	}
	return result, nil
}

// resolveExclude returns all secrets except those specified in the config
func (sr *SecretResolver) resolveExclude() (map[string]string, error) {
	if !sr.config.InheritAll {
		return nil, errors.Errorf("mode 'exclude' requires inheritAll to be set to true")
	}
	result := make(map[string]string)
	excludedSecrets := sr.config.Secrets
	for name, value := range sr.baseSecrets {
		if _, excluded := excludedSecrets[name]; !excluded {
			result[name] = value
		}
	}
	return result, nil
}

// resolveOverride returns all secrets with specified overrides applied
func (sr *SecretResolver) resolveOverride() (map[string]string, error) {
	result := make(map[string]string)
	// Start with all base secrets
	for name, value := range sr.baseSecrets {
		result[name] = value
	}
	// Apply overrides
	for localName, sourceRef := range sr.config.Secrets {
		value, err := sr.resolveValue(sourceRef)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to resolve override for secret %q", localName)
		}
		result[localName] = value
	}
	return result, nil
}

// resolveValue resolves a single secret reference
// Supports three patterns:
// 1. Direct reference: "SECRET_NAME" - looks up SECRET_NAME in baseSecrets
// 2. Secret reference: "${secret:KEY}" - recursively resolves KEY from baseSecrets
// 3. Literal prefix: "literal:value" - uses the literal value as-is (without the prefix)
func (sr *SecretResolver) resolveValue(sourceRef string) (string, error) {
	// Check for secret reference syntax ${secret:KEY}
	if strings.HasPrefix(sourceRef, secretReferencePrefix) && strings.HasSuffix(sourceRef, "}") {
		secretKey := strings.TrimPrefix(sourceRef, secretReferencePrefix)
		secretKey = strings.TrimSuffix(secretKey, "}")
		if value, ok := sr.baseSecrets[secretKey]; ok {
			return value, nil
		}
		return "", errors.Errorf("secret key %q not found in base secrets", secretKey)
	}
	// Check for literal value prefix
	if strings.HasPrefix(sourceRef, "literal:") {
		return strings.TrimPrefix(sourceRef, "literal:"), nil
	}
	// Direct reference - look up the key in base secrets
	if value, ok := sr.baseSecrets[sourceRef]; ok {
		return value, nil
	}
	return "", errors.Errorf("secret reference %q not found in base secrets", sourceRef)
}

// ValidateSecretReference validates that a secret reference string is well-formed
func ValidateSecretReference(ref string) error {
	if !strings.HasPrefix(ref, secretReferencePrefix) {
		return nil
	}
	if !strings.HasSuffix(ref, "}") {
		return errors.Errorf("invalid secret reference %q: missing closing brace", ref)
	}
	secretKey := strings.TrimPrefix(ref, secretReferencePrefix)
	secretKey = strings.TrimSuffix(secretKey, "}")
	if secretKey == "" {
		return errors.Errorf("invalid secret reference %q: empty secret key", ref)
	}
	return nil
}

// IsSecretReference checks if a string is a secret reference using ${secret:KEY} syntax
func IsSecretReference(value string) bool {
	return strings.HasPrefix(value, secretReferencePrefix) && strings.HasSuffix(value, "}")
}

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
