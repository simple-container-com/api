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
	config    *EnvironmentSecretsConfig
	secretRef *regexp.Regexp
}

// NewSecretResolver creates a new secret resolver
func NewSecretResolver(config *EnvironmentSecretsConfig) *SecretResolver {
	return &SecretResolver{
		config:    config,
		secretRef: regexp.MustCompile(`^\$\{secret:([^}]+)\}$`),
	}
}

// ResolveSecrets applies environment-specific filtering to secrets
// Returns a filtered map of secret key-value pairs for the specified environment
func (sr *SecretResolver) ResolveSecrets(allSecrets map[string]string, env string) (map[string]string, error) {
	if sr.config == nil {
		// No filtering configured, return all secrets
		return allSecrets, nil
	}

	envConfig, ok := sr.config.Secrets[env]
	if !ok {
		// No config for this environment, return all secrets (backwards compatibility)
		return allSecrets, nil
	}

	switch sr.config.Mode {
	case "include":
		return sr.resolveIncludeMode(allSecrets, envConfig)
	case "exclude":
		return sr.resolveExcludeMode(allSecrets, envConfig)
	case "override":
		return sr.resolveOverrideMode(allSecrets, envConfig)
	default:
		return nil, errors.Errorf("unknown secrets config mode: %q", sr.config.Mode)
	}
}

// resolveIncludeMode resolves secrets in include mode
// Only secrets listed in the Include array are available
func (sr *SecretResolver) resolveIncludeMode(allSecrets map[string]string, envConfig SecretsConfigMap) (map[string]string, error) {
	result := make(map[string]string)

	for _, ref := range envConfig.Include {
		value, err := sr.resolveSecretReference(ref, allSecrets)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to resolve secret reference %q", ref)
		}
		// Extract the key name from the reference
		key := sr.extractKeyFromReference(ref)
		result[key] = value
	}

	return result, nil
}

// resolveExcludeMode resolves secrets in exclude mode
// All secrets are available except those in the Exclude array (when InheritAll is true)
func (sr *SecretResolver) resolveExcludeMode(allSecrets map[string]string, envConfig SecretsConfigMap) (map[string]string, error) {
	result := make(map[string]string)

	if envConfig.InheritAll {
		// Copy all secrets first
		for k, v := range allSecrets {
			result[k] = v
		}
	}

	// Remove excluded secrets
	for _, ref := range envConfig.Exclude {
		key := sr.extractKeyFromReference(ref)
		delete(result, key)
	}

	return result, nil
}

// resolveOverrideMode resolves secrets in override mode
// Secrets can be literal values or references to other secrets
func (sr *SecretResolver) resolveOverrideMode(allSecrets map[string]string, envConfig SecretsConfigMap) (map[string]string, error) {
	result := make(map[string]string)

	for key, refOrValue := range envConfig.Override {
		value, err := sr.resolveSecretReference(refOrValue, allSecrets)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to resolve override for secret %q", key)
		}
		result[key] = value
	}

	return result, nil
}

// resolveSecretReference resolves a secret reference which can be:
// 1. Direct reference: "SECRET_NAME" or "~SECRET_NAME" - fetch directly from allSecrets
// 2. Mapped reference: "${secret:OTHER_SECRET}" - fetch OTHER_SECRET from allSecrets
// 3. Literal value: any other string - use as-is
func (sr *SecretResolver) resolveSecretReference(ref string, allSecrets map[string]string) (string, error) {
	// Check for mapped reference pattern ${secret:KEY}
	if matches := sr.secretRef.FindStringSubmatch(ref); matches != nil {
		// Mapped reference - fetch the referenced secret
		referencedKey := matches[1]
		value, ok := allSecrets[referencedKey]
		if !ok {
			return "", errors.Errorf("referenced secret %q not found", referencedKey)
		}
		return value, nil
	}

	// Check for direct reference with ~ prefix
	if strings.HasPrefix(ref, "~") {
		// Direct reference - remove the ~ and fetch
		key := strings.TrimPrefix(ref, "~")
		value, ok := allSecrets[key]
		if !ok {
			return "", errors.Errorf("secret %q not found", key)
		}
		return value, nil
	}

	// Check if ref exists as a key in allSecrets (plain reference)
	if value, ok := allSecrets[ref]; ok {
		return value, nil
	}

	// Otherwise, treat as literal value
	return ref, nil
}

// extractKeyFromReference extracts the secret key from a reference string
// For "SECRET_NAME" or "~SECRET_NAME" -> "SECRET_NAME"
// For "${secret:OTHER_SECRET}" -> the key that would reference this (not the referenced key)
func (sr *SecretResolver) extractKeyFromReference(ref string) string {
	// If it's a mapped reference, we need to determine what key this would be stored as
	// For simplicity, we use the reference string itself as the key
	if matches := sr.secretRef.FindStringSubmatch(ref); matches != nil {
		// This is a mapped reference - the key depends on context
		// For include mode, the key is the reference itself
		return ref
	}

	// For ~ prefix, remove it
	if strings.HasPrefix(ref, "~") {
		return strings.TrimPrefix(ref, "~")
	}

	return ref
}

// GetAvailableSecrets returns the list of secret keys that will be available for the given environment
func (sr *SecretResolver) GetAvailableSecrets(allSecrets map[string]string, env string) ([]string, error) {
	if sr.config == nil {
		// No filtering, all secrets are available
		keys := make([]string, 0, len(allSecrets))
		for k := range allSecrets {
			keys = append(keys, k)
		}
		return keys, nil
	}

	envConfig, ok := sr.config.Secrets[env]
	if !ok {
		// No config for this environment, all secrets are available
		keys := make([]string, 0, len(allSecrets))
		for k := range allSecrets {
			keys = append(keys, k)
		}
		return keys, nil
	}

	switch sr.config.Mode {
	case "include":
		result := make([]string, 0, len(envConfig.Include))
		for _, ref := range envConfig.Include {
			key := sr.extractKeyFromReference(ref)
			result = append(result, key)
		}
		return result, nil
	case "exclude":
		if envConfig.InheritAll {
			result := make([]string, 0, len(allSecrets))
			for k := range allSecrets {
				excluded := false
				for _, excRef := range envConfig.Exclude {
					excKey := sr.extractKeyFromReference(excRef)
					if k == excKey {
						excluded = true
						break
					}
				}
				if !excluded {
					result = append(result, k)
				}
			}
			return result, nil
		}
		// If not inheriting all, only include non-excluded secrets
		return []string{}, nil
	case "override":
		result := make([]string, 0, len(envConfig.Override))
		for k := range envConfig.Override {
			result = append(result, k)
		}
		return result, nil
	default:
		return nil, errors.Errorf("unknown secrets config mode: %q", sr.config.Mode)
	}
}
