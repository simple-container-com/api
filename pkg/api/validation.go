package api

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// ValidateSecretsConfigInStacks validates that all secret references in client stacks
// are available based on the parent stack's secretsConfig (if configured)
func ValidateSecretsConfigInStacks(stacks *StacksMap, params StackParams) error {
	if stacks == nil {
		return errors.Errorf("stacks map is nil")
	}

	for stackName, stack := range *stacks {
		if len(stack.Client.Stacks) == 0 {
			continue
		}

		clientDesc, ok := stack.Client.Stacks[params.Environment]
		if !ok && stackName != params.StackName {
			continue
		}
		if !ok {
			return errors.Errorf("client stack %q is not configured for %q", stackName, params.Environment)
		}

		// Get parent stack info
		if clientDesc.ParentStack == "" {
			continue
		}

		parentStackParts := strings.SplitN(clientDesc.ParentStack, "/", 3)
		parentStackName := parentStackParts[len(parentStackParts)-1]
		parentStack, ok := (*stacks)[parentStackName]
		if !ok {
			continue
		}

		// Skip validation if no secretsConfig is set
		if parentStack.Server.Secrets.SecretsConfig == nil {
			continue
		}

		// Validate secret references in client configuration
		if err := validateSecretReferencesInClientConfig(stackName, stack, parentStack, params); err != nil {
			return err
		}
	}

	return nil
}

// validateSecretReferencesInClientConfig validates that all secret references in the client config
// are available based on the parent's secretsConfig
func validateSecretReferencesInClientConfig(stackName string, stack, parentStack Stack, params StackParams) error {
	// Get the resolved secrets that would be available
	resolver, err := NewSecretResolver(&parentStack.Secrets, parentStack.Server.Secrets.SecretsConfig)
	if err != nil {
		return errors.Wrapf(err, "failed to create secret resolver for parent stack %q", params.ParentEnv)
	}

	availableSecrets, err := resolver.Resolve()
	if err != nil {
		return errors.Wrapf(err, "failed to resolve secrets for parent stack %q", params.ParentEnv)
	}

	clientDesc := stack.Client.Stacks[params.Environment]

	// Collect all secrets referenced in client config
	referencedSecrets := collectReferencedSecrets(clientDesc)

	// Validate that all referenced secrets are available
	for _, secretRef := range referencedSecrets {
		if _, found := availableSecrets[secretRef]; !found {
			mode := parentStack.Server.Secrets.SecretsConfig.Mode
			return errors.Errorf("secret %q referenced in client stack %q is not available in parent stack %q (mode: %q). "+
				"Either add the secret to secretsConfig or check it exists in secrets.yaml",
				secretRef, stackName, params.ParentEnv, mode)
		}
	}

	return nil
}

// collectReferencedSecrets collects all secret references from a client configuration
func collectReferencedSecrets(clientDesc StackClientDescriptor) []string {
	secrets := make(map[string]bool)

	// Collect from compose config
	if composeCfg, ok := clientDesc.Config.Config.(*StackConfigCompose); ok && composeCfg != nil {
		for secretName := range composeCfg.Secrets {
			secrets[secretName] = true
		}
	}

	// Collect from single image config
	if singleCfg, ok := clientDesc.Config.Config.(*StackConfigSingleImage); ok && singleCfg != nil {
		for secretName := range singleCfg.Secrets {
			secrets[secretName] = true
		}
	}

	// Return sorted list for consistent error messages
	return lo.Keys(secrets)
}

// ValidateSecretReferenceFormat validates a single secret reference string format
func ValidateSecretReferenceFormat(ref string) error {
	if ref == "" {
		return errors.Errorf("secret reference cannot be empty")
	}

	// Check if it's a ${secret:KEY} reference
	if IsSecretReference(ref) {
		return ValidateSecretReference(ref)
	}

	// Direct key reference is also valid
	return nil
}

// FormatSecretsConfigError formats a validation error message for secretsConfig issues
func FormatSecretsConfigError(stackName, parentEnv, secretName, mode string) string {
	return fmt.Sprintf("secret %q in stack %q is not available in parent environment %q (mode: %s). "+
		"Please add the secret to the parent stack's secretsConfig section or verify it exists in secrets.yaml",
		secretName, stackName, parentEnv, mode)
}

// GetAvailableSecrets returns the list of available secret names based on secretsConfig
func GetAvailableSecrets(secrets *SecretsDescriptor, config *EnvironmentSecretsConfigDescriptor) ([]string, error) {
	if secrets == nil {
		return nil, errors.Errorf("secrets descriptor is nil")
	}

	if config == nil {
		// No filtering, return all secrets
		return lo.Keys(secrets.Values), nil
	}

	resolver, err := NewSecretResolver(secrets, config)
	if err != nil {
		return nil, err
	}

	resolved, err := resolver.Resolve()
	if err != nil {
		return nil, err
	}

	return lo.Keys(resolved), nil
}
