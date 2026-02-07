package api

import (
	"strings"

	"github.com/pkg/errors"
)

// ValidateSecretReferences validates that all secret references in the configuration
// can be resolved from the available secrets
func ValidateSecretReferences(config *EnvironmentSecretsConfig, availableSecrets map[string]string) error {
	if config == nil {
		return nil
	}

	// Validate configuration structure first
	if err := ValidateSecretConfig(config); err != nil {
		return err
	}

	// Validate each secret reference
	for key, ref := range config.Secrets {
		refStr := string(ref)

		// Direct reference (~) - check if key exists
		if ref == DirectSecretReference {
			if _, found := availableSecrets[key]; !found {
				return errors.Errorf("secret %q (direct reference) not found in available secrets", key)
			}
			continue
		}

		// Mapped reference (${secret:KEY}) - check if mapped key exists
		if strings.HasPrefix(refStr, "${secret:") && strings.HasSuffix(refStr, "}") {
			mappedKey := strings.TrimPrefix(refStr, "${secret:")
			mappedKey = strings.TrimSuffix(mappedKey, "}")
			if _, found := availableSecrets[mappedKey]; !found {
				return errors.Errorf("mapped secret %q (referenced from %q) not found in available secrets", mappedKey, key)
			}
			continue
		}

		// Literal value - no validation needed
	}

	return nil
}

// ValidateSecretAccess validates that a client stack can access a specific secret
// based on the parent stack's secrets configuration
func ValidateSecretAccess(stack Stack, secretKey string, params StackParams) error {
	// Get the parent stack's server configuration
	if stack.Server.Secrets.SecretsConfig == nil {
		// No environment-specific config, all secrets are accessible
		return nil
	}

	// Check if the secret is accessible based on the mode
	config := stack.Server.Secrets.SecretsConfig
	accessibleSecrets, err := NewSecretResolver(config, stack.Secrets.Values).Resolve()
	if err != nil {
		return errors.Wrapf(err, "failed to resolve secrets for validation")
	}

	if _, found := accessibleSecrets[secretKey]; !found {
		// Secret is not accessible
		return errors.Errorf("secret %q is not accessible in environment %q (mode: %s)", secretKey, params.Environment, config.Mode)
	}

	return nil
}
