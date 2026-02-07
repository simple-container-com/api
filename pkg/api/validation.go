package api

import (
	"regexp"

	"github.com/pkg/errors"
	"github.com/samber/lo"
)

var (
	// secretRefPattern matches ${secret:KEY} references
	secretRefPattern = regexp.MustCompile(`\$\{secret:([^}]+)\}`)
)

// ValidateSecretReferences validates that all secret references in a client configuration
// are valid and reference available secrets
func ValidateSecretReferences(clientConfig *StackConfigCompose, availableSecrets []string) []error {
	var errs []error

	// Check secrets referenced in the config
	for secretName := range clientConfig.Secrets {
		if !lo.Contains(availableSecrets, secretName) {
			errs = append(errs, errors.Errorf("secret %q is not available in the current environment", secretName))
		}
	}

	return errs
}

// ValidateSecretAccess validates that all secrets referenced in a client config
// will be available after applying the parent stack's secretsConfig
func ValidateSecretAccess(descriptor *ServerDescriptor, clientConfig *StackConfigCompose, env string) []error {
	var errs []error

	// If no secretsConfig is defined, all secrets are available (backwards compatibility)
	if descriptor.Secrets.SecretsConfig == nil {
		return nil
	}

	// We need to check against all potential secrets (from the parent stack's secrets descriptor)
	// Since we don't have the actual values here, we'll validate the configuration structure
	envConfig, ok := descriptor.Secrets.SecretsConfig.Secrets[env]
	if !ok {
		// No config for this environment - in include/override mode this means no secrets available
		if descriptor.Secrets.SecretsConfig.Mode == "include" || descriptor.Secrets.SecretsConfig.Mode == "override" {
			for secretName := range clientConfig.Secrets {
				errs = append(errs, errors.Errorf("environment %q is not configured in secretsConfig, secret %q will not be available", env, secretName))
			}
			return errs
		}
		// For exclude mode, if no config, all secrets are available
		return nil
	}

	// Check based on mode
	switch descriptor.Secrets.SecretsConfig.Mode {
	case "include":
		// Only secrets in the Include list are available
		for secretName := range clientConfig.Secrets {
			found := false
			for _, ref := range envConfig.Include {
				key := extractKeyFromRef(ref)
				if key == secretName {
					found = true
					break
				}
			}
			if !found {
				errs = append(errs, errors.Errorf("secret %q is not in the include list for environment %q", secretName, env))
			}
		}
	case "exclude":
		// Check if any referenced secret is in the exclude list
		for secretName := range clientConfig.Secrets {
			for _, excRef := range envConfig.Exclude {
				excKey := extractKeyFromRef(excRef)
				if excKey == secretName {
					errs = append(errs, errors.Errorf("secret %q is excluded for environment %q", secretName, env))
				}
			}
		}
	case "override":
		// Only secrets in the Override map are available
		for secretName := range clientConfig.Secrets {
			if _, ok := envConfig.Override[secretName]; !ok {
				errs = append(errs, errors.Errorf("secret %q is not in the override list for environment %q", secretName, env))
			}
		}
	}

	return errs
}

// extractKeyFromRef extracts the secret key from a reference string
func extractKeyFromRef(ref string) string {
	// Check for ${secret:KEY} pattern
	if matches := secretRefPattern.FindStringSubmatch(ref); matches != nil {
		return matches[1]
	}
	// Check for ~KEY pattern
	if len(ref) > 0 && ref[0] == '~' {
		return ref[1:]
	}
	return ref
}

// ValidateSecretConfigReferences validates that all secret references in a secretsConfig
// are valid (e.g., ${secret:KEY} references point to existing secrets)
func ValidateSecretConfigValues(config *EnvironmentSecretsConfig, allSecrets map[string]string) []error {
	var errs []error

	if config == nil {
		return nil
	}

	for envName, envConfig := range config.Secrets {
		// Validate override references
		for key, refOrValue := range envConfig.Override {
			if matches := secretRefPattern.FindStringSubmatch(refOrValue); matches != nil {
				// This is a ${secret:KEY} reference, validate the referenced key exists
				referencedKey := matches[1]
				if _, ok := allSecrets[referencedKey]; !ok {
					errs = append(errs, errors.Errorf("in environment %q, override for secret %q references non-existent secret %q", envName, key, referencedKey))
				}
			}
		}

		// Validate include/exclude references
		for _, ref := range envConfig.Include {
			if matches := secretRefPattern.FindStringSubmatch(ref); matches != nil {
				referencedKey := matches[1]
				if _, ok := allSecrets[referencedKey]; !ok {
					errs = append(errs, errors.Errorf("in environment %q, include list references non-existent secret %q", envName, referencedKey))
				}
			}
		}

		for _, ref := range envConfig.Exclude {
			if matches := secretRefPattern.FindStringSubmatch(ref); matches != nil {
				referencedKey := matches[1]
				if _, ok := allSecrets[referencedKey]; !ok {
					errs = append(errs, errors.Errorf("in environment %q, exclude list references non-existent secret %q", envName, referencedKey))
				}
			}
		}
	}

	return errs
}
