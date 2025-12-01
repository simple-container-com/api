package kubernetes

import (
	"fmt"

	"github.com/simple-container-com/api/pkg/api"
)

// ValidateParentEnvConfiguration validates parentEnv configuration for custom stacks
func ValidateParentEnvConfiguration(stackEnv, parentEnv string, descriptor *api.StackClientDescriptor) error {
	if parentEnv == "" {
		// No parentEnv specified - standard stack
		return nil
	}

	if parentEnv == stackEnv {
		// Self-reference - treated as standard stack
		return nil
	}

	// Custom stack validation
	// At this point we have a real custom stack (parentEnv != stackEnv)
	// Future validation can check if parentEnv environment exists in server.yaml

	return nil
}

// ValidateDomainUniqueness checks for domain conflicts in the same namespace
// This validation should be performed when multiple stacks deploy to the same namespace
func ValidateDomainUniqueness(domain, namespace string, existingDomains map[string]string) error {
	if domain == "" {
		return nil
	}

	if existingStack, exists := existingDomains[domain]; exists {
		return fmt.Errorf("domain %q conflicts with existing stack %q in namespace %q", domain, existingStack, namespace)
	}

	return nil
}
