package api

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// ValidateSecretsConfigInStacks validates that all secret references in client stacks
// are available based on the parent stack's secrets configuration
func ValidateSecretsConfigInStacks(stacks *StacksMap) error {
	for stackName, stack := range *stacks {
		// Validate each environment's client configuration
		for envName, clientConfig := range stack.Client.Stacks {
			if err := validateSecretReferencesInClientConfig(stackName, envName, clientConfig, stacks); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateSecretReferencesInClientConfig validates secret references in a client stack configuration
func validateSecretReferencesInClientConfig(stackName, envName string, clientConfig StackClientDescriptor, stacks *StacksMap) error {
	// Get the parent stack
	if clientConfig.ParentStack == "" {
		return nil
	}
	parentStackParts := strings.SplitN(clientConfig.ParentStack, "/", 3)
	parentStackName := parentStackParts[len(parentStackParts)-1]
	parentStack, ok := (*stacks)[parentStackName]
	if !ok {
		return errors.Errorf("parent stack %q not found", parentStackName)
	}

	// Determine the parent environment (use ParentEnv if specified, otherwise use the deployment env)
	parentEnv := clientConfig.ParentEnv
	if parentEnv == "" {
		parentEnv = envName
	}

	// Collect all secrets referenced in the client config
	referencedSecrets := collectReferencedSecrets(clientConfig)

	// Get available secrets based on the parent's secrets configuration
	availableSecrets := GetAvailableSecrets(parentStack, parentEnv)

	// Validate that all referenced secrets are available
	for secretName := range referencedSecrets {
		if _, available := availableSecrets[secretName]; !available {
			return errors.Errorf("secret %q referenced in stack %q (env %q) is not available in parent stack %q (parent env %q). Available secrets: %v",
				secretName, stackName, envName, parentStackName, parentEnv, lo.Keys(availableSecrets))
		}
	}

	return nil
}

// collectReferencedSecrets collects all secret references from a client stack configuration
func collectReferencedSecrets(clientConfig StackClientDescriptor) map[string]bool {
	referenced := make(map[string]bool)

	// Collect from Secrets map in StackConfigCompose if that's the config type
	if configCompose, ok := clientConfig.Config.Config.(*StackConfigCompose); ok {
		for secretName := range configCompose.Secrets {
			referenced[secretName] = true
		}
	}

	return referenced
}

// GetAvailableSecrets returns the secrets that are available for a given parent environment
// based on the parent stack's secrets configuration
func GetAvailableSecrets(stack Stack, parentEnv string) map[string]bool {
	secretsConfig := stack.Server.Secrets.SecretsConfig
	if secretsConfig == nil {
		// No environment-specific config, all secrets are available
		result := make(map[string]bool, len(stack.Secrets.Values))
		for k := range stack.Secrets.Values {
			result[k] = true
		}
		return result
	}

	envConfig, ok := secretsConfig[parentEnv]
	if !ok || envConfig == nil {
		// No config for this environment, all secrets are available
		result := make(map[string]bool, len(stack.Secrets.Values))
		for k := range stack.Secrets.Values {
			result[k] = true
		}
		return result
	}

	// Resolve the secrets based on the environment config
	resolver := NewSecretResolver(stack.Secrets.Values, envConfig)
	filteredSecrets, err := resolver.Resolve()
	if err != nil {
		// On error, return empty to be safe
		return make(map[string]bool)
	}

	result := make(map[string]bool, len(filteredSecrets))
	for k := range filteredSecrets {
		result[k] = true
	}
	return result
}
